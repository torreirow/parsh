package commands

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/bwhaley/ssmsh/parameterstore"
)

// pathCompleter provides completion for directory paths
func pathCompleter(args []string) []string {
	// Debug: log that completer was called
	if os.Getenv("SSMSH_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] pathCompleter called with args: %v, enabled: %v, cwd: %s\n", args, isCompletionEnabled(), ps.Cwd)
	}

	if !isCompletionEnabled() {
		if os.Getenv("SSMSH_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] completion is disabled\n")
		}
		return []string{}
	}

	// CRITICAL: We need to return items from MULTIPLE directories to handle absolute paths!
	// When user types "cd /ecs/te", ishell filters by "/ecs/te" but we return items from cwd.
	// Solution: Return items from both cwd AND common parent directories with FULL PATHS.

	var allResults []string

	// 1. Items from current working directory (relative paths work)
	cwdItems := getPathSuggestions(ps.Cwd, true)
	allResults = append(allResults, cwdItems...)

	// 2. Items from root (for absolute path completion like /ecs/te)
	if ps.Cwd != "/" {
		rootItems := getPathSuggestions("/", true)
		for _, item := range rootItems {
			// Add with full path
			allResults = append(allResults, "/"+item)
		}
	}

	// 3. For common paths, fetch one level deep
	commonPaths := []string{"/ecs", "/ec2_asg", "/rds", "/s3"}
	for _, path := range commonPaths {
		items := getPathSuggestions(path, true)
		for _, item := range items {
			// Add with full path
			allResults = append(allResults, path+"/"+item)
		}
	}

	if os.Getenv("SSMSH_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] pathCompleter returning %d total results\n", len(allResults))
	}

	return allResults
}

// parameterCompleter provides completion for parameters and paths
func parameterCompleter(args []string) []string {
	if !isCompletionEnabled() {
		return []string{}
	}

	// Return ALL items in current directory (ishell filters by prefix)
	return getPathSuggestions(ps.Cwd, false) // false = include parameters
}

// getPathSuggestions fetches suggestions for a given prefix
func getPathSuggestions(searchPath string, dirsOnly bool) []string {
	// searchPath should be the directory to list (e.g., "/ecs" or ps.Cwd)
	if searchPath == "" {
		searchPath = "/"
	}

	// Get region - if ps.Region is empty, AWS SDK determines it from profile
	// For completion to work, we just pass empty string and let ps.List handle it
	region := ps.Region

	// Check cache first
	cacheKey := searchPath + ":" + region
	if cached := getCached(cacheKey); cached != nil {
		if os.Getenv("SSMSH_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] Cache hit for %s: %d items\n", searchPath, len(cached))
		}
		return filterAndLimit(cached, "", dirsOnly)
	}

	if os.Getenv("SSMSH_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] Cache miss for %s, fetching in background...\n", searchPath)
	}

	// CRITICAL FIX: Tab completion MUST NOT block!
	// Instead of fetching synchronously (which can freeze the terminal),
	// start a background goroutine to populate cache for next TAB press.
	// This prevents the terminal from hanging on slow AWS API calls.
	go func() {
		results := fetchParameterList(searchPath, region)
		if results != nil && len(results) > 0 {
			setCached(cacheKey, results)
			if os.Getenv("SSMSH_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "[DEBUG] Background fetch completed for %s: %d items\n", searchPath, len(results))
			}
		}
	}()

	// Return empty results immediately on cache miss
	// User can press TAB again in a moment to see cached results
	return []string{}
}

// fetchParameterList queries AWS Parameter Store
func fetchParameterList(path string, region string) []string {
	if os.Getenv("SSMSH_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] fetchParameterList START: path=%s, region=%s\n", path, region)
	}

	// Check if we're in throttle backoff
	if tracker.isThrottled() {
		if os.Getenv("SSMSH_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] fetchParameterList: throttled, returning empty\n")
		}
		return []string{} // Silent fail - no suggestions
	}

	// Rate limit check
	if !tracker.allowRequest() {
		if os.Getenv("SSMSH_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] fetchParameterList: rate limited, returning empty\n")
		}
		return []string{} // Too many requests - skip
	}

	lr := make(chan parameterstore.ListResult)
	quit := make(chan bool)

	paramPath := parameterstore.ParameterPath{
		Name:   path,
		Region: region,
	}

	start := time.Now()

	go func() {
		ps.List(paramPath, false, lr, quit)
		close(lr)
	}()

	// For background fetches (not interactive), use a longer timeout
	// We allow up to 2 seconds for the fetch to complete
	timeout := 2 * time.Second

	if os.Getenv("SSMSH_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] fetchParameterList using timeout: %v\n", timeout)
	}

	select {
	case result := <-lr:
		latency := time.Since(start)
		perfTracker.recordLatency(latency)

		if os.Getenv("SSMSH_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] fetchParameterList RESULT: path=%s, error=%v, items=%d\n",
				path, result.Error, len(result.Result))
		}

		if result.Error != nil {
			// Check if it's a throttling error
			if aerr, ok := result.Error.(awserr.Error); ok {
				if os.Getenv("SSMSH_DEBUG") != "" {
					fmt.Fprintf(os.Stderr, "[DEBUG] fetchParameterList AWS ERROR: %s - %s\n",
						aerr.Code(), aerr.Message())
				}
				switch aerr.Code() {
				case "ThrottlingException":
					tracker.markThrottled()
					return []string{}

				case "AccessDeniedException":
					// Don't retry this path
					return []string{}

				case "ServiceUnavailable", "InternalServerError", "RequestTimeout":
					// AWS issues - fail silently
					return []string{}
				}
			}
			if os.Getenv("SSMSH_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "[DEBUG] fetchParameterList UNHANDLED ERROR: %v\n", result.Error)
			}
			return []string{}
		}

		tracker.recordRequest()
		if os.Getenv("SSMSH_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] fetchParameterList SUCCESS: path=%s, returning %d items\n",
				path, len(result.Result))
		}
		return result.Result

	case <-time.After(timeout):
		if os.Getenv("SSMSH_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] fetchParameterList TIMEOUT: path=%s after %v\n", path, timeout)
		}
		quit <- true
		<-lr // Wait for goroutine to finish
		return []string{}
	}
}

// filterAndLimit filters results by prefix and applies max limit
func filterAndLimit(items []string, prefix string, dirsOnly bool) []string {
	if items == nil {
		return []string{}
	}

	var filtered []string

	for _, item := range items {
		// Skip if contains control characters or too long
		if containsControlChars(item) || len(item) > 200 {
			continue
		}

		// Filter by prefix (only if prefix is provided, for backwards compatibility)
		// In normal tab-completion, prefix will be empty as ishell handles filtering
		if prefix != "" && !strings.HasPrefix(item, prefix) {
			continue
		}

		// Filter directories vs parameters
		isDir := strings.HasSuffix(item, "/")
		if dirsOnly && !isDir {
			continue
		}

		filtered = append(filtered, item)

		// Apply limit
		if len(filtered) >= getCompletionMaxItems() {
			break
		}
	}

	return filtered
}

// containsControlChars checks for control characters in string
func containsControlChars(s string) bool {
	for _, r := range s {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}

// wrapCompleter wraps a completer to check if completion is enabled
func wrapCompleter(realCompleter func([]string) []string) func([]string) []string {
	return func(args []string) []string {
		if !isCompletionEnabled() {
			return []string{}
		}
		return realCompleter(args)
	}
}

// warmupCache pre-fetches common paths to make first TAB instant
func warmupCache() {
	if os.Getenv("SSMSH_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] warmupCache() called\n")
	}

	// Don't warm up if completion is disabled
	if !isCompletionEnabled() {
		if os.Getenv("SSMSH_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] warmupCache: completion is disabled, skipping\n")
		}
		return
	}

	// Don't warm up if ParameterStore is not initialized (e.g., in tests)
	if ps == nil {
		if os.Getenv("SSMSH_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] warmupCache: ps is nil, skipping\n")
		}
		return
	}

	if os.Getenv("SSMSH_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] warmupCache: starting cache warmup...\n")
	}

	// List of paths to pre-fetch (in order of priority)
	pathsToWarmup := []string{
		"/",           // Root - most common
		ps.Cwd,        // Current directory
		"/dev",        // Common paths
		"/prod",
		"/staging",
		"/test",
	}

	// Warm up cache in background with rate limiting
	go func() {
		if os.Getenv("SSMSH_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] Warmup goroutine started\n")
		}
		for i, path := range pathsToWarmup {
			// Skip empty paths and duplicates
			if path == "" || path == "/" && i > 0 {
				continue
			}

			// Rate limit: wait between requests to avoid throttling
			if i > 0 {
				time.Sleep(200 * time.Millisecond)
			}

			// Fetch and cache
			region := ps.Region
			cacheKey := path + ":" + region

			// Skip if already cached
			if getCached(cacheKey) != nil {
				if os.Getenv("SSMSH_DEBUG") != "" {
					fmt.Fprintf(os.Stderr, "[DEBUG] Warmup: %s already cached, skipping\n", path)
				}
				continue
			}

			if os.Getenv("SSMSH_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "[DEBUG] Warmup: fetching %s (region: %s)...\n", path, region)
			}

			// Fetch from AWS
			results := fetchParameterList(path, region)
			if results != nil && len(results) > 0 {
				setCached(cacheKey, results)
				if os.Getenv("SSMSH_DEBUG") != "" {
					fmt.Fprintf(os.Stderr, "[DEBUG] Warmed up cache for %s: %d items\n", path, len(results))
				}
			} else {
				if os.Getenv("SSMSH_DEBUG") != "" {
					fmt.Fprintf(os.Stderr, "[DEBUG] Warmup: %s returned no results\n", path)
				}
			}
		}
		if os.Getenv("SSMSH_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] Warmup goroutine finished\n")
		}
	}()
}

package commands

import (
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/torreirow/parsh/config"
)

// cacheEntry holds cached parameter list with metadata
type cacheEntry struct {
	Values       []string
	Timestamp    time.Time
	LastAccessed time.Time
}

// completionCache manages in-memory cache with LRU eviction
type completionCache struct {
	mu    sync.RWMutex
	items map[string]*cacheEntry
}

const maxCacheEntries = 1000

var memoryCache = &completionCache{
	items: make(map[string]*cacheEntry),
}

// get retrieves from cache if not expired
func (c *completionCache) get(key string, ttl time.Duration) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.items[key]
	if !exists {
		return nil
	}

	// Check TTL
	if time.Since(entry.Timestamp) > ttl {
		delete(c.items, key)
		return nil
	}

	// Update last accessed for LRU
	entry.LastAccessed = time.Now()

	return entry.Values
}

// set stores in cache with LRU eviction
func (c *completionCache) set(key string, values []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict old entries if cache is full
	if len(c.items) >= maxCacheEntries {
		c.evictLRU()
	}

	c.items[key] = &cacheEntry{
		Values:       values,
		Timestamp:    time.Now(),
		LastAccessed: time.Now(),
	}
}

// evictLRU removes the least recently used entry
func (c *completionCache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.items {
		if oldestKey == "" || entry.LastAccessed.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.LastAccessed
		}
	}

	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}

// invalidatePathAndParents invalidates cache entries for a path and all its parents
func invalidatePathAndParents(path string, region string) {
	memoryCache.mu.Lock()
	defer memoryCache.mu.Unlock()

	// Build list of affected cache keys
	var keysToDelete []string

	// The path itself
	keysToDelete = append(keysToDelete, path+":"+region)

	// All parent paths
	parts := strings.Split(path, "/")
	for i := len(parts) - 1; i > 0; i-- {
		parentPath := strings.Join(parts[:i], "/")
		if parentPath == "" {
			parentPath = "/"
		}
		keysToDelete = append(keysToDelete, parentPath+":"+region)
	}

	// Delete from cache
	for _, key := range keysToDelete {
		delete(memoryCache.items, key)
	}

	// Also invalidate persistent cache if it exists
	if persistentCache != nil {
		persistentCache.invalidate(keysToDelete)
	}
}

// clearCache clears all in-memory cache entries
func clearCache() {
	memoryCache.mu.Lock()
	defer memoryCache.mu.Unlock()
	memoryCache.items = make(map[string]*cacheEntry)
}

// stats returns cache statistics
func (c *completionCache) stats() cacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalItems int
	for _, entry := range c.items {
		totalItems += len(entry.Values)
	}

	return cacheStats{
		Entries:    len(c.items),
		TotalItems: totalItems,
	}
}

type cacheStats struct {
	Entries    int
	TotalItems int
}

// requestTracker tracks API request rate
type requestTracker struct {
	mu             sync.Mutex
	recentRequests []time.Time
	throttled      bool
	throttledUntil time.Time
}

var tracker = &requestTracker{
	recentRequests: make([]time.Time, 0, 100),
}

// allowRequest checks if request is within rate limit (10 req/sec)
func (t *requestTracker) allowRequest() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Second)

	// Remove requests older than 1 second
	var recent []time.Time
	for _, ts := range t.recentRequests {
		if ts.After(cutoff) {
			recent = append(recent, ts)
		}
	}
	t.recentRequests = recent

	// Allow if less than 10 requests in last second
	return len(t.recentRequests) < 10
}

// recordRequest records a successful request
func (t *requestTracker) recordRequest() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recentRequests = append(t.recentRequests, time.Now())
}

// isThrottled checks if we're in throttle backoff period
func (t *requestTracker) isThrottled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.throttled && time.Now().Before(t.throttledUntil) {
		return true
	}
	t.throttled = false
	return false
}

// markThrottled sets throttle backoff for 5 seconds
func (t *requestTracker) markThrottled() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.throttled = true
	t.throttledUntil = time.Now().Add(5 * time.Second)
}

// performanceTracker tracks API latency for adaptive timeout
type performanceTracker struct {
	mu              sync.RWMutex
	recentLatencies []time.Duration
}

var perfTracker = &performanceTracker{
	recentLatencies: make([]time.Duration, 0, 10),
}

// recordLatency records API call latency
func (pt *performanceTracker) recordLatency(lat time.Duration) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.recentLatencies = append(pt.recentLatencies, lat)

	// Keep only last 10
	if len(pt.recentLatencies) > 10 {
		pt.recentLatencies = pt.recentLatencies[1:]
	}
}

// inFlightTracker de-duplicates background fetches so that at most one fetch
// is running per cache key at a time. Without this, repeated TAB presses on a
// slow path (e.g. root "/") would each spawn another full recursive read,
// which can trip AWS ThrottlingException.
type inFlightTracker struct {
	mu   sync.Mutex
	keys map[string]struct{}
}

var inFlight = &inFlightTracker{keys: make(map[string]struct{})}

// tryBeginFetch atomically marks key as in-flight. It returns true if the
// caller acquired the slot (and should start a fetch), or false if a fetch for
// this key is already running.
func tryBeginFetch(key string) bool {
	inFlight.mu.Lock()
	defer inFlight.mu.Unlock()
	if _, running := inFlight.keys[key]; running {
		return false
	}
	inFlight.keys[key] = struct{}{}
	return true
}

// endFetch releases the in-flight slot for key so a later cache miss can
// trigger a fresh fetch.
func endFetch(key string) {
	inFlight.mu.Lock()
	defer inFlight.mu.Unlock()
	delete(inFlight.keys, key)
}

// getCached is a unified cache lookup function
func getCached(key string) []string {
	ttl := time.Duration(getCompletionCacheTTL()) * time.Second

	// Try memory cache first (fast)
	if result := memoryCache.get(key, ttl); result != nil {
		return result
	}

	// Try persistent cache (slower)
	if persistentCache != nil {
		if result := persistentCache.get(key, ttl); result != nil {
			// Populate memory cache for next time
			memoryCache.set(key, result)
			return result
		}
	}

	return nil
}

// setCached writes to both cache tiers
func setCached(key string, values []string) {
	memoryCache.set(key, values)
	if persistentCache != nil {
		persistentCache.set(key, values)
	}
}

// PersistentCache manages on-disk cache
type PersistentCache struct {
	mu           sync.RWMutex
	filePath     string
	maxSizeBytes int64
	compression  bool
	entries      map[string]*cacheEntry
	dirty        bool
}

// persistentCacheData is the serialized format
type persistentCacheData struct {
	Version int
	Entries map[string]*cacheEntry
	SavedAt time.Time
}

var persistentCache *PersistentCache

// NewPersistentCache creates or loads a persistent cache
func NewPersistentCache(cfg *config.Config) (*PersistentCache, error) {
	if !cfg.Default.CacheEnabled {
		return nil, nil // Cache disabled
	}

	// Resolve cache location
	cachePath := cfg.Default.CacheLocation
	if cachePath == "" {
		paths, err := config.GetConfigPaths()
		if err != nil {
			return nil, err
		}
		cachePath = paths.CacheFile
	}

	// Expand tilde if present
	if expanded, err := config.ExpandPath(cachePath); err == nil {
		cachePath = expanded
	}

	// Convert MB to bytes
	maxSizeBytes := int64(cfg.Default.CacheMaxSize) * 1024 * 1024
	if maxSizeBytes == 0 {
		maxSizeBytes = 50 * 1024 * 1024 // Default 50MB
	}

	pc := &PersistentCache{
		filePath:     cachePath,
		maxSizeBytes: maxSizeBytes,
		compression:  cfg.Default.CacheCompression,
		entries:      make(map[string]*cacheEntry),
	}

	// Try to load existing cache
	if err := pc.load(); err != nil {
		// Non-fatal - just start with empty cache
		fmt.Fprintf(os.Stderr, "Warning: Could not load cache: %v\n", err)
	}

	return pc, nil
}

// load reads cache from disk
func (pc *PersistentCache) load() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	file, err := os.Open(pc.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No cache file yet - OK
		}
		return err
	}
	defer file.Close()

	// Check file size
	stat, err := file.Stat()
	if err != nil {
		return err
	}

	if stat.Size() > pc.maxSizeBytes {
		// Cache file too large - delete and start fresh
		os.Remove(pc.filePath)
		return fmt.Errorf("cache file exceeds max size (%d MB), removed",
			pc.maxSizeBytes/(1024*1024))
	}

	var reader io.Reader = file

	// Decompress if enabled
	if pc.compression {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("decompression failed: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// Decode
	var data persistentCacheData
	decoder := gob.NewDecoder(reader)
	if err := decoder.Decode(&data); err != nil {
		// Cache file is corrupted - delete it and start fresh
		file.Close()
		os.Remove(pc.filePath)
		return fmt.Errorf("cache file corrupted, deleted and starting fresh: %w", err)
	}

	// Validate version
	if data.Version != 1 {
		// Unsupported version - delete and start fresh
		file.Close()
		os.Remove(pc.filePath)
		return fmt.Errorf("unsupported cache version %d, deleted and starting fresh", data.Version)
	}

	// Load entries
	pc.entries = data.Entries
	pc.dirty = false

	return nil
}

// save writes cache to disk
func (pc *PersistentCache) save() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if !pc.dirty {
		return nil // No changes
	}

	// Create directory if needed
	dir := filepath.Dir(pc.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Write to temp file first (atomic write)
	tempPath := pc.filePath + ".tmp"
	file, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	var writer io.Writer = file
	var gzWriter *gzip.Writer

	// Compress if enabled
	if pc.compression {
		gzWriter = gzip.NewWriter(file)
		writer = gzWriter
	}

	// Encode
	data := persistentCacheData{
		Version: 1,
		Entries: pc.entries,
		SavedAt: time.Now(),
	}

	encoder := gob.NewEncoder(writer)
	if err := encoder.Encode(data); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("encode failed: %w", err)
	}

	// Flush gzip writer if used
	if gzWriter != nil {
		if err := gzWriter.Close(); err != nil {
			os.Remove(tempPath)
			return err
		}
	}

	// Close file
	if err := file.Close(); err != nil {
		os.Remove(tempPath)
		return err
	}

	// Check size
	stat, err := os.Stat(tempPath)
	if err != nil {
		os.Remove(tempPath)
		return err
	}

	if stat.Size() > pc.maxSizeBytes {
		os.Remove(tempPath)
		return fmt.Errorf("cache would exceed max size (%d MB), not saving",
			pc.maxSizeBytes/(1024*1024))
	}

	// Atomic rename
	if err := os.Rename(tempPath, pc.filePath); err != nil {
		os.Remove(tempPath)
		return err
	}

	pc.dirty = false
	return nil
}

// get retrieves from persistent cache
func (pc *PersistentCache) get(key string, ttl time.Duration) []string {
	if pc == nil {
		return nil
	}

	pc.mu.RLock()
	defer pc.mu.RUnlock()

	entry, exists := pc.entries[key]
	if !exists {
		return nil
	}

	// Check TTL
	if time.Since(entry.Timestamp) > ttl {
		return nil
	}

	return entry.Values
}

// set stores in persistent cache
func (pc *PersistentCache) set(key string, values []string) {
	if pc == nil {
		return
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.entries[key] = &cacheEntry{
		Values:       values,
		Timestamp:    time.Now(),
		LastAccessed: time.Now(),
	}

	pc.dirty = true
}

// invalidate removes entries from persistent cache
func (pc *PersistentCache) invalidate(keys []string) {
	if pc == nil {
		return
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	for _, key := range keys {
		delete(pc.entries, key)
	}

	pc.dirty = true
}

// clear removes all entries from persistent cache
func (pc *PersistentCache) clear() error {
	if pc == nil {
		return nil
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.entries = make(map[string]*cacheEntry)
	pc.dirty = true

	// Delete file
	return os.Remove(pc.filePath)
}

// Close saves cache before shutdown
func (pc *PersistentCache) Close() error {
	if pc == nil {
		return nil
	}

	return pc.save()
}

// Stats returns cache statistics
func (pc *PersistentCache) Stats() CacheStats {
	if pc == nil {
		return CacheStats{Enabled: false}
	}

	pc.mu.RLock()
	defer pc.mu.RUnlock()

	var totalItems int
	for _, entry := range pc.entries {
		totalItems += len(entry.Values)
	}

	// Get file size
	var fileSize int64
	if stat, err := os.Stat(pc.filePath); err == nil {
		fileSize = stat.Size()
	}

	return CacheStats{
		Enabled:     true,
		Entries:     len(pc.entries),
		TotalItems:  totalItems,
		FileSizeMB:  float64(fileSize) / (1024 * 1024),
		MaxSizeMB:   float64(pc.maxSizeBytes) / (1024 * 1024),
		Compression: pc.compression,
		Location:    pc.filePath,
	}
}

// CacheStats contains cache metrics
type CacheStats struct {
	Enabled     bool
	Entries     int
	TotalItems  int
	FileSizeMB  float64
	MaxSizeMB   float64
	Compression bool
	Location    string
}

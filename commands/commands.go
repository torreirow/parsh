package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"

	"github.com/abiosoft/ishell"
	"github.com/bwhaley/ssmsh/config"
	"github.com/bwhaley/ssmsh/parameterstore"
)

type fn func(*ishell.Context)

var (
	shell *ishell.Shell
	ps    *parameterstore.ParameterStore
	cfg   *config.Config

	// Completion runtime state
	completionMutex    sync.RWMutex
	completionEnabled  bool
	completionMaxItems int
	completionCacheTTL int
)

// Init initializes the ssmsh subcommands
func Init(iShell *ishell.Shell, iPs *parameterstore.ParameterStore, iCfg *config.Config) {
	shell = iShell
	ps = iPs
	cfg = iCfg

	// Initialize completion settings
	initCompletionSettings(iCfg)

	// Initialize persistent cache
	var err error
	persistentCache, err = NewPersistentCache(iCfg)
	if err != nil {
		shell.Printf("Warning: Could not initialize persistent cache: %v\n", err)
	}

	// Register commands with completers
	registerCommand("cd", "change your relative location within the parameter store", cd, cdUsage, pathCompleter)
	registerCommand("completion", "toggle tab completion on/off", completionCmd, completionUsage, nil)
	registerCommand("config", "manage configuration", configCmd, configUsage, nil)
	registerCommand("cp", "copy source to dest", cp, cpUsage, parameterCompleter)
	registerCommand("decrypt", "toggle parameter decryption", decrypt, decryptUsage, nil)
	registerCommand("get", "get parameters", get, getUsage, parameterCompleter)
	registerCommand("history", "get parameter history", history, historyUsage, nil)
	registerCommand("key", "set the KMS key", key, keyUsage, nil)
	registerCommand("ls", "list parameters", ls, lsUsage, parameterCompleter)
	registerCommand("mv", "move parameters", mv, mvUsage, parameterCompleter)
	registerCommand("policy", "create named parameter policy", policy, policyUsage, nil)
	registerCommand("profile", "switch to a different AWS IAM profile", profile, profileUsage, nil)
	registerCommand("put", "set parameter", put, putUsage, nil)
	registerCommand("region", "change region", region, regionUsage, nil)
	registerCommand("rm", "remove parameters", rm, rmUsage, parameterCompleter)
	setPrompt(parameterstore.Delimiter)
}

// initCompletionSettings initializes completion state from config
func initCompletionSettings(cfg *config.Config) {
	completionMutex.Lock()
	completionEnabled = cfg.Default.Completion
	if cfg.Default.CompletionMaxItems == 0 {
		completionMaxItems = 50
	} else {
		completionMaxItems = cfg.Default.CompletionMaxItems
	}

	if cfg.Default.CompletionCacheTTL == 0 {
		completionCacheTTL = 30
	} else {
		completionCacheTTL = cfg.Default.CompletionCacheTTL
	}
	enabled := completionEnabled
	completionMutex.Unlock()

	// Warm up cache on startup if completion is enabled
	// (done after unlocking to avoid holding lock during network calls)
	if enabled {
		warmupCache()
	}
}

// isCompletionEnabled checks if completion is currently enabled
func isCompletionEnabled() bool {
	completionMutex.RLock()
	defer completionMutex.RUnlock()
	return completionEnabled
}

// setCompletionEnabled sets the runtime completion state
func setCompletionEnabled(enabled bool) {
	completionMutex.Lock()
	completionEnabled = enabled
	shouldWarmup := enabled
	completionMutex.Unlock()

	// Warm up cache when enabling completion
	// (done after unlocking to avoid holding lock during network calls)
	if shouldWarmup {
		warmupCache()
	}
}

// getCompletionMaxItems returns the max items setting
func getCompletionMaxItems() int {
	completionMutex.RLock()
	defer completionMutex.RUnlock()
	return completionMaxItems
}

// getCompletionCacheTTL returns the cache TTL setting
func getCompletionCacheTTL() int {
	completionMutex.RLock()
	defer completionMutex.RUnlock()
	return completionCacheTTL
}

// Cleanup saves persistent cache before shutdown
func Cleanup() {
	if persistentCache != nil {
		if err := persistentCache.Close(); err != nil {
			shell.Printf("Warning: Could not save cache: %v\n", err)
		}
	}
}

// registerCommand adds a command to the shell
func registerCommand(name string, helpText string, f fn, usageText string, completer func([]string) []string) {
	cmd := &ishell.Cmd{
		Name:     name,
		Help:     helpText,
		LongHelp: usageText,
		Func:     f,
	}

	// Wrap the completer if provided
	if completer != nil {
		cmd.Completer = wrapCompleter(completer)
	}

	shell.AddCmd(cmd)
}

// setPrompt configures the shell prompt
func setPrompt(prompt string) {
	shell.SetPrompt(prompt + ">")
}

// remove deletes an element from a slice of strings
func remove(slice []string, i int) []string {
	return append(slice[:i], slice[i+1:]...)
}

// checkRecursion searches a slice of strings for an element matching -r or -R
func checkRecursion(paths []string) ([]string, bool) {
	for i, p := range paths {
		if strings.EqualFold(p, "-r") {
			paths = remove(paths, i)
			return paths, true
		}
	}
	return paths, false
}

// parsePath determines whether a path includes a region
func parsePath(path string) (parameterPath parameterstore.ParameterPath) {
	pathParts := strings.Split(path, ":")
	switch len(pathParts) {
	case 1:
		parameterPath.Name = pathParts[0]
		parameterPath.Region = ps.Region
	case 2:
		parameterPath.Region = pathParts[0]
		parameterPath.Name = pathParts[1]
	}
	ps.InitClient(parameterPath.Region)
	return parameterPath
}

func groupByRegion(params []parameterstore.ParameterPath) map[string][]string {
	paramsByRegion := make(map[string][]string)
	for _, p := range params {
		paramsByRegion[p.Region] = append(paramsByRegion[p.Region], p.Name)
	}
	return paramsByRegion
}

func trim(with []string) (without []string) {
	for i := range with {
		without = append(without, strings.TrimSpace(with[i]))
	}
	return without
}

func printResult(result interface{}) {
	switch cfg.Default.Output {
	case "json":
		printJSON(result)
	default:
		printJSON(result)
	}
}

func printJSON(result interface{}) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "    ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(result); err != nil {
		shell.Println("Error with result: ", err)
	} else {
		shell.Print(buf.String())
	}
}

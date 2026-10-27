package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/abiosoft/ishell"
	"github.com/mattn/go-shellwords"
	"github.com/torreirow/parsh/commands"
	"github.com/torreirow/parsh/config"
	"github.com/torreirow/parsh/parameterstore"
)

var Version string

func main() {
	cfgFile := flag.String("config", "", "Load configuration from the specified file")
	file := flag.String("file", "", "Read commands from file (use - for stdin)")
	version := flag.Bool("version", false, "Display the current version")
	generateConfig := flag.Bool("generate-config", false, "Generate a default configuration file")
	flag.Parse()

	if *version {
		fmt.Println("Version", Version)
		os.Exit(0)
	}

	if *generateConfig {
		err := config.GenerateDefaultConfig()
		if err != nil {
			fmt.Printf("Error generating default configuration: %s\n", err)
			os.Exit(1)
		}
		fmt.Println("Default configuration generated successfully")
		os.Exit(0)
	}

	// Auto-migrate legacy config if no explicit config is specified
	if *cfgFile == "" && os.Getenv("PARSH_CONFIG") == "" {
		err := config.AutoMigrateConfig()
		if err != nil {
			fmt.Printf("Warning: config migration failed: %s\n", err)
		}
	}

	// Ensure config directory exists
	err := config.EnsureConfigDir()
	if err != nil {
		fmt.Printf("Warning: failed to create config directory: %s\n", err)
	}

	cfg, err := config.ReadConfig(*cfgFile)
	if err != nil {
		fmt.Printf("Error reading configuration file %s: %s\n", *cfgFile, err)
		os.Exit(1)
	}

	// Validate and apply defaults to config
	config.ValidateAndApplyDefaults(&cfg)

	// Setup cleanup on exit
	defer commands.Cleanup()

	// Setup signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down gracefully...")
		commands.Cleanup()
		os.Exit(0)
	}()

	shell := createShellWithHistory()
	var ps parameterstore.ParameterStore
	ps.SetDefaults(cfg)
	err = ps.NewParameterStore(true)
	if err != nil {
		shell.Println("Error initializing session. Is your authentication correct?", err)
		os.Exit(1)
	}
	commands.Init(shell, &ps, &cfg)

	if *file == "-" {
		processStdin(shell)
	} else if *file != "" {
		processFile(shell, *file)
	} else if len(flag.Args()) > 1 {
		err := shell.Process(flag.Args()...)
		if err != nil {
			shell.Println("Error executing shell process:", err)
			shell.Println("This might be a bug. Please open an issue at github.com/torreirow/ssmsh.\n")
			os.Exit(1)
		}
	} else {
		shell.Run()
		shell.Close()
	}
}

func processStdin(shell *ishell.Shell) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		shell.Println("Error reading from stdin:", err)
		os.Exit(1)
	}
	processData(shell, string(data))
}

func processFile(shell *ishell.Shell, fn string) {
	data, err := os.ReadFile(fn)
	if err != nil {
		shell.Println("Error reading from file:", err)
	}
	processData(shell, string(data))
}

func processData(shell *ishell.Shell, data string) {
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		if line == "" || string(line[0]) == "#" {
			continue
		}
		args, err := shellwords.Parse(line)
		if err != nil {
			msg := fmt.Errorf("Error parsing %s: %v", line, err)
			shell.Println(msg)
			os.Exit(1)
		}
		err = shell.Process(args...)
		if err != nil {
			msg := fmt.Errorf("Error executing %s: %v", line, err)
			shell.Println(msg)
			os.Exit(1)
		}
	}
}

// createShellWithHistory creates a new shell with history file support
func createShellWithHistory() *ishell.Shell {
	paths, err := config.GetConfigPaths()
	if err != nil {
		// Fall back to shell without history if we can't get paths
		return ishell.New()
	}

	shell := ishell.New()
	shell.SetHistoryPath(paths.HistoryFile)
	return shell
}

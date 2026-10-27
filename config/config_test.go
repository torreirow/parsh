package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetConfigPaths(t *testing.T) {
	// Save original env
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

	tests := []struct {
		name           string
		xdgConfigHome  string
		expectedSuffix string
	}{
		{
			name:           "default location",
			xdgConfigHome:  "",
			expectedSuffix: ".config/parsh",
		},
		{
			name:           "custom XDG_CONFIG_HOME",
			xdgConfigHome:  "/custom/config",
			expectedSuffix: "custom/config/parsh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.xdgConfigHome != "" {
				os.Setenv("XDG_CONFIG_HOME", tt.xdgConfigHome)
			} else {
				os.Unsetenv("XDG_CONFIG_HOME")
			}

			paths, err := GetConfigPaths()
			if err != nil {
				t.Fatalf("GetConfigPaths() error = %v", err)
			}

			if !strings.HasSuffix(paths.ConfigDir, tt.expectedSuffix) {
				t.Errorf("ConfigDir = %v, want suffix %v", paths.ConfigDir, tt.expectedSuffix)
			}
		})
	}
}

func TestReadConfig(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	configContent := `[default]
completion=true
completion-max-items=100
completion-cache-ttl=60
cache-enabled=true
cache-max-size=25
`

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := readConfigFromFile(configPath)
	if err != nil {
		t.Fatalf("readConfigFromFile() error = %v", err)
	}

	// Verify parsed values
	if !cfg.Default.Completion {
		t.Error("Completion should be true")
	}
	if cfg.Default.CompletionMaxItems != 100 {
		t.Errorf("CompletionMaxItems = %d, want 100", cfg.Default.CompletionMaxItems)
	}
	if cfg.Default.CacheMaxSize != 25 {
		t.Errorf("CacheMaxSize = %d, want 25", cfg.Default.CacheMaxSize)
	}
}

func TestAutoMigrateConfig(t *testing.T) {
	// Create temp home directory
	tmpHome := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", oldHome)

	// Also unset XDG_CONFIG_HOME to use default
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Unsetenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

	// Create legacy config
	legacyPath := filepath.Join(tmpHome, LegacyConfigFileName)
	legacyContent := `[default]
region=us-west-2
`
	if err := os.WriteFile(legacyPath, []byte(legacyContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Run migration
	if err := AutoMigrateConfig(); err != nil {
		t.Fatalf("AutoMigrateConfig() error = %v", err)
	}

	// Verify new config exists
	paths, _ := GetConfigPaths()
	if _, err := os.Stat(paths.ConfigFile); os.IsNotExist(err) {
		t.Error("New config file should exist after migration")
	}

	// Verify legacy is backed up
	backupPath := legacyPath + ".backup"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Legacy config should be backed up")
	}

	// Verify content migrated
	cfg, err := readConfigFromFile(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Default.Region != "us-west-2" {
		t.Error("Config content not migrated correctly")
	}
}

func TestEnsureConfigDir(t *testing.T) {
	tmpHome := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", oldHome)

	// Unset XDG_CONFIG_HOME
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Unsetenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("EnsureConfigDir() error = %v", err)
	}

	paths, _ := GetConfigPaths()

	// Verify directory exists
	info, err := os.Stat(paths.ConfigDir)
	if err != nil {
		t.Fatalf("Config directory should exist: %v", err)
	}

	// Verify permissions
	if info.Mode().Perm() != 0700 {
		t.Errorf("Config dir permissions = %o, want 0700", info.Mode().Perm())
	}
}

func TestValidateAndApplyDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    Config
		expected func(*Config) bool
	}{
		{
			name: "cap excessive max-items",
			input: Config{Default: struct {
				Decrypt            bool
				Key                string
				Profile            string
				Region             string
				Overwrite          bool
				Type               string
				Output             string
				Completion         bool
				CompletionMaxItems int    `gcfg:"completion-max-items"`
				CompletionCacheTTL int    `gcfg:"completion-cache-ttl"`
				CacheEnabled       bool   `gcfg:"cache-enabled"`
				CacheLocation      string `gcfg:"cache-location"`
				CacheMaxSize       int    `gcfg:"cache-max-size"`
				CacheCompression   bool   `gcfg:"cache-compression"`
				HistorySize        int    `gcfg:"history-size"`
				HistoryFile        bool   `gcfg:"history-file"`
				EnableLogging      bool   `gcfg:"enable-logging"`
				LogLevel           string `gcfg:"log-level"`
			}{CompletionMaxItems: 1000}},
			expected: func(cfg *Config) bool {
				return cfg.Default.CompletionMaxItems == 500
			},
		},
		{
			name: "apply default TTL",
			input: Config{Default: struct {
				Decrypt            bool
				Key                string
				Profile            string
				Region             string
				Overwrite          bool
				Type               string
				Output             string
				Completion         bool
				CompletionMaxItems int    `gcfg:"completion-max-items"`
				CompletionCacheTTL int    `gcfg:"completion-cache-ttl"`
				CacheEnabled       bool   `gcfg:"cache-enabled"`
				CacheLocation      string `gcfg:"cache-location"`
				CacheMaxSize       int    `gcfg:"cache-max-size"`
				CacheCompression   bool   `gcfg:"cache-compression"`
				HistorySize        int    `gcfg:"history-size"`
				HistoryFile        bool   `gcfg:"history-file"`
				EnableLogging      bool   `gcfg:"enable-logging"`
				LogLevel           string `gcfg:"log-level"`
			}{CompletionCacheTTL: 0}},
			expected: func(cfg *Config) bool {
				return cfg.Default.CompletionCacheTTL == 30
			},
		},
		{
			name: "enable completion by default",
			input: Config{Default: struct {
				Decrypt            bool
				Key                string
				Profile            string
				Region             string
				Overwrite          bool
				Type               string
				Output             string
				Completion         bool
				CompletionMaxItems int    `gcfg:"completion-max-items"`
				CompletionCacheTTL int    `gcfg:"completion-cache-ttl"`
				CacheEnabled       bool   `gcfg:"cache-enabled"`
				CacheLocation      string `gcfg:"cache-location"`
				CacheMaxSize       int    `gcfg:"cache-max-size"`
				CacheCompression   bool   `gcfg:"cache-compression"`
				HistorySize        int    `gcfg:"history-size"`
				HistoryFile        bool   `gcfg:"history-file"`
				EnableLogging      bool   `gcfg:"enable-logging"`
				LogLevel           string `gcfg:"log-level"`
			}{}},
			expected: func(cfg *Config) bool {
				return cfg.Default.Completion == true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.input
			ValidateAndApplyDefaults(&cfg)
			if !tt.expected(&cfg) {
				t.Errorf("Validation failed for %s", tt.name)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(string) bool
	}{
		{
			name:  "expand tilde",
			input: "~/custom/cache.db",
			check: func(s string) bool {
				return strings.Contains(s, "/custom/cache.db") && !strings.HasPrefix(s, "~")
			},
		},
		{
			name:  "no tilde",
			input: "/absolute/path",
			check: func(s string) bool {
				return s == "/absolute/path"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.check(got) {
				t.Errorf("ExpandPath() = %v, check failed", got)
			}
		})
	}
}

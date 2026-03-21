# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.5.2] - 2026-03-21

### Fixed
- **Deadlock in tab completion**: Fixed test deadlock in `TestSetCompletionEnabled` that caused 10-minute timeout and build failures
  - Refactored `setCompletionEnabled()` to release mutex lock before calling `warmupCache()`
  - Applied consistent unlock-before-warmup pattern matching `initCompletionSettings()`
  - Added defensive nil check in `warmupCache()` to prevent crashes in test contexts
  - All tests now pass without deadlock in normal timeframe (< 1 second)

## [1.5.1] - 2026-03-21

### Added
- **Tab completion** for AWS Parameter Store paths and parameter names
  - **Async/non-blocking architecture**: Background goroutines with 2-second timeout prevent terminal freezing
  - **Two-TAB pattern**: First TAB starts background fetch, second TAB shows cached results
  - **Cache warmup on startup**: Pre-fetches common paths (/, /dev, /prod, /ecs, etc.) for instant first TAB
  - Intelligent completion for paths and parameters
  - Cross-region completion support (e.g., `us-west-2:/path`)
  - Smart two-tier caching (in-memory + persistent)
  - Adaptive timeouts based on network performance
  - Graceful handling of AWS throttling and errors
  - Runtime toggle with `completion` command
  - Configurable max items, cache TTL, and cache size
- **XDG Base Directory** compliance for configuration files
  - New default config location: `~/.config/ssmsh/config`
  - History stored in `~/.config/ssmsh/history`
  - Cache stored in `~/.cache/ssmsh/cache.gob.gz`
- **Automatic configuration migration** from `~/.ssmshrc` to `~/.config/ssmsh/config`
  - Runs automatically on first startup (unless explicit config provided)
  - Creates backup of original file (`~/.ssmshrc.backup`)
  - Migration banner displays file locations
- **New commands**:
  - `completion [true|false|stats|clear-cache|save-cache|reload-cache]` - Control tab completion
  - `config [show|edit]` - Manage configuration files
- **New flags**:
  - `get -d` / `get --decrypt` - Decrypt SecureString parameters inline without changing global decrypt setting
- **Configuration options**:
  - `completion` - Enable/disable tab completion (default: true)
  - `completion-max-items` - Max suggestions shown (default: 50, max: 500)
  - `completion-cache-ttl` - Cache TTL in seconds (default: 30, range: 0-3600)
  - `cache-enabled` - Enable persistent cache (default: true)
  - `cache-location` - Cache file path (supports tilde expansion)
  - `cache-max-size` - Max cache size in MB (default: 50, max: 500)
  - `cache-compression` - Compress cache file (default: true)
  - `history-size` - Command history size (default: 1000)
  - `history-file` - History file location
- **CLI flags**:
  - `--generate-config` - Generate default configuration file with comments
- **Performance optimizations**:
  - In-memory cache with LRU eviction (< 1 μs lookups)
  - Persistent cache with compression (< 1 ms to load)
  - Request rate limiting (10 req/sec)
  - Throttle backoff (5 seconds on AWS errors)
  - Cache invalidation on mutations (put, rm, cp, mv)

### Changed
- Configuration file location changed from `~/.ssmshrc` to `~/.config/ssmsh/config`
  - **Fully backwards compatible** - automatic migration with backup
  - Old location still supported (no breaking changes)
  - Use `SSMSH_CONFIG` env var or `--config` flag to override
- **Configuration priority chain**:
  1. `--config` flag (highest priority)
  2. `SSMSH_CONFIG` environment variable
  3. `~/.config/ssmsh/config` (XDG)
  4. `~/.ssmshrc` (legacy, for backwards compatibility)
- Command history now stored in `~/.config/ssmsh/history` instead of shell default
- Graceful shutdown on SIGINT/SIGTERM (saves persistent cache)

### Fixed
- Parameter value visibility in `get` command output
- Put validation to properly handle all required fields
- Edge cases in parameter path parsing
- Region detection now reads from AWS profile config (not just `AWS_REGION` env var)
- Terminal freezing during tab completion on slow AWS API calls
- Background fetch timeout increased from 200ms to 2s to handle slower networks

### Improved
- **Debug mode**: Set `SSMSH_DEBUG=1` for comprehensive debug logging of completion, cache, and AWS API calls

### Migration Guide

#### Automatic Migration (Recommended)
Simply run `ssmsh` and it will automatically migrate your configuration from `~/.ssmshrc` to `~/.config/ssmsh/config`. The original file will be backed up to `~/.ssmshrc.backup`.

#### Manual Migration
If you prefer to migrate manually:
1. Create the new config directory: `mkdir -p ~/.config/ssmsh`
2. Copy your config: `cp ~/.ssmshrc ~/.config/ssmsh/config`
3. (Optional) Remove old config: `rm ~/.ssmshrc`

#### Using Legacy Location
To continue using `~/.ssmshrc`:
- Just leave it in place - it will continue to work
- Set `SSMSH_CONFIG=~/.ssmshrc` to explicitly use it
- Use `ssmsh --config ~/.ssmshrc` on each invocation

#### Verifying Configuration
```bash
# Check which config file is being used
ssmsh
/> config show

# Edit your current config
/> config edit
```

### Notes
- **Tab completion behavior**: Uses async background fetching to prevent terminal freezing
  - First TAB press: Starts background AWS API fetch (may show no results initially)
  - Second TAB press (after 1-2 seconds): Shows cached results instantly
  - After cache warmup (~10 seconds on startup), common paths complete on first TAB
- Tab completion makes AWS API calls which may incur costs (cached for 30s by default)
- Persistent cache improves performance across sessions
- Cache is automatically invalidated when you modify parameters
- Cache TTL and max items can be adjusted in config to balance performance vs. accuracy
- Debug logging available via `SSMSH_DEBUG=1` environment variable

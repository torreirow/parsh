# parsh

> **Note:** This is a fork of [bwhaley/ssmsh](https://github.com/bwhaley/ssmsh) (originally forked from [torreirow/ssmsh](https://github.com/torreirow/ssmsh)).
> Starting with v1.0.0, the project has been renamed to **parsh** (Parameter Store Shell) to establish its own identity
> while continuing active development, bug fixes, and new features like tab completion.

parsh is an interactive shell for the AWS EC2 Parameter Store. Features:
* Interact with the parameter store hierarchy using familiar commands like cd, ls, cp, mv, and rm
* Supports relative paths and shorthand (`..`) syntax
* Operate on parameters between regions
* Recursively list, copy, and remove parameters
* Get parameter history
* Create new parameters using put
* Advanced parameters (with policies)
* Supports emacs-style command shell navigation hotkeys
* Submit batch commands with the `-file` flag
* Inline commands

## Installation

### Binaries

Download binaries for MacOS, Linux, or Windows from the latest release [here](https://github.com/torreirow/parsh/releases).

### Nix

**Using Flakes (Recommended):**

This repository includes a `flake.nix` for the latest version:

```bash
# Try without installing
nix run github:torreirow/parsh

# Install to your profile
nix profile install github:torreirow/parsh

# Use in NixOS configuration - see below
```

**As a Flake Input:**

Add `parsh` to your flake inputs and use it in your configuration:

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    parsh.url = "github:torreirow/parsh";
    # Optional: pin to specific version
    # parsh.url = "github:torreirow/parsh/v1.0.0";
  };

  outputs = { self, nixpkgs, parsh, ... }: {
    # NixOS system configuration
    nixosConfigurations.your-hostname = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        ({ pkgs, ... }: {
          environment.systemPackages = [
            parsh.packages.${pkgs.system}.default
          ];
        })
      ];
    };

    # Home Manager configuration
    homeConfigurations.your-user = home-manager.lib.homeManagerConfiguration {
      pkgs = nixpkgs.legacyPackages.x86_64-linux;
      modules = [
        ({ pkgs, ... }: {
          home.packages = [
            parsh.packages.${pkgs.system}.default
          ];
        })
      ];
    };

    # Development shell
    devShells.x86_64-linux.default = nixpkgs.legacyPackages.x86_64-linux.mkShell {
      packages = [
        parsh.packages.x86_64-linux.default
      ];
    };
  };
}
```

**Flake Output Structure:**

```nix
# Available outputs
parsh.packages.<system>.default  # The parsh package
parsh.apps.<system>.default      # Runnable app (used by nix run)
```

**Supported Systems:**
- `x86_64-linux`
- `aarch64-linux`
- `x86_64-darwin` (macOS Intel)
- `aarch64-darwin` (macOS Apple Silicon)

## Configuration

Set up [AWS credentials](http://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#specifying-credentials).

### Configuration File Location

`parsh` follows the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html) and stores its configuration in `~/.config/parsh/config` by default.

**Configuration file priority (highest to lowest):**
1. `--config` flag: `parsh --config /path/to/config`
2. `PARSH_CONFIG` environment variable: `export PARSH_CONFIG=/path/to/config`
3. XDG config directory: `~/.config/parsh/config`
4. Legacy location: `~/.parshrc` (for backwards compatibility)

**Generate a default configuration file:**
```bash
parsh --generate-config
```

This creates `~/.config/parsh/config` with all available settings and comments.

### Configuration Options

Example configuration file:

```bash
[default]
# Core settings
type=SecureString
overwrite=true
decrypt=true
profile=my-profile
region=us-east-1
key=3example-89a6-4880-b544-73ad3db2ff3b
output=json

# Tab completion settings
completion=true              # Enable/disable tab completion (default: true)
completion-max-items=50      # Maximum completion suggestions (default: 50, max: 500)
completion-cache-ttl=30      # Cache TTL in seconds (default: 30, range: 0-3600)

# Persistent cache settings
cache-enabled=true           # Enable persistent cache across sessions (default: true)
cache-location=~/.cache/parsh/cache.gob.gz  # Cache file location
cache-max-size=50            # Maximum cache size in MB (default: 50, max: 500)
cache-compression=true       # Compress cache file (default: true)

# Shell history
history-size=1000            # Command history size (default: 1000)
history-file=~/.config/parsh/history  # History file location
```

**Configuration notes:**
* When setting the region, the `AWS_REGION` env var takes top priority, followed by the setting in the config file, followed by the value set in the AWS profile (if configured)
* When setting the profile, the `AWS_PROFILE` env var takes top priority, followed by the setting in the config file
* If you set a KMS key, it will only work in the region where that key is located. You can use the `key` command while in the shell to change the key.
* If the configuration file has `output=json`, the results of the `get` and `history` commands will be printed in JSON. The fields of the JSON results will be the same as in the respective Go structs. See the [`Parameter`](https://docs.aws.amazon.com/sdk-for-go/api/service/ssm/#Parameter) and [`ParameterHistory`](https://docs.aws.amazon.com/sdk-for-go/api/service/ssm/#ParameterHistory) docs.

### Automatic Migration

If you have an existing `~/.parshrc` configuration file, `parsh` will automatically migrate it to `~/.config/parsh/config` on first run. The original file will be backed up to `~/.parshrc.backup`.

**Migration behavior:**
* Happens automatically on startup (unless `--config` or `PARSH_CONFIG` is set)
* Creates backup of original file
* Preserves all settings
* Displays migration banner with file locations
* Only runs once (won't re-migrate if new config already exists)

## Tab Completion

`parsh` provides intelligent tab completion for AWS Parameter Store paths and parameter names.

### Features

* **Path completion**: Press TAB after typing a partial path to see suggestions
* **Parameter completion**: Complete parameter names within the current directory
* **Cross-region completion**: Works with region prefixes like `us-west-2:/path`
* **Smart caching**: Two-tier cache (in-memory + persistent) for fast responses
* **Adaptive timeouts**: Automatically adjusts API timeouts based on network performance
* **Graceful degradation**: Handles AWS throttling, permissions errors, and network issues

### Usage

**Important:** Tab completion uses **async background fetching** to prevent terminal freezing. This means:

1. **First TAB press**: Starts background AWS API fetch (may show no results initially)
2. **Second TAB press** (after 1-2 seconds): Shows cached results instantly

**Example workflow:**

```bash
# First TAB - starts background fetch
/> cd /dev/app<TAB>
# (no results yet, background fetch in progress)

# Wait 1-2 seconds, then TAB again
/> cd /dev/app<TAB>
/dev/application/  /dev/app-config/   # ← Results appear!

# After warmup (~10 seconds on startup), common paths complete on first TAB:
/> cd /ecs/tech<TAB>
technative/   # ← Instant! (from cache warmup)

# Parameter completion
/> get /dev/app/d<TAB>
database-url  domain  debug-mode

# Cross-region completion
/> ls us-west-2:/prod/<TAB>
api/  web/  db/
```

**Why two TABs?**
- AWS API calls can take 200ms-2s depending on network/region
- Blocking the terminal for that long feels like a freeze
- Async approach keeps terminal responsive
- After cache warmup, most paths complete on first TAB

### Runtime Commands

Control tab completion behavior at runtime:

```bash
/> completion true          # Enable tab completion
/> completion false         # Disable tab completion
/> completion stats         # Show cache statistics
/> completion clear-cache   # Clear all caches
/> completion save-cache    # Force save persistent cache
/> completion reload-cache  # Reload cache from disk
```

### Configuration

Tab completion can be configured in `~/.config/parsh/config`:

```bash
[default]
completion=true              # Enable/disable (default: true)
completion-max-items=50      # Max suggestions shown (default: 50)
completion-cache-ttl=30      # Cache TTL in seconds (default: 30)
cache-enabled=true           # Persistent cache (default: true)
cache-location=~/.cache/parsh/cache.gob.gz
cache-max-size=50            # Max cache size in MB (default: 50)
cache-compression=true       # Compress cache file (default: true)
```

### Performance

* **Memory cache**: < 1 microsecond per lookup
* **Persistent cache**: < 1 millisecond to load
* **AWS API**: First request may take 200ms-2s, then cached for 30 seconds
* **Background fetch timeout**: 2 seconds (prevents terminal hanging)
* **Cache warmup**: Happens on startup, pre-fetches common paths (/, /dev, /prod, etc.)
* **Cache invalidation**: Automatic on put, rm, cp, mv operations
* **Two-TAB pattern**: First TAB starts fetch, second TAB shows results (instant after cache hit)

## Usage
### Help
```bash
/> help

Commands:
cd           change your relative location within the parameter store
clear        clear the screen
completion   control tab completion settings
config       manage parsh configuration
cp           copy source to dest
decrypt      toggle parameter decryption
exit         exit the program
get          get parameters
help         display help
history      get parameter history
key          set the KMS key
ls           list parameters
mv           move parameters
policy       create named parameter policy
profile      switch to a different AWS IAM profile
put          set parameter
region       change region
rm           remove parameters
```

### List contents of a path
Note: Listing a large number of parameters may take a long time because the maximum number of results per API call is 10. Press ^C to interrupt if a listing is taking too long. Example usage:
```bash
/> ls
dev/
/> ls -r
/dev/app/url
/dev/db/password
/dev/db/username
/> ls /dev/app
url
/>
```

### Change dir and list from current working dir
```bash
/> cd /dev
/dev> ls
app/
db/
/dev>
```

### Get a parameter
```bash
/> get /dev/db/username
[{
  ARN: "arn:aws:ssm:us-east-1:012345678901:parameter/dev/db/username",
  LastModifiedDate: 2019-09-29 23:22:19 +0000 UTC,
  Name: "/dev/db/username",
  Type: "SecureString",
  Value: "foo",
  Version: 1
}]
/> cd /dev/db
/dev/db> get ../app/url
[{
  ARN: "arn:aws:ssm:us-east-1:318677964956:parameter/dev/app/url",
  LastModifiedDate: 2019-09-29 23:22:49 +0000 UTC,
  Name: "/dev/app/url",
  Type: "SecureString",
  Value: "https://www.example.com",
  Version: 1
}]
/dev/db>
```

### Decryption for SecureString parameters

**Global toggle** (affects all get commands):
```bash
/> decrypt
Decrypt is false
/> decrypt true
Decrypt is true
/>
```

**Per-command decryption** (with `-d` or `--decrypt` flag):
```bash
# Decrypt just for this one get (doesn't change global setting)
/> get -d /dev/db/password
[{
  Name: "/dev/db/password",
  Type: "SecureString",
  Value: "secret123",    # ← Decrypted!
  Version: 1
}]

# Without -d flag, uses global decrypt setting
/> get /dev/db/password
[{
  Name: "/dev/db/password",
  Type: "SecureString",
  Value: "<sensitive>",  # ← Hidden (decrypt=false globally)
  Version: 1
}]

# Decrypt multiple parameters at once
/> get -d /dev/db/password /dev/api/key
```

### Get parameter history
```bash
/> history /dev/app/url
[{
  KeyId: "alias/aws/ssm",
  Labels: [],
  LastModifiedDate: 2019-09-29 23:22:49 +0000 UTC,
  LastModifiedUser: "arn:aws:iam::318677964956:root",
  Name: "/dev/app/url",
  Policies: [],
  Tier: "Standard",
  Type: "SecureString",
  Value: "https://www.example.com",
  Version: 1
}]
```

### Copy a parameter
```bash
/> cp /dev/app/url /test/app/url
/> ls -r /dev/app /test/app
/dev/app:
/dev/app/url
/test/app:
/test/app/url
```

### Copy an entire hierarchy
```bash
/> cp -r /dev /test
/> ls -r /test
/test/app/url
/test/db/password
/test/db/username
```

### Remove parameters
```bash
/> rm /test/app/url
/> ls -r /test
/test/db/password
/test/db/username
/> rm -r /test
/> ls -r /test
/>
```

### Put new parameters
```bash
Multiline:
/> put
Input options. End with a blank line.
... name=/dev/app/domain
... value="www.example.com"
... type=String
... description="The domain of the app in dev"
...
/>
```
Single line version:

```bash
/> put name=/dev/app/domain value="www.example.com" type=String description="The domain of the app in dev"
```

Put with a value containing line breaks:

```
/>put name=/secrets/key/private type=SecureString value="-----BEGIN RSA PRIVATE KEY-----\
... data\
... -----END RSA PRIVATE KEY-----"
Put /secrets/key/private version 1
```

### Advanced parameters with policies
Use [parameter policies](https://docs.aws.amazon.com/systems-manager/latest/userguide/parameter-store-policies.html) to do things like expire (automatically delete) parameters at a specified time:
```bash
/> policy urlExpiration Expiration(Timestamp=2013-03-31T21:00:00.000Z)
/> policy ReminderPolicy ExpirationNotification(Before=30,Unit=days) NoChangeNotification(After=7,Unit=days)
/> put name=/dev/app/url value="www.example.com" type=String policies=[urlExpiration,ReminderPolicy]
```

### Switch AWS profile
Switches to another profile as configured in `~/.aws/config`.
```bash
/> profile
default
/> profile project1
/> profile
project1
```

### Change active region
```bash
/> region eu-central-1
/> region
eu-central-1
/>
```

### Operate on other regions
A few examples of working with regions.
```bash
/> put region=eu-central-1  name=/dev/app/domain value="www.example.com" type=String description="The domain of the app in dev"
/> cp -r us-east-1:/dev us-west-2:/dev
/> ls -r us-west-2:/dev
/> region us-east-2
/> get us-west-2:/dev/db/username us-east-1:/dev/db/password
```

###  Read commands in batches
```bash
$ cat << EOF > commands.txt
put name=/dev/app/domain value="www.example.com" type=String description="The domain of the app in dev"
rm /dev/app/domain
cp -r /dev /test
EOF
$ parsh -file commands.txt
$ cat commands.txt | parsh -file -  # Read commands from STDIN
```

###  Inline commands
```
$ parsh put name=/dev/app/domain value="www.example.com" type=String description="The domain of the app in dev"
```

## todo (maybe)
* [ ] Flexible and improved output formats
* [ ] Copy between accounts using profiles
* [ ] Find parameter
* [ ] Integration w/ CloudWatch Events for scheduled parameter updates
* [ ] Export/import
* [ ] Support globbing and/or regex
* [ ] Read parameters as local env variables


## License
MIT

## Contributing/compiling
1. Ensure you have at least go v1.17
```
$ go version
go version go1.17.6 darwin/arm64
```
2. Ensure your `$GOPATH` exists and is in your `$PATH`
```
export GOPATH=$HOME/go
export PATH=$PATH:$GOROOT/bin:$GOPATH/bin
```
3. Clone this repository: `git clone https://github.com/torreirow/parsh.git`
4. Run `cd parsh && make` to build and install the binary to `$GOPATH/bin/parsh`


## Related tools
Tool | Description
---- | -----------
[Chamber](https://github.com/segmentio/chamber) | A tool for managing secrets
[Parameter Store Manager](https://github.com/smblee/parameter-store-manager) | A GUI for working with the Parameter Store
[ssmple](https://github.com/adamcin/ssmple) | Serialize parameter store to properties

## Credits

### Original Project
**ssmsh** by [Ben Whaley](https://github.com/bwhaley/ssmsh) - The original Parameter Store shell

### Libraries
Library | Use
------- | -----
[abiosoft/ishell](https://github.com/abiosoft/ishell) | The interactive shell for golang
[aws-sdk-go](https://github.com/aws/aws-sdk-go) | The AWS SDK for Go
[mattn/go-shellwords](github.com/mattn/go-shellwords) | Parsing for the shell made easy

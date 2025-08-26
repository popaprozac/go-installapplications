# go-installapplications

A modern, Go-based spiritual successor to [InstallApplications](https://github.com/macadmins/installapplications), taking inspiration from [installapplications-swiftly](https://github.com/MichalMMac/installapplications-swiftly).

## ✨ Key Features

### 🏗️ **Architecture & Configuration**
- **Unified mobileconfig**: Single configuration for daemon, agent arguments AND bootstrap payload
- **Configuration hierarchy**: defaults → mobileconfig (shared + mode-specific) → command line
- **Bootstrap sources**: JSON URL OR embedded mobileconfig (with conflict detection)
- **Execution modes**: `daemon`, `agent`, `standalone` (DEP recovery mechanism)
- **Orchestration model**: Daemon is the single orchestrator; agent executes user-context tasks via Unix domain socket IPC

### 🔐 **Authentication & Security**
- **HTTP Basic Authentication**: Username/password for protected servers
- **Custom HTTP Headers**: API keys, Bearer tokens, advanced authentication
- **Flexible header formats**: Both dictionary and array formats supported
- **User context execution**:
  - Daemon/agent: `userscript`/`userfile` run via the agent (user context)
  - Standalone: `userscript`/`userfile` executed via `launchctl asuser` (root → user delegation)
  - Optional reboot: when `Reboot=true`, daemon/standalone initiate a reboot after successful completion

### ⚡ **Enhanced Features from Swift Version**
- **Fail Policy Support**: `failure_is_not_an_option`, `failable`, `failable_execution`
- **Retry Logic**: Per-item retry settings with custom delays
- **Package Receipt Checking**: `pkg_required` logic (skip if already installed)
- **HTTP Redirect Following**: Automatic redirect handling
- **Background Process Tracking**: Optional tracking for `donotwait` items

### 🛠️ **Developer Experience**
- **Shebang Detection**: Support for any interpreter (bash, python, node.js, etc.)
- **Multi-architecture Support**: Intel, Apple Silicon, Universal builds
- **Comprehensive Logging**: File logging for all modes with verbose debugging
- **Binary Size Optimization**: Optimized Go build flags

## 🚀 Quick Start

### Installation via MDM

Deploy the signed `.pkg` via your MDM system with an appropriate mobileconfig.

### Manual Testing (Standalone Mode)

```bash
# Build from source
make build

# Test with embedded mobileconfig
sudo ./go-installapplications --debug --mode standalone

# Test with remote bootstrap
sudo ./go-installapplications --debug --mode standalone --jsonurl https://your-server.com/bootstrap.json
```

## 📋 Configuration

### ⚖️ Configuration Hierarchy

**Program arguments ALWAYS take precedence:**

```
defaults → mobileconfig (shared) → mobileconfig (mode-specific) → command line arguments
```

> **Note**: For `agent` mode, the hierarchy is simplified to `defaults → mobileconfig (shared) → command line arguments` since the agent doesn't use mode-specific overrides.

> **⚠️ Important**: In the mobileconfig itself, `JSONURL` and embedded `bootstrap` are mutually exclusive **per mode**. Choose one bootstrap source per mode:
> - **Option 1**: Top-level embedded bootstrap (shared across all modes)
> - **Option 2**: Remote bootstrap via `JSONURL` in shared/mode-specific settings
> - **Option 3**: Embedded bootstrap in mode-specific sections
> 
> If a mode has both `JSONURL` and `bootstrap` defined, an error is raised. However, when running the binary, a CLI `--jsonurl` will override any embedded bootstrap present in the profile.

### 📱 Mobile Config Structure

Single configuration file for all modes + optional embedded bootstrap:

> **📄 Example**: See `example.mobileconfig` in the root directory for a comprehensive example with all available options.

**Note**: The `agent` mode does not require mode-specific overrides since it acts as an IPC server and uses shared settings (e.g., `Debug`, `Verbose`). Mode-specific overrides are only needed for `daemon` and `standalone` modes. The agent receives its configuration from the daemon via IPC and doesn't need separate configuration.

**Structure**: Both `JSONURL` and `bootstrap` can be placed in multiple locations with the following hierarchy:
- **Top-level `bootstrap`**: Shared embedded bootstrap (equivalent to shared)
- **`shared` section**: Shared settings including `JSONURL` or `bootstrap`
- **Mode-specific sections**: Override bootstrap source for that mode (e.g., standalone can use different JSONURL or embedded bootstrap for testing)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>PayloadContent</key>
    <array>
        <dict>
            <key>PayloadType</key>
            <string>com.github.go-installapplications</string>
            
            <!-- Bootstrap source options (choose one) -->
            <!-- Option 1: Top-level embedded bootstrap (shared) -->
            <!--
            <key>bootstrap</key>
            <dict>
                <key>preflight</key>
                <array>
                    <dict>
                        <key>name</key>
                        <string>System Setup</string>
                        <key>type</key>
                        <string>rootscript</string>
                        <key>url</key>
                        <string>https://your-server.com/setup.sh</string>
                        <key>file</key>
                        <string>setup.sh</string>
                    </dict>
                </array>
                <key>userland</key>
                <array>
                    <dict>
                        <key>name</key>
                        <string>Your Pkg</string>
                        <key>type</key>
                        <string>package</string>
                        <key>url</key>
                        <string>https://downloads.your-pkg.com/mac_releases/your-pkg.pkg</string>
                        <key>file</key>
                        <string>your-pkg.pkg</string>
                        <key>packageid</key>
                        <string>com.your.pkg</string>
                        <key>version</key>
                        <string>4.32.0</string>
                        <key>pkg_required</key>
                        <true/>
                        <key>fail_policy</key>
                        <string>failure_is_not_an_option</string>
                    </dict>
                </array>
            </dict>
            -->
            
            <!-- Option 2: Remote JSON URL in shared settings -->
            <key>shared</key>
            <dict>
                <key>JSONURL</key>
                <string>https://your-server.com/bootstrap.json</string>
                <key>Debug</key>
                <true/>
                <key>HTTPAuthUser</key>
                <string>your-username</string>
                <key>HTTPAuthPassword</key>
                <string>your-password</string>
            </dict>
            
            <!-- Mode-specific overrides -->
            <key>daemon</key>
            <dict>
                <key>MaxRetries</key>
                <integer>5</integer>
            </dict>
            
            <!-- Option 3: Mode-specific bootstrap override -->
            <!--
            <key>standalone</key>
            <dict>
                <key>JSONURL</key>
                <string>https://test-server.com/test-bootstrap.json</string>
            </dict>
            -->
        </dict>
    </array>
</dict>
</plist>
```

### 📦 Install paths and compatibility

- `--installpath /Library/go-installapplications` (default) controls the program’s internal working directory (e.g., where the runtime may store bootstrap.json when downloaded). It does NOT rewrite `item.file` in your JSON; `item.file` always controls the actual destination of downloads and executions.
- `--compat` sets the internal working directory to `/Library/installapplications` (original IA layout). It is mutually exclusive with `--installpath`.
- Update your LaunchDaemon/LaunchAgent plists to include either `--compat` or an explicit `--installpath` so the daemon/agent use the intended layout in production.
- Tip: if you used `--compat` when generating `bootstrap.json` with the helper in `generatejson/`, you will usually want to run the main program with `--compat` as well to keep paths consistent.

### 📄 JSON Bootstrap Format

When using `--jsonurl` or for reference when creating embedded bootstrap:

```json
{
  "preflight": [
    {
      "name": "System Setup",
      "type": "rootscript",
      "url": "https://your-server.com/setup.sh",
      "file": "setup.sh",
      "hash": "sha256-hash-here",
      "fail_policy": "failure_is_not_an_option"
    }
  ],
  "setupassistant": [
    {
      "name": "Configuration Script",
      "type": "rootscript", 
      "url": "https://your-server.com/config.sh",
      "file": "config.sh",
      "donotwait": true,
      "retries": 3,
      "retrywait": 5
    }
  ],
  "userland": [
    {
      "name": "Your Pkg",
      "type": "package",
      "url": "https://downloads.your-pkg.com/mac_releases/your-pkg.pkg",
      "file": "your-pkg.pkg",
      "packageid": "com.your.pkg",
      "version": "4.32.0",
      "pkg_required": true,
      "fail_policy": "failure_is_not_an_option",
      "hash": "sha256-package-hash-here"
    },
    {
      "name": "User Preferences",
      "type": "userscript",
      "url": "https://your-server.com/user-setup.sh", 
      "file": "user-setup.sh",
      "fail_policy": "failable",
      "skip_if": "intel"
    },
    {
      "name": "Company Logo",
      "type": "userfile",
      "url": "https://your-server.com/logo.png",
      "file": "/Users/Shared/logo.png"
    }
  ]
}
```

### 📊 Configuration Options

| Setting | Default | Description | Modes | Command Line |
|---------|---------|-------------|-------|-------------|
| **Mode** | `standalone` | Execution mode | All | `--mode` |
| **Debug** | `false` | Enable debug logging | All | `--debug` |
| **Verbose** | `false` | Enable verbose logging | All | `--verbose` |
| **DryRun** | `false` | Simulate without executing | All | `--dry-run` |
| **JSONURL** | `""` | Remote bootstrap URL | All | `--jsonurl` |
| **InstallPath** | `/Library/go-installapplications` | Installation directory | All | `--installpath` |
| **Compat** | `false` | Use original InstallApplications layout | All | `--compat` |
| **MaxRetries** | `3` | Maximum retry attempts | All | `--max-retries` |
| **RetryDelay** | `5` | Delay between retries (seconds) | All | `--retry-delay` |
| **TrackBackgroundProcesses** | `false` | Track `donotwait` processes | All | `--track-background-processes` |
| **BackgroundTimeout** | `300s` | Background process timeout | All | `--background-timeout` |
| **DownloadMaxConcurrency** | `4` | Maximum concurrent downloads | All | `--download-max-concurrency` |
| **WaitForAgentTimeout** | `86400s` | How long daemon waits for agent socket | Daemon | `--wait-for-agent-timeout` |
| **AgentRequestTimeout** | `7200s` | Timeout per agent RPC request | Daemon | `--agent-request-timeout` |
| **HTTPAuthUser** | `""` | HTTP Basic Auth username | All | `--http-auth-user` |
| **HTTPAuthPassword** | `""` | HTTP Basic Auth password | All | `--http-auth-password` |
| **HTTPHeaders** | `{}` | Custom HTTP headers | All | `--headers` |
| **Reboot** | `false` | Reboot after completion | All | `--reboot` |
| **CleanupOnFailure** | `true` | Clean up files on failure | All | `--cleanup-on-failure` |
| **CleanupOnSuccess** | `true` | Clean up files on success | All | `--cleanup-on-success` |
| **KeepFailedFiles** | `false` | Keep corrupted files for debugging | All | `--keep-failed-files` |
| **ResetRetries** | `false` | Clear retry state before running | All | `--reset-retries` |
| **ProfileDomain** | `com.github.go-installapplications` | macOS preference domain | All | `--profile-domain` |
| **LogFilePath** | `""` | Force logs to file | All | `--log-file` |
| **RetainLogFiles** | `false` | Retain log files from previous runs | All | `--retain-log-files` |
| **FollowRedirects** | `false` | Follow HTTP redirects | All | `--follow-redirects` |
| **SkipValidation** | `false` | Skip bootstrap.json validation | All | `--skip-validation` |
| **LaunchAgentIdentifier** | `com.github.go-installapplications.agent` | LaunchAgent identifier | All | `--laidentifier` |
| **LaunchDaemonIdentifier** | `com.github.go-installapplications.daemon` | LaunchDaemon identifier | All | `--ldidentifier` |

#### Item-Level Options

| Setting | Default | Description | Example Values |
|---------|---------|-------------|----------------|
| **retries** | `3` | Item-specific retry count | `0`, `5`, `10` |
| **retrywait** | `5` | Retry delay in seconds | `3`, `10`, `30` |
| **donotwait** | `false` | Execute in background | `true`, `false` |
| **pkg_required** | `false` | Skip if package already installed | `true`, `false` |
| **fail_policy** | `failable_execution` | Error handling strategy | See table above |
| **skip_if** | `""` | Skip based on architecture | `"intel"`, `"arm64"`, `"x86_64"`, `"apple_silicon"` |
| **hash** | `""` | SHA256 hash for verification | `"sha256-abc123..."` |

#### Phase Execution Order

1. **`preflight`**: System-level preparation (root context, single `rootscript` only)
2. **`setupassistant`**: System configuration (root context, packages + `rootscript`/`rootfile`)  
3. **`userland`**: Mixed root/user items processed in strict order
   - Root items (`package`, `rootscript`, `rootfile`): executed by the daemon (root context)
   - User items (`userscript`, `userfile`): delegated to the agent (user context) via IPC

#### Item Types

| Type | Context | Phases | Description |
|------|---------|--------|-------------|
| **`package`** | Root | All | macOS installer package (`.pkg`) |
| **`rootscript`** | Root | All | Script executed as root |
| **`rootfile`** | Root | setupassistant, userland | File placed with root permissions |
| **`userscript`** | User | userland only | Script executed as logged-in user |
| **`userfile`** | User | userland only | File placed in user context |

#### Fail Policy Values

| Policy | Behavior | Use Case |
|--------|----------|----------|
| **`failure_is_not_an_option`** | Stop phase on any error | Critical components |
| **`failable`** | Continue on all errors | Optional components |  
| **`failable_execution`** | Continue on script errors only | Scripts that may fail, but packages must install |

## 🔧 Advanced Features

### HTTP Authentication (summary)

Use mobileconfig keys:
- `HTTPAuthUser` + `HTTPAuthPassword` for Basic Auth
- `HTTPHeaders` (dict or array) for arbitrary headers
- `HeaderAuthorization` convenience to set `Authorization`

CLI conveniences:
- `--headers "Bearer TOKEN"` sets `Authorization: Bearer TOKEN`
- `--follow-redirects` to control HTTP 30x following (default: false). By default, downloads do not follow redirects; enabling this flag will follow 3xx responses. This differs from Go's default behavior to maintain compatibility with original InstallApplications.

See the shortened guide in `HTTP_AUTH.md` for details.

### Retry Configuration

Per-item retry settings:

```json
{
  "name": "Flaky Download",
  "type": "package",
  "url": "https://unreliable-server.com/app.pkg",
  "file": "app.pkg", 
  "retries": 5,
  "retrywait": 10
}
```

## 📊 Logging & Debugging

### Log Locations

- **Daemon**: `/var/log/go-installapplications/go-installapplications.daemon.log`
- **Agent**: `/var/log/go-installapplications/go-installapplications.agent.log`
- **Standalone**: `/var/log/go-installapplications/go-installapplications.standalone.log` + console

Request/response headers are logged in verbose mode with sensitive values redacted (e.g., Authorization).

The LaunchAgent uses RunAtLoad with KeepAlive SuccessfulExit=false so a clean shutdown does not relaunch it.

The included LaunchDaemon/LaunchAgent plists redirect stdout/stderr to the paths above (via `StandardOutPath`/`StandardErrorPath`). The installer creates `/var/log/go-installapplications` with safe permissions for agent logging.

<!-- ### Remote log shipping (optional)

Ship logs to an external endpoint (e.g., Datadog) in addition to local files:

- `--log-destination https://...` (POST target)
- `--log-provider generic|datadog` (payload shape)
- `--log-header Name=Value` (repeatable) for auth/headers

Example (Datadog v2 intake):

```bash
--log-provider datadog \
--log-destination https://http-intake.logs.datadoghq.com/api/v2/logs \
--log-header DD-API-KEY=XXXX --log-header Content-Type=application/json
``` -->

### Manual testing tips

- Tee daemon/agent logs to a file while still printing to console:

```bash
sudo ./go-installapplications --mode daemon --log-file /tmp/gi-daemon.log
./go-installapplications --mode agent --log-file /tmp/gi-agent.log
```

When `--log-file` is used in daemon mode, the directory is made world-writable (sticky) as a best-effort so the agent can also write logs in the same folder during testing.

### Debug Commands

```bash
# Enable full debugging
sudo ./go-installapplications --debug --verbose --mode standalone

# Monitor logs in real-time
sudo tail -f /var/log/go-installapplications.standalone.log

# Search for authentication issues
sudo grep -i "auth\|header" /var/log/go-installapplications.*.log
```

## 🏗️ Building & Deployment

### Prerequisites

- Go 1.19+
- [munkipkg](https://github.com/munki/munkipkg) (for package creation)
- Apple Developer ID certificate (for code signing)

### Build Commands

```bash
# Build for current architecture
make build

# Build universal binary
make build-universal

# Create signed installer package
make package-universal
```

### Architecture Support

- **Intel Macs**: `make build-intel` / `make package-intel`
- **Apple Silicon**: `make build-arm` / `make package-arm`  
- **Universal**: `make build-universal` / `make package-universal`

## 📚 Documentation

- **[HTTP Authentication Guide](HTTP_AUTH.md)**: Authentication configuration (mobileconfig + CLI)
- **[Build Guide](BUILD.md)**: Building and packaging instructions

## 🔄 Migration from Original InstallApplications

go-installapplications is designed for **seamless migration**:

- ✅ **Backwards compatible** JSON bootstrap format
- ✅ **Same mobile config keys** for authentication  
- ✅ **Identical package/script behavior**
- ✅ **Enhanced features** without breaking changes

Simply replace the original binary with go-installapplications and update your mobileconfig to use the new unified format.

## 🆚 Comparison to InstallApplications

| Feature | Original | go-installapplications |
|---------|----------|----------------------|
| **Core Functionality** | ✅ | ✅ |
| **HTTP Authentication** | ✅ | ✅ Enhanced |
| **Package Receipt Checking** | ✅ | ✅ |
| **Fail Policy Control** | ❌ | ✅ New |
| **User Context Execution** | ✅ | ✅ Enhanced |
| **Unified Configuration** | ❌ | ✅ New |
| **Standalone Recovery Mode** | ❌ | ✅ New |
| **Comprehensive Logging** | Basic | ✅ Enhanced |
| **Multi-architecture** | ❌ | ✅ New |

## 🤝 Contributing

Contributions welcome! Please ensure:

- Code follows Go best practices
- All features have appropriate tests
- Documentation is updated for new features
- Backwards compatibility is maintained

## 📄 License

This project maintains compatibility with the original InstallApplications while adding modern enhancements for enterprise macOS management.

## 🤖 AI

This project used AI to assist in testing, authoring, and documentation. Please raise an issue if you encounter discrepancies!
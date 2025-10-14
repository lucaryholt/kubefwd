# kubefwd

A TUI tool for managing Kubernetes port forwards to GKE services.

## Features

- 🎮 Real-time port forward management for all services
- ⚡ Start/stop individual services or all at once
- 🎯 Quick-start default services with one key press
- 📋 Presets for quickly starting predefined sets of services
- 🔄 Switch between cluster contexts on-the-fly with safety confirmation
- 🌐 Per-service context and namespace overrides
- ⚙️ YAML-based configuration
- 📊 Automatic status monitoring
- 🔍 Debug mode to troubleshoot kubectl commands

## Prerequisites

- Go 1.16 or later
- `kubectl` installed and configured
- Access to a GKE cluster

## Installation

1. Clone or download this repository
2. Install dependencies:
   ```bash
   go mod download
   ```
3. Build the binary:
   ```bash
   go build -o service-helper
   ```
4. (Optional) Move to your PATH:
   ```bash
   sudo mv service-helper /usr/local/bin/
   ```

## Configuration

Create a `.kubefwd.yaml` file in your home directory (or specify a custom path):

```yaml
# The GKE cluster context (use 'kubectl config get-contexts' to list available contexts)
cluster_context: gke_my-project_us-central1_my-cluster

# The namespace where the services are located
namespace: default

# List of services available for port forwarding
services:
  - name: API Server
    service_name: api-service
    remote_port: 8080
    local_port: 8080
    selected_by_default: true

  - name: Database
    service_name: postgres
    remote_port: 5432
    local_port: 5432
    selected_by_default: false

  - name: Redis Cache
    service_name: redis
    remote_port: 6379
    local_port: 6379
    selected_by_default: false

  # Example with per-service context/namespace overrides
  - name: Staging DB
    service_name: postgres
    remote_port: 5432
    local_port: 5433
    selected_by_default: false
    context: gke_staging_cluster    # Override cluster context
    namespace: staging              # Override namespace
```

### Configuration Fields

- **cluster_context**: The kubectl context name for your GKE cluster (global default)
- **cluster_name** (optional): Friendly name for the default cluster (shown in management view and context selection)
- **namespace**: The Kubernetes namespace containing the services (global default)
- **alternative_contexts** (optional): List of alternative cluster contexts for quick switching
  - **name**: Display name for the context
  - **context**: The kubectl context name
- **presets** (optional): Predefined sets of services for quick activation
  - **name**: Display name for the preset
  - **services**: List of service names (must match the `name` field in services list)
- **services**: List of services with the following fields:
  - **name**: Display name shown in the TUI
  - **service_name**: Actual Kubernetes service name
  - **remote_port**: Port on the Kubernetes service
  - **local_port**: Port on your local machine
  - **selected_by_default**: Whether this service should be started with the "d" key
  - **context** (optional): Override the global cluster context for this service
  - **namespace** (optional): Override the global namespace for this service

## Usage

Run the tool with the default config file (`~/.kubefwd.yaml`):
```bash
./service-helper
```

Or specify a custom config file:
```bash
./service-helper --config /path/to/config.yaml
```

Enable debug mode to see kubectl commands:
```bash
./service-helper --debug
```

**Command-line flags:**
- `--config <path>`: Path to configuration file (default: `~/.kubefwd.yaml`)
- `--debug`: Enable debug output showing kubectl commands
- `--help`: Show help message

**First-time setup:**
```bash
# Copy the example config to your home directory
cp config.example.yaml ~/.kubefwd.yaml

# Edit it with your cluster details
nano ~/.kubefwd.yaml
```

### Controls

**Navigation:**
- `↑`/`↓` or `k`/`j`: Navigate through services

**Service Control:**
- `Enter` or `s`: Start/stop the selected service
- `d`: Start all services marked with `selected_by_default: true`
- `a`: Start all services
- `x`: Stop all services
- `p`: Apply a preset (if presets are configured)
- `c`: Change cluster context (if alternative contexts are configured)
- `q`: Stop all services and quit

**Status Indicators:**
- `[RUNNING]` (green): Port forward is active
- `[STARTING]` (orange): Port forward is initializing
- `[STOPPED]` (gray): Port forward is not active
- `[ERROR]` (red): Port forward encountered an error

**Display Information:**
- Services marked as default show a ★ (star) indicator
- Services with overridden context/namespace show `[ctx: name]` or `[ns: name]` tags
- Error messages are displayed below failed services with full kubectl command for debugging

### Context Switching

If you have alternative contexts configured, you can switch between clusters without restarting the tool:

1. Press `c` in the management view to open the context selection screen
2. Navigate to the desired context and press Enter
3. A confirmation screen will appear requiring you to type `cluster_change` exactly
4. Upon confirmation, all running port forwards will be stopped
5. The tool will switch to the new context and return to the management view

**Safety Features:**
- Cannot accidentally switch contexts (requires typing exact phrase)
- All port forwards are cleanly stopped before switching
- Current context is clearly marked in the selection screen

### Presets

Presets allow you to define and quickly apply specific sets of services. This is useful for different development scenarios:

**Using Presets:**
1. Press `p` in the management view to open the preset selection screen
2. Navigate to the desired preset and press Enter
3. The tool will automatically:
   - Stop all currently running port forwards
   - Start only the services defined in the preset
4. Return to the management view with your preset active

## Tips

1. **Find your cluster context**: Run `kubectl config get-contexts` to list available contexts
2. **Check service names**: Run `kubectl get services -n <namespace>` to see available services
3. **Avoid port conflicts**: Make sure the local ports you specify aren't already in use
4. **Test connectivity**: After starting a port forward, test it with `curl localhost:<port>` or your preferred tool

## Troubleshooting

### Port forward fails to start
Check if:
- The service exists: `kubectl get service <service-name> -n <namespace>`
- The port is correct: `kubectl describe service <service-name> -n <namespace>`
- The local port is available: `lsof -i :<local-port>`

### Permission denied
Ensure you have proper RBAC permissions in the cluster:
```bash
kubectl auth can-i get services -n <namespace>
```

## Development

### Project Structure

```
kubefwd/
├── main.go                 # Application entry point
├── config.go               # Configuration parsing
├── portforward.go          # Port forward management
├── selection_view.go       # Service selection screen
├── management_view.go      # Port forward control screen
├── config.example.yaml     # Example configuration
├── README.md               # Documentation
└── go.mod                  # Go dependencies
```

This project uses:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [yaml.v3](https://gopkg.in/yaml.v3) - YAML parsing

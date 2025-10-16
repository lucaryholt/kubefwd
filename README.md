# kubefwd

A TUI tool for managing Kubernetes port forwards to GKE services and proxy connections to GCP resources.

## Features

- 🎮 Real-time port forward management for all services
- ⚡ Start/stop individual services or all at once
- ☁️ Proxy pod support for GCP services (CloudSQL, MemoryStore, etc.)
- 🎯 Quick-start default services with one key press
- 📋 Presets for quickly starting predefined sets of services
- 🔄 Switch between cluster contexts on-the-fly with safety confirmation
- 🌐 Per-service context and namespace overrides
- 🔁 Automatic retry with exponential backoff when connections fail
- ⚙️ YAML-based configuration
- 📊 Automatic status monitoring
- 🔍 Debug mode to troubleshoot kubectl commands

## Prerequisites

- Go 1.16 or later
- `kubectl` installed and configured
- Access to a GKE cluster

## Installation

### Build yourself

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

### Prebuilt

A prebuilt binary is available on the [releases page](https://github.com/lucaryholt/kubefwd/releases) of this repo.

## Configuration

Create a `.kubefwd.yaml` file in your home directory (or specify a custom path):

```yaml
# The GKE cluster context (use 'kubectl config get-contexts' to list available contexts)
cluster_context: gke_my-project_us-central1_my-cluster

# The namespace where the services are located
namespace: default

# Optional: Maximum retry attempts for port forwards when they fail
# -1 = infinite retries (default), 0 = no retries, N = retry N times
# Uses exponential backoff: 1s, 2s, 4s, 8s, ... up to 60s max
max_retries: -1

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
  
  # Example with custom retry configuration
  - name: Flaky Service
    service_name: unstable-api
    remote_port: 8080
    local_port: 8081
    selected_by_default: false
    max_retries: 5                  # Override global retry setting

# Optional: Proxy services for GCP resources that need a proxy pod
# These services create a shared proxy pod in the cluster to relay traffic
proxy_services:
  - name: CloudSQL Production
    target_host: 10.1.2.3          # Private IP of CloudSQL instance
    target_port: 5432
    local_port: 5432
    selected_by_default: false

  - name: Redis MemoryStore
    target_host: 10.1.3.5          # Private IP of MemoryStore instance
    target_port: 6379
    local_port: 6380
    selected_by_default: true
```

### Configuration Fields

- **cluster_context**: The kubectl context name for your GKE cluster (global default)
- **cluster_name** (optional): Friendly name for the default cluster (shown in management view and context selection)
- **namespace**: The Kubernetes namespace containing the services (global default)
- **max_retries** (optional): Maximum retry attempts for port forwards when they fail (default: -1 for infinite)
  - `-1`: Infinite retries (keeps trying until manually stopped)
  - `0`: No retries (fails immediately on error)
  - `N`: Retry up to N times before giving up
  - Uses exponential backoff: 1s, 2s, 4s, 8s, 16s, 32s, 60s (capped at 60s)
- **alternative_contexts** (optional): List of alternative cluster contexts for quick switching
  - **name**: Display name for the context
  - **context**: The kubectl context name
- **presets** (optional): Predefined sets of services for quick activation
  - **name**: Display name for the preset
  - **services**: List of service names (must match the `name` field in services list)
- **services**: List of direct Kubernetes services with the following fields:
  - **name**: Display name shown in the TUI
  - **service_name**: Actual Kubernetes service name
  - **remote_port**: Port on the Kubernetes service
  - **local_port**: Port on your local machine
  - **selected_by_default**: Whether this service should be started with the "d" key
  - **context** (optional): Override the global cluster context for this service
  - **namespace** (optional): Override the global namespace for this service
  - **max_retries** (optional): Override the global max_retries setting for this specific service
- **proxy_pod_name** (optional): Name for the shared proxy pod (default: `kubefwd-proxy`)
- **proxy_pod_image** (optional): Container image for proxy pod (default: `alpine/socat:latest`)
- **proxy_pod_context** (optional): Context where the proxy pod should be created (default: uses `cluster_context`)
- **proxy_pod_namespace** (optional): Namespace where the proxy pod should be created (default: uses `namespace`)
- **proxy_services** (optional): List of proxy services for GCP resources with the following fields:
  - **name**: Display name shown in the TUI
  - **target_host**: IP address or hostname of the target GCP resource (e.g., CloudSQL private IP)
  - **target_port**: Port on the target resource
  - **local_port**: Port on your local machine
  - **selected_by_default**: Whether this service should be started with the "d" key
  - **context** (optional): Override the global cluster context for this proxy
  - **namespace** (optional): Override the global namespace for this proxy
  - **max_retries** (optional): Override the global max_retries setting for this specific proxy

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
- `[RETRYING X/Y]` (orange): Port forward failed and is retrying (X = current attempt, Y = max attempts)
- `[RETRYING X]` (orange): Port forward failed and is retrying infinitely (X = current attempt)
- `[ERROR]` (red): Port forward encountered an error (max retries exceeded or retries disabled)

**Display Information:**
- ⚡ Direct services (kubectl port-forward to Kubernetes services)
- ☁️ Proxy services (proxy pod connections to GCP resources)
- Services marked as default show a ★ (star) indicator
- Services with overridden context/namespace show `[ctx: name]` or `[ns: name]` tags
- Error messages are displayed below failed services with full kubectl command for debugging
- Proxy pod status shows creation state and number of active connections

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

## Proxy Pod for GCP Resources

The proxy pod feature allows you to connect to GCP resources (CloudSQL, MemoryStore, etc.) that don't have direct Kubernetes services but are accessible from within the cluster.

### How It Works

1. **Shared Proxy Pod**: All selected proxy services share a single-container pod with multiple socat processes
2. **Selection-Based Management**: Use the proxy selection screen (press `r`) to choose which services to activate
3. **Traffic Relay**: The pod uses `socat` to relay TCP traffic from the pod to the target GCP resource
4. **Port Forwarding**: Standard kubectl port-forward connects your local machine to the proxy pod
5. **Manual Lifecycle**: Pod is created/recreated when you apply selection changes

### Configuration

Add proxy services to your `.kubefwd.yaml`:

```yaml
# Optional: Customize proxy pod settings
proxy_pod_name: kubefwd-proxy           # Default name for the pod
proxy_pod_image: alpine/socat:latest    # Container image (must have socat)
proxy_pod_context: gke_proxy_cluster    # Optional: different cluster for proxy pod
proxy_pod_namespace: proxy-infra        # Optional: different namespace for proxy pod

proxy_services:
  - name: CloudSQL Production
    target_host: 10.1.2.3              # Private IP of CloudSQL instance
    target_port: 5432                  # PostgreSQL port
    local_port: 5432                   # Local port to bind
    selected_by_default: false

  - name: Redis MemoryStore
    target_host: 10.1.3.5              # Private IP of MemoryStore
    target_port: 6379                  # Redis port
    local_port: 6380                   # Use different port locally
    selected_by_default: true
```

### Getting GCP Resource IPs

**CloudSQL:**
```bash
# Get the private IP address of your CloudSQL instance
gcloud sql instances describe INSTANCE_NAME --format="value(ipAddresses[0].ipAddress)"
```

**MemoryStore (Redis):**
```bash
# Get the Redis instance IP
gcloud redis instances describe INSTANCE_NAME --region=REGION --format="value(host)"
```

**MemoryStore (Memcached):**
```bash
# Get the Memcached instance IP
gcloud memcache instances describe INSTANCE_NAME --region=REGION --format="value(memcacheNodes[0].host)"
```

### Split-View Interface

The main management view displays proxy services in a separate pane:

```
┌─────────────────────────────────────┬──────────────────────┐
│ kubefwd                              │ Proxy Services       │
│                                      │                      │
│ Cluster: Production                  │ Pod: [READY] (2)     │
│ Namespace: default                   │                      │
│                                      │ [✓] CloudSQL Staging │
│ ▶ ★ API Server    [RUNNING] :8080   │ [✓] Redis Store      │
│   ★ Database      [STOPPED] :5432   │ [ ] CloudSQL Prod    │
│     Redis Cache   [STOPPED] :6379   │ [ ] MySQL Prod       │
│                                      │                      │
│ ↑/↓: navigate • r: proxy • q: quit  │ Press 'r' to manage  │
└─────────────────────────────────────┴──────────────────────┘
```

**Left Pane**: Interactive direct Kubernetes service management  
**Right Pane**: Read-only proxy service status overview

### Using Proxy Services

**Step 1: Press `r` to open proxy selection**

```
Select Proxy Services

  [ ] CloudSQL Production       :5432 -> 10.1.2.3:5432
  [✓] CloudSQL Staging          :5433 -> 10.1.2.4:5432
  [✓] Redis MemoryStore         :6380 -> 10.1.3.5:6379
  [ ] MySQL Production          :3306 -> 10.1.4.6:3306

Currently active: CloudSQL Staging, Redis MemoryStore

space: toggle • enter: apply changes • esc: cancel
```

**Step 2: Select/deselect services**
- Use `↑`/`↓` or `j`/`k` to navigate
- Press `space` to toggle selection
- Services with `[✓]` will be activated

**Step 3: Press `enter` to apply**
- Stops all existing proxy forwards
- Deletes old proxy pod (if selection changed)
- Creates new proxy pod with selected services
- Starts port-forwards for all selected services

**Step 4: Press `esc` or `q` to cancel** without changes

**Step 5: Connect to services**
- Connect to `localhost:<local_port>` with your database client
- Services remain active until you change selection

### Proxy Pod Status

The management view shows the proxy pod status when proxy services are configured:
- `[NOT CREATED]`: Pod hasn't been created yet
- `[CREATING]`: Pod is being created and starting
- `[READY]`: Pod is running and ready for connections
- `[ERROR]`: Pod creation or readiness check failed
- Active connection count shows how many proxy services are currently using the pod

### Proxy Pod Context and Namespace

By default, the proxy pod is created in the same context and namespace as your global configuration. However, you can override these:

**Use Cases:**
- **Different Cluster**: If your main services are in one cluster but only a specific cluster has VPC access to GCP resources
- **Separate Namespace**: For organizational purposes, network policies, or RBAC separation
- **Dedicated Infrastructure**: Keep proxy infrastructure separate from application services

**Example Configuration:**
```yaml
cluster_context: gke_my-app-cluster      # Main cluster for services
namespace: default                       # Main namespace

# Proxy pod in different cluster that has VPC peering to GCP
proxy_pod_context: gke_my-proxy-cluster
proxy_pod_namespace: proxy-infra
```

### Behavior Notes

- **Shared Resource**: All proxy services share the same pod to minimize cluster resource usage
- **Automatic Management**: Pod lifecycle is fully automatic - no manual intervention needed
- **Retry Support**: Like direct services, proxy connections support automatic retry with exponential backoff
- **Context Overrides**: Each proxy service can use different cluster contexts/namespaces
- **Proxy Pod Location**: The proxy pod itself is created in the configured proxy_pod_context/proxy_pod_namespace
- **Network Requirements**: The proxy pod must have network access to the target GCP resources (VPC peering, etc.)

## Automatic Retry Feature

The tool automatically retries failed port forwards with exponential backoff to handle transient network issues and connection drops.

### How It Works

1. **When a port forward fails** (connection lost, pod restart, network issue), the tool automatically attempts to reconnect
2. **Exponential backoff**: Delays between retries increase exponentially: 1s, 2s, 4s, 8s, 16s, 32s, up to a maximum of 60s
3. **Configurable limits**: Set globally or per-service
   - `-1` (default): Retry indefinitely until manually stopped
   - `0`: Disable retries (fail immediately)
   - `N`: Retry up to N times before giving up

### Configuration Examples

**Global infinite retries (default):**
```yaml
max_retries: -1  # Never give up, keep retrying
```

**Disable retries globally:**
```yaml
max_retries: 0  # Fail immediately on error
```

**Limited retries globally:**
```yaml
max_retries: 5  # Try 5 times then give up
```

**Per-service override:**
```yaml
max_retries: -1  # Global default: infinite

services:
  - name: Critical Service
    service_name: api
    remote_port: 8080
    local_port: 8080
    max_retries: 10  # Only retry 10 times for this service
  
  - name: Development Service
    service_name: dev-api
    remote_port: 8080
    local_port: 8081
    # Uses global default (infinite retries)
```

### Behavior Notes

- **Manual stop prevents retry**: When you manually stop a service (press `s` or `x`), it will not automatically retry
- **Starting resets retry count**: Manually starting a service in retry/error state resets the retry counter
- **UI indication**: Services in retry mode show `[RETRYING X/Y]` or `[RETRYING X]` status with retry attempt count
- **Error messages**: After max retries exceeded, full error details are displayed including the number of retry attempts

## Tips

1. **Find your cluster context**: Run `kubectl config get-contexts` to list available contexts
2. **Check service names**: Run `kubectl get services -n <namespace>` to see available services
3. **Avoid port conflicts**: Make sure the local ports you specify aren't already in use
4. **Test connectivity**: After starting a port forward, test it with `curl localhost:<port>` or your preferred tool
5. **Unreliable connections**: For flaky networks or frequently restarting pods, keep the default infinite retries enabled
6. **Development environment**: Consider setting `max_retries: 3` for development services that may not always be available
7. **GCP Resources**: Use proxy services for CloudSQL, MemoryStore, and other GCP resources with private IPs
8. **Unified Management**: Both direct services and proxy services work identically in the UI - start/stop them the same way

## Troubleshooting

### Port forward fails to start
Check if:
- The service exists: `kubectl get service <service-name> -n <namespace>`
- The port is correct: `kubectl describe service <service-name> -n <namespace>`
- The local port is available: `lsof -i :<local-port>`

### Port forward keeps retrying
If a service is stuck in retry mode:
- Check the error message displayed below the service (may require scrolling)
- Verify the pod is running: `kubectl get pods -n <namespace>`
- Check pod logs: `kubectl logs <pod-name> -n <namespace>`
- Manually stop and restart the service (press `s` twice) to reset retry counter
- If the issue persists, set `max_retries: 0` for that service to disable retries

### Too many retries
If retries are too aggressive for your use case:
- Set a lower `max_retries` value globally or per-service
- Set `max_retries: 0` to disable retries for specific services
- Use `--debug` flag to see detailed retry information in the logs

### Permission denied
Ensure you have proper RBAC permissions in the cluster:
```bash
kubectl auth can-i get services -n <namespace>
kubectl auth can-i create pods -n <namespace>  # For proxy services
```

### Proxy pod fails to create
If proxy services show errors:
- Check RBAC permissions: `kubectl auth can-i create pods -n <namespace>`
- Verify the namespace exists: `kubectl get namespace <namespace>`
- Check pod status manually: `kubectl get pod kubefwd-proxy -n <namespace>`
- View pod logs: `kubectl logs kubefwd-proxy -n <namespace> --all-containers`
- Verify network connectivity from cluster to GCP resource (VPC peering, firewall rules)
- Use `--debug` flag to see the exact pod spec being created

### Proxy connection works but can't reach GCP resource
- Verify the target IP/hostname is correct (use `gcloud` commands to get current IPs)
- Check VPC peering between GKE and GCP resource
- Verify firewall rules allow traffic from GKE cluster to the GCP resource
- Test connectivity from within the cluster: `kubectl run -it --rm debug --image=alpine --restart=Never -- sh` then `nc -zv <target_host> <target_port>`

## Development

### Project Structure

```
kubefwd/
├── main.go                       # Application entry point
├── app_model.go                  # Root application model
├── config.go                     # Configuration parsing
├── portforward.go                # Direct port forward management
├── proxypod.go                   # Proxy pod and forward management
├── context_selection_view.go     # Context switching screen
├── confirmation_view.go          # Context change confirmation
├── preset_selection_view.go      # Preset selection screen
├── management_view.go            # Port forward control screen
├── config.example.yaml           # Example configuration
├── README.md                     # Documentation
└── go.mod                        # Go dependencies
```

This project uses:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [yaml.v3](https://gopkg.in/yaml.v3) - YAML parsing

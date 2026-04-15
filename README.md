# kubefwd

A web-based tool for managing Kubernetes port forwards to GKE services and proxy connections to GCP resources.

## Features

- Real-time port forward management for all services
- Start/stop individual services or all at once — click a service row or use the toolbar buttons
- Proxy pod support for GCP services (CloudSQL, MemoryStore, etc.)
- Quick-start default services on launch
- Presets for quickly starting predefined sets of services
- Switch between cluster contexts on-the-fly with safety confirmation
- Per-service context and namespace overrides
- Automatic retry with exponential backoff when connections fail
- Port status checker to identify and kill processes using configured ports
- SQL traffic monitoring via [sql-tap](https://github.com/mickamy/sql-tap)
- **Explore tab**: discover Kubernetes services and GCP resources (Cloud SQL, Memorystore) and add them to your config with one click
- **YAML file** or **SQLite** configuration (normalized relational schema in the database)
- Add or remove normal and proxy services from the web UI (persisted to the active store)
- Import a full YAML config from the Config tab (or seed SQLite via CLI)
- Live status updates via Server-Sent Events (no polling)
- Debug mode to troubleshoot kubectl commands

## Prerequisites

- Go 1.25 or later (see `go.mod`)
- `kubectl` installed and configured
- Access to a GKE cluster
- `gcloud` CLI (optional — required only for GCP resource discovery in the Explore tab)

## Installation

### Build yourself

1. Clone or download this repository
2. Install dependencies:
   ```bash
   go mod download
   ```
3. Build the binary:
   ```bash
   go build -o kubefwd
   ```
4. (Optional) Move to your PATH:
   ```bash
   sudo mv kubefwd /usr/local/bin/
   ```

### Prebuilt

A prebuilt binary is available on the [releases page](https://github.com/lucaryholt/kubefwd/releases) of this repo.

## Configuration

Configuration can be stored in a **YAML file** (default) or a **SQLite database**. Both support the same schema: cluster settings, `services`, `proxy_services`, `presets`, and `alternative_contexts`.

### YAML file (default)

Create a `.kubefwd.yaml` file in your home directory (or specify a custom path with `--config`):

```yaml
# The GKE cluster context (use 'kubectl config get-contexts' to list available contexts)
cluster_context: gke_my-project_us-central1_my-cluster

# The namespace where the services are located
namespace: default

# Optional: Port for the web UI (default: 8765)
web_port: 8765

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
# Each entry must set proxy_pod_context and proxy_pod_namespace (see config.example.yaml)
proxy_services:
  - name: CloudSQL Production
    target_host: 10.1.2.3          # Private IP of CloudSQL instance
    target_port: 5432
    local_port: 5432
    selected_by_default: false
    proxy_pod_context: gke_my-project_us-central1_my-cluster
    proxy_pod_namespace: default

  - name: Redis MemoryStore
    target_host: 10.1.3.5          # Private IP of MemoryStore instance
    target_port: 6379
    local_port: 6380
    selected_by_default: true
    proxy_pod_context: gke_my-project_us-central1_my-cluster
    proxy_pod_namespace: default
```

### SQLite storage

Use **`--db /path/to/config.db`** to read and write configuration in a SQLite file instead of YAML. The database uses normalized tables (`settings`, `services`, `proxy_services`, `presets`, etc.); there is no single YAML blob stored in the DB.

- **First-time setup:** the database must contain a valid config before kubefwd starts. Use **`--import-yaml /path/to/config.yaml`** together with **`--db`** to import a YAML file and then run. If the DB is empty, kubefwd exits with an error until you import. After the first successful start, you can paste YAML in the **Config** tab or add services from the **Services** / **Proxy** tabs; changes are written back to the SQLite file.

Example:

```bash
./kubefwd --db ~/.kubefwd/kubefwd.db --import-yaml ./config.example.yaml
# later:
./kubefwd --db ~/.kubefwd/kubefwd.db
```

When `--db` is set, **`--config` is ignored** (YAML path is not used).

### Configuration Fields

- **cluster_context**: The kubectl context name for your GKE cluster (global default)
- **cluster_name** (optional): Friendly name for the cluster shown in the web UI header
- **namespace**: The Kubernetes namespace containing the services (global default)
- **web_port** (optional): Port the web UI listens on (default: `8765`)
- **max_retries** (optional): Maximum retry attempts for port forwards (default: `-1` for infinite)
  - `-1`: Infinite retries (keeps trying until manually stopped)
  - `0`: No retries (fails immediately on error)
  - `N`: Retry up to N times before giving up
  - Uses exponential backoff: 1s, 2s, 4s, 8s, 16s, 32s, 60s (capped at 60s)
- **alternative_contexts** (optional): List of alternative cluster contexts for quick switching
  - **name**: Display name for the context
  - **context**: The kubectl context name
- **presets** (optional): Predefined sets of services for quick activation
  - **name**: Display name for the preset
  - **services**: List of service names (must match the `name` field in the services list)
- **services**: List of direct Kubernetes services with the following fields:
  - **name**: Display name shown in the UI
  - **service_name**: Actual Kubernetes service name
  - **remote_port**: Port on the Kubernetes service
  - **local_port**: Port on your local machine
  - **selected_by_default**: Whether this service is started with `--default` or "Start Defaults"
  - **context** (optional): Override the global cluster context for this service
  - **namespace** (optional): Override the global namespace for this service
  - **max_retries** (optional): Override the global max_retries setting for this service
  - **sql_tap_port** (optional): Port for sql-tap proxy (enables SQL traffic monitoring)
  - **sql_tap_driver** (optional): Database driver for sql-tap (`postgres` or `mysql`)
  - **sql_tap_grpc_port** (optional): gRPC port for sql-tap client (default: auto-assigned starting at 9091)
- **proxy_pod_name** (optional): Name for the shared proxy pod (default: `kubefwd-proxy`)
- **proxy_pod_image** (optional): Container image for proxy pod (default: `alpine/socat:latest`)
- **proxy_pod_context** (optional): Context where the proxy pod is created (default: uses `cluster_context`)
- **proxy_pod_namespace** (optional): Namespace where the proxy pod is created (default: uses `namespace`)
- **proxy_services** (optional): List of proxy services for GCP resources with the following fields:
  - **name**: Display name shown in the UI
  - **target_host**: IP address or hostname of the target GCP resource (e.g., CloudSQL private IP)
  - **target_port**: Port on the target resource
  - **local_port**: Port on your local machine
  - **selected_by_default**: Whether this service is started with `--default-proxy` or "Start Defaults"
  - **proxy_pod_context** (required): kubectl context where the proxy pod is created
  - **proxy_pod_namespace** (required): Namespace where the proxy pod is created
  - **max_retries** (optional): Override the global max_retries setting for this proxy
  - **sql_tap_port** (optional): Port for sql-tap proxy (enables SQL traffic monitoring)
  - **sql_tap_driver** (optional): Database driver for sql-tap (`postgres` or `mysql`)
  - **sql_tap_grpc_port** (optional): gRPC port for sql-tap client (default: auto-assigned starting at 9091)

## Usage

Run with the default config file (`~/.kubefwd.yaml`):
```bash
./kubefwd
```

Or specify a custom config file:
```bash
./kubefwd --config /path/to/config.yaml
```

Use SQLite instead of a YAML file:
```bash
./kubefwd --db ~/.kubefwd/kubefwd.db --import-yaml /path/to/config.yaml   # first time
./kubefwd --db ~/.kubefwd/kubefwd.db
```

On startup, kubefwd prints the URL of its web interface and keeps running:

```
kubefwd running at http://localhost:8765
```

Open the URL in your browser. Press `Ctrl+C` to stop all services and exit.

**Command-line flags:**
- `--config <path>`: Path to YAML configuration file (default: `~/.kubefwd.yaml`). Ignored when `--db` is set.
- `--db <path>`: Use a SQLite file for configuration instead of YAML.
- `--import-yaml <path>`: Import YAML into the SQLite database (only with `--db`), then continue startup.
- `--debug`: Enable debug output showing kubectl commands (written to stderr and `/tmp/kubefwd-debug.log`)
- `--default`: Auto-start services marked with `selected_by_default: true` on launch
- `--default-proxy`: Auto-start proxy services marked with `selected_by_default: true` on launch

**First-time setup:**
```bash
cp config.example.yaml ~/.kubefwd.yaml
nano ~/.kubefwd.yaml
```

**Quick Start with Defaults:**
```bash
# Launch with default services already running
./kubefwd --default

# Launch with default proxy services already running
./kubefwd --default-proxy

# Launch with both types already running
./kubefwd --default --default-proxy
```

## Web Interface

kubefwd serves a browser-based dashboard at `http://localhost:<web_port>` (default: `http://localhost:8765`). The port is configurable via `web_port` in your config file.

The dashboard has seven tabs.

### Services tab

Displays all configured port forwards with live status indicators:

- **Status dot colours**: green = running, amber (pulsing) = starting, red = error, grey = stopped
- **Click any row** to toggle that service on/off, or use the dedicated Start/Stop button on the right
- **Toolbar buttons**: Start Defaults, Start All, Stop All, **＋ Add service** (form to append a service to the saved configuration)
- **✕** on a row removes that service from the saved configuration (with confirmation)
- **Running count** shown in the toolbar right area
- Services in retry mode show the attempt counter (e.g. `↻ 2/5` or `↻ 3/∞`)
- Error messages appear inline below a failed service row

### Proxy tab

Always available. When there are no proxy services yet, the tab explains how to add one.

- **＋ Add proxy service**: form to add a proxy entry (target host/port, local port, proxy pod context/namespace)
- **▶ Start Defaults** / **↺ Reset All Pods** in the header for bulk actions
- Proxy services are grouped by **proxy pod context + namespace**; each group shows **pod status**, **▶ Start Pod**, and **✕ Kill Pod**
- Per-row **▶ Start** / **■ Stop** for the port-forward, **✕** to remove the entry from the saved configuration
- **ℹ sql-tap** (when configured): expands an inline panel with ports and `sql-tap localhost:<grpc_port>`

### Port Checker tab

Shows every port defined in your config (both direct and proxy services) and whether anything is currently listening on it:

- **free** (green): nothing is using the port
- **kubefwd** (blue): in use by a kubefwd-managed process
- **external** (amber): in use by a process not managed by kubefwd

Click **Kill** next to an external process to send it SIGTERM (with a confirmation dialog). Click **↻ Refresh** to re-query.

### Presets tab

Shown only when `presets` are configured. Click any preset card to stop all running services and start only the services in that preset (requires confirmation).

### Contexts tab

Shown only when `alternative_contexts` are configured. Click a context row to switch the active cluster context — all running services will be stopped and the context changes immediately (requires confirmation).

### Explore tab

Browse available Kubernetes services and GCP resources and add them to your configuration with one click. Both sections are collapsible and collapsed by default; expanding a section automatically triggers the initial data load.

**Kubernetes Services** — discover services that can be port-forwarded:

1. Expand the section — contexts are loaded automatically from `kubectl config get-contexts`
2. Select a **context** (pre-selects the current kubefwd context)
3. Select a **namespace** (pre-selects the current kubefwd namespace)
4. Services are listed with their type (ClusterIP, NodePort, etc.) and ports
5. Click **+ Add :port** to add a service to the config — the service name, remote port, and local port are pre-filled
6. Services already in the config show an **added** badge instead

**GCP Resources** — discover Cloud SQL and Memorystore instances (requires `gcloud` CLI):

1. Expand the section — GCP projects are loaded automatically from `gcloud projects list`
2. Select a **GCP project** (pre-selects the active gcloud project)
3. Select a **proxy pod context** and **namespace** (these are the K8s context/namespace where the socat proxy pod will run)
4. Click **Scan** to discover Cloud SQL and Memorystore (Redis) instances in the selected project
5. Click **+ Add** to add an instance as a proxy service — target host (private IP), target port, and proxy pod context/namespace are pre-filled
6. If `gcloud` is not installed, the section shows a message instead of failing

### Config tab

- **Import YAML**: paste a full kubefwd YAML document and click **Import** to replace the stored configuration and reload the app (same validation as the file/DB load path)
- **↻ Reload from disk / DB**: reload configuration from the YAML file or SQLite store without restarting the process (stops all running services first). The active cluster context is preserved when switching was done only in memory (same as before).
- Summary rows include **`config_source`**: path to the YAML file, or `sqlite:` plus the database path

Changes made from the Services/Proxy tabs or Config import are **persisted** to whichever store is in use (atomic YAML replace or SQLite transaction).

## Proxy Pod for GCP Resources

The proxy pod feature allows you to connect to GCP resources (CloudSQL, MemoryStore, etc.) that don't have direct Kubernetes services but are accessible from within the cluster.

### How It Works

1. **Shared Proxy Pod**: All selected proxy services share a single-container pod running multiple `socat` processes
2. **Traffic Relay**: The pod relays TCP traffic from unique pod-internal ports to the target GCP resource IPs
3. **Port Forwarding**: Standard `kubectl port-forward` connects your local machine to the proxy pod
4. **Managed Lifecycle**: The pod is created/recreated when you apply a selection change or reset it

### Configuration

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
    target_host: 10.1.3.5             # Private IP of MemoryStore
    target_port: 6379                 # Redis port
    local_port: 6380                  # Use different port locally
    selected_by_default: true
```

### Getting GCP Resource IPs

**CloudSQL:**
```bash
gcloud sql instances describe INSTANCE_NAME --format="value(ipAddresses[0].ipAddress)"
```

**MemoryStore (Redis):**
```bash
gcloud redis instances describe INSTANCE_NAME --region=REGION --format="value(host)"
```

### Proxy Pod Context and Namespace

By default, the proxy pod is created in the same context and namespace as your global configuration. Override when:
- Only a specific cluster has VPC access to GCP resources
- You want proxy infrastructure in a separate namespace

```yaml
cluster_context: gke_my-app-cluster      # Main cluster for services
namespace: default

# Proxy pod in different cluster with VPC peering to GCP
proxy_pod_context: gke_my-proxy-cluster
proxy_pod_namespace: proxy-infra
```

## SQL Traffic Monitoring with sql-tap

kubefwd supports real-time SQL traffic monitoring through [sql-tap](https://github.com/mickamy/sql-tap), a transparent database proxy that captures and displays SQL queries as they flow between your application and database.

### Installation

**Homebrew (macOS/Linux):**
```bash
brew install mickamy/tap/sql-tap
```

**Go Install:**
```bash
go install github.com/mickamy/sql-tap/cmd/sql-tapd@latest
go install github.com/mickamy/sql-tap/cmd/sql-tap@latest
```

### Configuration

```yaml
services:
  - name: Postgres Database
    service_name: postgres
    remote_port: 5432
    local_port: 5432
    sql_tap_port: 5433          # Port where sql-tapd listens
    sql_tap_driver: postgres    # postgres or mysql

proxy_services:
  - name: CloudSQL Production
    target_host: 10.1.2.3
    target_port: 5432
    local_port: 5432
    sql_tap_port: 5433
    sql_tap_driver: postgres
```

**Configuration fields:**
- `sql_tap_port`: Port where sql-tapd listens (your application connects here)
- `sql_tap_driver`: Database driver type (`postgres` or `mysql`)
- `sql_tap_grpc_port` (optional): gRPC port for the client (default: auto-assigned starting at 9091)

**Important:** `sql_tap_port` must differ from `local_port`. Both `sql_tap_port` and `sql_tap_driver` are required when sql-tap is enabled.

### How It Works

```
Application                                    Port Forward              Database
     |                                               |                       |
     | Connect to localhost:5433                     |                       |
     v                                               v                       |
sql-tapd (proxy)  ------>  localhost:5432  ------>  kubectl  ------------>  DB
(sql_tap_port)            (local_port)            port-forward         (remote DB)
```

1. kubefwd starts the normal port-forward (`localhost:5432` → database)
2. kubefwd automatically starts `sql-tapd` (`localhost:5433` → `localhost:5432`)
3. Your application connects to `localhost:5433` (the sql-tap port)
4. sql-tapd forwards traffic while logging queries
5. Run `sql-tap` in a terminal to view queries in real-time

### Usage Workflow

**Step 1:** Start the service via the web UI. kubefwd automatically starts both the port-forward and sql-tapd.

**Step 2:** Update your application's connection string to the sql-tap port:
```bash
# Before (direct)
DATABASE_URL="postgres://user:pass@localhost:5432/mydb"

# After (via sql-tap)
DATABASE_URL="postgres://user:pass@localhost:5433/mydb"
```

**Step 3:** View queries in real-time. On the **Proxy** tab, click **ℹ sql-tap** next to the service to see the exact command:
```bash
sql-tap localhost:9091
```

For multiple databases, each gets its own gRPC port (9091, 9092, 9093, …).

### Lifecycle Management

- **Starting**: Port-forward starts first, then sql-tapd after a brief delay
- **Stopping**: sql-tapd stops first, then the port-forward
- **Reset Pod**: Clicking "↺ Reset Pod" on the Proxy tab also stops all sql-tap instances before deleting the pod
- **Retries**: When auto-retry fires, both processes restart together

## Automatic Retry

The tool automatically retries failed port forwards with exponential backoff (1s, 2s, 4s, … up to 60s).

### Configuration

```yaml
max_retries: -1   # Infinite (default)
max_retries: 0    # Disabled
max_retries: 5    # Fixed limit

services:
  - name: Critical Service
    service_name: api
    remote_port: 8080
    local_port: 8080
    max_retries: 10   # Per-service override
```

### Behaviour

- Manual stop prevents retry
- Starting a service in retry/error state resets the counter
- The web UI shows `↻ X/Y` (or `↻ X/∞`) in the service row when retrying

## Tips

1. **Find your cluster context**: `kubectl config get-contexts` (or use the Explore tab)
2. **Check service names**: `kubectl get services -n <namespace>` (or browse them in the Explore tab)
3. **Avoid port conflicts**: Make sure the local ports you specify aren't already in use
4. **Test connectivity**: After starting a port forward, test with `curl localhost:<port>`
5. **Unreliable connections**: Keep the default infinite retries for flaky networks or frequently restarting pods
6. **Development environment**: Consider `max_retries: 3` for services that may not always be available
7. **GCP Resources**: Use the Explore tab to discover CloudSQL/MemoryStore instances, or manually configure proxy services for GCP resources with private IPs
8. **Quick Start**: Use `--default` / `--default-proxy` to auto-start common services on launch
9. **Custom port**: Set `web_port` in your config to change the web UI port (e.g. `web_port: 9000`)
10. **Multiple environments**: Use different YAML files (`--config`) or different SQLite files (`--db`) to manage separate clusters

## Troubleshooting

### Port forward fails to start
```bash
kubectl get service <service-name> -n <namespace>
kubectl describe service <service-name> -n <namespace>
lsof -i :<local-port>
```

### Port forward keeps retrying
- Check the error shown in the service row in the web UI
- Verify the pod is running: `kubectl get pods -n <namespace>`
- Check pod logs: `kubectl logs <pod-name> -n <namespace>`
- Stop and restart the service to reset the retry counter

### Permission denied
```bash
kubectl auth can-i get services -n <namespace>
kubectl auth can-i create pods -n <namespace>  # For proxy services
```

### Proxy pod fails to create
```bash
kubectl auth can-i create pods -n <namespace>
kubectl get pod kubefwd-proxy -n <namespace>
kubectl logs kubefwd-proxy -n <namespace> --all-containers
```

### Proxy connection works but can't reach GCP resource
- Verify the target IP: `gcloud sql instances describe …`
- Check VPC peering between GKE and GCP resource
- Check firewall rules
- Test from within the cluster:
  ```bash
  kubectl run -it --rm debug --image=alpine --restart=Never -- sh
  # then: nc -zv <target_host> <target_port>
  ```

### sql-tapd not starting
- Ensure sql-tapd is installed and in your PATH
- Check that `sql_tap_port` doesn't conflict with other services
- Use `--debug` flag to see the full sql-tapd command being run

### Can't connect sql-tap client
- Use the **ℹ sql-tap** button on the Proxy tab to get the exact command and gRPC port
- Check debug logs: `tail -f /tmp/kubefwd-debug.log`
- Ensure no other process is using the gRPC port

## Development

### Project Structure

```
kubefwd/
├── main.go                 # Entry point — CLI flags, HTTP server, signal handling
├── web_server.go           # WebApp state, HTTP handlers, SSE broadcaster
├── web/
│   └── index.html          # Embedded web UI (inline CSS + JS)
├── config.go               # Config struct, validation, YAML parse/load
├── config_store.go         # ConfigStore: YAML file + SQLite (normalized schema)
├── config_test.go          # Tests for config parsing / validation
├── explorer.go             # K8s service & GCP resource discovery (kubectl/gcloud)
├── portforward.go          # kubectl port-forward process management
├── proxypod.go             # Proxy pod lifecycle and ProxyForward
├── sqltap.go               # sql-tapd process management
├── port_utils.go           # lsof-based port inspection and kill
├── terminal_launcher.go    # Launch sql-tap TUI in a new terminal tab
├── config.example.yaml     # Annotated config template
├── port_utils_test.go      # Tests for port utilities
└── go.mod                  # Go dependencies
```

This project uses:
- [yaml.v3](https://gopkg.in/yaml.v3) — YAML parsing and file export
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — pure-Go SQLite driver (optional; only when using `--db`)
- Go standard library for the web server (`net/http`, `embed`)

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
- 🔍 Port status checker to identify and kill processes using configured ports
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
   go build -o kubefwd
   ```
4. (Optional) Move to your PATH:
   ```bash
   sudo mv kubefwd /usr/local/bin/
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
  - **sql_tap_port** (optional): Port for sql-tap proxy (enables SQL traffic monitoring)
  - **sql_tap_driver** (optional): Database driver for sql-tap (`postgres` or `mysql`)
  - **sql_tap_grpc_port** (optional): gRPC port for sql-tap TUI client (default: auto-assigned starting at 9091)
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
  - **sql_tap_port** (optional): Port for sql-tap proxy (enables SQL traffic monitoring)
  - **sql_tap_driver** (optional): Database driver for sql-tap (`postgres` or `mysql`)
  - **sql_tap_grpc_port** (optional): gRPC port for sql-tap TUI client (default: auto-assigned starting at 9091)

## Usage

Run the tool with the default config file (`~/.kubefwd.yaml`):
```bash
./kubefwd
```

Or specify a custom config file:
```bash
./kubefwd --config /path/to/config.yaml
```

Enable debug mode to see kubectl commands:
```bash
./kubefwd --debug
```

Auto-start default services on launch:
```bash
./kubefwd --default
```

Run in background mode (headless, no TUI):
```bash
./kubefwd --default --background &
```

**Command-line flags:**
- `--config <path>`: Path to configuration file (default: `~/.kubefwd.yaml`)
- `--debug`: Enable debug output showing kubectl commands
- `--default`: Auto-start services marked with `selected_by_default: true`
- `--default-proxy`: Auto-start proxy services marked with `selected_by_default: true`
- `--background`: Run in background mode without TUI (requires `--default` or `--default-proxy`)
- `--help`: Show help message

**First-time setup:**
```bash
# Copy the example config to your home directory
cp config.example.yaml ~/.kubefwd.yaml

# Edit it with your cluster details
nano ~/.kubefwd.yaml
```

**Quick Start with Defaults:**

The `--default` and `--default-proxy` flags work in both TUI and background modes:

```bash
# Launch TUI with default services auto-started
./kubefwd --default

# Launch TUI with default proxy services auto-started
./kubefwd --default-proxy

# Launch TUI with both types auto-started
./kubefwd --default --default-proxy
```

This is useful when you want to quickly start your common services without manually pressing `d` in the TUI.

### Controls

**Navigation:**
- `↑`/`↓` or `k`/`j`: Navigate through services

**Service Control:**
- `Enter` or `s`: Start/stop the selected service
- `d`: Start all services marked with `selected_by_default: true`
- `a`: Start all services
- `x`: Stop all services
- `o`: Toggle display of context/namespace override details
- `p`: Apply a preset (if presets are configured)
- `c`: Change cluster context (if alternative contexts are configured)
- `r`: Manage proxy services (if proxy services are configured)
- `l`: Open port status checker to view and manage port usage
- `g`: Open config management screen
- `q`: Stop all services and quit

**Status Indicators:**
- `●` (green): Port forward is active and running
- `◐` (orange): Port forward is initializing/starting
- `○` (gray): Port forward is stopped
- `↻ X/Y` (orange): Port forward failed and is retrying (X = current attempt, Y = max attempts)
- `↻ X` (orange): Port forward failed and is retrying infinitely (X = current attempt)
- `✗` (red): Port forward encountered an error (max retries exceeded or retries disabled)

**Display Information:**
- ⚡ Direct services (kubectl port-forward to Kubernetes services)
- ☁️ Proxy services (proxy pod connections to GCP resources)
- Services marked as default show a `★` (star) indicator
- Services with context/namespace overrides show a `⚙` (gear) icon
- Press `o` to toggle detailed override information `[ctx: name]` and/or `[ns: name]`
- Error messages are displayed below failed services with full kubectl command for debugging
- Proxy pod status shows creation state and number of active connections
- UI automatically adapts to your terminal width for optimal display

### Background Mode

Background mode allows you to run kubefwd as a headless service without the TUI, perfect for automated environments or running services persistently.

**Requirements:**
- The `--background` flag requires at least one of `--default` or `--default-proxy`
- Services must be marked with `selected_by_default: true` in the config

**Starting in Background:**
```bash
# Start default services in background (use & to free up terminal)
./kubefwd --default --background &

# Start default proxy services in background
./kubefwd --default-proxy --background &

# Start both in background
./kubefwd --default --default-proxy --background &

# Or use nohup to keep it running after logout
nohup ./kubefwd --default --background > /dev/null 2>&1 &
```

**How it Works:**
1. The tool validates flags and starts selected services
2. Waits for all services to reach running or error state (60 second timeout)
3. Creates a PID file at `~/.kubefwd.pid`
4. Prints status messages to stderr
5. Runs indefinitely until stopped

**Note:** Use `&` at the end of the command to run it as a background process and free up your terminal:
```bash
./kubefwd --default --background &
```
The process will run in the background and you can continue using your terminal.

**Stopping Background Services:**
```bash
# Using the PID file (recommended)
kill $(cat ~/.kubefwd.pid)

# Or manually find and kill the process
ps aux | grep kubefwd
kill <PID>
```

**Checking Status:**
```bash
# Check if the process is running
ps -p $(cat ~/.kubefwd.pid)

# View the PID
cat ~/.kubefwd.pid
```

**Important Notes:**
- The tool prevents multiple instances by checking for existing PID files
- Services are gracefully stopped on SIGTERM or SIGINT
- Proxy pods are automatically cleaned up on exit
- If some services fail to start, the tool still runs (warnings printed to stderr)
- PID file is automatically removed on clean shutdown

### Port Status Checker

The port status checker allows you to view all configured ports and identify which processes are using them. This is especially useful for debugging port conflicts or cleaning up stale processes.

**Accessing the Port Checker:**
- Press `l` from the main management view

**Features:**
- View all ports defined in your configuration (both Direct and Proxy services)
- See the current status of each port:
  - `✓ FREE` (gray): Port is not in use
  - `● KUBEFWD` (green): Port is in use by a kubefwd-managed process
  - `⚠ EXTERNAL` (yellow): Port is in use by an external process (not managed by kubefwd)
- Display PID and process information for ports in use
- Kill processes using the configured ports with confirmation

**Port Checker Controls:**
- `↑`/`↓` or `k`/`j`: Navigate through ports
- `K` (capital K): Kill the process using the selected port
- `r`: Manually refresh port status
- `Esc` or `q`: Return to main management view

**Kill Process Flow:**
1. Navigate to a port that's in use
2. Press `K` (capital K) to initiate kill
3. A confirmation dialog will appear showing:
   - The PID to be killed
   - The service name
   - Whether it's a kubefwd or external process
4. Press `y` to confirm or `n`/`Esc` to cancel
5. After killing, the port list automatically refreshes

**Status Detection:**
- The checker uses `lsof` to identify processes using each configured port
- It cross-references PIDs with kubefwd's internal tracking to distinguish between:
  - Kubefwd-managed kubectl port-forward processes
  - External processes (e.g., other applications using the same port)
- Port status auto-refreshes every 2 seconds

**Use Cases:**
- Identify port conflicts before starting services
- Clean up stale port-forward processes that didn't shut down cleanly
- Kill external processes occupying ports you need for kubefwd
- Verify which ports are actually in use vs. configured

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

### Config Management

The config management screen allows you to view and manage your configuration file without leaving the application.

**Accessing Config Management:**
- Press `g` in the management view to open the config management screen

**Features:**
- **View Config Path**: See the full path to your current configuration file
- **Edit Config**: Press `e` to open the config file in your preferred editor
  - Uses the `$EDITOR` environment variable if set
  - Falls back to `nano` or `vi` if `$EDITOR` is not set
  - The application suspends while you edit and resumes when you close the editor
- **Reload Config**: Press `r` to reload the configuration from the file
  - The tool will preserve running port forwards for services that still exist in the new config
  - Services that were running and still exist will be automatically restarted
  - Services that no longer exist will be stopped
  - New services from the config will be available but not automatically started

**Typical Workflow:**
1. Press `g` to open config management
2. Press `e` to edit your config file
3. Make your changes and save
4. Close the editor (the app will resume)
5. Press `r` to reload the configuration
6. Press `Esc` or `q` to return to the management view

This feature is especially useful when you need to quickly add new services, change ports, or update service configurations without restarting the entire application.

## Proxy Pod for GCP Resources

The proxy pod feature allows you to connect to GCP resources (CloudSQL, MemoryStore, etc.) that don't have direct Kubernetes services but are accessible from within the cluster.

## SQL Traffic Monitoring with sql-tap

kubefwd supports real-time SQL traffic monitoring through [sql-tap](https://github.com/mickamy/sql-tap), a transparent database proxy that captures and displays SQL queries as they flow between your application and database.

### What is sql-tap?

sql-tap is a tool that sits between your application and database, capturing every SQL query in real-time without modifying your application code. It works by:

1. **sql-tapd**: A proxy daemon that forwards traffic while logging queries
2. **sql-tap**: A TUI client that displays captured queries with syntax highlighting

This is incredibly useful for:
- Debugging database queries in development
- Understanding what queries your application makes
- Performance analysis and optimization
- Learning how ORMs translate to SQL

### Installation

Install sql-tap before using this feature:

**Homebrew (macOS/Linux):**
```bash
brew install mickamy/tap/sql-tap
```

**Go Install:**
```bash
go install github.com/mickamy/sql-tap/cmd/sql-tapd@latest
go install github.com/mickamy/sql-tap/cmd/sql-tap@latest
```

**Docker:**
```bash
docker pull ghcr.io/mickamy/sql-tap:latest
```

### Configuration

Add sql-tap fields to any service (direct or proxy) that connects to a database:

```yaml
services:
  - name: Postgres Database
    service_name: postgres
    remote_port: 5432
    local_port: 5432
    selected_by_default: false
    # sql-tap configuration
    sql_tap_port: 5433                                                # Port where sql-tapd listens
    sql_tap_driver: postgres                                          # Driver: postgres or mysql

proxy_services:
  - name: CloudSQL Production
    target_host: 10.1.2.3
    target_port: 5432
    local_port: 5432
    selected_by_default: false
    # sql-tap configuration for proxy services
    sql_tap_port: 5433
    sql_tap_driver: postgres
```

**Configuration fields:**
- `sql_tap_port`: The port where sql-tapd listens (your application connects here)
- `sql_tap_driver`: Database driver type (`postgres` or `mysql`)
- `sql_tap_grpc_port` (optional): gRPC port for TUI client (default: auto-assigned starting at 9091)

**Important notes:**
- `sql_tap_port` must be different from `local_port`
- Both `sql_tap_port` and `sql_tap_driver` are required when sql-tap is enabled
- DATABASE_URL is automatically composed as `postgresql://127.0.0.1:<local_port>` (or `mysql://...` for MySQL)
- `sql_tap_grpc_port` is optional and will be auto-assigned if not specified
- Multiple services automatically get incremented gRPC ports (9091, 9092, 9093...)

### How It Works

When sql-tap is enabled, kubefwd automatically manages the complete setup:

```
Application                                    Port Forward              Database
     |                                               |                       |
     | Connect to localhost:5433                     |                       |
     v                                               v                       |
sql-tapd (proxy)  ------>  localhost:5432  ------>  kubectl  ------------>  DB
(sql_tap_port)            (local_port)            port-forward         (remote DB)
```

**Flow:**
1. kubefwd starts the normal port-forward (`localhost:5432` → database)
2. kubefwd automatically starts `sql-tapd` (`localhost:5433` → `localhost:5432`)
3. Your application connects to `localhost:5433` (the sql-tap port)
4. sql-tapd forwards traffic to `localhost:5432` while logging queries
5. Run `sql-tap` in another terminal to view queries in real-time

### Usage Workflow

**Step 1: Start the service with sql-tap**

```bash
./kubefwd
# Navigate to your database service and press 's' to start it
# kubefwd will automatically start both the port-forward and sql-tapd
```

The TUI will show sql-tap status:
```
▶ ★ Postgres Database     ● :5432 → postgres:5432 [SQL-TAP ●:5433]
```

**Step 2: Update your application**

Configure your application to connect to the sql-tap port:

```bash
# Before (direct connection)
DATABASE_URL="postgres://user:pass@localhost:5432/mydb"

# After (via sql-tap)
DATABASE_URL="postgres://user:pass@localhost:5433/mydb"
```

**Step 3: View queries in real-time**

In a separate terminal, run the sql-tap TUI:

```bash
# Connect to the first service (default gRPC port 9091)
sql-tap localhost:9091

# If you have multiple services with sql-tap enabled:
# Second service uses port 9092, third uses 9093, etc.
sql-tap localhost:9092
```

To find which gRPC port a service is using:
- Check the kubefwd UI (shows gRPC port in sql-tap status)
- Look at debug logs: `tail -f /tmp/kubefwd-debug.log`
- Custom ports are shown in your config file

This opens an interactive interface showing:
- All SQL queries in real-time
- Query execution time
- Transaction boundaries
- Ability to run EXPLAIN on any query

**Tip:** With multiple databases, open multiple terminals running `sql-tap` on different gRPC ports to monitor them simultaneously.

### Status Indicators

In the kubefwd UI, sql-tap shows status alongside the service:

- `[SQL-TAP ●:5433 gRPC:9091]` - sql-tap running successfully (shows both proxy port and gRPC port)
- `[SQL-TAP ◐:5433 gRPC:9091]` - sql-tap starting
- `[SQL-TAP ✗:5433 gRPC:9091]` - sql-tap encountered an error

For proxy services in the right pane:
- `[ST ●:5433/9091]` - Compact format showing sql-tap proxy port and gRPC port

### Troubleshooting

**sql-tapd not starting:**
- Ensure sql-tapd is installed and in your PATH
- Check that `sql_tap_port` doesn't conflict with other services
- Verify the `sql_tap_database_url` format is correct for your driver
- Use `--debug` flag to see the full sql-tapd command

**Can't connect to sql-tap port:**
- Ensure the port-forward is running first (sql-tap depends on it)
- Check that your application is connecting to the correct port
- Verify no firewall is blocking the sql-tap port

**Queries not appearing in sql-tap:**
- Ensure your application is connecting to `sql_tap_port`, not `local_port`
- Check the sql-tap TUI is connected to the correct gRPC port
- Verify sql-tapd is running (check status in kubefwd UI)
- Try connecting to the correct gRPC port: `sql-tap localhost:9091` (or 9092, 9093, etc.)

**Can't connect sql-tap TUI client:**
- Check which gRPC port the service is using (shown in kubefwd UI)
- Verify the port in debug logs: `tail -f /tmp/kubefwd-debug.log`
- For the first service, it's usually 9091, second is 9092, and so on
- Custom gRPC ports are specified in `sql_tap_grpc_port` config field
- Ensure no other process is using the gRPC port

**Authentication errors:**
- The `sql_tap_database_url` must match your actual database credentials
- Ensure the connection string format is correct for your driver

### Examples

**PostgreSQL direct service:**
```yaml
services:
  - name: Dev Database
    service_name: postgres-service
    remote_port: 5432
    local_port: 5432
    sql_tap_port: 5433
    sql_tap_driver: postgres
```

**MySQL CloudSQL via proxy:**
```yaml
proxy_services:
  - name: Production MySQL
    target_host: 10.1.2.3
    target_port: 3306
    local_port: 3306
    sql_tap_port: 3307
    sql_tap_driver: mysql
```

**Multiple databases with different sql-tap ports:**
```yaml
services:
  - name: Users DB
    service_name: users-postgres
    remote_port: 5432
    local_port: 5432
    sql_tap_port: 5433  # First database
    sql_tap_driver: postgres

  - name: Orders DB
    service_name: orders-postgres
    remote_port: 5432
    local_port: 5532
    sql_tap_port: 5533  # Second database (different ports)
    sql_tap_driver: postgres
```

### Lifecycle Management

kubefwd automatically handles the complete lifecycle:

- **Starting**: Port-forward starts first, then sql-tapd after a brief delay
- **Stopping**: sql-tapd stops first, then the port-forward
- **Errors**: If sql-tapd fails, the port-forward is also stopped
- **Retries**: When auto-retry is enabled, both processes restart together
- **Background mode**: Works seamlessly with `--background` flag

No manual intervention needed - just configure and use!

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

### Split-View Interface

The main management view displays proxy services in a separate pane with a responsive layout:

```
┌────────────────────────────────────────────────┬─────────────────────────┐
│ kubefwd                                         │ Proxy Services          │
│                                                 │                         │
│ Cluster: Production                             │ Pod: ● Ready (2)        │
│ Namespace: default                              │                         │
│                                                 │ [✓] CloudSQL Staging ●  │
│ ▶ ★ API Server           ● :8080 → api:8080    │ [✓] Redis Store ●       │
│   ★ Database             ○ :5432 → db:5432     │ [ ] CloudSQL Prod       │
│   ⚙ Redis Cache          ○ :6379 → redis:6379  │ [ ] MySQL Prod          │
│                                                 │                         │
│ ↑↓/jk:nav • s:toggle • d:def • a:all • x:stop  │ Press 'r' to manage     │
│ p:presets • c:context • r:proxy • q:quit       │                         │
└────────────────────────────────────────────────┴─────────────────────────┘
```

**Left Pane**: Interactive direct Kubernetes service management (70% width)
**Right Pane**: Read-only proxy service status overview (30% width)
**Responsive**: Layout automatically adjusts to your terminal width

### Using Proxy Services

**Step 1: Press `r` to open proxy selection modal**

A centered modal dialog appears over the main view:

```
╭─────────────────────────────────────────────────────────╮
│ Select Proxy Services • ● Ready (2)                     │
│                                                          │
│   [ ]★ CloudSQL Production     :5432 → 10.1.2.3:5432    │
│   [✓]  CloudSQL Staging        :5433 → 10.1.2.4:5432    │
│ ▶ [✓]★ Redis MemoryStore       :6380 → 10.1.3.5:6379    │
│   [ ]  MySQL Production        :3306 → 10.1.4.6:3306    │
│                                                          │
│ space:toggle • enter:apply • esc:cancel                 │
╰─────────────────────────────────────────────────────────╯
```

**Default Selection Behavior:**
- **First time**: Services with `selected_by_default: true` are pre-checked (marked with ★)
- **Editing**: Currently active services are pre-checked
- Services marked as default always show a ★ indicator for reference

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
- `○ Not Created`: Pod hasn't been created yet
- `◐ Creating`: Pod is being created and starting
- `● Ready`: Pod is running and ready for connections
- `✗ Error`: Pod creation or readiness check failed
- Active connection count shows how many proxy services are currently using the pod
- Individual proxy service status shown with `●` (running) or `✗` (error) next to the service name

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
- **UI indication**: Services in retry mode show `↻ X/Y` or `↻ X` status with retry attempt count
- **Error messages**: After max retries exceeded, full error details are displayed including the number of retry attempts

## User Interface

### Responsive Layout

The UI automatically adapts to your terminal size:
- **Dynamic Width**: The interface expands to use your full terminal width for better readability
- **Split View**: When proxy services are configured, the view splits 70/30 between direct services and proxy status
- **Equal Heights**: The divider between panes extends the full height of the terminal
- **Minimum Widths**: The layout maintains minimum widths to ensure usability on smaller terminals

### Compact Status Display

Services use color-coded symbols instead of verbose text:
- **●** (green) = Running
- **◐** (orange) = Starting
- **○** (gray) = Stopped
- **↻ X** (orange) = Retrying (shows attempt count)
- **✗** (red) = Error

This compact display allows more services to fit on screen without line wrapping.

### Context and Namespace Overrides

Services with per-service context or namespace overrides show a **⚙** (gear) icon:
- By default, override details are hidden to save space
- Press **`o`** to toggle the display of full override information
- When shown, overrides appear as `[ctx: name]` or `[ns: name]` tags next to the service

### Modal Dialogs

Some screens (like proxy service selection) appear as centered modal overlays:
- Modal appears on top of the main view
- Main view remains visible in the background
- Easy to see your current state while making selections
- Press `esc` or `q` to close without changes

## Tips

1. **Find your cluster context**: Run `kubectl config get-contexts` to list available contexts
2. **Check service names**: Run `kubectl get services -n <namespace>` to see available services
3. **Avoid port conflicts**: Make sure the local ports you specify aren't already in use
4. **Test connectivity**: After starting a port forward, test it with `curl localhost:<port>` or your preferred tool
5. **Unreliable connections**: For flaky networks or frequently restarting pods, keep the default infinite retries enabled
6. **Development environment**: Consider setting `max_retries: 3` for development services that may not always be available
7. **GCP Resources**: Use proxy services for CloudSQL, MemoryStore, and other GCP resources with private IPs
8. **Unified Management**: Both direct services and proxy services work identically in the UI - start/stop them the same way
9. **Override Visibility**: Use `o` to toggle override details - keep them hidden for a cleaner view, show them when you need context
10. **Wide Terminals**: The UI takes advantage of wide terminals - expand your window for the best experience
11. **Quick Start**: Use `--default` and `--default-proxy` flags to auto-start common services on launch (works in both TUI and background modes)
12. **Background Mode**: Run as a daemon with `--background &` for CI/CD, Docker containers, or long-running development environments
13. **Multiple Environments**: Use different config files (`--config`) with background mode to run separate instances for different environments
14. **Shell Background**: Remember to use `&` at the end of background mode commands to free up your terminal

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

### Background mode issues

**Error: "--background requires at least one of --default or --default-proxy"**
- You must specify `--default` or `--default-proxy` (or both) when using `--background`
- This ensures services are actually started when running headless

**Error: "kubefwd is already running"**
- Another instance is already running in background mode
- Stop it first: `kill $(cat ~/.kubefwd.pid)`
- If the PID file is stale, remove it: `rm ~/.kubefwd.pid`

**Services not starting in background mode**
- Check stderr output for error messages
- Ensure services are marked with `selected_by_default: true` in your config
- Verify kubectl context and namespace are correct
- Run with `--debug` flag for detailed output: `./kubefwd --default --background --debug 2>&1 | tee kubefwd.log &`
- If running without `&`, the terminal will be blocked (this is expected - use `&` to free it up)

**Timeout waiting for services to start**
- Some services may be slow to initialize (60 second timeout)
- Check if pods exist: `kubectl get pods -n <namespace>`
- Check for port conflicts: `lsof -i :<port>`
- Verify service configuration is correct

**Can't stop background process**
- If PID file is missing: `ps aux | grep kubefwd | grep -v grep` then `kill <PID>`
- Force stop if needed: `kill -9 <PID>`
- Clean up manually: `rm ~/.kubefwd.pid`

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

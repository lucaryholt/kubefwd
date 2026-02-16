package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// SqlTapManager manages a single sql-tapd process
type SqlTapManager struct {
	enabled      bool
	driver       string
	listenPort   int
	upstreamPort int
	grpcPort     int // gRPC port for TUI client connection
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	status       PortForwardStatus
	errorMessage string
	mu           sync.Mutex
}

// NewSqlTapManager creates a new sql-tap manager instance
func NewSqlTapManager(enabled bool, driver string, listenPort, upstreamPort, grpcPort int) *SqlTapManager {
	return &SqlTapManager{
		enabled:      enabled,
		driver:       driver,
		listenPort:   listenPort,
		upstreamPort: upstreamPort,
		grpcPort:     grpcPort,
		status:       StatusStopped,
	}
}

// composeDatabaseURL creates the DATABASE_URL from driver and upstream port
func (stm *SqlTapManager) composeDatabaseURL() string {
	// Map driver names to protocol names
	protocol := stm.driver
	if stm.driver == "postgres" {
		protocol = "postgresql"
	}
	return fmt.Sprintf("%s://127.0.0.1:%d", protocol, stm.upstreamPort)
}

// Start initiates the sql-tapd process
func (stm *SqlTapManager) Start() error {
	stm.mu.Lock()
	defer stm.mu.Unlock()

	if !stm.enabled {
		return nil // Not enabled, nothing to do
	}

	if stm.status == StatusRunning || stm.status == StatusStarting {
		return fmt.Errorf("sql-tapd already running")
	}

	stm.status = StatusStarting
	stm.errorMessage = ""

	// Create context for the command
	ctx, cancel := context.WithCancel(context.Background())
	stm.cancel = cancel

	// Build sql-tapd command
	// Example: DATABASE_URL="postgresql://127.0.0.1:5432" sql-tapd --driver=postgres --listen=:5433 --upstream=localhost:5432 --grpc=:9091
	listenAddr := fmt.Sprintf(":%d", stm.listenPort)
	upstreamAddr := fmt.Sprintf("localhost:%d", stm.upstreamPort)
	grpcAddr := fmt.Sprintf(":%d", stm.grpcPort)
	databaseUrl := stm.composeDatabaseURL()

	args := []string{
		fmt.Sprintf("--driver=%s", stm.driver),
		fmt.Sprintf("--listen=%s", listenAddr),
		fmt.Sprintf("--upstream=%s", upstreamAddr),
		fmt.Sprintf("--grpc=%s", grpcAddr),
	}

	debugLog("Starting sql-tapd: DATABASE_URL=%s sql-tapd %s", databaseUrl, strings.Join(args, " "))

	stm.cmd = exec.CommandContext(ctx, "sql-tapd", args...)
	stm.cmd.Env = append(os.Environ(), fmt.Sprintf("DATABASE_URL=%s", databaseUrl))

	// Capture stderr for error messages
	var stderr strings.Builder
	stm.cmd.Stderr = &stderr

	// Start the command
	if err := stm.cmd.Start(); err != nil {
		stm.status = StatusError
		stm.errorMessage = fmt.Sprintf("Failed to start sql-tapd: %v", err)
		if stderr.Len() > 0 {
			stm.errorMessage += fmt.Sprintf(" | stderr: %s", stderr.String())
		}
		cancel()
		return fmt.Errorf("%s", stm.errorMessage)
	}

	// Monitor the process in a goroutine
	go stm.monitor(&stderr)

	// Wait a moment to ensure it starts successfully
	time.Sleep(500 * time.Millisecond)

	// Check if it's still running
	if stm.cmd.ProcessState != nil && stm.cmd.ProcessState.Exited() {
		stm.status = StatusError
		stm.errorMessage = "sql-tapd exited immediately"
		if stderr.Len() > 0 {
			stm.errorMessage += fmt.Sprintf(" | stderr: %s", stderr.String())
		}
		return fmt.Errorf("%s", stm.errorMessage)
	}

	stm.status = StatusRunning
	return nil
}

// monitor watches the sql-tapd process and updates status
func (stm *SqlTapManager) monitor(stderr *strings.Builder) {
	err := stm.cmd.Wait()

	stm.mu.Lock()
	defer stm.mu.Unlock()

	if err != nil && stm.status != StatusStopped {
		stm.status = StatusError
		stm.errorMessage = fmt.Sprintf("sql-tapd process exited: %v", err)
		if stderr.Len() > 0 {
			stm.errorMessage += fmt.Sprintf(" | stderr: %s", strings.TrimSpace(stderr.String()))
		}
	} else {
		if stm.status == StatusRunning || stm.status == StatusStarting {
			stm.status = StatusStopped
		}
	}
}

// Stop terminates the sql-tapd process
func (stm *SqlTapManager) Stop() error {
	stm.mu.Lock()
	defer stm.mu.Unlock()

	if !stm.enabled {
		return nil // Not enabled, nothing to do
	}

	if stm.status != StatusRunning && stm.status != StatusStarting {
		return nil // Already stopped
	}

	if stm.cancel != nil {
		stm.cancel()
		stm.cancel = nil
	}

	stm.status = StatusStopped
	stm.errorMessage = ""
	return nil
}

// IsRunning returns true if the sql-tapd process is currently running
func (stm *SqlTapManager) IsRunning() bool {
	stm.mu.Lock()
	defer stm.mu.Unlock()
	return stm.status == StatusRunning || stm.status == StatusStarting
}

// GetStatus returns the current status and error message
func (stm *SqlTapManager) GetStatus() (PortForwardStatus, string) {
	stm.mu.Lock()
	defer stm.mu.Unlock()
	return stm.status, stm.errorMessage
}

// IsEnabled returns whether sql-tap is enabled for this service
func (stm *SqlTapManager) IsEnabled() bool {
	stm.mu.Lock()
	defer stm.mu.Unlock()
	return stm.enabled
}

// GetListenPort returns the port sql-tapd is listening on
func (stm *SqlTapManager) GetListenPort() int {
	stm.mu.Lock()
	defer stm.mu.Unlock()
	return stm.listenPort
}

// GetGrpcPort returns the gRPC port for sql-tap client connection
func (stm *SqlTapManager) GetGrpcPort() int {
	stm.mu.Lock()
	defer stm.mu.Unlock()
	return stm.grpcPort
}

// CheckSqlTapdAvailable verifies that sql-tapd is installed and available
func CheckSqlTapdAvailable() error {
	cmd := exec.Command("sql-tapd", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sql-tapd not available: %w\nOutput: %s", err, string(output))
	}
	return nil
}

package main

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// SqlTapStatus represents the current state of a sql-tap daemon
type SqlTapStatus string

const (
	SqlTapStatusStopped  SqlTapStatus = "stopped"
	SqlTapStatusStarting SqlTapStatus = "starting"
	SqlTapStatusRunning  SqlTapStatus = "running"
	SqlTapStatusError    SqlTapStatus = "error"
)

// SqlTapManager manages the sql-tapd daemon lifecycle
type SqlTapManager struct {
	serviceName  string
	driver       string // "postgres" or "mysql"
	listenPort   int    // User-facing port (e.g., 5432)
	upstreamPort int    // Internal port to proxy pod (e.g., 5433)
	grpcPort     int    // gRPC port for TUI
	dsnEnv       string // DSN environment variable
	
	cmd            *exec.Cmd
	cancel         context.CancelFunc
	status         SqlTapStatus
	errorMessage   string
	commandString  string
	retryCount     int
	retrying       bool
	manualStop     bool
	mu             sync.Mutex
}

// NewSqlTapManager creates a new SQL-Tap manager
func NewSqlTapManager(serviceName, driver string, listenPort, upstreamPort, grpcPort int, dsnEnv string) *SqlTapManager {
	return &SqlTapManager{
		serviceName:  serviceName,
		driver:       driver,
		listenPort:   listenPort,
		upstreamPort: upstreamPort,
		grpcPort:     grpcPort,
		dsnEnv:       dsnEnv,
		status:       SqlTapStatusStopped,
	}
}

// Start launches the sql-tapd daemon
func (stm *SqlTapManager) Start() error {
	stm.mu.Lock()
	defer stm.mu.Unlock()

	if stm.status == SqlTapStatusRunning || stm.status == SqlTapStatusStarting {
		return fmt.Errorf("sql-tap daemon already running")
	}

	stm.status = SqlTapStatusStarting
	stm.errorMessage = ""
	stm.manualStop = false
	stm.retrying = false

	// Check if sql-tapd is available
	if _, err := exec.LookPath("sql-tapd"); err != nil {
		stm.status = SqlTapStatusError
		stm.errorMessage = "sql-tapd not found (install: brew install --cask mickamy/tap/sql-tap)"
		stm.manualStop = true // Don't retry if binary is missing
		return fmt.Errorf("%s", stm.errorMessage)
	}

	// Create context for the command
	ctx, cancel := context.WithCancel(context.Background())
	stm.cancel = cancel

	// Build sql-tapd command
	args := []string{
		fmt.Sprintf("--driver=%s", stm.driver),
		fmt.Sprintf("--listen=:%d", stm.listenPort),
		fmt.Sprintf("--upstream=localhost:%d", stm.upstreamPort),
		fmt.Sprintf("--grpc=:%d", stm.grpcPort),
		fmt.Sprintf("--dsn-env=%s", stm.dsnEnv),
	}

	// Store the command string for debugging
	stm.commandString = fmt.Sprintf("sql-tapd %s", strings.Join(args, " "))

	// Print the command for debugging if enabled
	debugLog("Executing sql-tapd: %s", stm.commandString)

	stm.cmd = exec.CommandContext(ctx, "sql-tapd", args...)

	// Capture stderr for error messages
	var stderr strings.Builder
	stm.cmd.Stderr = &stderr

	// Start the command
	if err := stm.cmd.Start(); err != nil {
		stm.status = SqlTapStatusError
		stm.errorMessage = fmt.Sprintf("Failed to start: %v", err)
		if stderr.Len() > 0 {
			stm.errorMessage += fmt.Sprintf(" | stderr: %s", stderr.String())
		}
		cancel()
		return err
	}

	// Monitor the process in a goroutine
	go stm.monitor(&stderr)

	stm.status = SqlTapStatusRunning
	stm.retryCount = 0 // Reset retry count on successful start
	return nil
}

// monitor watches the sql-tapd process and updates status
func (stm *SqlTapManager) monitor(stderr *strings.Builder) {
	err := stm.cmd.Wait()

	stm.mu.Lock()

	if err != nil && stm.status != SqlTapStatusStopped {
		// Auto-retry with exponential backoff (infinite retries for sql-tap)
		shouldRetry := !stm.manualStop

		if shouldRetry {
			// Calculate exponential backoff delay: min(2^retryCount seconds, 60 seconds)
			backoffSeconds := math.Min(math.Pow(2, float64(stm.retryCount)), 60)
			stm.retryCount++
			stm.retrying = true
			stm.status = SqlTapStatusError // Temporarily set to error while waiting
			stm.errorMessage = fmt.Sprintf("sql-tapd crashed, retrying in %.0fs (attempt %d)...", backoffSeconds, stm.retryCount)

			debugLog("%s: sql-tapd crashed, retrying after %.0fs (attempt %d)", stm.serviceName, backoffSeconds, stm.retryCount)

			stm.mu.Unlock()

			// Wait for backoff period
			time.Sleep(time.Duration(backoffSeconds) * time.Second)

			// Attempt to restart
			if err := stm.Start(); err != nil {
				stm.mu.Lock()
				stm.status = SqlTapStatusError
				stm.errorMessage = fmt.Sprintf("Retry failed: %v", err)
				stm.mu.Unlock()
			}
		} else {
			// Manual stop
			stm.status = SqlTapStatusError
			stm.retrying = false
			stm.errorMessage = fmt.Sprintf("Process exited: %v", err)
			if stderr.Len() > 0 {
				stm.errorMessage += fmt.Sprintf(" | stderr: %s", strings.TrimSpace(stderr.String()))
			}
			stm.errorMessage += fmt.Sprintf(" | Command: %s", stm.commandString)
			stm.mu.Unlock()
		}
	} else {
		if stm.status == SqlTapStatusRunning {
			stm.status = SqlTapStatusStopped
		}
		stm.mu.Unlock()
	}
}

// Stop terminates the sql-tapd daemon
func (stm *SqlTapManager) Stop() error {
	stm.mu.Lock()
	defer stm.mu.Unlock()

	if stm.status != SqlTapStatusRunning && stm.status != SqlTapStatusStarting {
		return nil // Already stopped
	}

	if stm.cancel != nil {
		stm.cancel()
		stm.cancel = nil
	}

	stm.status = SqlTapStatusStopped
	stm.errorMessage = ""
	stm.manualStop = true // Prevent auto-retry
	stm.retrying = false
	return nil
}

// GetStatus returns the current status and error message
func (stm *SqlTapManager) GetStatus() (SqlTapStatus, string) {
	stm.mu.Lock()
	defer stm.mu.Unlock()
	return stm.status, stm.errorMessage
}

// IsRunning returns true if sql-tapd is currently running
func (stm *SqlTapManager) IsRunning() bool {
	stm.mu.Lock()
	defer stm.mu.Unlock()
	return stm.status == SqlTapStatusRunning
}

// GetGrpcPort returns the gRPC port for TUI connection
func (stm *SqlTapManager) GetGrpcPort() int {
	return stm.grpcPort
}

// GetCommandString returns the command string for debugging
func (stm *SqlTapManager) GetCommandString() string {
	stm.mu.Lock()
	defer stm.mu.Unlock()
	return stm.commandString
}

// CheckSqlTapdAvailable checks if sql-tapd binary is available in PATH
func CheckSqlTapdAvailable() error {
	if _, err := exec.LookPath("sql-tapd"); err != nil {
		return fmt.Errorf("sql-tapd not found in PATH (install: brew install --cask mickamy/tap/sql-tap)")
	}
	return nil
}

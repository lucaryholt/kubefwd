package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// PortForwardStatus represents the current state of a port forward
type PortForwardStatus string

const (
	StatusStopped  PortForwardStatus = "stopped"
	StatusStarting PortForwardStatus = "starting"
	StatusRunning  PortForwardStatus = "running"
	StatusError    PortForwardStatus = "error"
)

// PortForward manages a single kubectl port-forward process
type PortForward struct {
	Service       Service
	Status        PortForwardStatus
	ErrorMessage  string
	CommandString string
	cmd           *exec.Cmd
	cancel        context.CancelFunc
	mu            sync.Mutex
	context       string
	namespace     string
	retryCount    int  // Current retry attempt number
	maxRetries    int  // Maximum retry attempts (-1 for infinite, 0 to disable)
	manualStop    bool // Flag to prevent retries when user stops manually
	retrying      bool // Indicates if currently in retry mode
	sqlTapManager *SqlTapManager // Manages sql-tapd process if enabled
}

// NewPortForward creates a new PortForward instance
func NewPortForward(service Service, globalContext, globalNamespace string, globalMaxRetries int) *PortForward {
	// Use service-specific context/namespace or fall back to global
	context := service.GetContext(globalContext)
	namespace := service.GetNamespace(globalNamespace)
	maxRetries := service.GetMaxRetries(globalMaxRetries)
	
	// Initialize sql-tap manager if configured
	var sqlTapManager *SqlTapManager
	if service.SqlTapPort != nil {
		grpcPort := 9091 // Default
		if service.SqlTapGrpcPort != nil {
			grpcPort = *service.SqlTapGrpcPort
		}
		sqlTapManager = NewSqlTapManager(
			true,
			service.SqlTapDriver,
			service.SqlTapDatabaseUrl,
			*service.SqlTapPort,
			service.LocalPort,
			grpcPort,
		)
	} else {
		sqlTapManager = NewSqlTapManager(false, "", "", 0, 0, 0)
	}
	
	return &PortForward{
		Service:       service,
		Status:        StatusStopped,
		context:       context,
		namespace:     namespace,
		maxRetries:    maxRetries,
		retryCount:    0,
		manualStop:    false,
		retrying:      false,
		sqlTapManager: sqlTapManager,
	}
}

// Start initiates the kubectl port-forward process
func (pf *PortForward) Start() error {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	if pf.Status == StatusRunning || pf.Status == StatusStarting {
		return fmt.Errorf("port forward already running")
	}

	pf.Status = StatusStarting
	pf.ErrorMessage = ""
	pf.manualStop = false
	pf.retrying = false

	// Create context for the command
	ctx, cancel := context.WithCancel(context.Background())
	pf.cancel = cancel

	// Build kubectl command
	portSpec := fmt.Sprintf("%d:%d", pf.Service.LocalPort, pf.Service.RemotePort)
	serviceSpec := fmt.Sprintf("service/%s", pf.Service.ServiceName)

	args := []string{
		"--context=" + pf.context,
		"-n", pf.namespace,
		"port-forward",
		serviceSpec,
		portSpec,
	}

	// Store the command string for debugging
	pf.CommandString = fmt.Sprintf("kubectl %s", strings.Join(args, " "))
	
	// Print the command for debugging if enabled
	if debugMode {
		fmt.Fprintf(os.Stderr, "[DEBUG] Executing: %s\n", pf.CommandString)
	}

	pf.cmd = exec.CommandContext(ctx, "kubectl", args...)
	
	// Capture stderr for error messages
	var stderr strings.Builder
	pf.cmd.Stderr = &stderr

	// Start the command
	if err := pf.cmd.Start(); err != nil {
		pf.Status = StatusError
		pf.ErrorMessage = fmt.Sprintf("Failed to start: %v", err)
		if stderr.Len() > 0 {
			pf.ErrorMessage += fmt.Sprintf(" | stderr: %s", stderr.String())
		}
		cancel()
		return err
	}

	// Monitor the process in a goroutine
	go pf.monitor(&stderr)

	pf.Status = StatusRunning
	pf.retryCount = 0 // Reset retry count on successful start
	
	// Start sql-tap if enabled
	if pf.sqlTapManager.IsEnabled() {
		// Unlock before starting sql-tap to avoid deadlock
		pf.mu.Unlock()
		
		// Wait briefly for port-forward to be ready
		time.Sleep(2 * time.Second)
		
		// Start sql-tapd
		if err := pf.sqlTapManager.Start(); err != nil {
			// If sql-tap fails, stop the port-forward
			pf.mu.Lock()
			debugLog("Failed to start sql-tapd for %s: %v", pf.Service.Name, err)
			pf.Status = StatusError
			pf.ErrorMessage = fmt.Sprintf("sql-tap failed: %v", err)
			if pf.cancel != nil {
				pf.cancel()
			}
			return err
		}
		
		// Re-lock for the deferred unlock
		pf.mu.Lock()
	}
	
	return nil
}

// monitor watches the port-forward process and updates status
func (pf *PortForward) monitor(stderr *strings.Builder) {
	err := pf.cmd.Wait()

	pf.mu.Lock()
	
	if err != nil && pf.Status != StatusStopped {
		// Check if we should retry
		shouldRetry := !pf.manualStop && (pf.maxRetries == -1 || pf.retryCount < pf.maxRetries)
		
		if shouldRetry {
			// Calculate exponential backoff delay: min(2^retryCount seconds, 60 seconds)
			backoffSeconds := math.Min(math.Pow(2, float64(pf.retryCount)), 60)
			pf.retryCount++
			pf.retrying = true
			pf.Status = StatusError // Temporarily set to error while waiting
			pf.ErrorMessage = fmt.Sprintf("Connection lost, retrying in %.0fs (attempt %d", backoffSeconds, pf.retryCount)
			if pf.maxRetries == -1 {
				pf.ErrorMessage += ")..."
			} else {
				pf.ErrorMessage += fmt.Sprintf("/%d)...", pf.maxRetries)
			}
			
			if debugMode {
				fmt.Fprintf(os.Stderr, "[DEBUG] %s: Retrying after %.0fs (attempt %d)\n", 
					pf.Service.Name, backoffSeconds, pf.retryCount)
			}
			
			pf.mu.Unlock()
			
			// Wait for backoff period
			time.Sleep(time.Duration(backoffSeconds) * time.Second)
			
			// Attempt to restart
			if err := pf.Start(); err != nil {
				pf.mu.Lock()
				pf.Status = StatusError
				pf.ErrorMessage = fmt.Sprintf("Retry failed: %v", err)
				pf.mu.Unlock()
			}
		} else {
			// Max retries exceeded or manual stop
			pf.Status = StatusError
			pf.retrying = false
			pf.ErrorMessage = fmt.Sprintf("Process exited: %v", err)
			if stderr.Len() > 0 {
				pf.ErrorMessage += fmt.Sprintf(" | stderr: %s", strings.TrimSpace(stderr.String()))
			}
			if pf.retryCount > 0 {
				pf.ErrorMessage += fmt.Sprintf(" | Failed after %d retries", pf.retryCount)
			}
			pf.ErrorMessage += fmt.Sprintf(" | Command: %s", pf.CommandString)
			pf.mu.Unlock()
		}
	} else {
		if pf.Status == StatusRunning {
			pf.Status = StatusStopped
		}
		pf.mu.Unlock()
	}
}

// Stop terminates the kubectl port-forward process
func (pf *PortForward) Stop() error {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	if pf.Status != StatusRunning && pf.Status != StatusStarting {
		return nil // Already stopped
	}

	// Stop sql-tap first if enabled
	if pf.sqlTapManager.IsEnabled() {
		pf.mu.Unlock()
		pf.sqlTapManager.Stop()
		pf.mu.Lock()
	}

	if pf.cancel != nil {
		pf.cancel()
		pf.cancel = nil
	}

	pf.Status = StatusStopped
	pf.ErrorMessage = ""
	pf.manualStop = true  // Prevent auto-retry
	pf.retrying = false
	return nil
}

// IsRunning returns true if the port forward is currently running
func (pf *PortForward) IsRunning() bool {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	return pf.Status == StatusRunning || pf.Status == StatusStarting
}

// GetStatus returns the current status and error message
func (pf *PortForward) GetStatus() (PortForwardStatus, string) {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	return pf.Status, pf.ErrorMessage
}

// GetRetryInfo returns retry information for UI display
func (pf *PortForward) GetRetryInfo() (retrying bool, attempt int, max int) {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	return pf.retrying, pf.retryCount, pf.maxRetries
}

// GetSqlTapManager returns the sql-tap manager for this port forward
func (pf *PortForward) GetSqlTapManager() *SqlTapManager {
	return pf.sqlTapManager
}

// CheckKubectlAvailable verifies that kubectl is installed and available
func CheckKubectlAvailable() error {
	cmd := exec.Command("kubectl", "version", "--client")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl not available: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// ValidateContext checks if the specified context exists
func ValidateContext(context string) error {
	cmd := exec.Command("kubectl", "config", "get-contexts", context, "--no-headers")
	output, err := cmd.CombinedOutput()
	if err != nil || len(strings.TrimSpace(string(output))) == 0 {
		return fmt.Errorf("context '%s' not found", context)
	}
	return nil
}


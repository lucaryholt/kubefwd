package main

import (
	"context"
	"fmt"
	"math"
	"net"
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

// PortConflictInfo contains information about a port conflict
type PortConflictInfo struct {
	HasConflict    bool
	IsKubectl      bool
	ProcessPID     int
	ProcessCommand string
}

// PortForward manages a single kubectl port-forward process
type PortForward struct {
	Service       Service
	Status        PortForwardStatus
	ErrorMessage  string
	CommandString string
	ConflictInfo  PortConflictInfo // Information about port conflicts
	cmd           *exec.Cmd
	cancel        context.CancelFunc
	mu            sync.Mutex
	context       string
	namespace     string
	retryCount    int  // Current retry attempt number
	maxRetries    int  // Maximum retry attempts (-1 for infinite, 0 to disable)
	manualStop    bool // Flag to prevent retries when user stops manually
	retrying      bool // Indicates if currently in retry mode
}

// NewPortForward creates a new PortForward instance
func NewPortForward(service Service, globalContext, globalNamespace string, globalMaxRetries int) *PortForward {
	// Use service-specific context/namespace or fall back to global
	context := service.GetContext(globalContext)
	namespace := service.GetNamespace(globalNamespace)
	maxRetries := service.GetMaxRetries(globalMaxRetries)
	
	pf := &PortForward{
		Service:    service,
		Status:     StatusStopped,
		context:    context,
		namespace:  namespace,
		maxRetries: maxRetries,
		retryCount: 0,
		manualStop: false,
		retrying:   false,
	}
	
	// Check for port conflicts on initialization
	pf.ConflictInfo = DetectPortConflict(service.LocalPort)
	if pf.ConflictInfo.HasConflict {
		pf.Status = StatusError
		if pf.ConflictInfo.IsKubectl {
			pf.ErrorMessage = fmt.Sprintf("Port already in use by kubectl port-forward (PID: %d). Press 'K' to kill it.", 
				pf.ConflictInfo.ProcessPID)
		} else if pf.ConflictInfo.ProcessPID > 0 {
			pf.ErrorMessage = fmt.Sprintf("Port already in use by PID %d. Press 'K' to kill it.", 
				pf.ConflictInfo.ProcessPID)
		} else {
			pf.ErrorMessage = "Port already in use by another process"
		}
	}
	
	return pf
}

// Start initiates the kubectl port-forward process
func (pf *PortForward) Start() error {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	if pf.Status == StatusRunning || pf.Status == StatusStarting {
		return fmt.Errorf("port forward already running")
	}

	// Check for port conflicts before starting
	conflictInfo := DetectPortConflict(pf.Service.LocalPort)
	if conflictInfo.HasConflict {
		pf.ConflictInfo = conflictInfo
		pf.Status = StatusError
		if conflictInfo.IsKubectl {
			pf.ErrorMessage = fmt.Sprintf("Port already in use by kubectl port-forward (PID: %d). Press 'K' to kill it.", 
				conflictInfo.ProcessPID)
		} else if conflictInfo.ProcessPID > 0 {
			pf.ErrorMessage = fmt.Sprintf("Port already in use by PID %d. Press 'K' to kill it.", 
				conflictInfo.ProcessPID)
		} else {
			pf.ErrorMessage = "Port already in use by another process"
		}
		pf.manualStop = true // Prevent retries
		return fmt.Errorf("port %d already in use", pf.Service.LocalPort)
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
				// Check if Start() set manualStop (e.g., port conflict)
				pf.mu.Lock()
				if pf.manualStop {
					// This is a permanent error (port conflict, etc.) - don't retry further
					pf.Status = StatusError
					pf.retrying = false
					// Error message already set by Start()
					pf.mu.Unlock()
					return
				}
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

// isPortAvailable checks if a port is available for use
func isPortAvailable(port int) bool {
	// Try to listen on IPv4 (127.0.0.1)
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	listener.Close()
	
	// Try to listen on IPv6 ([::1])
	listener6, err := net.Listen("tcp", fmt.Sprintf("[::1]:%d", port))
	if err != nil {
		// If IPv6 fails but IPv4 succeeded, the port is still considered unavailable
		// because something might be using it on IPv6
		return false
	}
	listener6.Close()
	
	return true
}

// DetectPortConflict checks if a port is in use and tries to identify the process
func DetectPortConflict(port int) PortConflictInfo {
	info := PortConflictInfo{
		HasConflict: false,
		IsKubectl:   false,
	}

	// First check if port is available
	if isPortAvailable(port) {
		return info
	}

	info.HasConflict = true

	// Try to identify the process using the port
	// Use lsof to find the process (works on macOS and Linux)
	cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port), "-t")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		pidStr := strings.TrimSpace(string(output))
		lines := strings.Split(pidStr, "\n")
		if len(lines) > 0 {
			var pid int
			fmt.Sscanf(lines[0], "%d", &pid)
			if pid > 0 {
				info.ProcessPID = pid
				
				// Get process command line
				psCmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "command=")
				cmdOutput, err := psCmd.Output()
				if err == nil {
					info.ProcessCommand = strings.TrimSpace(string(cmdOutput))
					// Check if it's a kubectl port-forward
					if strings.Contains(info.ProcessCommand, "kubectl") && 
					   strings.Contains(info.ProcessCommand, "port-forward") {
						info.IsKubectl = true
					}
				}
			}
		}
	}

	return info
}

// KillProcess attempts to kill a process by PID
func KillProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID")
	}
	
	cmd := exec.Command("kill", fmt.Sprintf("%d", pid))
	return cmd.Run()
}

// HasPortConflict returns true if this port forward has a detected conflict
func (pf *PortForward) HasPortConflict() bool {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	return pf.ConflictInfo.HasConflict
}

// GetConflictInfo returns the conflict information
func (pf *PortForward) GetConflictInfo() PortConflictInfo {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	return pf.ConflictInfo
}

// ClearConflict clears the conflict status (e.g., after killing the process)
func (pf *PortForward) ClearConflict() {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	pf.ConflictInfo = PortConflictInfo{HasConflict: false}
	if pf.Status == StatusError && strings.Contains(pf.ErrorMessage, "Port already in use") {
		pf.Status = StatusStopped
		pf.ErrorMessage = ""
	}
}

// RefreshConflictStatus re-checks the port and updates conflict information
func (pf *PortForward) RefreshConflictStatus() {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	
	// Don't check conflicts for running services
	if pf.Status == StatusRunning || pf.Status == StatusStarting {
		return
	}
	
	// Re-detect port conflict
	newInfo := DetectPortConflict(pf.Service.LocalPort)
	pf.ConflictInfo = newInfo
	
	// Update status and error message based on new conflict info
	if newInfo.HasConflict {
		pf.Status = StatusError
		if newInfo.IsKubectl {
			pf.ErrorMessage = fmt.Sprintf("Port already in use by kubectl port-forward (PID: %d). Press 'K' to kill it.", 
				newInfo.ProcessPID)
		} else if newInfo.ProcessPID > 0 {
			pf.ErrorMessage = fmt.Sprintf("Port already in use by PID %d. Press 'K' to kill it.", 
				newInfo.ProcessPID)
		} else {
			pf.ErrorMessage = "Port already in use by another process"
		}
	} else {
		// Port is now available, clear error if it was a port conflict error
		if pf.Status == StatusError && strings.Contains(pf.ErrorMessage, "Port already in use") {
			pf.Status = StatusStopped
			pf.ErrorMessage = ""
		}
	}
}


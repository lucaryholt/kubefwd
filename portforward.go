package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
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
}

// NewPortForward creates a new PortForward instance
func NewPortForward(service Service, globalContext, globalNamespace string) *PortForward {
	// Use service-specific context/namespace or fall back to global
	context := service.GetContext(globalContext)
	namespace := service.GetNamespace(globalNamespace)
	
	return &PortForward{
		Service:   service,
		Status:    StatusStopped,
		context:   context,
		namespace: namespace,
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
	return nil
}

// monitor watches the port-forward process and updates status
func (pf *PortForward) monitor(stderr *strings.Builder) {
	err := pf.cmd.Wait()

	pf.mu.Lock()
	defer pf.mu.Unlock()

	if err != nil && pf.Status != StatusStopped {
		pf.Status = StatusError
		pf.ErrorMessage = fmt.Sprintf("Process exited: %v", err)
		if stderr.Len() > 0 {
			pf.ErrorMessage += fmt.Sprintf(" | stderr: %s", strings.TrimSpace(stderr.String()))
		}
		pf.ErrorMessage += fmt.Sprintf(" | Command: %s", pf.CommandString)
	} else if pf.Status == StatusRunning {
		pf.Status = StatusStopped
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


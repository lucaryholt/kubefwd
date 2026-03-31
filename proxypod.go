package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ProxyPodStatus represents the current state of the proxy pod
type ProxyPodStatus string

const (
	ProxyPodStatusNotCreated ProxyPodStatus = "not_created"
	ProxyPodStatusCreating   ProxyPodStatus = "creating"
	ProxyPodStatusReady      ProxyPodStatus = "ready"
	ProxyPodStatusError      ProxyPodStatus = "error"
)

// ProxyPodManager manages the shared proxy pod lifecycle
type ProxyPodManager struct {
	podName         string
	podImage        string
	namespace       string
	context         string
	currentServices []ProxyService    // Services currently in the pod
	podPorts        map[string]int    // Maps service name to unique pod port
	status          ProxyPodStatus
	errorMessage    string
	mu              sync.Mutex
}

// sanitizePodNameSegment converts an arbitrary string into a valid k8s name segment:
// lowercase, non-alphanumeric chars replaced with '-', leading/trailing dashes removed.
var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

func sanitizePodNameSegment(s string) string {
	s = strings.ToLower(s)
	s = nonAlphanumRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// BuildPodName returns a stable, valid k8s pod name for the given base name,
// context and namespace. The result is at most 63 characters.
func BuildPodName(baseName, podContext, podNamespace string) string {
	ctx := sanitizePodNameSegment(podContext)
	ns := sanitizePodNameSegment(podNamespace)
	name := baseName + "-" + ctx + "-" + ns
	if len(name) > 63 {
		name = name[:63]
	}
	name = strings.TrimRight(name, "-")
	return name
}

// NewProxyPodManager creates a new proxy pod manager
func NewProxyPodManager(podName, podImage, namespace, context string) *ProxyPodManager {
	return &ProxyPodManager{
		podName:         podName,
		podImage:        podImage,
		namespace:       namespace,
		context:         context,
		currentServices: []ProxyService{},
		podPorts:        make(map[string]int),
		status:          ProxyPodStatusNotCreated,
	}
}

// CreatePodWithServices creates a single-container pod with all selected services
func (pm *ProxyPodManager) CreatePodWithServices(selectedServices []ProxyService) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.status = ProxyPodStatusCreating
	pm.errorMessage = ""

	// Delete old pod if it exists
	pm.deletePodUnsafe()

	if len(selectedServices) == 0 {
		// No services selected, just ensure pod is deleted
		pm.status = ProxyPodStatusNotCreated
		pm.currentServices = []ProxyService{}
		pm.podPorts = make(map[string]int)
		return nil
	}

	// Assign unique pod ports for each service
	pm.podPorts = make(map[string]int)
	for i, svc := range selectedServices {
		pm.podPorts[svc.Name] = 10000 + i
	}

	// Build socat commands for each service (running in background)
	socatCommands := []string{}
	for _, svc := range selectedServices {
		podPort := pm.podPorts[svc.Name]
		cmd := fmt.Sprintf("socat TCP-LISTEN:%d,fork,reuseaddr TCP:%s:%d",
			podPort, svc.TargetHost, svc.TargetPort)
		socatCommands = append(socatCommands, cmd+" &")
	}
	socatCommands = append(socatCommands, "wait") // Wait for all background processes

	shellCommand := strings.Join(socatCommands, " ")

	debugLog("Creating proxy pod with command: %s", shellCommand)

	// Create pod using kubectl run
	cmd := exec.Command("kubectl",
		"--context="+pm.context,
		"run", "-n", pm.namespace, pm.podName,
		"--image="+pm.podImage,
		"--restart=Never",
		"--command", "--", "sh", "-c", shellCommand)

	output, err := debugRunCmd(cmd)
	if err != nil {
		// Check if it's an AlreadyExists error even though we tried to delete
		if strings.Contains(string(output), "AlreadyExists") {
			debugLog("Pod still exists after deletion attempt, force deleting and retrying...")

			// Force delete and wait
			pm.deletePodUnsafe()
			time.Sleep(3 * time.Second)

			// Retry creation
			retryCmd := exec.Command("kubectl",
				"--context="+pm.context,
				"run", "-n", pm.namespace, pm.podName,
				"--image="+pm.podImage,
				"--restart=Never",
				"--command", "--", "sh", "-c", shellCommand)

			output, err = debugRunCmd(retryCmd)
			if err != nil {
				pm.status = ProxyPodStatusError
				pm.errorMessage = fmt.Sprintf("Failed to create pod (retry): %v | %s", err, string(output))
				return fmt.Errorf("%s", pm.errorMessage)
			}
		} else {
			pm.status = ProxyPodStatusError
			pm.errorMessage = fmt.Sprintf("Failed to create pod: %v | %s", err, string(output))
			return fmt.Errorf("%s", pm.errorMessage)
		}
	}

	// Wait for pod to be ready
	if err := pm.waitForPodReady(60 * time.Second); err != nil {
		pm.status = ProxyPodStatusError
		pm.errorMessage = fmt.Sprintf("Pod failed to become ready: %v", err)
		
		// Get pod status for debugging
		descCmd := exec.Command("kubectl",
			"--context="+pm.context,
			"-n", pm.namespace,
			"describe", "pod", pm.podName)
		debugRunCmd(descCmd) //nolint — result already logged by debugRunCmd

		// Also get logs
		logsCmd := exec.Command("kubectl",
			"--context="+pm.context,
			"-n", pm.namespace,
			"logs", pm.podName,
			"--all-containers=true")
		debugRunCmd(logsCmd) //nolint — result already logged by debugRunCmd
		
		return err
	}

	pm.status = ProxyPodStatusReady
	pm.currentServices = selectedServices
	pm.errorMessage = ""
	return nil
}

// checkPodExists checks if the proxy pod exists and is ready
func (pm *ProxyPodManager) checkPodExists() (exists bool, ready bool, err error) {
	cmd := exec.Command("kubectl",
		"--context="+pm.context,
		"-n", pm.namespace,
		"get", "pod", pm.podName,
		"-o", "json")

	output, err := debugRunCmd(cmd)
	if err != nil {
		// Pod doesn't exist
		if strings.Contains(string(output), "NotFound") {
			return false, false, nil
		}
		return false, false, fmt.Errorf("kubectl get pod failed: %v | %s", err, string(output))
	}

	// Parse JSON to check if pod is ready
	var podData struct {
		Status struct {
			Phase      string `json:"phase"`
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	}

	if err := json.Unmarshal(output, &podData); err != nil {
		return true, false, fmt.Errorf("failed to parse pod JSON: %v", err)
	}

	// Check if pod is in Running phase and Ready condition is True
	if podData.Status.Phase != "Running" {
		return true, false, nil
	}

	for _, cond := range podData.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == "True" {
			return true, true, nil
		}
	}

	return true, false, nil
}

// deletePodUnsafe deletes the proxy pod without locking (caller must hold lock)
func (pm *ProxyPodManager) deletePodUnsafe() error {
	// First, try normal deletion
	cmd := exec.Command("kubectl",
		"--context="+pm.context,
		"-n", pm.namespace,
		"delete", "pod", pm.podName,
		"--ignore-not-found=true",
		"--wait=false",
		"--force")

	output, err := debugRunCmd(cmd)
	if err != nil && !strings.Contains(string(output), "NotFound") {
		return fmt.Errorf("kubectl delete pod failed: %v | %s", err, string(output))
	}

	// Wait for pod to actually be deleted (max 30 seconds)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		// Check if pod still exists
		checkCmd := exec.Command("kubectl",
			"--context="+pm.context,
			"-n", pm.namespace,
			"get", "pod", pm.podName,
			"--ignore-not-found=true",
			"--no-headers")

		checkOutput, _ := debugRunCmd(checkCmd)

		// If no output, pod is gone
		if len(strings.TrimSpace(string(checkOutput))) == 0 {
			debugLog("Pod successfully deleted")
			return nil
		}

		debugLog("Waiting for pod deletion... (status: %s)", strings.TrimSpace(string(checkOutput)))

		time.Sleep(1 * time.Second)
	}

	// If we got here, deletion timed out - try one more force delete
	debugLog("Deletion timeout, attempting final force delete")

	finalCmd := exec.Command("kubectl",
		"--context="+pm.context,
		"-n", pm.namespace,
		"delete", "pod", pm.podName,
		"--grace-period=0",
		"--force",
		"--ignore-not-found=true")

	debugRunCmd(finalCmd) //nolint — result already logged by debugRunCmd

	// Wait a bit more after force delete
	time.Sleep(2 * time.Second)

	return nil
}

// waitForPodReady waits for the pod to become ready with a timeout
func (pm *ProxyPodManager) waitForPodReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		exists, ready, err := pm.checkPodExists()
		if err != nil {
			return err
		}
		if exists && ready {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for pod to become ready")
}

// DeletePod deletes the proxy pod
func (pm *ProxyPodManager) DeletePod() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	err := pm.deletePodUnsafe()
	if err != nil {
		return err
	}

	pm.status = ProxyPodStatusNotCreated
	pm.currentServices = []ProxyService{}
	pm.podPorts = make(map[string]int)
	pm.errorMessage = ""

	return nil
}

// GetActiveServiceNames returns the names of services currently in the pod
func (pm *ProxyPodManager) GetActiveServiceNames() []string {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	names := make([]string, len(pm.currentServices))
	for i, svc := range pm.currentServices {
		names[i] = svc.Name
	}
	return names
}

// IsServiceActive checks if a service is currently active in the pod
func (pm *ProxyPodManager) IsServiceActive(name string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	for _, svc := range pm.currentServices {
		if svc.Name == name {
			return true
		}
	}
	return false
}

// GetPodPort returns the pod port for a given service name
func (pm *ProxyPodManager) GetPodPort(serviceName string) (int, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	port, exists := pm.podPorts[serviceName]
	return port, exists
}

// GetStatus returns the current status and active service count
func (pm *ProxyPodManager) GetStatus() (ProxyPodStatus, string, int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.status, pm.errorMessage, len(pm.currentServices)
}

// ProxyForward manages a port-forward to the proxy pod
type ProxyForward struct {
	ProxyService  ProxyService
	PodManager    *ProxyPodManager
	Status        PortForwardStatus
	ErrorMessage  string
	CommandString string
	cmd           *exec.Cmd
	cancel        context.CancelFunc
	mu            sync.Mutex
	sqlTapManager *SqlTapManager // Manages sql-tapd process if enabled
}

// NewProxyForward creates a new proxy forward instance
func NewProxyForward(proxyService ProxyService, podManager *ProxyPodManager) *ProxyForward {
	// Initialize sql-tap manager if configured
	var sqlTapManager *SqlTapManager
	if proxyService.SqlTapPort != nil {
		grpcPort := 9091 // Default
		if proxyService.SqlTapGrpcPort != nil {
			grpcPort = *proxyService.SqlTapGrpcPort
		}
		httpPort := 0
		if proxyService.SqlTapHttpPort != nil {
			httpPort = *proxyService.SqlTapHttpPort
		}
		sqlTapManager = NewSqlTapManager(
			true,
			proxyService.SqlTapDriver,
			*proxyService.SqlTapPort,
			proxyService.LocalPort,
			grpcPort,
			httpPort,
		)
	} else {
		sqlTapManager = NewSqlTapManager(false, "", 0, 0, 0, 0)
	}

	return &ProxyForward{
		ProxyService:  proxyService,
		PodManager:    podManager,
		Status:        StatusStopped,
		sqlTapManager: sqlTapManager,
	}
}

// Start initiates the proxy forward
func (pf *ProxyForward) Start() error {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	if pf.Status == StatusRunning || pf.Status == StatusStarting {
		return fmt.Errorf("proxy forward already running")
	}

	// Get the pod port for this service
	podPort, exists := pf.PodManager.GetPodPort(pf.ProxyService.Name)
	if !exists {
		pf.Status = StatusError
		pf.ErrorMessage = "Service not found in proxy pod"
		return fmt.Errorf("%s", pf.ErrorMessage)
	}

	pf.Status = StatusStarting
	pf.ErrorMessage = ""

	// Create context for the command
	ctx, cancel := context.WithCancel(context.Background())
	pf.cancel = cancel

	// Build kubectl port-forward command to the proxy pod
	portSpec := fmt.Sprintf("%d:%d", pf.ProxyService.LocalPort, podPort)
	podSpec := fmt.Sprintf("pod/%s", pf.PodManager.podName)

	args := []string{
		"--context=" + pf.PodManager.context,
		"-n", pf.PodManager.namespace,
		"port-forward",
		podSpec,
		portSpec,
	}

	pf.CommandString = fmt.Sprintf("kubectl %s", strings.Join(args, " "))

	debugLog("Executing proxy port-forward: %s", pf.CommandString)

	pf.cmd = exec.CommandContext(ctx, "kubectl", args...)

	var stderr strings.Builder
	pf.cmd.Stderr = &stderr

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
			debugLog("Failed to start sql-tapd for proxy %s: %v", pf.ProxyService.Name, err)
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

// monitor watches the proxy forward process and updates status
func (pf *ProxyForward) monitor(stderr *strings.Builder) {
	err := pf.cmd.Wait()

	pf.mu.Lock()
	defer pf.mu.Unlock()

	if err != nil && pf.Status != StatusStopped {
		pf.Status = StatusError
		pf.ErrorMessage = fmt.Sprintf("Process exited: %v", err)
		if stderr.Len() > 0 {
			pf.ErrorMessage += fmt.Sprintf(" | stderr: %s", strings.TrimSpace(stderr.String()))
		}
	} else {
		if pf.Status == StatusRunning {
			pf.Status = StatusStopped
		}
	}
}

// Stop terminates the proxy forward
func (pf *ProxyForward) Stop() error {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	if pf.Status != StatusRunning && pf.Status != StatusStarting {
		return nil
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
	return nil
}

// IsRunning returns true if the proxy forward is currently running
func (pf *ProxyForward) IsRunning() bool {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	return pf.Status == StatusRunning || pf.Status == StatusStarting
}

// GetStatus returns the current status and error message
func (pf *ProxyForward) GetStatus() (PortForwardStatus, string) {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	return pf.Status, pf.ErrorMessage
}

// GetSqlTapManager returns the sql-tap manager for this proxy forward
func (pf *ProxyForward) GetSqlTapManager() *SqlTapManager {
	return pf.sqlTapManager
}

// GetPID returns the process ID of the kubectl port-forward process
func (pf *ProxyForward) GetPID() int {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	if pf.cmd != nil && pf.cmd.Process != nil {
		return pf.cmd.Process.Pid
	}
	return 0
}


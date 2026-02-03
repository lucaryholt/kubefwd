package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const defaultConfigFile = ".kubefwd.yaml"

// Global debug flag
var debugMode bool

// getDefaultConfigPath returns the full path to the config file in the user's home directory
func getDefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home directory can't be determined
		return defaultConfigFile
	}
	return filepath.Join(home, defaultConfigFile)
}

// getPidFilePath returns the full path to the PID file in the user's home directory
func getPidFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".kubefwd.pid"
	}
	return filepath.Join(home, ".kubefwd.pid")
}

// writePidFile writes the current process ID to the PID file
func writePidFile(path string) error {
	pid := os.Getpid()
	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", pid)), 0644)
}

// readPidFile reads the PID from the PID file
func readPidFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pidStr := strings.TrimSpace(string(data))
	return strconv.Atoi(pidStr)
}

// removePidFile removes the PID file
func removePidFile(path string) error {
	return os.Remove(path)
}

// isProcessRunning checks if a process with the given PID is running
func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix systems, FindProcess always succeeds, so we need to send signal 0
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// BackgroundManager manages services in background mode
type BackgroundManager struct {
	config          *Config
	portForwards    []*PortForward
	proxyPodManager *ProxyPodManager
	proxyForwards   map[string]*ProxyForward
}

// NewBackgroundManager creates a new background manager
func NewBackgroundManager(config *Config) *BackgroundManager {
	// Create port forwards for direct services
	portForwards := make([]*PortForward, len(config.Services))
	for i, svc := range config.Services {
		portForwards[i] = NewPortForward(svc, config.ClusterContext, config.Namespace, config.MaxRetries)
	}

	// Create proxy pod manager if proxy services exist
	var proxyPodManager *ProxyPodManager
	if len(config.ProxyServices) > 0 {
		proxyPodManager = NewProxyPodManager(
			config.ProxyPodName,
			config.ProxyPodImage,
			config.ProxyPodNamespace,
			config.ProxyPodContext,
		)
	}

	return &BackgroundManager{
		config:          config,
		portForwards:    portForwards,
		proxyPodManager: proxyPodManager,
		proxyForwards:   make(map[string]*ProxyForward),
	}
}

// StartDefaultServices starts all services marked with selected_by_default
func (bm *BackgroundManager) StartDefaultServices() error {
	for i, svc := range bm.config.Services {
		if svc.SelectedByDefault {
			if err := bm.portForwards[i].Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to start %s: %v\n", svc.Name, err)
			}
		}
	}
	return nil
}

// StartDefaultProxyServices starts all proxy services marked with selected_by_default
func (bm *BackgroundManager) StartDefaultProxyServices() error {
	// Collect default proxy services
	var defaultProxyServices []ProxyService
	for _, pxSvc := range bm.config.ProxyServices {
		if pxSvc.SelectedByDefault {
			defaultProxyServices = append(defaultProxyServices, pxSvc)
		}
	}

	if len(defaultProxyServices) == 0 {
		return nil
	}

	// Create pod with selected services
	if err := bm.proxyPodManager.CreatePodWithServices(defaultProxyServices); err != nil {
		return fmt.Errorf("failed to create proxy pod: %w", err)
	}

	// Start port forwards for selected services
	for _, pxSvc := range defaultProxyServices {
		pxf := NewProxyForward(pxSvc, bm.proxyPodManager, bm.config.ClusterContext, bm.config.Namespace)
		if err := pxf.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to start proxy forward for %s: %v\n", pxSvc.Name, err)
		}
		bm.proxyForwards[pxSvc.Name] = pxf
	}

	return nil
}

// WaitForServicesToStart waits for all started services to reach running or error state
func (bm *BackgroundManager) WaitForServicesToStart(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Collect services to monitor
	var servicesToMonitor []*PortForward
	for i, svc := range bm.config.Services {
		if svc.SelectedByDefault {
			servicesToMonitor = append(servicesToMonitor, bm.portForwards[i])
		}
	}

	// Collect proxy services to monitor
	var proxyServicesToMonitor []*ProxyForward
	for _, pxf := range bm.proxyForwards {
		proxyServicesToMonitor = append(proxyServicesToMonitor, pxf)
	}

	totalServices := len(servicesToMonitor) + len(proxyServicesToMonitor)
	if totalServices == 0 {
		return nil
	}

	for {
		select {
		case <-ticker.C:
			runningCount := 0
			errorCount := 0

			// Check direct services
			for _, pf := range servicesToMonitor {
				status, _ := pf.GetStatus()
				if status == StatusRunning {
					runningCount++
				} else if status == StatusError {
					errorCount++
					runningCount++ // Count errors as "done" to avoid timeout
				}
			}

			// Check proxy services
			for _, pxf := range proxyServicesToMonitor {
				status, _ := pxf.GetStatus()
				if status == StatusRunning {
					runningCount++
				} else if status == StatusError {
					errorCount++
					runningCount++ // Count errors as "done" to avoid timeout
				}
			}

			// Print progress
			fmt.Fprintf(os.Stderr, "Waiting for services to start... (%d/%d ready)\n", runningCount, totalServices)

			// Check if all services are ready
			if runningCount >= totalServices {
				if errorCount > 0 {
					fmt.Fprintf(os.Stderr, "Warning: %d service(s) failed to start\n", errorCount)
				}
				return nil
			}

			// Check timeout
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for services to start (%d/%d ready)", runningCount, totalServices)
			}
		}
	}
}

// StopAll stops all running services and cleans up
func (bm *BackgroundManager) StopAll() {
	// Stop all port forwards
	for _, pf := range bm.portForwards {
		if pf.IsRunning() {
			pf.Stop()
		}
	}

	// Stop all proxy forwards
	for _, pxf := range bm.proxyForwards {
		pxf.Stop()
	}

	// Delete proxy pod
	if bm.proxyPodManager != nil {
		bm.proxyPodManager.DeletePod()
	}
}

func main() {
	// Parse command-line flags
	configFile := flag.String("config", getDefaultConfigPath(), "Path to configuration file")
	debug := flag.Bool("debug", false, "Enable debug output")
	defaultFlag := flag.Bool("default", false, "Auto-start services marked with selected_by_default")
	defaultProxyFlag := flag.Bool("default-proxy", false, "Auto-start proxy services marked with selected_by_default")
	backgroundFlag := flag.Bool("background", false, "Run in background mode (headless, no TUI)")
	flag.Parse()

	// Set global debug mode
	debugMode = *debug

	// Validate flag combinations
	if *backgroundFlag && !*defaultFlag && !*defaultProxyFlag {
		fmt.Fprintf(os.Stderr, "Error: --background requires at least one of --default or --default-proxy\n")
		os.Exit(1)
	}

	// Check if kubectl is available
	if err := CheckKubectlAvailable(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please ensure kubectl is installed and available in your PATH\n")
		os.Exit(1)
	}

	// Load configuration
	config, err := LoadConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration from %s: %v\n", *configFile, err)
		os.Exit(1)
	}

	// Validate cluster context
	if err := ValidateContext(config.ClusterContext); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please ensure the context exists and kubectl is configured correctly\n")
		os.Exit(1)
	}

	// Run in background mode if --background flag is set
	if *backgroundFlag {
		runBackgroundMode(config, *defaultFlag, *defaultProxyFlag)
		return
	}

	// Initialize the app model
	model := NewAppModel(config, *configFile, *defaultFlag, *defaultProxyFlag)

	// Start the Bubble Tea program
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}

// runBackgroundMode runs the application in background mode (headless, no TUI)
func runBackgroundMode(config *Config, startDefault, startDefaultProxy bool) {
	pidFile := getPidFilePath()

	// Check if PID file already exists
	if _, err := os.Stat(pidFile); err == nil {
		// PID file exists, check if process is still running
		if existingPid, err := readPidFile(pidFile); err == nil {
			if isProcessRunning(existingPid) {
				fmt.Fprintf(os.Stderr, "Error: kubefwd is already running (PID: %d)\n", existingPid)
				fmt.Fprintf(os.Stderr, "Stop it first: kill %d\n", existingPid)
				os.Exit(1)
			}
			// Stale PID file, remove it
			removePidFile(pidFile)
		}
	}

	// Create background manager
	manager := NewBackgroundManager(config)

	// Set up cleanup on exit
	cleanup := func() {
		fmt.Fprintf(os.Stderr, "\nShutting down...\n")
		manager.StopAll()
		if err := removePidFile(pidFile); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: Failed to remove PID file: %v\n", err)
		}
	}
	defer cleanup()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start services based on flags
	fmt.Fprintf(os.Stderr, "Starting services in background mode...\n")

	if startDefault {
		if err := manager.StartDefaultServices(); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting default services: %v\n", err)
			os.Exit(1)
		}
	}

	if startDefaultProxy {
		if err := manager.StartDefaultProxyServices(); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting default proxy services: %v\n", err)
			os.Exit(1)
		}
	}

	// Wait for services to start
	fmt.Fprintf(os.Stderr, "Waiting for services to be ready...\n")
	if err := manager.WaitForServicesToStart(60 * time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Write PID file
	if err := writePidFile(pidFile); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to write PID file: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Services started successfully. PID file: %s\n", pidFile)
		fmt.Fprintf(os.Stderr, "To stop: kill $(cat %s)\n", pidFile)
	}

	// Keep process alive until signal received
	<-sigChan
}

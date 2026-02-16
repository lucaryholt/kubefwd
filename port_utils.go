package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// PortStatus represents the status of a port
type PortStatus string

const (
	PortStatusFree     PortStatus = "free"
	PortStatusKubefwd  PortStatus = "kubefwd"
	PortStatusExternal PortStatus = "external"
)

// PortUsageInfo contains information about port usage
type PortUsageInfo struct {
	InUse       bool
	PID         int
	ProcessInfo string
	Status      PortStatus
}

// GetPortUsage checks if a port is in use and returns information about the process
func GetPortUsage(port int) (PortUsageInfo, error) {
	info := PortUsageInfo{
		InUse:  false,
		PID:    0,
		Status: PortStatusFree,
	}

	// Use lsof to check if the port is in use
	// -i :PORT checks for internet connections on the specified port
	// -P prevents port names from being converted to service names
	// -n prevents hostname lookups
	// -sTCP:LISTEN only shows listening TCP connections
	cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port), "-P", "-n", "-sTCP:LISTEN")
	output, err := cmd.CombinedOutput()

	// If lsof returns an error, it might mean the port is not in use or lsof is not available
	if err != nil {
		// Check if it's because no process is using the port (exit code 1)
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				// Port is not in use (this is normal)
				return info, nil
			}
		}
		// Other errors (lsof not found, permission denied, etc.)
		return info, fmt.Errorf("failed to run lsof: %w", err)
	}

	// Parse lsof output
	// Example output:
	// COMMAND   PID USER   FD   TYPE DEVICE SIZE/OFF NODE NAME
	// kubectl 12345 user   3u  IPv4  0x1234      0t0  TCP *:5432 (LISTEN)
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		// No process found (only header or empty)
		return info, nil
	}

	// Parse the first data line (skip header)
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		command := fields[0]
		pidStr := fields[1]

		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		info.InUse = true
		info.PID = pid
		info.ProcessInfo = command
		info.Status = PortStatusExternal // Will be updated by caller if it's kubefwd

		// Get more detailed process information
		if detailedInfo := getProcessDetails(pid); detailedInfo != "" {
			info.ProcessInfo = detailedInfo
		}

		break
	}

	return info, nil
}

// getProcessDetails retrieves detailed information about a process
func getProcessDetails(pid int) string {
	// Use ps to get the full command line
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	cmdLine := strings.TrimSpace(string(output))
	
	// Truncate very long commands
	if len(cmdLine) > 60 {
		cmdLine = cmdLine[:57] + "..."
	}

	return cmdLine
}

// KillProcess sends a SIGTERM signal to a process
func KillProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}

	// Send SIGTERM to allow graceful shutdown
	err := syscall.Kill(pid, syscall.SIGTERM)
	if err != nil {
		return fmt.Errorf("failed to kill process %d: %w", pid, err)
	}

	return nil
}

// GetAllPortsFromConfig extracts all local ports from the configuration
func GetAllPortsFromConfig(config *Config) []ConfigPort {
	var ports []ConfigPort

	// Collect ports from direct services
	for _, svc := range config.Services {
		ports = append(ports, ConfigPort{
			Port:        svc.LocalPort,
			ServiceName: svc.Name,
			Type:        "Direct",
		})

		// Add sql-tap port if configured
		if svc.SqlTapPort != nil {
			ports = append(ports, ConfigPort{
				Port:        *svc.SqlTapPort,
				ServiceName: svc.Name + " (SQL-Tap)",
				Type:        "Direct",
			})
		}
	}

	// Collect ports from proxy services
	for _, pxSvc := range config.ProxyServices {
		ports = append(ports, ConfigPort{
			Port:        pxSvc.LocalPort,
			ServiceName: pxSvc.Name,
			Type:        "Proxy",
		})

		// Add sql-tap port if configured
		if pxSvc.SqlTapPort != nil {
			ports = append(ports, ConfigPort{
				Port:        *pxSvc.SqlTapPort,
				ServiceName: pxSvc.Name + " (SQL-Tap)",
				Type:        "Proxy",
			})
		}
	}

	return ports
}

// ConfigPort represents a port from the configuration
type ConfigPort struct {
	Port        int
	ServiceName string
	Type        string // "Direct" or "Proxy"
}

// IsKubefwdProcess checks if a PID belongs to a kubefwd-managed process
func IsKubefwdProcess(pid int, portForwards []*PortForward, proxyForwards map[string]*ProxyForward) bool {
	if pid <= 0 {
		return false
	}

	// Check direct port forwards
	for _, pf := range portForwards {
		if pf.GetPID() == pid {
			return true
		}
		
		// Check sql-tap manager
		sqlTapMgr := pf.GetSqlTapManager()
		if sqlTapMgr != nil && sqlTapMgr.GetPID() == pid {
			return true
		}
	}

	// Check proxy forwards
	for _, pxf := range proxyForwards {
		if pxf.GetPID() == pid {
			return true
		}
		
		// Check sql-tap manager
		sqlTapMgr := pxf.GetSqlTapManager()
		if sqlTapMgr != nil && sqlTapMgr.GetPID() == pid {
			return true
		}
	}

	return false
}

// ExtractPIDFromCommand extracts PID from a command string if it contains kubectl port-forward
func ExtractPIDFromCommand(command string) int {
	// This is a helper function for debugging
	// Look for patterns like "kubectl port-forward" in the command
	if strings.Contains(command, "kubectl") && strings.Contains(command, "port-forward") {
		// Try to extract PID from the process tree
		re := regexp.MustCompile(`\((\d+)\)`)
		matches := re.FindStringSubmatch(command)
		if len(matches) > 1 {
			pid, _ := strconv.Atoi(matches[1])
			return pid
		}
	}
	return 0
}

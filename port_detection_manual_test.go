package main

import (
	"fmt"
	"testing"
)

// TestPortDetectionOnRunningKubectl tests detection on a port with kubectl running
func TestPortDetectionOnRunningKubectl(t *testing.T) {
	// This should be run manually when you have kubectl port-forward on port 4480
	port := 4480
	
	t.Logf("Testing port %d (should have kubectl port-forward running)", port)
	
	available := isPortAvailable(port)
	t.Logf("Port available: %v", available)
	
	if available {
		t.Error("Port should NOT be available (kubectl is using it)")
	}
	
	info := DetectPortConflict(port)
	t.Logf("Conflict detected: %v", info.HasConflict)
	t.Logf("Is kubectl: %v", info.IsKubectl)
	t.Logf("PID: %d", info.ProcessPID)
	t.Logf("Command: %s", info.ProcessCommand)
	
	if !info.HasConflict {
		t.Error("Should detect conflict on port 4480")
	}
	
	if info.ProcessPID <= 0 {
		t.Error("Should detect PID of process using the port")
	}
	
	if !info.IsKubectl {
		t.Error("Should identify process as kubectl port-forward")
	}
}

func TestManualPortCheck(t *testing.T) {
	// Test a few ports to see what's happening
	ports := []int{4480, 8080, 9999}
	
	for _, port := range ports {
		available := isPortAvailable(port)
		info := DetectPortConflict(port)
		
		fmt.Printf("\nPort %d:\n", port)
		fmt.Printf("  Available: %v\n", available)
		fmt.Printf("  HasConflict: %v\n", info.HasConflict)
		if info.HasConflict {
			fmt.Printf("  IsKubectl: %v\n", info.IsKubectl)
			fmt.Printf("  PID: %d\n", info.ProcessPID)
			fmt.Printf("  Command: %s\n", info.ProcessCommand)
		}
	}
}

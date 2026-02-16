package main

import (
	"net"
	"testing"
	"time"
)

// TestGetPortUsage tests port usage detection
func TestGetPortUsage(t *testing.T) {
	// Start a simple TCP listener on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	
	// Give the OS a moment to register the port
	time.Sleep(100 * time.Millisecond)

	// Test that the port is detected as in use
	info, err := GetPortUsage(port)
	if err != nil {
		t.Errorf("GetPortUsage failed: %v", err)
	}

	if !info.InUse {
		t.Errorf("Expected port %d to be in use, but it was reported as free", port)
	}

	if info.PID == 0 {
		t.Errorf("Expected PID to be set for port %d, but got 0", port)
	}

	t.Logf("Port %d is in use by PID %d (%s)", port, info.PID, info.ProcessInfo)
}

// TestGetPortUsageFree tests that free ports are reported correctly
func TestGetPortUsageFree(t *testing.T) {
	// Use a very high port number that is unlikely to be in use
	port := 62345

	info, err := GetPortUsage(port)
	if err != nil {
		t.Errorf("GetPortUsage failed: %v", err)
	}

	if info.InUse {
		t.Errorf("Expected port %d to be free, but it was reported as in use (PID: %d)", port, info.PID)
	}

	if info.PID != 0 {
		t.Errorf("Expected PID to be 0 for free port, but got %d", info.PID)
	}

	if info.Status != PortStatusFree {
		t.Errorf("Expected status to be PortStatusFree, but got %v", info.Status)
	}
}

// TestIsKubefwdProcess tests the kubefwd process detection
func TestIsKubefwdProcess(t *testing.T) {
	// Test with empty lists
	if IsKubefwdProcess(12345, nil, nil) {
		t.Error("Expected false for empty port forward lists")
	}

	if IsKubefwdProcess(0, nil, nil) {
		t.Error("Expected false for PID 0")
	}

	if IsKubefwdProcess(-1, nil, nil) {
		t.Error("Expected false for negative PID")
	}
}

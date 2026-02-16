package main

import (
	"fmt"
	"net"
	"testing"
	"time"
)

// TestDetectPortConflict tests the port conflict detection functionality
func TestDetectPortConflict(t *testing.T) {
	// Find an available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Test 1: Port should be available initially
	t.Run("NoConflict", func(t *testing.T) {
		info := DetectPortConflict(port)
		if info.HasConflict {
			t.Errorf("Expected no conflict for available port %d, but got conflict", port)
		}
	})

	// Test 2: Create a conflict by starting a listener
	t.Run("WithConflict", func(t *testing.T) {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			// Try another way to create the listener
			listener, err = net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("Failed to create test listener: %v", err)
			}
			port = listener.Addr().(*net.TCPAddr).Port
		}
		defer listener.Close()

		// Give the OS a moment to register the listener
		time.Sleep(100 * time.Millisecond)

		info := DetectPortConflict(port)
		if !info.HasConflict {
			t.Errorf("Expected conflict for occupied port %d, but got no conflict", port)
		}
		if info.ProcessPID <= 0 {
			t.Logf("Warning: Could not detect PID for conflicting process (PID: %d)", info.ProcessPID)
		}
	})
}

// TestIsPortAvailable tests the port availability check
func TestIsPortAvailable(t *testing.T) {
	// Find an available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Test available port
	if !isPortAvailable(port) {
		t.Errorf("Port %d should be available", port)
	}

	// Create listener to occupy the port
	listener, err = net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to create test listener: %v", err)
		}
		port = listener.Addr().(*net.TCPAddr).Port
	}
	defer listener.Close()

	// Test occupied port
	time.Sleep(100 * time.Millisecond)
	if isPortAvailable(port) {
		t.Errorf("Port %d should not be available", port)
	}
}

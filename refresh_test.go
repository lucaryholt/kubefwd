package main

import (
	"fmt"
	"net"
	"testing"
	"time"
)

// TestRefreshConflictStatus tests that RefreshConflictStatus updates correctly
func TestRefreshConflictStatus(t *testing.T) {
	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	
	// Create a test service
	service := Service{
		Name:        "test-service",
		ServiceName: "test",
		LocalPort:   port,
		RemotePort:  8080,
	}
	
	// Create port forward (should be available initially)
	pf := NewPortForward(service, "test-context", "default", -1)
	
	if pf.HasPortConflict() {
		t.Error("Port should be available initially")
	}
	
	// Create a listener to occupy the port on IPv4
	listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("Failed to occupy port: %v", err)
	}
	
	// Give OS time to register the listener
	time.Sleep(100 * time.Millisecond)
	
	// Refresh status - should now detect conflict
	pf.RefreshConflictStatus()
	
	if !pf.HasPortConflict() {
		t.Error("RefreshConflictStatus should detect port conflict")
	}
	
	conflictInfo := pf.GetConflictInfo()
	if !conflictInfo.HasConflict {
		t.Error("Conflict info should show HasConflict=true")
	}
	
	// Close the listener to free the port
	listener.Close()
	time.Sleep(100 * time.Millisecond)
	
	// Refresh again - should clear conflict
	pf.RefreshConflictStatus()
	
	if pf.HasPortConflict() {
		t.Error("RefreshConflictStatus should clear conflict when port is freed")
	}
	
	status, errMsg := pf.GetStatus()
	t.Logf("Final status: %v, error: %s", status, errMsg)
	
	if status != StatusStopped {
		t.Errorf("Status should be StatusStopped after clearing conflict, got %v", status)
	}
}

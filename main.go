package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

const defaultConfigFile = ".kubefwd.yaml"

// Global debug flag
var debugMode bool

// getDefaultConfigPath returns the full path to the config file in the user's home directory
func getDefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultConfigFile
	}
	return filepath.Join(home, defaultConfigFile)
}

func main() {
	configFile := flag.String("config", getDefaultConfigPath(), "Path to configuration file")
	debug := flag.Bool("debug", false, "Enable debug output")
	defaultFlag := flag.Bool("default", false, "Auto-start services marked with selected_by_default")
	defaultProxyFlag := flag.Bool("default-proxy", false, "Auto-start proxy services marked with selected_by_default")
	flag.Parse()

	debugMode = *debug

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

	// Warn if sql-tapd is required but unavailable
	sqlTapRequired := false
	for _, svc := range config.Services {
		if svc.SqlTapPort != nil {
			sqlTapRequired = true
			break
		}
	}
	if !sqlTapRequired {
		for _, pxSvc := range config.ProxyServices {
			if pxSvc.SqlTapPort != nil {
				sqlTapRequired = true
				break
			}
		}
	}
	if sqlTapRequired {
		if err := CheckSqlTapdAvailable(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			fmt.Fprintf(os.Stderr, "sql-tap features will not work. Install sql-tap from https://github.com/mickamy/sql-tap\n")
		}
	}

	// Create the web application state
	app := NewWebApp(config, *configFile)

	// Auto-start default services if requested
	if *defaultFlag {
		app.StartDefaults()
	}
	if *defaultProxyFlag {
		app.StartDefaultProxies()
	}

	// Start SSE broadcaster
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go app.startSSEBroadcaster(ctx)

	// Print the URL and start the HTTP server
	url := fmt.Sprintf("http://localhost:%d", config.WebPort)
	fmt.Printf("kubefwd running at %s\n", url)

	// Graceful shutdown on SIGINT / SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintf(os.Stderr, "\nShutting down…\n")
		cancel()
		app.StopAll()
		os.Exit(0)
	}()

	if err := app.ListenAndServe(config.WebPort); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting web server: %v\n", err)
		os.Exit(1)
	}
}

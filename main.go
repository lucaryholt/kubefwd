package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

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

func main() {
	// Parse command-line flags
	configFile := flag.String("config", getDefaultConfigPath(), "Path to configuration file")
	debug := flag.Bool("debug", false, "Enable debug output")
	flag.Parse()

	// Set global debug mode
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

	// Initialize the app model
	model := NewAppModel(config, *configFile)

	// Start the Bubble Tea program
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}


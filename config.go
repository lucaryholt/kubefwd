package main

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// AlternativeContext represents an alternative cluster context
type AlternativeContext struct {
	Name    string `yaml:"name"`
	Context string `yaml:"context"`
}

// Preset represents a preset configuration of services
type Preset struct {
	Name     string   `yaml:"name"`
	Services []string `yaml:"services"` // List of service names to start
}

// Config represents the complete configuration file structure
type Config struct {
	ClusterContext       string               `yaml:"cluster_context"`
	ClusterName          string               `yaml:"cluster_name,omitempty"`
	Namespace            string               `yaml:"namespace"`
	MaxRetries           int                  `yaml:"max_retries,omitempty"` // Global default: -1 for infinite, 0 to disable, N for specific limit
	AlternativeContexts  []AlternativeContext `yaml:"alternative_contexts,omitempty"`
	Presets              []Preset             `yaml:"presets,omitempty"`
	Services             []Service            `yaml:"services"`
}

// Service represents a single service configuration
type Service struct {
	Name              string `yaml:"name"`
	ServiceName       string `yaml:"service_name"`
	RemotePort        int    `yaml:"remote_port"`
	LocalPort         int    `yaml:"local_port"`
	SelectedByDefault bool   `yaml:"selected_by_default"`
	Context           string `yaml:"context,omitempty"`           // Optional: override cluster context
	Namespace         string `yaml:"namespace,omitempty"`         // Optional: override namespace
	MaxRetries        *int   `yaml:"max_retries,omitempty"`       // Optional: override global max_retries
}

// GetContext returns the service-specific context or falls back to global context
func (s *Service) GetContext(globalContext string) string {
	if s.Context != "" {
		return s.Context
	}
	return globalContext
}

// GetNamespace returns the service-specific namespace or falls back to global namespace
func (s *Service) GetNamespace(globalNamespace string) string {
	if s.Namespace != "" {
		return s.Namespace
	}
	return globalNamespace
}

// GetMaxRetries returns the service-specific max retries or falls back to global max retries
func (s *Service) GetMaxRetries(globalMaxRetries int) int {
	if s.MaxRetries != nil {
		return *s.MaxRetries
	}
	return globalMaxRetries
}

// LoadConfig reads and parses the YAML configuration file
func LoadConfig(filepath string) (*Config, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set default max_retries if not specified
	if config.MaxRetries == 0 {
		config.MaxRetries = -1 // Default to infinite retries
	}

	// Validate configuration
	if config.ClusterContext == "" {
		return nil, fmt.Errorf("cluster_context is required")
	}
	if config.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	if len(config.Services) == 0 {
		return nil, fmt.Errorf("at least one service must be defined")
	}

	// Validate each service
	for i, svc := range config.Services {
		if svc.Name == "" {
			return nil, fmt.Errorf("service %d: name is required", i)
		}
		if svc.ServiceName == "" {
			return nil, fmt.Errorf("service %d (%s): service_name is required", i, svc.Name)
		}
		if svc.RemotePort <= 0 || svc.RemotePort > 65535 {
			return nil, fmt.Errorf("service %d (%s): invalid remote_port", i, svc.Name)
		}
		if svc.LocalPort <= 0 || svc.LocalPort > 65535 {
			return nil, fmt.Errorf("service %d (%s): invalid local_port", i, svc.Name)
		}
	}

	// Sort services alphabetically by name
	sort.Slice(config.Services, func(i, j int) bool {
		return config.Services[i].Name < config.Services[j].Name
	})

	return &config, nil
}


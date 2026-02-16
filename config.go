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
	ProxyPodName         string               `yaml:"proxy_pod_name,omitempty"`      // Name for the shared proxy pod (default: kubefwd-proxy)
	ProxyPodImage        string               `yaml:"proxy_pod_image,omitempty"`     // Container image for proxy pod (default: alpine/socat:latest)
	ProxyPodContext      string               `yaml:"proxy_pod_context,omitempty"`   // Context where proxy pod is created (default: cluster_context)
	ProxyPodNamespace    string               `yaml:"proxy_pod_namespace,omitempty"` // Namespace where proxy pod is created (default: namespace)
	ProxyServices        []ProxyService       `yaml:"proxy_services,omitempty"`      // Proxy services for GCP connections
}

// Service represents a single service configuration
type Service struct {
	Name               string `yaml:"name"`
	ServiceName        string `yaml:"service_name"`
	RemotePort         int    `yaml:"remote_port"`
	LocalPort          int    `yaml:"local_port"`
	SelectedByDefault  bool   `yaml:"selected_by_default"`
	Context            string `yaml:"context,omitempty"`             // Optional: override cluster context
	Namespace          string `yaml:"namespace,omitempty"`           // Optional: override namespace
	MaxRetries         *int   `yaml:"max_retries,omitempty"`         // Optional: override global max_retries
	SqlTapPort         *int   `yaml:"sql_tap_port,omitempty"`        // Optional: port for sql-tap proxy
	SqlTapDriver       string `yaml:"sql_tap_driver,omitempty"`      // Optional: driver (postgres or mysql)
	SqlTapGrpcPort     *int   `yaml:"sql_tap_grpc_port,omitempty"`   // Optional: gRPC port for sql-tap client (default: auto-assigned)
}

// ProxyService represents a proxy pod service configuration
type ProxyService struct {
	Name               string `yaml:"name"`
	TargetHost         string `yaml:"target_host"`
	TargetPort         int    `yaml:"target_port"`
	LocalPort          int    `yaml:"local_port"`
	SelectedByDefault  bool   `yaml:"selected_by_default"`
	Context            string `yaml:"context,omitempty"`             // Optional: override cluster context
	Namespace          string `yaml:"namespace,omitempty"`           // Optional: override namespace
	MaxRetries         *int   `yaml:"max_retries,omitempty"`         // Optional: override global max_retries
	SqlTapPort         *int   `yaml:"sql_tap_port,omitempty"`        // Optional: port for sql-tap proxy
	SqlTapDriver       string `yaml:"sql_tap_driver,omitempty"`      // Optional: driver (postgres or mysql)
	SqlTapGrpcPort     *int   `yaml:"sql_tap_grpc_port,omitempty"`   // Optional: gRPC port for sql-tap client (default: auto-assigned)
}

// GetContext returns the service-specific context or falls back to global context
func (ps *ProxyService) GetContext(globalContext string) string {
	if ps.Context != "" {
		return ps.Context
	}
	return globalContext
}

// GetNamespace returns the service-specific namespace or falls back to global namespace
func (ps *ProxyService) GetNamespace(globalNamespace string) string {
	if ps.Namespace != "" {
		return ps.Namespace
	}
	return globalNamespace
}

// GetMaxRetries returns the service-specific max retries or falls back to global max retries
func (ps *ProxyService) GetMaxRetries(globalMaxRetries int) int {
	if ps.MaxRetries != nil {
		return *ps.MaxRetries
	}
	return globalMaxRetries
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

	// Set default proxy pod configuration
	if config.ProxyPodName == "" {
		config.ProxyPodName = "kubefwd-proxy"
	}
	if config.ProxyPodImage == "" {
		config.ProxyPodImage = "alpine/socat:latest"
	}
	if config.ProxyPodContext == "" {
		config.ProxyPodContext = config.ClusterContext
	}
	if config.ProxyPodNamespace == "" {
		config.ProxyPodNamespace = config.Namespace
	}

	// Validate configuration
	if config.ClusterContext == "" {
		return nil, fmt.Errorf("cluster_context is required")
	}
	if config.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	if len(config.Services) == 0 && len(config.ProxyServices) == 0 {
		return nil, fmt.Errorf("at least one service or proxy service must be defined")
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
		// Validate sql-tap configuration
		if svc.SqlTapPort != nil {
			if *svc.SqlTapPort <= 0 || *svc.SqlTapPort > 65535 {
				return nil, fmt.Errorf("service %d (%s): invalid sql_tap_port", i, svc.Name)
			}
			if *svc.SqlTapPort == svc.LocalPort {
				return nil, fmt.Errorf("service %d (%s): sql_tap_port cannot be the same as local_port", i, svc.Name)
			}
			if svc.SqlTapDriver == "" {
				return nil, fmt.Errorf("service %d (%s): sql_tap_driver is required when sql_tap_port is set", i, svc.Name)
			}
			if svc.SqlTapDriver != "postgres" && svc.SqlTapDriver != "mysql" {
				return nil, fmt.Errorf("service %d (%s): sql_tap_driver must be 'postgres' or 'mysql'", i, svc.Name)
			}
		}
		// Validate sql-tap gRPC port if specified
		if svc.SqlTapGrpcPort != nil {
			if *svc.SqlTapGrpcPort <= 0 || *svc.SqlTapGrpcPort > 65535 {
				return nil, fmt.Errorf("service %d (%s): invalid sql_tap_grpc_port", i, svc.Name)
			}
		}
	}

	// Validate each proxy service
	for i, pxSvc := range config.ProxyServices {
		if pxSvc.Name == "" {
			return nil, fmt.Errorf("proxy_service %d: name is required", i)
		}
		if pxSvc.TargetHost == "" {
			return nil, fmt.Errorf("proxy_service %d (%s): target_host is required", i, pxSvc.Name)
		}
		if pxSvc.TargetPort <= 0 || pxSvc.TargetPort > 65535 {
			return nil, fmt.Errorf("proxy_service %d (%s): invalid target_port", i, pxSvc.Name)
		}
		if pxSvc.LocalPort <= 0 || pxSvc.LocalPort > 65535 {
			return nil, fmt.Errorf("proxy_service %d (%s): invalid local_port", i, pxSvc.Name)
		}
		// Validate sql-tap configuration
		if pxSvc.SqlTapPort != nil {
			if *pxSvc.SqlTapPort <= 0 || *pxSvc.SqlTapPort > 65535 {
				return nil, fmt.Errorf("proxy_service %d (%s): invalid sql_tap_port", i, pxSvc.Name)
			}
			if *pxSvc.SqlTapPort == pxSvc.LocalPort {
				return nil, fmt.Errorf("proxy_service %d (%s): sql_tap_port cannot be the same as local_port", i, pxSvc.Name)
			}
			if pxSvc.SqlTapDriver == "" {
				return nil, fmt.Errorf("proxy_service %d (%s): sql_tap_driver is required when sql_tap_port is set", i, pxSvc.Name)
			}
			if pxSvc.SqlTapDriver != "postgres" && pxSvc.SqlTapDriver != "mysql" {
				return nil, fmt.Errorf("proxy_service %d (%s): sql_tap_driver must be 'postgres' or 'mysql'", i, pxSvc.Name)
			}
		}
		// Validate sql-tap gRPC port if specified
		if pxSvc.SqlTapGrpcPort != nil {
			if *pxSvc.SqlTapGrpcPort <= 0 || *pxSvc.SqlTapGrpcPort > 65535 {
				return nil, fmt.Errorf("proxy_service %d (%s): invalid sql_tap_grpc_port", i, pxSvc.Name)
			}
		}
	}

	// Sort services alphabetically by name
	sort.Slice(config.Services, func(i, j int) bool {
		return config.Services[i].Name < config.Services[j].Name
	})

	// Sort proxy services alphabetically by name
	sort.Slice(config.ProxyServices, func(i, j int) bool {
		return config.ProxyServices[i].Name < config.ProxyServices[j].Name
	})

	// Auto-assign gRPC ports for sql-tap services
	nextGrpcPort := 9091
	for i := range config.Services {
		if config.Services[i].SqlTapPort != nil {
			if config.Services[i].SqlTapGrpcPort == nil {
				// Auto-assign gRPC port
				config.Services[i].SqlTapGrpcPort = &nextGrpcPort
				nextGrpcPort++
			} else {
				// Track explicitly set ports to ensure next auto-assigned port doesn't conflict
				if *config.Services[i].SqlTapGrpcPort >= nextGrpcPort {
					nextGrpcPort = *config.Services[i].SqlTapGrpcPort + 1
				}
			}
		}
	}
	// Auto-assign gRPC ports for proxy services
	for i := range config.ProxyServices {
		if config.ProxyServices[i].SqlTapPort != nil {
			if config.ProxyServices[i].SqlTapGrpcPort == nil {
				// Auto-assign gRPC port
				config.ProxyServices[i].SqlTapGrpcPort = &nextGrpcPort
				nextGrpcPort++
			} else {
				// Track explicitly set ports to ensure next auto-assigned port doesn't conflict
				if *config.ProxyServices[i].SqlTapGrpcPort >= nextGrpcPort {
					nextGrpcPort = *config.ProxyServices[i].SqlTapGrpcPort + 1
				}
			}
		}
	}

	return &config, nil
}


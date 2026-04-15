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
	ClusterContext      string               `yaml:"cluster_context"`
	ClusterName         string               `yaml:"cluster_name,omitempty"`
	Namespace           string               `yaml:"namespace"`
	MaxRetries          int                  `yaml:"max_retries,omitempty"` // Global default: -1 for infinite, 0 to disable, N for specific limit
	WebPort             int                  `yaml:"web_port,omitempty"`    // Port for the web UI (default: 8765)
	AlternativeContexts []AlternativeContext `yaml:"alternative_contexts,omitempty"`
	Presets             []Preset             `yaml:"presets,omitempty"`
	Services            []Service            `yaml:"services"`
	ProxyPodName        string               `yaml:"proxy_pod_name,omitempty"`      // Name for the shared proxy pod (default: kubefwd-proxy)
	ProxyPodImage       string               `yaml:"proxy_pod_image,omitempty"`     // Container image for proxy pod (default: alpine/socat:latest)
	ProxyPodContext     string               `yaml:"proxy_pod_context,omitempty"`     // Context where proxy pod is created (default: cluster_context)
	ProxyPodNamespace   string               `yaml:"proxy_pod_namespace,omitempty"` // Namespace where proxy pod is created (default: namespace)
	ProxyServices       []ProxyService       `yaml:"proxy_services,omitempty"`      // Proxy services for GCP connections
}

// Service represents a single service configuration
type Service struct {
	Name              string `yaml:"name" json:"name"`
	ServiceName       string `yaml:"service_name" json:"service_name"`
	RemotePort        int    `yaml:"remote_port" json:"remote_port"`
	LocalPort         int    `yaml:"local_port" json:"local_port"`
	SelectedByDefault bool   `yaml:"selected_by_default" json:"selected_by_default"`
	Context           string `yaml:"context,omitempty" json:"context,omitempty"`
	Namespace         string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	MaxRetries        *int   `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`
	SqlTapPort        *int   `yaml:"sql_tap_port,omitempty" json:"sql_tap_port,omitempty"`
	SqlTapDriver      string `yaml:"sql_tap_driver,omitempty" json:"sql_tap_driver,omitempty"`
	SqlTapGrpcPort    *int   `yaml:"sql_tap_grpc_port,omitempty" json:"sql_tap_grpc_port,omitempty"`
	SqlTapHttpPort    *int   `yaml:"sql_tap_http_port,omitempty" json:"sql_tap_http_port,omitempty"`
}

// ProxyService represents a proxy pod service configuration
type ProxyService struct {
	Name              string `yaml:"name" json:"name"`
	TargetHost        string `yaml:"target_host" json:"target_host"`
	TargetPort        int    `yaml:"target_port" json:"target_port"`
	LocalPort         int    `yaml:"local_port" json:"local_port"`
	SelectedByDefault bool   `yaml:"selected_by_default" json:"selected_by_default"`
	ProxyPodContext   string `yaml:"proxy_pod_context" json:"proxy_pod_context"`
	ProxyPodNamespace string `yaml:"proxy_pod_namespace" json:"proxy_pod_namespace"`
	MaxRetries        *int   `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`
	SqlTapPort        *int   `yaml:"sql_tap_port,omitempty" json:"sql_tap_port,omitempty"`
	SqlTapDriver      string `yaml:"sql_tap_driver,omitempty" json:"sql_tap_driver,omitempty"`
	SqlTapGrpcPort    *int   `yaml:"sql_tap_grpc_port,omitempty" json:"sql_tap_grpc_port,omitempty"`
	SqlTapHttpPort    *int   `yaml:"sql_tap_http_port,omitempty" json:"sql_tap_http_port,omitempty"`
}

// GetMaxRetries returns the service-specific max retries or falls back to global max retries
func (ps *ProxyService) GetMaxRetries(globalMaxRetries int) int {
	if ps.MaxRetries != nil {
		return *ps.MaxRetries
	}
	return globalMaxRetries
}

// ProxyGroupKey returns the unique key for the context+namespace group this service belongs to
func (ps *ProxyService) ProxyGroupKey() string {
	return ps.ProxyPodContext + "/" + ps.ProxyPodNamespace
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

// ApplyConfigDefaults sets default values for unset fields (before validation).
func ApplyConfigDefaults(cfg *Config) {
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = -1
	}
	if cfg.WebPort == 0 {
		cfg.WebPort = 8765
	}
	if cfg.ProxyPodName == "" {
		cfg.ProxyPodName = "kubefwd-proxy"
	}
	if cfg.ProxyPodImage == "" {
		cfg.ProxyPodImage = "alpine/socat:latest"
	}
	if cfg.ProxyPodContext == "" {
		cfg.ProxyPodContext = cfg.ClusterContext
	}
	if cfg.ProxyPodNamespace == "" {
		cfg.ProxyPodNamespace = cfg.Namespace
	}
	for i := range cfg.ProxyServices {
		if cfg.ProxyServices[i].ProxyPodContext == "" {
			cfg.ProxyServices[i].ProxyPodContext = cfg.ProxyPodContext
		}
		if cfg.ProxyServices[i].ProxyPodNamespace == "" {
			cfg.ProxyServices[i].ProxyPodNamespace = cfg.ProxyPodNamespace
		}
	}
}

// ValidateConfig checks required fields and service definitions.
func ValidateConfig(cfg *Config) error {
	if cfg.ClusterContext == "" {
		return fmt.Errorf("cluster_context is required")
	}
	if cfg.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if len(cfg.Services) == 0 && len(cfg.ProxyServices) == 0 {
		return fmt.Errorf("at least one service or proxy service must be defined")
	}

	for i, svc := range cfg.Services {
		if svc.Name == "" {
			return fmt.Errorf("service %d: name is required", i)
		}
		if svc.ServiceName == "" {
			return fmt.Errorf("service %d (%s): service_name is required", i, svc.Name)
		}
		if svc.RemotePort <= 0 || svc.RemotePort > 65535 {
			return fmt.Errorf("service %d (%s): invalid remote_port", i, svc.Name)
		}
		if svc.LocalPort <= 0 || svc.LocalPort > 65535 {
			return fmt.Errorf("service %d (%s): invalid local_port", i, svc.Name)
		}
		if svc.SqlTapPort != nil {
			if *svc.SqlTapPort <= 0 || *svc.SqlTapPort > 65535 {
				return fmt.Errorf("service %d (%s): invalid sql_tap_port", i, svc.Name)
			}
			if *svc.SqlTapPort == svc.LocalPort {
				return fmt.Errorf("service %d (%s): sql_tap_port cannot be the same as local_port", i, svc.Name)
			}
			if svc.SqlTapDriver == "" {
				return fmt.Errorf("service %d (%s): sql_tap_driver is required when sql_tap_port is set", i, svc.Name)
			}
			if svc.SqlTapDriver != "postgres" && svc.SqlTapDriver != "mysql" {
				return fmt.Errorf("service %d (%s): sql_tap_driver must be 'postgres' or 'mysql'", i, svc.Name)
			}
		}
		if svc.SqlTapGrpcPort != nil {
			if *svc.SqlTapGrpcPort <= 0 || *svc.SqlTapGrpcPort > 65535 {
				return fmt.Errorf("service %d (%s): invalid sql_tap_grpc_port", i, svc.Name)
			}
		}
		if svc.SqlTapHttpPort != nil {
			if *svc.SqlTapHttpPort <= 0 || *svc.SqlTapHttpPort > 65535 {
				return fmt.Errorf("service %d (%s): invalid sql_tap_http_port", i, svc.Name)
			}
			if *svc.SqlTapHttpPort == svc.LocalPort {
				return fmt.Errorf("service %d (%s): sql_tap_http_port cannot be the same as local_port", i, svc.Name)
			}
			if svc.SqlTapPort != nil && *svc.SqlTapHttpPort == *svc.SqlTapPort {
				return fmt.Errorf("service %d (%s): sql_tap_http_port cannot be the same as sql_tap_port", i, svc.Name)
			}
		}
	}

	for i, pxSvc := range cfg.ProxyServices {
		if pxSvc.Name == "" {
			return fmt.Errorf("proxy_service %d: name is required", i)
		}
		if pxSvc.TargetHost == "" {
			return fmt.Errorf("proxy_service %d (%s): target_host is required", i, pxSvc.Name)
		}
		if pxSvc.TargetPort <= 0 || pxSvc.TargetPort > 65535 {
			return fmt.Errorf("proxy_service %d (%s): invalid target_port", i, pxSvc.Name)
		}
		if pxSvc.LocalPort <= 0 || pxSvc.LocalPort > 65535 {
			return fmt.Errorf("proxy_service %d (%s): invalid local_port", i, pxSvc.Name)
		}
		if pxSvc.ProxyPodContext == "" {
			return fmt.Errorf("proxy_service %d (%s): proxy_pod_context is required", i, pxSvc.Name)
		}
		if pxSvc.ProxyPodNamespace == "" {
			return fmt.Errorf("proxy_service %d (%s): proxy_pod_namespace is required", i, pxSvc.Name)
		}
		if pxSvc.SqlTapPort != nil {
			if *pxSvc.SqlTapPort <= 0 || *pxSvc.SqlTapPort > 65535 {
				return fmt.Errorf("proxy_service %d (%s): invalid sql_tap_port", i, pxSvc.Name)
			}
			if *pxSvc.SqlTapPort == pxSvc.LocalPort {
				return fmt.Errorf("proxy_service %d (%s): sql_tap_port cannot be the same as local_port", i, pxSvc.Name)
			}
			if pxSvc.SqlTapDriver == "" {
				return fmt.Errorf("proxy_service %d (%s): sql_tap_driver is required when sql_tap_port is set", i, pxSvc.Name)
			}
			if pxSvc.SqlTapDriver != "postgres" && pxSvc.SqlTapDriver != "mysql" {
				return fmt.Errorf("proxy_service %d (%s): sql_tap_driver must be 'postgres' or 'mysql'", i, pxSvc.Name)
			}
		}
		if pxSvc.SqlTapGrpcPort != nil {
			if *pxSvc.SqlTapGrpcPort <= 0 || *pxSvc.SqlTapGrpcPort > 65535 {
				return fmt.Errorf("proxy_service %d (%s): invalid sql_tap_grpc_port", i, pxSvc.Name)
			}
		}
		if pxSvc.SqlTapHttpPort != nil {
			if *pxSvc.SqlTapHttpPort <= 0 || *pxSvc.SqlTapHttpPort > 65535 {
				return fmt.Errorf("proxy_service %d (%s): invalid sql_tap_http_port", i, pxSvc.Name)
			}
			if *pxSvc.SqlTapHttpPort == pxSvc.LocalPort {
				return fmt.Errorf("proxy_service %d (%s): sql_tap_http_port cannot be the same as local_port", i, pxSvc.Name)
			}
			if pxSvc.SqlTapPort != nil && *pxSvc.SqlTapHttpPort == *pxSvc.SqlTapPort {
				return fmt.Errorf("proxy_service %d (%s): sql_tap_http_port cannot be the same as sql_tap_port", i, pxSvc.Name)
			}
		}
	}

	return nil
}

// FinalizeConfig sorts services and assigns sql-tap gRPC ports after validation.
func FinalizeConfig(cfg *Config) {
	sort.Slice(cfg.Services, func(i, j int) bool {
		return cfg.Services[i].Name < cfg.Services[j].Name
	})
	sort.Slice(cfg.ProxyServices, func(i, j int) bool {
		ki := cfg.ProxyServices[i].ProxyGroupKey()
		kj := cfg.ProxyServices[j].ProxyGroupKey()
		if ki != kj {
			return ki < kj
		}
		return cfg.ProxyServices[i].Name < cfg.ProxyServices[j].Name
	})

	nextGrpcPort := 9091
	for i := range cfg.Services {
		if cfg.Services[i].SqlTapPort != nil {
			if cfg.Services[i].SqlTapGrpcPort == nil {
				p := nextGrpcPort
				cfg.Services[i].SqlTapGrpcPort = &p
				nextGrpcPort++
			} else {
				if *cfg.Services[i].SqlTapGrpcPort >= nextGrpcPort {
					nextGrpcPort = *cfg.Services[i].SqlTapGrpcPort + 1
				}
			}
		}
	}
	for i := range cfg.ProxyServices {
		if cfg.ProxyServices[i].SqlTapPort != nil {
			if cfg.ProxyServices[i].SqlTapGrpcPort == nil {
				p := nextGrpcPort
				cfg.ProxyServices[i].SqlTapGrpcPort = &p
				nextGrpcPort++
			} else {
				if *cfg.ProxyServices[i].SqlTapGrpcPort >= nextGrpcPort {
					nextGrpcPort = *cfg.ProxyServices[i].SqlTapGrpcPort + 1
				}
			}
		}
	}
}

// ParseConfigYAML parses YAML bytes into a normalized, validated Config.
func ParseConfigYAML(data []byte) (*Config, error) {
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	ApplyConfigDefaults(&config)
	if err := ValidateConfig(&config); err != nil {
		return nil, err
	}
	FinalizeConfig(&config)
	return &config, nil
}

// LoadConfig reads and parses the YAML configuration file
func LoadConfig(filepath string) (*Config, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	cfg, err := ParseConfigYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	return cfg, nil
}

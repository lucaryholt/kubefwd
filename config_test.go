package main

import "testing"

func TestParseConfigYAMLMinimal(t *testing.T) {
	y := `
cluster_context: ctx1
namespace: default
services:
  - name: A
    service_name: svc-a
    remote_port: 80
    local_port: 8080
    selected_by_default: false
`
	cfg, err := ParseConfigYAML([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClusterContext != "ctx1" || cfg.Namespace != "default" {
		t.Fatalf("globals: %+v", cfg)
	}
	if len(cfg.Services) != 1 || cfg.Services[0].Name != "A" {
		t.Fatalf("services: %+v", cfg.Services)
	}
}

func TestValidateConfigRequiresService(t *testing.T) {
	cfg := &Config{ClusterContext: "c", Namespace: "n", MaxRetries: -1, WebPort: 8765}
	ApplyConfigDefaults(cfg)
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error with no services")
	}
}

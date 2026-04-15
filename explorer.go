package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const explorerTimeout = 10 * time.Second

// K8s discovery types

type K8sServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int    `json:"port"`
	TargetPort any    `json:"target_port"` // can be int or string
	Protocol   string `json:"protocol"`
}

type K8sServiceInfo struct {
	Name      string           `json:"name"`
	Namespace string           `json:"namespace"`
	Type      string           `json:"type"`
	ClusterIP string           `json:"cluster_ip"`
	Ports     []K8sServicePort `json:"ports"`
	InConfig  bool             `json:"in_config"`
}

// GCP discovery types

type CloudSQLInstance struct {
	Name      string `json:"name"`
	Project   string `json:"project"`
	PrivateIP string `json:"private_ip"`
	PublicIP  string `json:"public_ip,omitempty"`
	Region    string `json:"region"`
	DBVersion string `json:"db_version"`
	InConfig  bool   `json:"in_config"`
}

type MemorystoreInstance struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Region   string `json:"region"`
	Tier     string `json:"tier"`
	Version  string `json:"version"`
	InConfig bool   `json:"in_config"`
}

type GCPProject struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
}

type GCPDiscoveryResult struct {
	Available   bool                  `json:"available"`
	Project     string                `json:"project,omitempty"`
	CloudSQL    []CloudSQLInstance    `json:"cloudsql,omitempty"`
	Memorystore []MemorystoreInstance `json:"memorystore,omitempty"`
	Error       string                `json:"error,omitempty"`
}

// Explorer holds cached state for gcloud availability checks.
type Explorer struct {
	gcloudOnce      sync.Once
	gcloudAvailable bool
}

func NewExplorer() *Explorer {
	return &Explorer{}
}

func (e *Explorer) DiscoverContexts() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), explorerTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "config", "get-contexts", "-o", "name")
	out, err := debugRunCmd(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list contexts: %w", err)
	}

	var contexts []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			contexts = append(contexts, line)
		}
	}
	return contexts, nil
}

func (e *Explorer) DiscoverNamespaces(kubeCtx string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), explorerTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "get", "namespaces",
		"--context", kubeCtx,
		"-o", "jsonpath={.items[*].metadata.name}")
	out, err := debugRunCmd(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	var namespaces []string
	for _, ns := range strings.Fields(strings.TrimSpace(string(out))) {
		if ns != "" {
			namespaces = append(namespaces, ns)
		}
	}
	return namespaces, nil
}

func (e *Explorer) DiscoverServices(kubeCtx, namespace string, config *Config) ([]K8sServiceInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), explorerTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "get", "services",
		"--context", kubeCtx,
		"-n", namespace,
		"-o", "json")
	out, err := debugRunCmd(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	var result struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				Type      string `json:"type"`
				ClusterIP string `json:"clusterIP"`
				Ports     []struct {
					Name       string `json:"name"`
					Port       int    `json:"port"`
					TargetPort any    `json:"targetPort"`
					Protocol   string `json:"protocol"`
				} `json:"ports"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("failed to parse services JSON: %w", err)
	}

	configSvcNames := make(map[string]bool)
	if config != nil {
		for _, s := range config.Services {
			configSvcNames[s.ServiceName] = true
		}
	}

	var services []K8sServiceInfo
	for _, item := range result.Items {
		if item.Spec.Type == "ExternalName" {
			continue
		}

		var ports []K8sServicePort
		for _, p := range item.Spec.Ports {
			ports = append(ports, K8sServicePort{
				Name:       p.Name,
				Port:       p.Port,
				TargetPort: p.TargetPort,
				Protocol:   p.Protocol,
			})
		}

		services = append(services, K8sServiceInfo{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			Type:      item.Spec.Type,
			ClusterIP: item.Spec.ClusterIP,
			Ports:     ports,
			InConfig:  configSvcNames[item.Metadata.Name],
		})
	}
	return services, nil
}

func (e *Explorer) isGcloudAvailable() bool {
	e.gcloudOnce.Do(func() {
		cmd := exec.Command("gcloud", "version")
		_, err := cmd.CombinedOutput()
		e.gcloudAvailable = err == nil
		if e.gcloudAvailable {
			debugLog("gcloud CLI detected")
		} else {
			debugLog("gcloud CLI not available: %v", err)
		}
	})
	return e.gcloudAvailable
}

func (e *Explorer) getGCPProject() string {
	ctx, cancel := context.WithTimeout(context.Background(), explorerTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gcloud", "config", "get-value", "project")
	out, err := debugRunCmd(cmd)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (e *Explorer) DiscoverGCPProjects() ([]GCPProject, string, error) {
	if !e.isGcloudAvailable() {
		return nil, "", fmt.Errorf("gcloud CLI not available")
	}

	activeProject := e.getGCPProject()

	ctx, cancel := context.WithTimeout(context.Background(), explorerTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gcloud", "projects", "list", "--format=json(projectId,name)", "--sort-by=name")
	out, err := debugRunCmd(cmd)
	if err != nil {
		return nil, activeProject, fmt.Errorf("gcloud projects list failed: %w", err)
	}

	var raw []struct {
		ProjectID string `json:"projectId"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, activeProject, fmt.Errorf("failed to parse projects JSON: %w", err)
	}

	projects := make([]GCPProject, len(raw))
	for i, r := range raw {
		projects[i] = GCPProject{ProjectID: r.ProjectID, Name: r.Name}
	}
	return projects, activeProject, nil
}

func (e *Explorer) DiscoverGCP(project string, config *Config) GCPDiscoveryResult {
	if !e.isGcloudAvailable() {
		return GCPDiscoveryResult{Available: false}
	}

	if project == "" {
		project = e.getGCPProject()
	}

	proxyHosts := make(map[string]bool)
	if config != nil {
		for _, ps := range config.ProxyServices {
			proxyHosts[ps.TargetHost] = true
		}
	}

	var wg sync.WaitGroup
	var sqlInstances []CloudSQLInstance
	var redisInstances []MemorystoreInstance
	var sqlErr, redisErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		sqlInstances, sqlErr = e.discoverCloudSQL(project, proxyHosts)
	}()
	go func() {
		defer wg.Done()
		redisInstances, redisErr = e.discoverMemorystore(project, proxyHosts)
	}()
	wg.Wait()

	result := GCPDiscoveryResult{
		Available:   true,
		Project:     project,
		CloudSQL:    sqlInstances,
		Memorystore: redisInstances,
	}

	var errs []string
	if sqlErr != nil {
		errs = append(errs, "Cloud SQL: "+sqlErr.Error())
	}
	if redisErr != nil {
		errs = append(errs, "Memorystore: "+redisErr.Error())
	}
	if len(errs) > 0 {
		result.Error = strings.Join(errs, "; ")
	}

	return result
}

func (e *Explorer) discoverCloudSQL(project string, proxyHosts map[string]bool) ([]CloudSQLInstance, error) {
	ctx, cancel := context.WithTimeout(context.Background(), explorerTimeout)
	defer cancel()

	args := []string{"sql", "instances", "list", "--format=json"}
	if project != "" {
		args = append(args, "--project="+project)
	}
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	out, err := debugRunCmd(cmd)
	if err != nil {
		return nil, fmt.Errorf("gcloud sql instances list failed: %w", err)
	}

	var raw []struct {
		Name          string `json:"name"`
		Project       string `json:"project"`
		Region        string `json:"region"`
		DatabaseVersion string `json:"databaseVersion"`
		IPAddresses   []struct {
			Type      string `json:"type"`
			IPAddress string `json:"ipAddress"`
		} `json:"ipAddresses"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse Cloud SQL JSON: %w", err)
	}

	var instances []CloudSQLInstance
	for _, r := range raw {
		inst := CloudSQLInstance{
			Name:      r.Name,
			Project:   r.Project,
			Region:    r.Region,
			DBVersion: r.DatabaseVersion,
		}
		if inst.Project == "" {
			inst.Project = project
		}
		for _, ip := range r.IPAddresses {
			switch ip.Type {
			case "PRIVATE":
				inst.PrivateIP = ip.IPAddress
			case "PRIMARY":
				inst.PublicIP = ip.IPAddress
			}
		}
		if inst.PrivateIP != "" {
			inst.InConfig = proxyHosts[inst.PrivateIP]
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

func (e *Explorer) discoverMemorystore(project string, proxyHosts map[string]bool) ([]MemorystoreInstance, error) {
	ctx, cancel := context.WithTimeout(context.Background(), explorerTimeout)
	defer cancel()

	args := []string{"redis", "instances", "list", "--region=-", "--format=json"}
	if project != "" {
		args = append(args, "--project="+project)
	}
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	out, err := debugRunCmd(cmd)
	if err != nil {
		return nil, fmt.Errorf("gcloud redis instances list failed: %w", err)
	}

	var raw []struct {
		Name           string `json:"name"`
		Host           string `json:"host"`
		Port           int    `json:"port"`
		LocationID     string `json:"locationId"`
		Tier           string `json:"tier"`
		RedisVersion   string `json:"redisVersion"`
		CurrentLocationID string `json:"currentLocationId"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse Memorystore JSON: %w", err)
	}

	var instances []MemorystoreInstance
	for _, r := range raw {
		region := r.LocationID
		if region == "" {
			region = r.CurrentLocationID
		}
		// Extract short name from full resource path
		name := r.Name
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}

		inst := MemorystoreInstance{
			Name:    name,
			Host:    r.Host,
			Port:    r.Port,
			Region:  region,
			Tier:    r.Tier,
			Version: r.RedisVersion,
		}
		if inst.Host != "" {
			inst.InConfig = proxyHosts[inst.Host]
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

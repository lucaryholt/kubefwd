package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/index.html
var webFS embed.FS

// WebApp is the central state holder for the web-based UI.
type WebApp struct {
	config           *Config
	store            ConfigStore
	portForwards     []*PortForward
	proxyForwards    map[string]*ProxyForward
	proxyPodManagers map[string]*ProxyPodManager // keyed by "context/namespace"
	mu               sync.RWMutex

	// SSE clients
	sseClients map[chan string]struct{}
	sseMu      sync.Mutex
}

// proxyGroupKey returns the map key for a context+namespace pair.
func proxyGroupKey(ctx, ns string) string { return ctx + "/" + ns }

// buildProxyPodManagers creates one ProxyPodManager per unique (context, namespace)
// group found in the proxy services list.
func buildProxyPodManagers(config *Config) map[string]*ProxyPodManager {
	managers := make(map[string]*ProxyPodManager)
	for _, ps := range config.ProxyServices {
		key := ps.ProxyGroupKey()
		if _, exists := managers[key]; !exists {
			podName := BuildPodName(config.ProxyPodName, ps.ProxyPodContext, ps.ProxyPodNamespace)
			managers[key] = NewProxyPodManager(
				podName,
				config.ProxyPodImage,
				ps.ProxyPodNamespace,
				ps.ProxyPodContext,
			)
		}
	}
	return managers
}

// NewWebApp creates and initialises a WebApp from the given config.
func NewWebApp(config *Config, store ConfigStore) *WebApp {
	pfs := make([]*PortForward, len(config.Services))
	for i, svc := range config.Services {
		pfs[i] = NewPortForward(svc, config.ClusterContext, config.Namespace, config.MaxRetries)
	}

	managers := buildProxyPodManagers(config)

	return &WebApp{
		config:           config,
		store:            store,
		portForwards:     pfs,
		proxyPodManagers: managers,
		proxyForwards:    make(map[string]*ProxyForward),
		sseClients:       make(map[chan string]struct{}),
	}
}

func (wa *WebApp) currentConfigClone() *Config {
	wa.mu.RLock()
	defer wa.mu.RUnlock()
	return cloneConfig(wa.config)
}

// reapplyConfig stops all forwards and rebuilds runtime state from cfg (already normalized).
func (wa *WebApp) reapplyConfig(cfg *Config) {
	wa.StopAll()
	wa.mu.Lock()
	defer wa.mu.Unlock()
	wa.config = cfg
	wa.portForwards = make([]*PortForward, len(cfg.Services))
	for i := range cfg.Services {
		wa.portForwards[i] = NewPortForward(cfg.Services[i], cfg.ClusterContext, cfg.Namespace, cfg.MaxRetries)
	}
	wa.proxyPodManagers = buildProxyPodManagers(cfg)
	wa.proxyForwards = make(map[string]*ProxyForward)
}

// StartDefaults starts all services marked selected_by_default.
func (wa *WebApp) StartDefaults() {
	for _, pf := range wa.portForwards {
		if pf.Service.SelectedByDefault {
			_ = pf.Start()
		}
	}
}

// StartDefaultProxies starts proxy services marked selected_by_default.
func (wa *WebApp) StartDefaultProxies() {
	// Group default services by context+namespace
	groups := make(map[string][]ProxyService)
	for _, ps := range wa.config.ProxyServices {
		if ps.SelectedByDefault {
			key := ps.ProxyGroupKey()
			groups[key] = append(groups[key], ps)
		}
	}
	if len(groups) == 0 {
		return
	}
	wa.mu.Lock()
	defer wa.mu.Unlock()
	for key, svcs := range groups {
		mgr, ok := wa.proxyPodManagers[key]
		if !ok {
			continue
		}
		if err := mgr.CreatePodWithServices(svcs); err != nil {
			continue
		}
		for _, ps := range svcs {
			pxf := NewProxyForward(ps, mgr)
			_ = pxf.Start()
			wa.proxyForwards[ps.Name] = pxf
		}
	}
}

// StopAll stops every running forward and deletes all proxy pods.
func (wa *WebApp) StopAll() {
	for _, pf := range wa.portForwards {
		if pf.IsRunning() {
			_ = pf.Stop()
		}
	}
	wa.mu.Lock()
	defer wa.mu.Unlock()
	for _, pxf := range wa.proxyForwards {
		pxf.Stop()
	}
	for _, mgr := range wa.proxyPodManagers {
		mgr.DeletePod()
	}
}

// --- SSE helpers ---

func (wa *WebApp) addSSEClient(ch chan string) {
	wa.sseMu.Lock()
	wa.sseClients[ch] = struct{}{}
	wa.sseMu.Unlock()
}

func (wa *WebApp) removeSSEClient(ch chan string) {
	wa.sseMu.Lock()
	delete(wa.sseClients, ch)
	wa.sseMu.Unlock()
}

func (wa *WebApp) broadcastState() {
	data := wa.buildStateJSON()
	msg := "data: " + data + "\n\n"
	wa.sseMu.Lock()
	for ch := range wa.sseClients {
		select {
		case ch <- msg:
		default:
		}
	}
	wa.sseMu.Unlock()
}

// startSSEBroadcaster pushes state to all SSE clients every 500 ms.
func (wa *WebApp) startSSEBroadcaster(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			wa.broadcastState()
		}
	}
}

// --- JSON state helpers ---

type serviceStateJSON struct {
	Name              string `json:"name"`
	LocalPort         int    `json:"local_port"`
	RemotePort        int    `json:"remote_port"`
	Status            string `json:"status"`
	Error             string `json:"error,omitempty"`
	Retrying          bool   `json:"retrying"`
	RetryAttempt      int    `json:"retry_attempt"`
	MaxRetries        int    `json:"max_retries"`
	IsDefault         bool   `json:"is_default"`
	HasSqlTap         bool   `json:"has_sql_tap"`
	SqlTapPort        int    `json:"sql_tap_port,omitempty"`
	SqlTapGrpcPort    int    `json:"sql_tap_grpc_port,omitempty"`
	SqlTapHttpPort    int    `json:"sql_tap_http_port,omitempty"`
}

type proxyServiceStateJSON struct {
	Name              string `json:"name"`
	LocalPort         int    `json:"local_port"`
	Status            string `json:"status"`
	Error             string `json:"error,omitempty"`
	IsDefault         bool   `json:"is_default"`
	Active            bool   `json:"active"`
	ProxyPodContext   string `json:"proxy_pod_context"`
	ProxyPodNamespace string `json:"proxy_pod_namespace"`
	HasSqlTap         bool   `json:"has_sql_tap"`
	SqlTapPort        int    `json:"sql_tap_port,omitempty"`
	SqlTapGrpcPort    int    `json:"sql_tap_grpc_port,omitempty"`
	SqlTapHttpPort    int    `json:"sql_tap_http_port,omitempty"`
}

type proxyGroupStateJSON struct {
	GroupKey  string                  `json:"group_key"`
	Context   string                  `json:"context"`
	Namespace string                  `json:"namespace"`
	PodStatus string                  `json:"pod_status"`
	PodError  string                  `json:"pod_error,omitempty"`
	Services  []proxyServiceStateJSON `json:"services"`
}

type stateJSON struct {
	ClusterContext   string                `json:"cluster_context"`
	ClusterName      string                `json:"cluster_name"`
	Namespace        string                `json:"namespace"`
	ConfigSource     string                `json:"config_source"`
	ConfigFile       string                `json:"config_file"` // same as config_source; kept for older UI
	Services         []serviceStateJSON    `json:"services"`
	ProxyGroups      []proxyGroupStateJSON `json:"proxy_groups"`
	Presets          []Preset              `json:"presets"`
	Contexts         []AlternativeContext  `json:"contexts"`
	HasProxyServices bool                  `json:"has_proxy_services"`
	DebugMode        bool                  `json:"debug_mode"`
	DebugLines       []string              `json:"debug_lines"`
}

func (wa *WebApp) buildStateJSON() string {
	wa.mu.RLock()
	defer wa.mu.RUnlock()

	services := make([]serviceStateJSON, len(wa.portForwards))
	for i, pf := range wa.portForwards {
		status, errMsg := pf.GetStatus()
		retrying, attempt, maxR := pf.GetRetryInfo()
		s := serviceStateJSON{
			Name:         pf.Service.Name,
			LocalPort:    pf.Service.LocalPort,
			RemotePort:   pf.Service.RemotePort,
			Status:       string(status),
			Error:        errMsg,
			Retrying:     retrying,
			RetryAttempt: attempt,
			MaxRetries:   maxR,
			IsDefault:    pf.Service.SelectedByDefault,
			HasSqlTap:    pf.Service.SqlTapPort != nil,
		}
		if pf.Service.SqlTapPort != nil {
			s.SqlTapPort = *pf.Service.SqlTapPort
		}
		if pf.Service.SqlTapGrpcPort != nil {
			s.SqlTapGrpcPort = *pf.Service.SqlTapGrpcPort
		}
		if pf.Service.SqlTapHttpPort != nil {
			s.SqlTapHttpPort = *pf.Service.SqlTapHttpPort
		}
		services[i] = s
	}

	// Build proxy groups: one entry per unique (context, namespace), preserving sorted order
	groupOrder := []string{}
	groupSeen := make(map[string]bool)
	for _, ps := range wa.config.ProxyServices {
		key := ps.ProxyGroupKey()
		if !groupSeen[key] {
			groupSeen[key] = true
			groupOrder = append(groupOrder, key)
		}
	}

	proxyGroups := make([]proxyGroupStateJSON, 0, len(groupOrder))
	for _, key := range groupOrder {
		mgr := wa.proxyPodManagers[key]
		podStatus := string(ProxyPodStatusNotCreated)
		podError := ""
		if mgr != nil {
			st, e, _ := mgr.GetStatus()
			podStatus = string(st)
			podError = e
		}

		var groupSvcs []proxyServiceStateJSON
		for _, ps := range wa.config.ProxyServices {
			if ps.ProxyGroupKey() != key {
				continue
			}
			status := string(StatusStopped)
			errMsg := ""
			if pxf, ok := wa.proxyForwards[ps.Name]; ok {
				st, e := pxf.GetStatus()
				status = string(st)
				errMsg = e
			}
			_, active := wa.proxyForwards[ps.Name]
			entry := proxyServiceStateJSON{
				Name:              ps.Name,
				LocalPort:         ps.LocalPort,
				Status:            status,
				Error:             errMsg,
				IsDefault:         ps.SelectedByDefault,
				Active:            active,
				ProxyPodContext:   ps.ProxyPodContext,
				ProxyPodNamespace: ps.ProxyPodNamespace,
				HasSqlTap:         ps.SqlTapPort != nil,
			}
			if ps.SqlTapPort != nil {
				entry.SqlTapPort = *ps.SqlTapPort
			}
			if ps.SqlTapGrpcPort != nil {
				entry.SqlTapGrpcPort = *ps.SqlTapGrpcPort
			}
			if ps.SqlTapHttpPort != nil {
				entry.SqlTapHttpPort = *ps.SqlTapHttpPort
			}
			groupSvcs = append(groupSvcs, entry)
		}

		// Split key back into context and namespace
		ctx, ns := splitGroupKey(key)
		proxyGroups = append(proxyGroups, proxyGroupStateJSON{
			GroupKey:  key,
			Context:   ctx,
			Namespace: ns,
			PodStatus: podStatus,
			PodError:  podError,
			Services:  groupSvcs,
		})
	}

	src := wa.store.Description()
	state := stateJSON{
		ClusterContext:   wa.config.ClusterContext,
		ClusterName:      wa.config.ClusterName,
		Namespace:        wa.config.Namespace,
		ConfigSource:     src,
		ConfigFile:       src,
		Services:         services,
		ProxyGroups:      proxyGroups,
		Presets:          wa.config.Presets,
		Contexts:         wa.config.AlternativeContexts,
		HasProxyServices: len(wa.config.ProxyServices) > 0,
		DebugMode:        debugMode,
		DebugLines:       getDebugLines(),
	}

	b, _ := json.Marshal(state)
	return string(b)
}

// splitGroupKey splits a "context/namespace" key back into its components.
// It handles contexts that may themselves contain slashes by splitting on the last slash.
func splitGroupKey(key string) (ctx, ns string) {
	// The namespace cannot contain slashes, so split on last '/'
	idx := strings.LastIndex(key, "/")
	if idx < 0 {
		return key, ""
	}
	return key[:idx], key[idx+1:]
}

// --- HTTP server ---

func (wa *WebApp) ListenAndServe(port int) error {
	mux := http.NewServeMux()

	// Static UI
	mux.HandleFunc("GET /", wa.handleIndex)

	// SSE stream
	mux.HandleFunc("GET /api/state", wa.handleSSE)

	// Services
	mux.HandleFunc("GET /api/services", wa.handleGetServices)
	mux.HandleFunc("POST /api/services/start-all", wa.handleStartAll)
	mux.HandleFunc("POST /api/services/stop-all", wa.handleStopAll)
	mux.HandleFunc("POST /api/services/start-defaults", wa.handleStartDefaults)
	mux.HandleFunc("POST /api/services/{name}/start", wa.handleServiceStart)
	mux.HandleFunc("POST /api/services/{name}/stop", wa.handleServiceStop)

	// Proxy services
	mux.HandleFunc("GET /api/proxy-services", wa.handleGetProxyServices)
	mux.HandleFunc("POST /api/proxy-services/start-pod", wa.handleStartProxyPod)
	mux.HandleFunc("POST /api/proxy-services/start-defaults", wa.handleStartDefaultProxies)
	mux.HandleFunc("POST /api/proxy-services/{name}/start", wa.handleStartProxyService)
	mux.HandleFunc("POST /api/proxy-services/{name}/stop", wa.handleStopProxyService)
	mux.HandleFunc("POST /api/proxy-services/reset", wa.handleResetProxyPod)
	mux.HandleFunc("POST /api/proxy-services/kill-pod", wa.handleKillProxyPod)

	// Presets
	mux.HandleFunc("GET /api/presets", wa.handleGetPresets)
	mux.HandleFunc("POST /api/presets/{name}/apply", wa.handleApplyPreset)

	// Contexts
	mux.HandleFunc("GET /api/contexts", wa.handleGetContexts)
	mux.HandleFunc("POST /api/contexts/switch", wa.handleSwitchContext)

	// Port checker
	mux.HandleFunc("GET /api/ports", wa.handleGetPorts)
	mux.HandleFunc("POST /api/ports/{port}/kill", wa.handleKillPort)

	// SQL Tap
	mux.HandleFunc("POST /api/sqltap/{name}/launch", wa.handleLaunchSqlTap)

	// Config
	mux.HandleFunc("POST /api/config/reload", wa.handleConfigReload)
	mux.HandleFunc("POST /api/config/import-yaml", wa.handleConfigImportYAML)
	mux.HandleFunc("POST /api/config/services", wa.handlePostConfigService)
	mux.HandleFunc("DELETE /api/config/services/{name}", wa.handleDeleteConfigService)
	mux.HandleFunc("POST /api/config/proxy-services", wa.handlePostConfigProxyService)
	mux.HandleFunc("DELETE /api/config/proxy-services/{name}", wa.handleDeleteConfigProxyService)

	addr := fmt.Sprintf(":%d", port)
	return http.ListenAndServe(addr, mux)
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// handleIndex serves the embedded index.html.
func (wa *WebApp) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "UI not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// handleSSE streams state updates as Server-Sent Events.
func (wa *WebApp) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan string, 8)
	wa.addSSEClient(ch)
	defer wa.removeSSEClient(ch)

	// Send initial state immediately
	fmt.Fprintf(w, "data: %s\n\n", wa.buildStateJSON())
	flusher.Flush()

	for {
		select {
		case msg := <-ch:
			fmt.Fprint(w, msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// handleGetServices returns the current service list with statuses.
func (wa *WebApp) handleGetServices(w http.ResponseWriter, r *http.Request) {
	wa.mu.RLock()
	defer wa.mu.RUnlock()
	type item struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	result := make([]item, len(wa.portForwards))
	for i, pf := range wa.portForwards {
		st, _ := pf.GetStatus()
		result[i] = item{Name: pf.Service.Name, Status: string(st)}
	}
	jsonOK(w, result)
}

// handleServiceStart starts a single service by name.
func (wa *WebApp) handleServiceStart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	for _, pf := range wa.portForwards {
		if pf.Service.Name == name {
			if err := pf.Start(); err != nil {
				jsonError(w, err.Error(), http.StatusConflict)
				return
			}
			jsonOK(w, map[string]string{"status": "starting"})
			return
		}
	}
	jsonError(w, "service not found", http.StatusNotFound)
}

// handleServiceStop stops a single service by name.
func (wa *WebApp) handleServiceStop(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	for _, pf := range wa.portForwards {
		if pf.Service.Name == name {
			if err := pf.Stop(); err != nil {
				jsonError(w, err.Error(), http.StatusConflict)
				return
			}
			jsonOK(w, map[string]string{"status": "stopped"})
			return
		}
	}
	jsonError(w, "service not found", http.StatusNotFound)
}

// handleStartAll starts all port forwards.
func (wa *WebApp) handleStartAll(w http.ResponseWriter, r *http.Request) {
	for _, pf := range wa.portForwards {
		_ = pf.Start()
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// handleStopAll stops all port forwards.
func (wa *WebApp) handleStopAll(w http.ResponseWriter, r *http.Request) {
	for _, pf := range wa.portForwards {
		if pf.IsRunning() {
			_ = pf.Stop()
		}
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// handleStartDefaults starts services marked selected_by_default.
func (wa *WebApp) handleStartDefaults(w http.ResponseWriter, r *http.Request) {
	for _, pf := range wa.portForwards {
		if pf.Service.SelectedByDefault {
			_ = pf.Start()
		}
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// handleGetProxyServices returns the proxy service list and pod status.
func (wa *WebApp) handleGetProxyServices(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, json.RawMessage(wa.buildStateJSON()))
}

// allServicesForGroup returns all ProxyServices belonging to the given group key.
func (wa *WebApp) allServicesForGroup(groupKey string) []ProxyService {
	var svcs []ProxyService
	for _, ps := range wa.config.ProxyServices {
		if ps.ProxyGroupKey() == groupKey {
			svcs = append(svcs, ps)
		}
	}
	return svcs
}

// stopForwardsForGroup stops and removes all active proxy forwards for a group.
// Must be called with wa.mu held.
func (wa *WebApp) stopForwardsForGroup(groupKey string) {
	for name, pxf := range wa.proxyForwards {
		if pxf.ProxyService.ProxyGroupKey() == groupKey {
			pxf.Stop()
			delete(wa.proxyForwards, name)
		}
	}
}

// handleStartProxyPod creates the proxy pod for a group with socat for all
// services in that group, but starts no port-forwards.
func (wa *WebApp) handleStartProxyPod(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupKey string `json:"group_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GroupKey == "" {
		jsonError(w, "invalid body: group_key required", http.StatusBadRequest)
		return
	}

	wa.mu.RLock()
	mgr, ok := wa.proxyPodManagers[body.GroupKey]
	wa.mu.RUnlock()
	if !ok {
		jsonError(w, "group not found", http.StatusNotFound)
		return
	}

	allSvcs := wa.allServicesForGroup(body.GroupKey)
	if len(allSvcs) == 0 {
		jsonError(w, "no services in group", http.StatusBadRequest)
		return
	}

	// Stop any existing forwards for this group
	wa.mu.Lock()
	wa.stopForwardsForGroup(body.GroupKey)
	wa.mu.Unlock()

	go func() {
		_ = mgr.CreatePodWithServices(allSvcs)
	}()

	jsonOK(w, map[string]string{"status": "starting"})
}

// handleStartDefaultProxies creates all pods and starts port-forwards for
// every is_default=true proxy service across all groups.
func (wa *WebApp) handleStartDefaultProxies(w http.ResponseWriter, r *http.Request) {
	if len(wa.proxyPodManagers) == 0 {
		jsonError(w, "no proxy services configured", http.StatusBadRequest)
		return
	}

	// Build a map of all services per group, and defaults per group
	type groupWork struct {
		mgr      *ProxyPodManager
		allSvcs  []ProxyService
		defSvcs  []ProxyService
	}
	works := make([]groupWork, 0, len(wa.proxyPodManagers))
	for key, mgr := range wa.proxyPodManagers {
		var allSvcs, defSvcs []ProxyService
		for _, ps := range wa.config.ProxyServices {
			if ps.ProxyGroupKey() == key {
				allSvcs = append(allSvcs, ps)
				if ps.SelectedByDefault {
					defSvcs = append(defSvcs, ps)
				}
			}
		}
		if len(allSvcs) > 0 {
			works = append(works, groupWork{mgr: mgr, allSvcs: allSvcs, defSvcs: defSvcs})
		}
	}

	// Stop all existing proxy forwards
	wa.mu.Lock()
	for _, pxf := range wa.proxyForwards {
		pxf.Stop()
	}
	wa.proxyForwards = make(map[string]*ProxyForward)
	wa.mu.Unlock()

	go func() {
		for _, w := range works {
			if err := w.mgr.CreatePodWithServices(w.allSvcs); err != nil {
				continue
			}
			wa.mu.Lock()
			for _, ps := range w.defSvcs {
				pxf := NewProxyForward(ps, w.mgr)
				_ = pxf.Start()
				wa.proxyForwards[ps.Name] = pxf
			}
			wa.mu.Unlock()
		}
	}()

	jsonOK(w, map[string]string{"status": "starting"})
}

// handleStartProxyService starts the port-forward for a single proxy service.
// The proxy pod for that service's group must already be running.
func (wa *WebApp) handleStartProxyService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var ps *ProxyService
	for i := range wa.config.ProxyServices {
		if wa.config.ProxyServices[i].Name == name {
			ps = &wa.config.ProxyServices[i]
			break
		}
	}
	if ps == nil {
		jsonError(w, "proxy service not found", http.StatusNotFound)
		return
	}

	wa.mu.RLock()
	mgr, ok := wa.proxyPodManagers[ps.ProxyGroupKey()]
	_, alreadyRunning := wa.proxyForwards[name]
	wa.mu.RUnlock()

	if !ok {
		jsonError(w, "group not found", http.StatusNotFound)
		return
	}
	if alreadyRunning {
		jsonOK(w, map[string]string{"status": "already running"})
		return
	}

	pxf := NewProxyForward(*ps, mgr)
	go func() {
		_ = pxf.Start()
	}()

	wa.mu.Lock()
	wa.proxyForwards[name] = pxf
	wa.mu.Unlock()

	jsonOK(w, map[string]string{"status": "starting"})
}

// handleStopProxyService stops the port-forward for a single proxy service.
func (wa *WebApp) handleStopProxyService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	wa.mu.Lock()
	pxf, ok := wa.proxyForwards[name]
	if ok {
		delete(wa.proxyForwards, name)
	}
	wa.mu.Unlock()

	if !ok {
		jsonOK(w, map[string]string{"status": "not running"})
		return
	}

	pxf.Stop()
	jsonOK(w, map[string]string{"status": "stopped"})
}

// handleResetProxyPod stops all proxy forwards, deletes all pods, then recreates
// them. Each pod gets all group services for socat; port-forwards are restored
// only for the services that had active forwards before the reset.
func (wa *WebApp) handleResetProxyPod(w http.ResponseWriter, r *http.Request) {
	if len(wa.proxyPodManagers) == 0 {
		jsonError(w, "no proxy services configured", http.StatusBadRequest)
		return
	}

	wa.mu.Lock()
	// Snapshot which services had active port-forwards, grouped by group key
	type groupSnapshot struct {
		mgr          *ProxyPodManager
		activeNames  map[string]struct{}
	}
	snapshots := make([]groupSnapshot, 0, len(wa.proxyPodManagers))
	for key, mgr := range wa.proxyPodManagers {
		active := make(map[string]struct{})
		for name, pxf := range wa.proxyForwards {
			if pxf.ProxyService.ProxyGroupKey() == key {
				active[name] = struct{}{}
				_ = name
			}
		}
		snapshots = append(snapshots, groupSnapshot{mgr: mgr, activeNames: active})
	}
	// Stop all proxy forwards
	for _, pxf := range wa.proxyForwards {
		pxf.Stop()
	}
	wa.proxyForwards = make(map[string]*ProxyForward)
	wa.mu.Unlock()

	// Delete all pods outside the lock
	for _, snap := range snapshots {
		snap.mgr.DeletePod()
	}

	// Rebuild: each pod gets all services for socat; port-forwards only for previously active ones
	type groupRecreate struct {
		mgr      *ProxyPodManager
		allSvcs  []ProxyService
		fwdSvcs  []ProxyService
	}
	wa.mu.RLock()
	recreates := make([]groupRecreate, 0, len(snapshots))
	for key, snap := range func() map[string]groupSnapshot {
		m := make(map[string]groupSnapshot, len(snapshots))
		for key, mgr := range wa.proxyPodManagers {
			for _, s := range snapshots {
				if s.mgr == mgr {
					m[key] = s
					break
				}
			}
		}
		return m
	}() {
		var allSvcs, fwdSvcs []ProxyService
		for _, ps := range wa.config.ProxyServices {
			if ps.ProxyGroupKey() != key {
				continue
			}
			allSvcs = append(allSvcs, ps)
			if _, ok := snap.activeNames[ps.Name]; ok {
				fwdSvcs = append(fwdSvcs, ps)
			}
		}
		if len(allSvcs) > 0 {
			recreates = append(recreates, groupRecreate{mgr: snap.mgr, allSvcs: allSvcs, fwdSvcs: fwdSvcs})
		}
	}
	wa.mu.RUnlock()

	if len(recreates) == 0 {
		jsonOK(w, map[string]string{"status": "reset"})
		return
	}

	go func() {
		for _, rec := range recreates {
			if err := rec.mgr.CreatePodWithServices(rec.allSvcs); err != nil {
				continue
			}
			wa.mu.Lock()
			for _, ps := range rec.fwdSvcs {
				pxf := NewProxyForward(ps, rec.mgr)
				_ = pxf.Start()
				wa.proxyForwards[ps.Name] = pxf
			}
			wa.mu.Unlock()
		}
	}()

	jsonOK(w, map[string]string{"status": "resetting"})
}

// handleKillProxyPod stops forwards for a specific group and deletes that pod.
func (wa *WebApp) handleKillProxyPod(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupKey string `json:"group_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GroupKey == "" {
		jsonError(w, "invalid body: group_key required", http.StatusBadRequest)
		return
	}

	wa.mu.Lock()
	mgr, ok := wa.proxyPodManagers[body.GroupKey]
	if !ok {
		wa.mu.Unlock()
		jsonError(w, "group not found", http.StatusNotFound)
		return
	}

	// Stop all proxy forwards belonging to this group
	for name, pxf := range wa.proxyForwards {
		if pxf.ProxyService.ProxyGroupKey() == body.GroupKey {
			pxf.Stop()
			delete(wa.proxyForwards, name)
		}
	}
	wa.mu.Unlock()

	// Delete pod outside lock (blocking kubectl call)
	mgr.DeletePod()

	jsonOK(w, map[string]string{"status": "killed"})
}

// handleGetPresets returns configured presets.
func (wa *WebApp) handleGetPresets(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, wa.config.Presets)
}

// handleApplyPreset stops all services and starts only those in the preset.
func (wa *WebApp) handleApplyPreset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var preset *Preset
	for i := range wa.config.Presets {
		if wa.config.Presets[i].Name == name {
			preset = &wa.config.Presets[i]
			break
		}
	}
	if preset == nil {
		jsonError(w, "preset not found", http.StatusNotFound)
		return
	}

	// Stop all first
	for _, pf := range wa.portForwards {
		if pf.IsRunning() {
			_ = pf.Stop()
		}
	}

	// Build a set of names in preset
	nameSet := make(map[string]struct{}, len(preset.Services))
	for _, n := range preset.Services {
		nameSet[n] = struct{}{}
	}

	// Start preset services
	for _, pf := range wa.portForwards {
		if _, ok := nameSet[pf.Service.Name]; ok {
			_ = pf.Start()
		}
	}

	jsonOK(w, map[string]string{"status": "ok"})
}

// handleGetContexts returns alternative contexts from config.
func (wa *WebApp) handleGetContexts(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, wa.config.AlternativeContexts)
}

// handleSwitchContext reloads the config with a new cluster context.
func (wa *WebApp) handleSwitchContext(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Context string `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Context == "" {
		jsonError(w, "invalid body: context required", http.StatusBadRequest)
		return
	}

	// Find the alternative context
	var found *AlternativeContext
	for i := range wa.config.AlternativeContexts {
		if wa.config.AlternativeContexts[i].Context == body.Context ||
			wa.config.AlternativeContexts[i].Name == body.Context {
			found = &wa.config.AlternativeContexts[i]
			break
		}
	}
	if found == nil {
		jsonError(w, "context not found in alternative_contexts", http.StatusNotFound)
		return
	}

	// Validate context exists
	if err := ValidateContext(found.Context); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	newConfig, err := wa.store.Load()
	if err != nil {
		jsonError(w, "failed to reload config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	newConfig.ClusterContext = found.Context
	newConfig.ClusterName = found.Name

	wa.reapplyConfig(newConfig)

	jsonOK(w, map[string]string{"status": "switched", "context": found.Context})
}

// portInfo is the response for the port checker.
type portInfo struct {
	Port        int    `json:"port"`
	ServiceName string `json:"service_name"`
	Type        string `json:"type"`
	InUse       bool   `json:"in_use"`
	PID         int    `json:"pid,omitempty"`
	Process     string `json:"process,omitempty"`
	Status      string `json:"status"`
}

// handleGetPorts returns port usage for all configured ports.
func (wa *WebApp) handleGetPorts(w http.ResponseWriter, r *http.Request) {
	cfgPorts := GetAllPortsFromConfig(wa.config)
	result := make([]portInfo, 0, len(cfgPorts))
	for _, cp := range cfgPorts {
		usage, err := GetPortUsage(cp.Port)
		info := portInfo{
			Port:        cp.Port,
			ServiceName: cp.ServiceName,
			Type:        cp.Type,
			Status:      string(PortStatusFree),
		}
		if err == nil {
			info.InUse = usage.InUse
			info.PID = usage.PID
			info.Process = usage.ProcessInfo
			info.Status = string(usage.Status)
		}
		result = append(result, info)
	}
	jsonOK(w, result)
}

// handleKillPort kills the process listening on the given port.
func (wa *WebApp) handleKillPort(w http.ResponseWriter, r *http.Request) {
	portStr := r.PathValue("port")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		jsonError(w, "invalid port", http.StatusBadRequest)
		return
	}

	usage, err := GetPortUsage(port)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !usage.InUse || usage.PID <= 0 {
		jsonError(w, "no process found on that port", http.StatusNotFound)
		return
	}

	if err := KillProcess(usage.PID); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "killed"})
}

// handleLaunchSqlTap opens a new terminal tab running sql-tap for the named service.
func (wa *WebApp) handleLaunchSqlTap(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	for _, pf := range wa.portForwards {
		if pf.Service.Name == name {
			mgr := pf.GetSqlTapManager()
			if mgr == nil || !mgr.enabled {
				jsonError(w, "sql-tap not configured for this service", http.StatusBadRequest)
				return
			}
			if err := LaunchSqlTapInNewTab(mgr.grpcPort); err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jsonOK(w, map[string]string{"status": "launched"})
			return
		}
	}

	// Check proxy forwards too
	wa.mu.RLock()
	defer wa.mu.RUnlock()
	for _, pxf := range wa.proxyForwards {
		if pxf.ProxyService.Name == name {
			mgr := pxf.GetSqlTapManager()
			if mgr == nil || !mgr.enabled {
				jsonError(w, "sql-tap not configured for this service", http.StatusBadRequest)
				return
			}
			if err := LaunchSqlTapInNewTab(mgr.grpcPort); err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jsonOK(w, map[string]string{"status": "launched"})
			return
		}
	}

	jsonError(w, "service not found", http.StatusNotFound)
}

// handleConfigReload reloads the config from the store without changing the active context.
func (wa *WebApp) handleConfigReload(w http.ResponseWriter, r *http.Request) {
	newConfig, err := wa.store.Load()
	if err != nil {
		jsonError(w, "failed to reload config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	wa.mu.RLock()
	currentContext := wa.config.ClusterContext
	wa.mu.RUnlock()
	newConfig.ClusterContext = currentContext

	wa.reapplyConfig(newConfig)

	jsonOK(w, map[string]string{"status": "reloaded"})
}

func (wa *WebApp) handleConfigImportYAML(w http.ResponseWriter, r *http.Request) {
	var body struct {
		YAML string `json:"yaml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.YAML) == "" {
		jsonError(w, "invalid body: yaml required", http.StatusBadRequest)
		return
	}
	cfg, err := ParseConfigYAML([]byte(body.YAML))
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := wa.store.Save(cfg); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	loaded, err := wa.store.Load()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wa.reapplyConfig(loaded)
	jsonOK(w, map[string]string{"status": "imported"})
}

func (wa *WebApp) handlePostConfigService(w http.ResponseWriter, r *http.Request) {
	var sv Service
	if err := json.NewDecoder(r.Body).Decode(&sv); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	cfg := wa.currentConfigClone()
	if cfg == nil {
		jsonError(w, "no config", http.StatusInternalServerError)
		return
	}
	for _, x := range cfg.Services {
		if x.Name == sv.Name {
			jsonError(w, "service name already exists", http.StatusConflict)
			return
		}
	}
	cfg.Services = append(cfg.Services, sv)
	if err := wa.store.Save(cfg); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	loaded, err := wa.store.Load()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wa.mu.RLock()
	loaded.ClusterContext = wa.config.ClusterContext
	wa.mu.RUnlock()
	wa.reapplyConfig(loaded)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (wa *WebApp) handleDeleteConfigService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg := wa.currentConfigClone()
	if cfg == nil {
		jsonError(w, "no config", http.StatusInternalServerError)
		return
	}
	found := false
	out := cfg.Services[:0]
	for _, x := range cfg.Services {
		if x.Name == name {
			found = true
			continue
		}
		out = append(out, x)
	}
	cfg.Services = out
	if !found {
		jsonError(w, "service not found", http.StatusNotFound)
		return
	}
	if err := wa.store.Save(cfg); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	loaded, err := wa.store.Load()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wa.mu.RLock()
	loaded.ClusterContext = wa.config.ClusterContext
	wa.mu.RUnlock()
	wa.reapplyConfig(loaded)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (wa *WebApp) handlePostConfigProxyService(w http.ResponseWriter, r *http.Request) {
	var ps ProxyService
	if err := json.NewDecoder(r.Body).Decode(&ps); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	cfg := wa.currentConfigClone()
	if cfg == nil {
		jsonError(w, "no config", http.StatusInternalServerError)
		return
	}
	for _, x := range cfg.ProxyServices {
		if x.Name == ps.Name {
			jsonError(w, "proxy service name already exists", http.StatusConflict)
			return
		}
	}
	cfg.ProxyServices = append(cfg.ProxyServices, ps)
	if err := wa.store.Save(cfg); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	loaded, err := wa.store.Load()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wa.mu.RLock()
	loaded.ClusterContext = wa.config.ClusterContext
	wa.mu.RUnlock()
	wa.reapplyConfig(loaded)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (wa *WebApp) handleDeleteConfigProxyService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg := wa.currentConfigClone()
	if cfg == nil {
		jsonError(w, "no config", http.StatusInternalServerError)
		return
	}
	found := false
	out := cfg.ProxyServices[:0]
	for _, x := range cfg.ProxyServices {
		if x.Name == name {
			found = true
			continue
		}
		out = append(out, x)
	}
	cfg.ProxyServices = out
	if !found {
		jsonError(w, "proxy service not found", http.StatusNotFound)
		return
	}
	if err := wa.store.Save(cfg); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	loaded, err := wa.store.Load()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wa.mu.RLock()
	loaded.ClusterContext = wa.config.ClusterContext
	wa.mu.RUnlock()
	wa.reapplyConfig(loaded)
	jsonOK(w, map[string]string{"status": "ok"})
}

// openBrowser tries to open the given URL in the system browser (macOS).
func openBrowser(url string) {
	p, err := os.StartProcess("/usr/bin/open", []string{"/usr/bin/open", url}, &os.ProcAttr{})
	if err != nil {
		return
	}
	go p.Wait()
}

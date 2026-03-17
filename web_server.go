package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

//go:embed web/index.html
var webFS embed.FS

// WebApp is the central state holder for the web-based UI.
type WebApp struct {
	config         *Config
	configFile     string
	portForwards   []*PortForward
	proxyForwards  map[string]*ProxyForward
	proxyPodManager *ProxyPodManager
	mu             sync.RWMutex

	// SSE clients
	sseClients   map[chan string]struct{}
	sseMu        sync.Mutex
}

// NewWebApp creates and initialises a WebApp from the given config.
func NewWebApp(config *Config, configFile string) *WebApp {
	pfs := make([]*PortForward, len(config.Services))
	for i, svc := range config.Services {
		pfs[i] = NewPortForward(svc, config.ClusterContext, config.Namespace, config.MaxRetries)
	}

	var ppm *ProxyPodManager
	if len(config.ProxyServices) > 0 {
		ppm = NewProxyPodManager(
			config.ProxyPodName,
			config.ProxyPodImage,
			config.ProxyPodNamespace,
			config.ProxyPodContext,
		)
	}

	return &WebApp{
		config:          config,
		configFile:      configFile,
		portForwards:    pfs,
		proxyPodManager: ppm,
		proxyForwards:   make(map[string]*ProxyForward),
		sseClients:      make(map[chan string]struct{}),
	}
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
	var defaults []ProxyService
	for _, ps := range wa.config.ProxyServices {
		if ps.SelectedByDefault {
			defaults = append(defaults, ps)
		}
	}
	if len(defaults) == 0 {
		return
	}
	wa.mu.Lock()
	defer wa.mu.Unlock()
	if err := wa.proxyPodManager.CreatePodWithServices(defaults); err != nil {
		return
	}
	for _, ps := range defaults {
		pxf := NewProxyForward(ps, wa.proxyPodManager, wa.config.ClusterContext, wa.config.Namespace)
		_ = pxf.Start()
		wa.proxyForwards[ps.Name] = pxf
	}
}

// StopAll stops every running forward and deletes the proxy pod.
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
	if wa.proxyPodManager != nil {
		wa.proxyPodManager.DeletePod()
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
	Name          string `json:"name"`
	LocalPort     int    `json:"local_port"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
	IsDefault     bool   `json:"is_default"`
	Active        bool   `json:"active"`
	HasSqlTap      bool   `json:"has_sql_tap"`
	SqlTapPort     int    `json:"sql_tap_port,omitempty"`
	SqlTapGrpcPort int    `json:"sql_tap_grpc_port,omitempty"`
	SqlTapHttpPort int    `json:"sql_tap_http_port,omitempty"`
}

type stateJSON struct {
	ClusterContext   string                  `json:"cluster_context"`
	ClusterName      string                  `json:"cluster_name"`
	Namespace        string                  `json:"namespace"`
	ConfigFile       string                  `json:"config_file"`
	Services         []serviceStateJSON      `json:"services"`
	ProxyServices    []proxyServiceStateJSON `json:"proxy_services"`
	ProxyPodStatus   string                  `json:"proxy_pod_status"`
	ProxyPodError    string                  `json:"proxy_pod_error,omitempty"`
	Presets          []Preset                `json:"presets"`
	Contexts         []AlternativeContext    `json:"contexts"`
	HasProxyServices bool                    `json:"has_proxy_services"`
	DebugMode        bool                    `json:"debug_mode"`
	DebugLines       []string                `json:"debug_lines"`
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

	proxyServices := make([]proxyServiceStateJSON, len(wa.config.ProxyServices))
	for i, ps := range wa.config.ProxyServices {
		status := string(StatusStopped)
		errMsg := ""
		if pxf, ok := wa.proxyForwards[ps.Name]; ok {
			st, e := pxf.GetStatus()
			status = string(st)
			errMsg = e
		}
		entry := proxyServiceStateJSON{
			Name:      ps.Name,
			LocalPort: ps.LocalPort,
			Status:    status,
			Error:     errMsg,
			IsDefault: ps.SelectedByDefault,
			Active:    wa.proxyPodManager != nil && wa.proxyPodManager.IsServiceActive(ps.Name),
			HasSqlTap: ps.SqlTapPort != nil,
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
		proxyServices[i] = entry
	}

	podStatus := "not_created"
	podError := ""
	if wa.proxyPodManager != nil {
		st, e, _ := wa.proxyPodManager.GetStatus()
		podStatus = string(st)
		podError = e
	}

	state := stateJSON{
		ClusterContext:   wa.config.ClusterContext,
		ClusterName:      wa.config.ClusterName,
		Namespace:        wa.config.Namespace,
		ConfigFile:       wa.configFile,
		Services:         services,
		ProxyServices:    proxyServices,
		ProxyPodStatus:   podStatus,
		ProxyPodError:    podError,
		Presets:          wa.config.Presets,
		Contexts:         wa.config.AlternativeContexts,
		HasProxyServices: len(wa.config.ProxyServices) > 0,
		DebugMode:        debugMode,
		DebugLines:       getDebugLines(),
	}

	b, _ := json.Marshal(state)
	return string(b)
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
	mux.HandleFunc("POST /api/proxy-services/apply", wa.handleApplyProxyServices)
	mux.HandleFunc("POST /api/proxy-services/reset", wa.handleResetProxyPod)

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

// handleApplyProxyServices applies a new proxy service selection.
func (wa *WebApp) handleApplyProxyServices(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	if wa.proxyPodManager == nil {
		jsonError(w, "no proxy services configured", http.StatusBadRequest)
		return
	}

	// Build selected proxy services
	selected := []ProxyService{}
	for _, ps := range wa.config.ProxyServices {
		for _, n := range body.Names {
			if ps.Name == n {
				selected = append(selected, ps)
				break
			}
		}
	}

	wa.mu.Lock()
	defer wa.mu.Unlock()

	// Stop existing proxy forwards
	for _, pxf := range wa.proxyForwards {
		pxf.Stop()
	}
	wa.proxyForwards = make(map[string]*ProxyForward)

	if len(selected) == 0 {
		wa.proxyPodManager.DeletePod()
		jsonOK(w, map[string]string{"status": "ok"})
		return
	}

	go func() {
		if err := wa.proxyPodManager.CreatePodWithServices(selected); err != nil {
			return
		}
		wa.mu.Lock()
		defer wa.mu.Unlock()
		for _, ps := range selected {
			pxf := NewProxyForward(ps, wa.proxyPodManager, wa.config.ClusterContext, wa.config.Namespace)
			_ = pxf.Start()
			wa.proxyForwards[ps.Name] = pxf
		}
	}()

	jsonOK(w, map[string]string{"status": "applying"})
}

// handleResetProxyPod stops all proxy forwards, deletes the pod, then recreates
// it with the same set of previously active services.
func (wa *WebApp) handleResetProxyPod(w http.ResponseWriter, r *http.Request) {
	if wa.proxyPodManager == nil {
		jsonError(w, "no proxy services configured", http.StatusBadRequest)
		return
	}

	wa.mu.Lock()
	// Capture which services were active before tearing down
	activeNames := wa.proxyPodManager.GetActiveServiceNames()

	// Stop all proxy forwards (each Stop() also stops its sql-tap)
	for _, pxf := range wa.proxyForwards {
		pxf.Stop()
	}
	wa.proxyForwards = make(map[string]*ProxyForward)
	wa.mu.Unlock()

	// Delete the proxy pod outside the lock (blocking kubectl call)
	wa.proxyPodManager.DeletePod()

	if len(activeNames) == 0 {
		jsonOK(w, map[string]string{"status": "reset"})
		return
	}

	// Rebuild the selected service list from config
	wa.mu.RLock()
	nameSet := make(map[string]struct{}, len(activeNames))
	for _, n := range activeNames {
		nameSet[n] = struct{}{}
	}
	selected := make([]ProxyService, 0, len(activeNames))
	for _, ps := range wa.config.ProxyServices {
		if _, ok := nameSet[ps.Name]; ok {
			selected = append(selected, ps)
		}
	}
	clusterContext := wa.config.ClusterContext
	namespace := wa.config.Namespace
	wa.mu.RUnlock()

	go func() {
		if err := wa.proxyPodManager.CreatePodWithServices(selected); err != nil {
			return
		}
		wa.mu.Lock()
		defer wa.mu.Unlock()
		for _, ps := range selected {
			pxf := NewProxyForward(ps, wa.proxyPodManager, clusterContext, namespace)
			_ = pxf.Start()
			wa.proxyForwards[ps.Name] = pxf
		}
	}()

	jsonOK(w, map[string]string{"status": "resetting"})
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

	// Stop everything
	wa.StopAll()

	// Reload config with new context
	newConfig, err := LoadConfig(wa.configFile)
	if err != nil {
		jsonError(w, "failed to reload config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	newConfig.ClusterContext = found.Context
	newConfig.ClusterName = found.Name

	wa.mu.Lock()
	wa.config = newConfig
	wa.portForwards = make([]*PortForward, len(newConfig.Services))
	for i, svc := range newConfig.Services {
		wa.portForwards[i] = NewPortForward(svc, newConfig.ClusterContext, newConfig.Namespace, newConfig.MaxRetries)
	}
	if len(newConfig.ProxyServices) > 0 {
		wa.proxyPodManager = NewProxyPodManager(
			newConfig.ProxyPodName, newConfig.ProxyPodImage,
			newConfig.ProxyPodNamespace, newConfig.ProxyPodContext,
		)
	}
	wa.proxyForwards = make(map[string]*ProxyForward)
	wa.mu.Unlock()

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

// handleConfigReload reloads the config file without changing the context.
func (wa *WebApp) handleConfigReload(w http.ResponseWriter, r *http.Request) {
	newConfig, err := LoadConfig(wa.configFile)
	if err != nil {
		jsonError(w, "failed to reload config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Preserve the current context in case it was switched at runtime
	wa.mu.RLock()
	currentContext := wa.config.ClusterContext
	wa.mu.RUnlock()
	newConfig.ClusterContext = currentContext

	wa.StopAll()

	wa.mu.Lock()
	wa.config = newConfig
	wa.portForwards = make([]*PortForward, len(newConfig.Services))
	for i, svc := range newConfig.Services {
		wa.portForwards[i] = NewPortForward(svc, newConfig.ClusterContext, newConfig.Namespace, newConfig.MaxRetries)
	}
	if len(newConfig.ProxyServices) > 0 {
		wa.proxyPodManager = NewProxyPodManager(
			newConfig.ProxyPodName, newConfig.ProxyPodImage,
			newConfig.ProxyPodNamespace, newConfig.ProxyPodContext,
		)
	} else {
		wa.proxyPodManager = nil
	}
	wa.proxyForwards = make(map[string]*ProxyForward)
	wa.mu.Unlock()

	jsonOK(w, map[string]string{"status": "reloaded"})
}

// openBrowser tries to open the given URL in the system browser (macOS).
func openBrowser(url string) {
	p, err := os.StartProcess("/usr/bin/open", []string{"/usr/bin/open", url}, &os.ProcAttr{})
	if err != nil {
		return
	}
	go p.Wait()
}

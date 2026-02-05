package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// MetricsCollector 指標收集器
type MetricsCollector struct {
	mu sync.RWMutex

	// 引擎指標
	engineStartTime time.Time
	engineState     string

	// Slave 指標
	totalSlaves   int
	activeSlaves  int
	stoppedSlaves int

	// 請求指標
	totalRequests   atomic.Uint64
	totalErrors     atomic.Uint64
	bytesReceived   atomic.Uint64
	bytesSent       atomic.Uint64

	// 場景指標
	currentScenario string

	// 歷史記錄 (用於計算速率)
	requestHistory []requestSample
	maxHistory     int

	// 參照
	engine *Engine
	logger *zap.Logger
}

type requestSample struct {
	timestamp time.Time
	requests  uint64
	errors    uint64
}

// MetricsSnapshot 指標快照
type MetricsSnapshot struct {
	Timestamp       time.Time `json:"timestamp"`
	Uptime          string    `json:"uptime"`
	EngineState     string    `json:"engine_state"`
	CurrentScenario string    `json:"current_scenario"`

	// Slave 指標
	TotalSlaves   int `json:"total_slaves"`
	ActiveSlaves  int `json:"active_slaves"`
	StoppedSlaves int `json:"stopped_slaves"`

	// 請求指標
	TotalRequests   uint64  `json:"total_requests"`
	TotalErrors     uint64  `json:"total_errors"`
	ErrorRate       float64 `json:"error_rate"`
	RequestsPerSec  float64 `json:"requests_per_sec"`
	BytesReceived   uint64  `json:"bytes_received"`
	BytesSent       uint64  `json:"bytes_sent"`

	// 暫存器指標 (樣本)
	SampleVoltage   float64 `json:"sample_voltage,omitempty"`
	SampleCurrent   float64 `json:"sample_current,omitempty"`
	SampleFrequency float64 `json:"sample_frequency,omitempty"`
	SamplePower     float64 `json:"sample_power,omitempty"`
}

// NewMetricsCollector 建立指標收集器
func NewMetricsCollector(engine *Engine, logger *zap.Logger) *MetricsCollector {
	return &MetricsCollector{
		engine:     engine,
		logger:     logger,
		maxHistory: 60, // 保留 60 個樣本 (用於計算每秒速率)
	}
}

// Start 啟動指標收集
func (m *MetricsCollector) Start(endpoint string, port int) error {
	m.engineStartTime = time.Now()

	// 啟動背景收集
	go m.collectLoop()

	// 啟動 HTTP 伺服器
	mux := http.NewServeMux()
	mux.HandleFunc(endpoint, m.handleMetrics)
	mux.HandleFunc("/health", m.handleHealth)
	mux.HandleFunc("/ready", m.handleReady)

	addr := fmt.Sprintf(":%d", port)
	m.logger.Info("啟動指標伺服器", zap.String("addr", addr))

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			m.logger.Error("指標伺服器錯誤", zap.Error(err))
		}
	}()

	return nil
}

// collectLoop 背景收集迴圈
func (m *MetricsCollector) collectLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.collect()
	}
}

// collect 收集指標
func (m *MetricsCollector) collect() {
	if m.engine == nil {
		return
	}

	stats := m.engine.Stats()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.engineState = m.engine.State().String()
	m.totalSlaves = stats.SlaveCount
	m.activeSlaves = stats.ActiveSlaves
	m.currentScenario = m.engine.GetScenario().String()

	// 更新累計值
	m.totalRequests.Store(stats.TotalRequests)
	m.totalErrors.Store(stats.TotalErrors)
	m.bytesReceived.Store(stats.BytesReceived)
	m.bytesSent.Store(stats.BytesSent)

	// 記錄歷史
	sample := requestSample{
		timestamp: time.Now(),
		requests:  stats.TotalRequests,
		errors:    stats.TotalErrors,
	}
	m.requestHistory = append(m.requestHistory, sample)
	if len(m.requestHistory) > m.maxHistory {
		m.requestHistory = m.requestHistory[1:]
	}
}

// Snapshot 取得指標快照
func (m *MetricsCollector) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalReqs := m.totalRequests.Load()
	totalErrs := m.totalErrors.Load()

	snapshot := MetricsSnapshot{
		Timestamp:       time.Now(),
		Uptime:          time.Since(m.engineStartTime).String(),
		EngineState:     m.engineState,
		CurrentScenario: m.currentScenario,
		TotalSlaves:     m.totalSlaves,
		ActiveSlaves:    m.activeSlaves,
		StoppedSlaves:   m.totalSlaves - m.activeSlaves,
		TotalRequests:   totalReqs,
		TotalErrors:     totalErrs,
		BytesReceived:   m.bytesReceived.Load(),
		BytesSent:       m.bytesSent.Load(),
	}

	// 計算錯誤率
	if totalReqs > 0 {
		snapshot.ErrorRate = float64(totalErrs) / float64(totalReqs) * 100
	}

	// 計算每秒請求數 (使用最近的歷史記錄)
	if len(m.requestHistory) >= 2 {
		first := m.requestHistory[0]
		last := m.requestHistory[len(m.requestHistory)-1]
		duration := last.timestamp.Sub(first.timestamp).Seconds()
		if duration > 0 {
			snapshot.RequestsPerSec = float64(last.requests-first.requests) / duration
		}
	}

	// 取得樣本暫存器值
	if m.engine != nil {
		slaves := m.engine.ListSlaves()
		if len(slaves) > 0 {
			regs := slaves[0].Registers()
			snapshot.SampleVoltage, _ = regs.GetScaledValue(40001)
			snapshot.SampleCurrent, _ = regs.GetScaledValue(40002)
			snapshot.SampleFrequency, _ = regs.GetScaledValue(40003)
			snapshot.SamplePower, _ = regs.GetScaledValue(40007)
		}
	}

	return snapshot
}

// handleMetrics 處理 /metrics 請求
func (m *MetricsCollector) handleMetrics(w http.ResponseWriter, r *http.Request) {
	snapshot := m.Snapshot()

	// 檢查 Accept header
	accept := r.Header.Get("Accept")
	if accept == "application/json" || r.URL.Query().Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(snapshot)
		return
	}

	// Prometheus 格式
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	fmt.Fprintf(w, "# HELP modbussim_uptime_seconds Uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE modbussim_uptime_seconds gauge\n")
	fmt.Fprintf(w, "modbussim_uptime_seconds %f\n\n", time.Since(m.engineStartTime).Seconds())

	fmt.Fprintf(w, "# HELP modbussim_slaves_total Total number of slaves\n")
	fmt.Fprintf(w, "# TYPE modbussim_slaves_total gauge\n")
	fmt.Fprintf(w, "modbussim_slaves_total %d\n\n", snapshot.TotalSlaves)

	fmt.Fprintf(w, "# HELP modbussim_slaves_active Active number of slaves\n")
	fmt.Fprintf(w, "# TYPE modbussim_slaves_active gauge\n")
	fmt.Fprintf(w, "modbussim_slaves_active %d\n\n", snapshot.ActiveSlaves)

	fmt.Fprintf(w, "# HELP modbussim_requests_total Total number of requests\n")
	fmt.Fprintf(w, "# TYPE modbussim_requests_total counter\n")
	fmt.Fprintf(w, "modbussim_requests_total %d\n\n", snapshot.TotalRequests)

	fmt.Fprintf(w, "# HELP modbussim_errors_total Total number of errors\n")
	fmt.Fprintf(w, "# TYPE modbussim_errors_total counter\n")
	fmt.Fprintf(w, "modbussim_errors_total %d\n\n", snapshot.TotalErrors)

	fmt.Fprintf(w, "# HELP modbussim_requests_per_second Requests per second\n")
	fmt.Fprintf(w, "# TYPE modbussim_requests_per_second gauge\n")
	fmt.Fprintf(w, "modbussim_requests_per_second %f\n\n", snapshot.RequestsPerSec)

	fmt.Fprintf(w, "# HELP modbussim_bytes_received_total Total bytes received\n")
	fmt.Fprintf(w, "# TYPE modbussim_bytes_received_total counter\n")
	fmt.Fprintf(w, "modbussim_bytes_received_total %d\n\n", snapshot.BytesReceived)

	fmt.Fprintf(w, "# HELP modbussim_bytes_sent_total Total bytes sent\n")
	fmt.Fprintf(w, "# TYPE modbussim_bytes_sent_total counter\n")
	fmt.Fprintf(w, "modbussim_bytes_sent_total %d\n\n", snapshot.BytesSent)

	fmt.Fprintf(w, "# HELP modbussim_sample_voltage Sample voltage reading\n")
	fmt.Fprintf(w, "# TYPE modbussim_sample_voltage gauge\n")
	fmt.Fprintf(w, "modbussim_sample_voltage %f\n\n", snapshot.SampleVoltage)

	fmt.Fprintf(w, "# HELP modbussim_sample_current Sample current reading\n")
	fmt.Fprintf(w, "# TYPE modbussim_sample_current gauge\n")
	fmt.Fprintf(w, "modbussim_sample_current %f\n\n", snapshot.SampleCurrent)

	fmt.Fprintf(w, "# HELP modbussim_sample_frequency Sample frequency reading\n")
	fmt.Fprintf(w, "# TYPE modbussim_sample_frequency gauge\n")
	fmt.Fprintf(w, "modbussim_sample_frequency %f\n\n", snapshot.SampleFrequency)

	fmt.Fprintf(w, "# HELP modbussim_sample_power Sample power reading\n")
	fmt.Fprintf(w, "# TYPE modbussim_sample_power gauge\n")
	fmt.Fprintf(w, "modbussim_sample_power %f\n", snapshot.SamplePower)
}

// handleHealth 處理 /health 請求
func (m *MetricsCollector) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// handleReady 處理 /ready 請求
func (m *MetricsCollector) handleReady(w http.ResponseWriter, r *http.Request) {
	if m.engine == nil || m.engine.State() != EngineStateRunning {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

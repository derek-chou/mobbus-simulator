package main

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// EngineState 引擎狀態
type EngineState int32

const (
	EngineStateStopped EngineState = iota
	EngineStateStarting
	EngineStateRunning
	EngineStateStopping
)

func (s EngineState) String() string {
	switch s {
	case EngineStateStopped:
		return "stopped"
	case EngineStateStarting:
		return "starting"
	case EngineStateRunning:
		return "running"
	case EngineStateStopping:
		return "stopping"
	default:
		return "unknown"
	}
}

// Engine Modbus Slave 引擎
type Engine struct {
	mu sync.RWMutex

	// 配置
	config *Config

	// 狀態
	state atomic.Int32

	// Slaves
	slaves map[string]*Slave

	// 統計
	stats EngineStats

	// 場景
	currentScenario ScenarioType

	// 日誌
	logger *zap.Logger
}

// EngineStats 引擎統計資訊
type EngineStats struct {
	StartTime      time.Time
	SlaveCount     int
	ActiveSlaves   int
	TotalRequests  uint64
	TotalErrors    uint64
	BytesReceived  uint64
	BytesSent      uint64
}

// NewEngine 建立新的引擎
func NewEngine(config *Config, logger *zap.Logger) *Engine {
	return &Engine{
		config:          config,
		slaves:          make(map[string]*Slave),
		currentScenario: ScenarioNormal,
		logger:          logger,
	}
}

// Start 啟動引擎
func (e *Engine) Start(ctx context.Context) error {
	if !e.state.CompareAndSwap(int32(EngineStateStopped), int32(EngineStateStarting)) {
		return fmt.Errorf("引擎已經在運行中")
	}

	e.stats.StartTime = time.Now()
	e.logger.Info("正在啟動引擎",
		zap.Int("slave_count", e.config.Slaves.Count),
		zap.Int("port", e.config.Server.Port),
	)

	// 取得要綁定的 IP 列表
	ips, err := e.getBindIPs()
	if err != nil {
		e.state.Store(int32(EngineStateStopped))
		return fmt.Errorf("取得綁定 IP 失敗: %w", err)
	}

	// 建立並啟動 Slaves
	var wg sync.WaitGroup
	errChan := make(chan error, len(ips))
	semaphore := make(chan struct{}, 100) // 限制並發啟動數量

	for i, ip := range ips {
		if i >= e.config.Slaves.Count {
			break
		}

		wg.Add(1)
		go func(ip net.IP, idx int) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			unitID := uint8((int(e.config.Slaves.UnitIDStart) + idx - 1) % 255 + 1)
			slave := NewSlave(
				ip,
				e.config.Server.Port,
				e.config,
				WithUnitID(unitID),
				WithLogger(e.logger.With(zap.String("slave_id", fmt.Sprintf("%s:%d", ip.String(), e.config.Server.Port)))),
			)

			if err := slave.Start(ctx); err != nil {
				errChan <- fmt.Errorf("啟動 Slave %s 失敗: %w", ip.String(), err)
				return
			}

			e.mu.Lock()
			e.slaves[slave.ID] = slave
			e.mu.Unlock()
		}(ip, i)
	}

	// 等待所有 Slaves 啟動
	wg.Wait()
	close(errChan)

	// 收集錯誤
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		e.logger.Warn("部分 Slaves 啟動失敗",
			zap.Int("failed", len(errors)),
			zap.Int("success", len(e.slaves)),
		)
		// 如果所有 Slaves 都失敗，返回錯誤
		if len(e.slaves) == 0 {
			e.state.Store(int32(EngineStateStopped))
			return fmt.Errorf("所有 Slaves 啟動失敗: %v", errors[0])
		}
	}

	e.stats.SlaveCount = len(e.slaves)
	e.stats.ActiveSlaves = len(e.slaves)
	e.state.Store(int32(EngineStateRunning))

	e.logger.Info("引擎啟動完成",
		zap.Int("active_slaves", e.stats.ActiveSlaves),
		zap.Duration("startup_time", time.Since(e.stats.StartTime)),
	)

	return nil
}

// Stop 停止引擎
func (e *Engine) Stop(ctx context.Context) error {
	if !e.state.CompareAndSwap(int32(EngineStateRunning), int32(EngineStateStopping)) {
		return nil
	}

	e.logger.Info("正在停止引擎", zap.Int("slave_count", len(e.slaves)))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 100)

	e.mu.RLock()
	slaves := make([]*Slave, 0, len(e.slaves))
	for _, slave := range e.slaves {
		slaves = append(slaves, slave)
	}
	e.mu.RUnlock()

	for _, slave := range slaves {
		wg.Add(1)
		go func(s *Slave) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if err := s.Stop(ctx); err != nil {
				e.logger.Warn("停止 Slave 失敗",
					zap.String("id", s.ID),
					zap.Error(err),
				)
			}
		}(slave)
	}

	// 等待所有 Slaves 停止或超時
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		e.logger.Warn("停止引擎超時")
	}

	e.mu.Lock()
	e.slaves = make(map[string]*Slave)
	e.mu.Unlock()

	e.state.Store(int32(EngineStateStopped))
	e.logger.Info("引擎已停止")

	return nil
}

// GetSlave 取得指定 IP 的 Slave
func (e *Engine) GetSlave(ip net.IP) (*Slave, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	id := fmt.Sprintf("%s:%d", ip.String(), e.config.Server.Port)
	slave, ok := e.slaves[id]
	return slave, ok
}

// GetSlaveByID 取得指定 ID 的 Slave
func (e *Engine) GetSlaveByID(id string) (*Slave, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	slave, ok := e.slaves[id]
	return slave, ok
}

// ListSlaves 列出所有 Slaves
func (e *Engine) ListSlaves() []*Slave {
	e.mu.RLock()
	defer e.mu.RUnlock()

	slaves := make([]*Slave, 0, len(e.slaves))
	for _, slave := range e.slaves {
		slaves = append(slaves, slave)
	}
	return slaves
}

// State 取得引擎狀態
func (e *Engine) State() EngineState {
	return EngineState(e.state.Load())
}

// Stats 取得統計資訊
func (e *Engine) Stats() EngineStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := e.stats

	// 彙整所有 Slaves 的統計
	for _, slave := range e.slaves {
		slaveStats := slave.GetStats()
		stats.TotalRequests += slaveStats.RequestCount.Load()
		stats.TotalErrors += slaveStats.ErrorCount.Load()
		stats.BytesReceived += slaveStats.BytesReceived.Load()
		stats.BytesSent += slaveStats.BytesSent.Load()
	}

	return stats
}

// ApplyScenario 套用場景到所有 Slaves
func (e *Engine) ApplyScenario(scenario ScenarioType) error {
	e.mu.Lock()
	e.currentScenario = scenario
	e.mu.Unlock()

	e.logger.Info("套用場景", zap.String("scenario", scenario.String()))

	for _, slave := range e.ListSlaves() {
		slave.ApplyScenario(scenario)
	}

	return nil
}

// GetScenario 取得當前場景
func (e *Engine) GetScenario() ScenarioType {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentScenario
}

// getBindIPs 取得要綁定的 IP 列表
func (e *Engine) getBindIPs() ([]net.IP, error) {
	// 如果有配置 IP 範圍，使用配置的範圍
	if len(e.config.Network.IPRanges) > 0 {
		return e.config.ExpandIPRanges()
	}

	// 否則使用本地 IP
	localIPs, err := getLocalIPs()
	if err != nil {
		return nil, err
	}

	// 如果沒有本地 IP，使用 0.0.0.0
	if len(localIPs) == 0 {
		return []net.IP{net.ParseIP("0.0.0.0")}, nil
	}

	// 如果 Slave 數量大於本地 IP 數量，複製 IP
	ips := make([]net.IP, 0, e.config.Slaves.Count)
	for len(ips) < e.config.Slaves.Count {
		for _, ip := range localIPs {
			if len(ips) >= e.config.Slaves.Count {
				break
			}
			ips = append(ips, ip)
		}
		if len(ips) == 0 {
			break
		}
	}

	return ips, nil
}

// getLocalIPs 取得本地 IP 列表
func getLocalIPs() ([]net.IP, error) {
	var ips []net.IP

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ips = append(ips, ipNet.IP)
			}
		}
	}

	return ips, nil
}

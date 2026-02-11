package main

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tbrandon/mbserver"
	"go.uber.org/zap"
)

// SlaveState Slave 狀態
type SlaveState int32

const (
	SlaveStateStopped SlaveState = iota
	SlaveStateStarting
	SlaveStateRunning
	SlaveStateStopping
)

func (s SlaveState) String() string {
	switch s {
	case SlaveStateStopped:
		return "stopped"
	case SlaveStateStarting:
		return "starting"
	case SlaveStateRunning:
		return "running"
	case SlaveStateStopping:
		return "stopping"
	default:
		return "unknown"
	}
}

// Slave 單一 Modbus Slave 實例
type Slave struct {
	mu sync.RWMutex

	// 基本資訊
	ID       string
	IP       net.IP
	Port     int
	UnitID   uint8

	// 狀態
	state atomic.Int32

	// 暫存器
	registers *RegisterMap

	// Modbus Server
	server *mbserver.Server

	// 統計
	stats SlaveStats

	// 場景
	scenario     ScenarioType
	scenarioCtx  context.Context
	scenarioStop context.CancelFunc

	// 日誌
	logger *zap.Logger

	// 配置
	config *Config
}

// SlaveStats Slave 統計資訊
type SlaveStats struct {
	StartTime       time.Time
	RequestCount    atomic.Uint64
	ErrorCount      atomic.Uint64
	LastRequestTime atomic.Int64
	BytesReceived   atomic.Uint64
	BytesSent       atomic.Uint64
}

// SlaveOption Slave 配置選項
type SlaveOption func(*Slave)

// WithUnitID 設定 Unit ID
func WithUnitID(id uint8) SlaveOption {
	return func(s *Slave) {
		s.UnitID = id
	}
}

// WithRegisters 設定自訂暫存器
func WithRegisters(rm *RegisterMap) SlaveOption {
	return func(s *Slave) {
		s.registers = rm
	}
}

// WithLogger 設定日誌
func WithLogger(logger *zap.Logger) SlaveOption {
	return func(s *Slave) {
		s.logger = logger
	}
}

// NewSlave 建立新的 Slave
func NewSlave(ip net.IP, port int, config *Config, opts ...SlaveOption) *Slave {
	s := &Slave{
		ID:        fmt.Sprintf("%s:%d", ip.String(), port),
		IP:        ip,
		Port:      port,
		UnitID:    1,
		registers: DefaultRegisterMap(),
		config:    config,
		scenario:  ScenarioNormal,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.logger == nil {
		s.logger, _ = zap.NewProduction()
	}

	return s
}

// Start 啟動 Slave
func (s *Slave) Start(ctx context.Context) error {
	if !s.state.CompareAndSwap(int32(SlaveStateStopped), int32(SlaveStateStarting)) {
		return fmt.Errorf("slave %s 已經在運行中", s.ID)
	}

	// 建立 mbserver
	s.server = mbserver.NewServer()

	// 設定暫存器資料
	s.syncRegistersToServer()

	// 啟動伺服器 (ListenTCP 同步建立 listener，內部以 goroutine accept)
	s.stats.StartTime = time.Now()
	addr := fmt.Sprintf("%s:%d", s.IP.String(), s.Port)

	if err := s.server.ListenTCP(addr); err != nil {
		s.state.Store(int32(SlaveStateStopped))
		return fmt.Errorf("監聽 %s 失敗: %w", addr, err)
	}

	// 啟動場景更新
	s.scenarioCtx, s.scenarioStop = context.WithCancel(ctx)
	go s.runScenarioUpdater()

	s.state.Store(int32(SlaveStateRunning))

	s.logger.Info("Slave 已啟動",
		zap.String("id", s.ID),
		zap.String("addr", addr),
		zap.Uint8("unitID", s.UnitID),
	)

	return nil
}

// Stop 停止 Slave
func (s *Slave) Stop(ctx context.Context) error {
	if !s.state.CompareAndSwap(int32(SlaveStateRunning), int32(SlaveStateStopping)) {
		return nil // 已經停止
	}

	// 停止場景更新
	if s.scenarioStop != nil {
		s.scenarioStop()
	}

	// 關閉伺服器
	if s.server != nil {
		s.server.Close()
	}

	s.state.Store(int32(SlaveStateStopped))

	s.logger.Info("Slave 已停止",
		zap.String("id", s.ID),
		zap.Duration("uptime", time.Since(s.stats.StartTime)),
		zap.Uint64("requests", s.stats.RequestCount.Load()),
	)

	return nil
}

// State 取得當前狀態
func (s *Slave) State() SlaveState {
	return SlaveState(s.state.Load())
}

// GetStats 取得統計資訊
func (s *Slave) GetStats() *SlaveStats {
	return &s.stats
}

// Registers 取得暫存器映射
func (s *Slave) Registers() *RegisterMap {
	return s.registers
}

// ApplyScenario 套用場景
func (s *Slave) ApplyScenario(scenario ScenarioType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scenario = scenario
}

// GetScenario 取得當前場景
func (s *Slave) GetScenario() ScenarioType {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scenario
}

// syncRegistersToServer 同步暫存器到 mbserver
func (s *Slave) syncRegistersToServer() {
	if s.server == nil {
		return
	}

	// mbserver 使用 byte slice 作為內部存儲
	// Holding Registers: 位址 0-65535
	holdingRegs := s.registers.GetRawHoldingRegisters()
	s.server.HoldingRegisters = make([]uint16, len(holdingRegs))
	copy(s.server.HoldingRegisters, holdingRegs)

	// Input Registers
	inputRegs := s.registers.GetRawInputRegisters()
	s.server.InputRegisters = make([]uint16, len(inputRegs))
	copy(s.server.InputRegisters, inputRegs)

	// Coils
	coils := s.registers.GetRawCoils()
	s.server.Coils = make([]byte, (len(coils)+7)/8)
	for i, coil := range coils {
		if coil {
			s.server.Coils[i/8] |= 1 << (i % 8)
		}
	}

	// Discrete Inputs
	discretes := s.registers.GetRawDiscreteInputs()
	s.server.DiscreteInputs = make([]byte, (len(discretes)+7)/8)
	for i, d := range discretes {
		if d {
			s.server.DiscreteInputs[i/8] |= 1 << (i % 8)
		}
	}
}

// runScenarioUpdater 運行場景更新器
func (s *Slave) runScenarioUpdater() {
	ticker := time.NewTicker(s.config.Scenario.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.scenarioCtx.Done():
			return
		case <-ticker.C:
			s.updateByScenario()
		}
	}
}

// updateByScenario 根據場景更新暫存器值
func (s *Slave) updateByScenario() {
	s.mu.RLock()
	scenario := s.scenario
	s.mu.RUnlock()

	handler := GetScenarioHandler(scenario)
	if handler == nil {
		return
	}

	params, ok := s.config.Scenario.Scenarios[scenario.String()]
	if !ok {
		params = ScenarioParams{}
	}

	// 更新暫存器值
	handler.Update(s.registers, params)

	// 同步到 mbserver
	s.mu.Lock()
	s.syncRegistersToServer()
	s.mu.Unlock()
}

// recordRequest 記錄請求
func (s *Slave) recordRequest(bytesIn, bytesOut int, hasError bool) {
	s.stats.RequestCount.Add(1)
	s.stats.LastRequestTime.Store(time.Now().UnixNano())
	s.stats.BytesReceived.Add(uint64(bytesIn))
	s.stats.BytesSent.Add(uint64(bytesOut))
	if hasError {
		s.stats.ErrorCount.Add(1)
	}
}

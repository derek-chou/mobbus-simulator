package main

import (
	"math/rand"
	"sync"
	"time"
)

// ScenarioType 場景類型
type ScenarioType int

const (
	ScenarioNormal ScenarioType = iota
	ScenarioVoltageSag
	ScenarioJitter
	ScenarioPacketLoss
)

func (s ScenarioType) String() string {
	switch s {
	case ScenarioNormal:
		return "normal"
	case ScenarioVoltageSag:
		return "voltage_sag"
	case ScenarioJitter:
		return "jitter"
	case ScenarioPacketLoss:
		return "packet_loss"
	default:
		return "unknown"
	}
}

// ParseScenarioType 解析場景類型
func ParseScenarioType(s string) ScenarioType {
	switch s {
	case "normal":
		return ScenarioNormal
	case "voltage_sag":
		return ScenarioVoltageSag
	case "jitter":
		return ScenarioJitter
	case "packet_loss":
		return ScenarioPacketLoss
	default:
		return ScenarioNormal
	}
}

// ScenarioHandler 場景處理介面
type ScenarioHandler interface {
	Type() ScenarioType
	Update(registers *RegisterMap, params ScenarioParams)
	Reset(registers *RegisterMap)
}

// 場景處理器註冊表
var (
	scenarioHandlers   = make(map[ScenarioType]ScenarioHandler)
	scenarioHandlersMu sync.RWMutex
)

func init() {
	// 註冊所有場景處理器
	RegisterScenarioHandler(&NormalScenario{})
	RegisterScenarioHandler(&VoltageSagScenario{})
	RegisterScenarioHandler(&JitterScenario{})
	RegisterScenarioHandler(&PacketLossScenario{})
}

// RegisterScenarioHandler 註冊場景處理器
func RegisterScenarioHandler(handler ScenarioHandler) {
	scenarioHandlersMu.Lock()
	defer scenarioHandlersMu.Unlock()
	scenarioHandlers[handler.Type()] = handler
}

// GetScenarioHandler 取得場景處理器
func GetScenarioHandler(scenarioType ScenarioType) ScenarioHandler {
	scenarioHandlersMu.RLock()
	defer scenarioHandlersMu.RUnlock()
	return scenarioHandlers[scenarioType]
}

// ListScenarioTypes 列出所有場景類型
func ListScenarioTypes() []ScenarioType {
	return []ScenarioType{
		ScenarioNormal,
		ScenarioVoltageSag,
		ScenarioJitter,
		ScenarioPacketLoss,
	}
}

// --- Normal Scenario ---

// NormalScenario 正常場景 - 小幅波動
type NormalScenario struct {
	baseVoltage   float64
	baseCurrent   float64
	baseFrequency float64
	basePower     float64
	energy        float64
	lastUpdate    time.Time
}

func (s *NormalScenario) Type() ScenarioType {
	return ScenarioNormal
}

func (s *NormalScenario) Update(registers *RegisterMap, params ScenarioParams) {
	// 初始化基準值
	if s.baseVoltage == 0 {
		s.baseVoltage = 220.0
		s.baseCurrent = 15.5
		s.baseFrequency = 60.0
		s.basePower = 3300.0
		s.lastUpdate = time.Now()
	}

	// 電壓波動 (±0.5%)
	voltageVariance := params.VoltageVariance
	if voltageVariance == 0 {
		voltageVariance = 0.005
	}
	voltage := s.baseVoltage * (1 + (rand.Float64()*2-1)*voltageVariance)

	// 頻率波動 (±0.05%)
	freqVariance := params.FrequencyVariance
	if freqVariance == 0 {
		freqVariance = 0.0005
	}
	frequency := s.baseFrequency * (1 + (rand.Float64()*2-1)*freqVariance)

	// 電流波動 (±2%)
	current := s.baseCurrent * (1 + (rand.Float64()*2-1)*0.02)

	// 功率計算
	power := voltage * current * 0.95 // PF = 0.95

	// 累積能量
	elapsed := time.Since(s.lastUpdate).Hours()
	s.energy += power * elapsed / 1000 // kWh
	s.lastUpdate = time.Now()

	// 更新暫存器
	registers.SetScaledValue(40001, voltage)
	registers.SetScaledValue(40002, current)
	registers.SetScaledValue(40003, frequency)
	registers.SetScaledValue(40004, s.energy)
	registers.SetScaledValue(40006, 0.95)
	registers.SetScaledValue(40007, power)
}

func (s *NormalScenario) Reset(registers *RegisterMap) {
	s.energy = 0
	s.lastUpdate = time.Now()
	registers.SetScaledValue(40001, 220.0)
	registers.SetScaledValue(40002, 15.5)
	registers.SetScaledValue(40003, 60.0)
	registers.SetScaledValue(40004, 0)
	registers.SetScaledValue(40006, 0.95)
	registers.SetScaledValue(40007, 3300.0)
}

// --- Voltage Sag Scenario ---

// VoltageSagScenario 電壓驟降場景
type VoltageSagScenario struct {
	normalScenario NormalScenario
	startTime      time.Time
	duration       time.Duration
	sagFactor      float64
}

func (s *VoltageSagScenario) Type() ScenarioType {
	return ScenarioVoltageSag
}

func (s *VoltageSagScenario) Update(registers *RegisterMap, params ScenarioParams) {
	// 初始化
	if s.startTime.IsZero() {
		s.startTime = time.Now()
		s.duration = params.Duration
		if s.duration == 0 {
			s.duration = 10 * time.Second
		}
		s.sagFactor = 1 - params.VoltageVariance
		if s.sagFactor <= 0 || s.sagFactor >= 1 {
			s.sagFactor = 0.8 // 預設降至 80%
		}
	}

	// 先用正常場景更新
	s.normalScenario.Update(registers, ScenarioParams{
		VoltageVariance:   0.005,
		FrequencyVariance: 0.0005,
	})

	// 在持續時間內套用電壓驟降
	if time.Since(s.startTime) < s.duration {
		voltage, _ := registers.GetScaledValue(40001)
		registers.SetScaledValue(40001, voltage*s.sagFactor)

		// 功率也跟著下降
		power, _ := registers.GetScaledValue(40007)
		registers.SetScaledValue(40007, power*s.sagFactor)
	}
}

func (s *VoltageSagScenario) Reset(registers *RegisterMap) {
	s.startTime = time.Time{}
	s.normalScenario.Reset(registers)
}

// --- Jitter Scenario ---

// JitterScenario 網路延遲場景
type JitterScenario struct {
	normalScenario NormalScenario
	jitterMin      time.Duration
	jitterMax      time.Duration
}

func (s *JitterScenario) Type() ScenarioType {
	return ScenarioJitter
}

func (s *JitterScenario) Update(registers *RegisterMap, params ScenarioParams) {
	// 設定延遲參數 (由 RequestHandler 使用)
	s.jitterMin = params.JitterMin
	s.jitterMax = params.JitterMax
	if s.jitterMin == 0 {
		s.jitterMin = 100 * time.Millisecond
	}
	if s.jitterMax == 0 {
		s.jitterMax = 500 * time.Millisecond
	}

	// 使用正常場景更新暫存器值
	s.normalScenario.Update(registers, ScenarioParams{
		VoltageVariance:   0.005,
		FrequencyVariance: 0.0005,
	})
}

func (s *JitterScenario) Reset(registers *RegisterMap) {
	s.normalScenario.Reset(registers)
}

// GetJitterRange 取得延遲範圍
func (s *JitterScenario) GetJitterRange() (min, max time.Duration) {
	return s.jitterMin, s.jitterMax
}

// --- Packet Loss Scenario ---

// PacketLossScenario 封包丟失場景
type PacketLossScenario struct {
	normalScenario NormalScenario
	lossRate       float64
}

func (s *PacketLossScenario) Type() ScenarioType {
	return ScenarioPacketLoss
}

func (s *PacketLossScenario) Update(registers *RegisterMap, params ScenarioParams) {
	// 設定丟失率 (由 RequestHandler 使用)
	s.lossRate = params.PacketLossRate
	if s.lossRate == 0 {
		s.lossRate = 0.05 // 預設 5%
	}

	// 使用正常場景更新暫存器值
	s.normalScenario.Update(registers, ScenarioParams{
		VoltageVariance:   0.005,
		FrequencyVariance: 0.0005,
	})
}

func (s *PacketLossScenario) Reset(registers *RegisterMap) {
	s.normalScenario.Reset(registers)
}

// GetLossRate 取得丟失率
func (s *PacketLossScenario) GetLossRate() float64 {
	return s.lossRate
}

// ScenarioEngine 場景引擎 (管理場景切換和更新)
type ScenarioEngine struct {
	mu sync.RWMutex

	currentType    ScenarioType
	currentHandler ScenarioHandler
	params         ScenarioParams

	// 定時器
	updateInterval time.Duration
	stopChan       chan struct{}
}

// NewScenarioEngine 建立場景引擎
func NewScenarioEngine(updateInterval time.Duration) *ScenarioEngine {
	return &ScenarioEngine{
		currentType:    ScenarioNormal,
		currentHandler: GetScenarioHandler(ScenarioNormal),
		updateInterval: updateInterval,
		stopChan:       make(chan struct{}),
	}
}

// SetScenario 設定場景
func (e *ScenarioEngine) SetScenario(scenarioType ScenarioType, params ScenarioParams) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.currentType = scenarioType
	e.currentHandler = GetScenarioHandler(scenarioType)
	e.params = params
}

// GetScenario 取得當前場景
func (e *ScenarioEngine) GetScenario() (ScenarioType, ScenarioParams) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentType, e.params
}

// Update 更新暫存器
func (e *ScenarioEngine) Update(registers *RegisterMap) {
	e.mu.RLock()
	handler := e.currentHandler
	params := e.params
	e.mu.RUnlock()

	if handler != nil {
		handler.Update(registers, params)
	}
}

// Reset 重設為正常場景
func (e *ScenarioEngine) Reset(registers *RegisterMap) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.currentHandler != nil {
		e.currentHandler.Reset(registers)
	}

	e.currentType = ScenarioNormal
	e.currentHandler = GetScenarioHandler(ScenarioNormal)
	e.params = ScenarioParams{}
}

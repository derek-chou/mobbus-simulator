package main

import (
	"math/rand"
	"time"

	"go.uber.org/zap"
)

// RequestHandler Modbus 請求處理器
type RequestHandler struct {
	slave  *Slave
	logger *zap.Logger

	// 場景相關
	jitterEnabled    bool
	jitterMin        time.Duration
	jitterMax        time.Duration
	packetLossRate   float64
}

// NewRequestHandler 建立請求處理器
func NewRequestHandler(slave *Slave, logger *zap.Logger) *RequestHandler {
	return &RequestHandler{
		slave:  slave,
		logger: logger,
	}
}

// SetJitter 設定延遲抖動
func (h *RequestHandler) SetJitter(enabled bool, min, max time.Duration) {
	h.jitterEnabled = enabled
	h.jitterMin = min
	h.jitterMax = max
}

// SetPacketLoss 設定封包丟失率
func (h *RequestHandler) SetPacketLoss(rate float64) {
	h.packetLossRate = rate
}

// applyJitter 套用延遲抖動
func (h *RequestHandler) applyJitter() {
	if !h.jitterEnabled {
		return
	}

	jitter := h.jitterMin + time.Duration(rand.Int63n(int64(h.jitterMax-h.jitterMin)))
	time.Sleep(jitter)
}

// shouldDropPacket 判斷是否應該丟棄封包
func (h *RequestHandler) shouldDropPacket() bool {
	if h.packetLossRate <= 0 {
		return false
	}
	return rand.Float64() < h.packetLossRate
}

// HandleReadCoils 處理讀取線圈請求 (FC 01)
func (h *RequestHandler) HandleReadCoils(address, quantity uint16) ([]bool, error) {
	h.applyJitter()

	if h.shouldDropPacket() {
		return nil, nil // 模擬封包丟失
	}

	coils, err := h.slave.registers.ReadCoils(address, quantity)
	if err != nil {
		h.slave.recordRequest(0, 0, true)
		h.logger.Debug("讀取線圈失敗",
			zap.Uint16("address", address),
			zap.Uint16("quantity", quantity),
			zap.Error(err),
		)
		return nil, err
	}

	h.slave.recordRequest(8, 3+(int(quantity)+7)/8, false)
	return coils, nil
}

// HandleReadDiscreteInputs 處理讀取離散輸入請求 (FC 02)
func (h *RequestHandler) HandleReadDiscreteInputs(address, quantity uint16) ([]bool, error) {
	h.applyJitter()

	if h.shouldDropPacket() {
		return nil, nil
	}

	inputs, err := h.slave.registers.ReadDiscreteInputs(address, quantity)
	if err != nil {
		h.slave.recordRequest(0, 0, true)
		h.logger.Debug("讀取離散輸入失敗",
			zap.Uint16("address", address),
			zap.Uint16("quantity", quantity),
			zap.Error(err),
		)
		return nil, err
	}

	h.slave.recordRequest(8, 3+(int(quantity)+7)/8, false)
	return inputs, nil
}

// HandleReadHoldingRegisters 處理讀取保持暫存器請求 (FC 03)
func (h *RequestHandler) HandleReadHoldingRegisters(address, quantity uint16) ([]uint16, error) {
	h.applyJitter()

	if h.shouldDropPacket() {
		return nil, nil
	}

	registers, err := h.slave.registers.ReadHoldingRegisters(address, quantity)
	if err != nil {
		h.slave.recordRequest(0, 0, true)
		h.logger.Debug("讀取保持暫存器失敗",
			zap.Uint16("address", address),
			zap.Uint16("quantity", quantity),
			zap.Error(err),
		)
		return nil, err
	}

	h.slave.recordRequest(8, 3+int(quantity)*2, false)
	return registers, nil
}

// HandleReadInputRegisters 處理讀取輸入暫存器請求 (FC 04)
func (h *RequestHandler) HandleReadInputRegisters(address, quantity uint16) ([]uint16, error) {
	h.applyJitter()

	if h.shouldDropPacket() {
		return nil, nil
	}

	registers, err := h.slave.registers.ReadInputRegisters(address, quantity)
	if err != nil {
		h.slave.recordRequest(0, 0, true)
		h.logger.Debug("讀取輸入暫存器失敗",
			zap.Uint16("address", address),
			zap.Uint16("quantity", quantity),
			zap.Error(err),
		)
		return nil, err
	}

	h.slave.recordRequest(8, 3+int(quantity)*2, false)
	return registers, nil
}

// HandleWriteSingleCoil 處理寫入單一線圈請求 (FC 05)
func (h *RequestHandler) HandleWriteSingleCoil(address uint16, value bool) error {
	h.applyJitter()

	if h.shouldDropPacket() {
		return nil
	}

	meta, ok := h.slave.registers.GetDefinition(address)
	if ok && !meta.Writable {
		h.slave.recordRequest(0, 0, true)
		return &ModbusError{Code: ExceptionCodeIllegalDataAddress}
	}

	if err := h.slave.registers.WriteCoil(address, value); err != nil {
		h.slave.recordRequest(0, 0, true)
		h.logger.Debug("寫入線圈失敗",
			zap.Uint16("address", address),
			zap.Bool("value", value),
			zap.Error(err),
		)
		return err
	}

	h.slave.recordRequest(8, 8, false)
	return nil
}

// HandleWriteSingleRegister 處理寫入單一暫存器請求 (FC 06)
func (h *RequestHandler) HandleWriteSingleRegister(address, value uint16) error {
	h.applyJitter()

	if h.shouldDropPacket() {
		return nil
	}

	meta, ok := h.slave.registers.GetDefinition(address)
	if ok && !meta.Writable {
		h.slave.recordRequest(0, 0, true)
		return &ModbusError{Code: ExceptionCodeIllegalDataAddress}
	}

	if err := h.slave.registers.WriteHoldingRegister(address, value); err != nil {
		h.slave.recordRequest(0, 0, true)
		h.logger.Debug("寫入暫存器失敗",
			zap.Uint16("address", address),
			zap.Uint16("value", value),
			zap.Error(err),
		)
		return err
	}

	h.slave.recordRequest(8, 8, false)
	return nil
}

// HandleWriteMultipleCoils 處理寫入多個線圈請求 (FC 15)
func (h *RequestHandler) HandleWriteMultipleCoils(address uint16, values []bool) error {
	h.applyJitter()

	if h.shouldDropPacket() {
		return nil
	}

	if err := h.slave.registers.WriteCoils(address, values); err != nil {
		h.slave.recordRequest(0, 0, true)
		h.logger.Debug("寫入多個線圈失敗",
			zap.Uint16("address", address),
			zap.Int("count", len(values)),
			zap.Error(err),
		)
		return err
	}

	h.slave.recordRequest(9+(len(values)+7)/8, 8, false)
	return nil
}

// HandleWriteMultipleRegisters 處理寫入多個暫存器請求 (FC 16)
func (h *RequestHandler) HandleWriteMultipleRegisters(address uint16, values []uint16) error {
	h.applyJitter()

	if h.shouldDropPacket() {
		return nil
	}

	if err := h.slave.registers.WriteHoldingRegisters(address, values); err != nil {
		h.slave.recordRequest(0, 0, true)
		h.logger.Debug("寫入多個暫存器失敗",
			zap.Uint16("address", address),
			zap.Int("count", len(values)),
			zap.Error(err),
		)
		return err
	}

	h.slave.recordRequest(9+len(values)*2, 8, false)
	return nil
}

// ModbusError Modbus 異常錯誤
type ModbusError struct {
	Code uint8
}

func (e *ModbusError) Error() string {
	switch e.Code {
	case ExceptionCodeIllegalFunction:
		return "非法功能碼"
	case ExceptionCodeIllegalDataAddress:
		return "非法資料位址"
	case ExceptionCodeIllegalDataValue:
		return "非法資料值"
	case ExceptionCodeSlaveDeviceFailure:
		return "從站設備故障"
	case ExceptionCodeAcknowledge:
		return "確認"
	case ExceptionCodeSlaveDeviceBusy:
		return "從站設備忙碌"
	default:
		return "未知錯誤"
	}
}

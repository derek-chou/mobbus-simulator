package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
)

// RegisterMap 線程安全的暫存器映射表
type RegisterMap struct {
	mu sync.RWMutex

	// 暫存器資料
	coils            []bool   // 0x - Coils
	discreteInputs   []bool   // 1x - Discrete Inputs
	inputRegisters   []uint16 // 3x - Input Registers
	holdingRegisters []uint16 // 4x - Holding Registers

	// 暫存器元資料
	definitions map[uint16]*RegisterMeta
}

// RegisterMeta 暫存器元資料
type RegisterMeta struct {
	Address     uint16
	Name        string
	DataType    DataType
	Scale       float64
	Unit        string
	Writable    bool
	MinValue    float64
	MaxValue    float64
}

// NewRegisterMap 建立新的暫存器映射表
func NewRegisterMap(coilSize, discreteSize, inputSize, holdingSize int) *RegisterMap {
	return &RegisterMap{
		coils:            make([]bool, coilSize),
		discreteInputs:   make([]bool, discreteSize),
		inputRegisters:   make([]uint16, inputSize),
		holdingRegisters: make([]uint16, holdingSize),
		definitions:      make(map[uint16]*RegisterMeta),
	}
}

// DefaultRegisterMap 建立預設暫存器映射表
func DefaultRegisterMap() *RegisterMap {
	rm := NewRegisterMap(10000, 10000, 10000, 10000)

	// 設定預設暫存器定義
	rm.DefineRegister(40001, "LineVoltage", DataTypeUint16, 10, "V", false)
	rm.DefineRegister(40002, "LineCurrent", DataTypeUint16, 100, "A", false)
	rm.DefineRegister(40003, "Frequency", DataTypeUint16, 100, "Hz", false)
	rm.DefineRegister(40004, "TotalEnergy", DataTypeUint32, 1, "kWh", false)
	rm.DefineRegister(40006, "PowerFactor", DataTypeUint16, 1000, "", false)
	rm.DefineRegister(40007, "ActivePower", DataTypeUint32, 10, "W", false)

	// 設定預設值
	rm.SetScaledValue(40001, 220.0)   // 220V
	rm.SetScaledValue(40002, 15.50)   // 15.50A
	rm.SetScaledValue(40003, 60.00)   // 60Hz
	rm.SetScaledValue(40004, 0)       // 0 kWh
	rm.SetScaledValue(40006, 0.95)    // 0.95 PF
	rm.SetScaledValue(40007, 3300.0)  // 3300W

	return rm
}

// DefineRegister 定義暫存器
func (rm *RegisterMap) DefineRegister(address uint16, name string, dataType DataType, scale float64, unit string, writable bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.definitions[address] = &RegisterMeta{
		Address:  address,
		Name:     name,
		DataType: dataType,
		Scale:    scale,
		Unit:     unit,
		Writable: writable,
	}
}

// GetDefinition 取得暫存器定義
func (rm *RegisterMap) GetDefinition(address uint16) (*RegisterMeta, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	meta, ok := rm.definitions[address]
	return meta, ok
}

// --- Coils (0x) ---

// ReadCoil 讀取單一線圈
func (rm *RegisterMap) ReadCoil(address uint16) (bool, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if int(address) >= len(rm.coils) {
		return false, fmt.Errorf("線圈位址超出範圍: %d", address)
	}
	return rm.coils[address], nil
}

// ReadCoils 讀取多個線圈
func (rm *RegisterMap) ReadCoils(address uint16, quantity uint16) ([]bool, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	end := int(address) + int(quantity)
	if end > len(rm.coils) {
		return nil, fmt.Errorf("線圈位址超出範圍: %d-%d", address, end-1)
	}

	result := make([]bool, quantity)
	copy(result, rm.coils[address:end])
	return result, nil
}

// WriteCoil 寫入單一線圈
func (rm *RegisterMap) WriteCoil(address uint16, value bool) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if int(address) >= len(rm.coils) {
		return fmt.Errorf("線圈位址超出範圍: %d", address)
	}
	rm.coils[address] = value
	return nil
}

// WriteCoils 寫入多個線圈
func (rm *RegisterMap) WriteCoils(address uint16, values []bool) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	end := int(address) + len(values)
	if end > len(rm.coils) {
		return fmt.Errorf("線圈位址超出範圍: %d-%d", address, end-1)
	}

	copy(rm.coils[address:end], values)
	return nil
}

// --- Discrete Inputs (1x) ---

// ReadDiscreteInput 讀取單一離散輸入
func (rm *RegisterMap) ReadDiscreteInput(address uint16) (bool, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if int(address) >= len(rm.discreteInputs) {
		return false, fmt.Errorf("離散輸入位址超出範圍: %d", address)
	}
	return rm.discreteInputs[address], nil
}

// ReadDiscreteInputs 讀取多個離散輸入
func (rm *RegisterMap) ReadDiscreteInputs(address uint16, quantity uint16) ([]bool, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	end := int(address) + int(quantity)
	if end > len(rm.discreteInputs) {
		return nil, fmt.Errorf("離散輸入位址超出範圍: %d-%d", address, end-1)
	}

	result := make([]bool, quantity)
	copy(result, rm.discreteInputs[address:end])
	return result, nil
}

// SetDiscreteInput 設定離散輸入 (內部用)
func (rm *RegisterMap) SetDiscreteInput(address uint16, value bool) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if int(address) >= len(rm.discreteInputs) {
		return fmt.Errorf("離散輸入位址超出範圍: %d", address)
	}
	rm.discreteInputs[address] = value
	return nil
}

// --- Input Registers (3x) ---

// ReadInputRegister 讀取單一輸入暫存器
func (rm *RegisterMap) ReadInputRegister(address uint16) (uint16, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if int(address) >= len(rm.inputRegisters) {
		return 0, fmt.Errorf("輸入暫存器位址超出範圍: %d", address)
	}
	return rm.inputRegisters[address], nil
}

// ReadInputRegisters 讀取多個輸入暫存器
func (rm *RegisterMap) ReadInputRegisters(address uint16, quantity uint16) ([]uint16, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	end := int(address) + int(quantity)
	if end > len(rm.inputRegisters) {
		return nil, fmt.Errorf("輸入暫存器位址超出範圍: %d-%d", address, end-1)
	}

	result := make([]uint16, quantity)
	copy(result, rm.inputRegisters[address:end])
	return result, nil
}

// SetInputRegister 設定輸入暫存器 (內部用)
func (rm *RegisterMap) SetInputRegister(address uint16, value uint16) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if int(address) >= len(rm.inputRegisters) {
		return fmt.Errorf("輸入暫存器位址超出範圍: %d", address)
	}
	rm.inputRegisters[address] = value
	return nil
}

// --- Holding Registers (4x) ---

// ReadHoldingRegister 讀取單一保持暫存器
func (rm *RegisterMap) ReadHoldingRegister(address uint16) (uint16, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	idx := rm.holdingIndex(address)
	if idx < 0 || idx >= len(rm.holdingRegisters) {
		return 0, fmt.Errorf("保持暫存器位址超出範圍: %d", address)
	}
	return rm.holdingRegisters[idx], nil
}

// ReadHoldingRegisters 讀取多個保持暫存器
func (rm *RegisterMap) ReadHoldingRegisters(address uint16, quantity uint16) ([]uint16, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	startIdx := rm.holdingIndex(address)
	endIdx := startIdx + int(quantity)
	if startIdx < 0 || endIdx > len(rm.holdingRegisters) {
		return nil, fmt.Errorf("保持暫存器位址超出範圍: %d-%d", address, address+quantity-1)
	}

	result := make([]uint16, quantity)
	copy(result, rm.holdingRegisters[startIdx:endIdx])
	return result, nil
}

// WriteHoldingRegister 寫入單一保持暫存器
func (rm *RegisterMap) WriteHoldingRegister(address uint16, value uint16) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	idx := rm.holdingIndex(address)
	if idx < 0 || idx >= len(rm.holdingRegisters) {
		return fmt.Errorf("保持暫存器位址超出範圍: %d", address)
	}
	rm.holdingRegisters[idx] = value
	return nil
}

// WriteHoldingRegisters 寫入多個保持暫存器
func (rm *RegisterMap) WriteHoldingRegisters(address uint16, values []uint16) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	startIdx := rm.holdingIndex(address)
	endIdx := startIdx + len(values)
	if startIdx < 0 || endIdx > len(rm.holdingRegisters) {
		return fmt.Errorf("保持暫存器位址超出範圍: %d-%d", address, address+uint16(len(values))-1)
	}

	copy(rm.holdingRegisters[startIdx:endIdx], values)
	return nil
}

// holdingIndex 將 Modbus 位址轉換為陣列索引
// 40001 -> 0, 40002 -> 1, etc.
func (rm *RegisterMap) holdingIndex(address uint16) int {
	if address >= 40001 {
		return int(address - 40001)
	}
	return int(address)
}

// --- 縮放值操作 ---

// SetScaledValue 設定縮放後的值
func (rm *RegisterMap) SetScaledValue(address uint16, value float64) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	meta, ok := rm.definitions[address]
	if !ok {
		// 沒有定義，直接寫入 uint16
		idx := rm.holdingIndex(address)
		if idx < 0 || idx >= len(rm.holdingRegisters) {
			return fmt.Errorf("保持暫存器位址超出範圍: %d", address)
		}
		rm.holdingRegisters[idx] = uint16(value)
		return nil
	}

	scaledValue := value * meta.Scale
	idx := rm.holdingIndex(address)
	if idx < 0 {
		return fmt.Errorf("無效位址: %d", address)
	}

	switch meta.DataType {
	case DataTypeUint16:
		if idx >= len(rm.holdingRegisters) {
			return fmt.Errorf("保持暫存器位址超出範圍: %d", address)
		}
		rm.holdingRegisters[idx] = uint16(scaledValue)

	case DataTypeInt16:
		if idx >= len(rm.holdingRegisters) {
			return fmt.Errorf("保持暫存器位址超出範圍: %d", address)
		}
		rm.holdingRegisters[idx] = uint16(int16(scaledValue))

	case DataTypeUint32:
		if idx+1 >= len(rm.holdingRegisters) {
			return fmt.Errorf("保持暫存器位址超出範圍: %d", address)
		}
		u32 := uint32(scaledValue)
		rm.holdingRegisters[idx] = uint16(u32 >> 16)   // High word
		rm.holdingRegisters[idx+1] = uint16(u32)       // Low word

	case DataTypeInt32:
		if idx+1 >= len(rm.holdingRegisters) {
			return fmt.Errorf("保持暫存器位址超出範圍: %d", address)
		}
		i32 := int32(scaledValue)
		rm.holdingRegisters[idx] = uint16(i32 >> 16)   // High word
		rm.holdingRegisters[idx+1] = uint16(i32)       // Low word

	case DataTypeFloat32:
		if idx+1 >= len(rm.holdingRegisters) {
			return fmt.Errorf("保持暫存器位址超出範圍: %d", address)
		}
		bits := math.Float32bits(float32(value)) // 注意：Float32 不縮放
		rm.holdingRegisters[idx] = uint16(bits >> 16)   // High word
		rm.holdingRegisters[idx+1] = uint16(bits)       // Low word
	}

	return nil
}

// GetScaledValue 取得縮放後的值
func (rm *RegisterMap) GetScaledValue(address uint16) (float64, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	meta, ok := rm.definitions[address]
	if !ok {
		// 沒有定義，直接讀取 uint16
		idx := rm.holdingIndex(address)
		if idx < 0 || idx >= len(rm.holdingRegisters) {
			return 0, fmt.Errorf("保持暫存器位址超出範圍: %d", address)
		}
		return float64(rm.holdingRegisters[idx]), nil
	}

	idx := rm.holdingIndex(address)
	if idx < 0 {
		return 0, fmt.Errorf("無效位址: %d", address)
	}

	var rawValue float64

	switch meta.DataType {
	case DataTypeUint16:
		if idx >= len(rm.holdingRegisters) {
			return 0, fmt.Errorf("保持暫存器位址超出範圍: %d", address)
		}
		rawValue = float64(rm.holdingRegisters[idx])

	case DataTypeInt16:
		if idx >= len(rm.holdingRegisters) {
			return 0, fmt.Errorf("保持暫存器位址超出範圍: %d", address)
		}
		rawValue = float64(int16(rm.holdingRegisters[idx]))

	case DataTypeUint32:
		if idx+1 >= len(rm.holdingRegisters) {
			return 0, fmt.Errorf("保持暫存器位址超出範圍: %d", address)
		}
		u32 := uint32(rm.holdingRegisters[idx])<<16 | uint32(rm.holdingRegisters[idx+1])
		rawValue = float64(u32)

	case DataTypeInt32:
		if idx+1 >= len(rm.holdingRegisters) {
			return 0, fmt.Errorf("保持暫存器位址超出範圍: %d", address)
		}
		i32 := int32(uint32(rm.holdingRegisters[idx])<<16 | uint32(rm.holdingRegisters[idx+1]))
		rawValue = float64(i32)

	case DataTypeFloat32:
		if idx+1 >= len(rm.holdingRegisters) {
			return 0, fmt.Errorf("保持暫存器位址超出範圍: %d", address)
		}
		bits := uint32(rm.holdingRegisters[idx])<<16 | uint32(rm.holdingRegisters[idx+1])
		return float64(math.Float32frombits(bits)), nil // Float32 不縮放
	}

	return rawValue / meta.Scale, nil
}

// --- 批量操作 ---

// GetRawHoldingRegisters 直接取得保持暫存器陣列 (供 mbserver 使用)
func (rm *RegisterMap) GetRawHoldingRegisters() []uint16 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]uint16, len(rm.holdingRegisters))
	copy(result, rm.holdingRegisters)
	return result
}

// GetRawInputRegisters 直接取得輸入暫存器陣列
func (rm *RegisterMap) GetRawInputRegisters() []uint16 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]uint16, len(rm.inputRegisters))
	copy(result, rm.inputRegisters)
	return result
}

// GetRawCoils 直接取得線圈陣列
func (rm *RegisterMap) GetRawCoils() []bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]bool, len(rm.coils))
	copy(result, rm.coils)
	return result
}

// GetRawDiscreteInputs 直接取得離散輸入陣列
func (rm *RegisterMap) GetRawDiscreteInputs() []bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]bool, len(rm.discreteInputs))
	copy(result, rm.discreteInputs)
	return result
}

// ToBytes 將暫存器值轉換為位元組陣列 (Big Endian)
func RegistersToBytes(registers []uint16) []byte {
	bytes := make([]byte, len(registers)*2)
	for i, reg := range registers {
		binary.BigEndian.PutUint16(bytes[i*2:], reg)
	}
	return bytes
}

// BytesToRegisters 將位元組陣列轉換為暫存器值 (Big Endian)
func BytesToRegisters(data []byte) []uint16 {
	registers := make([]uint16, len(data)/2)
	for i := range registers {
		registers[i] = binary.BigEndian.Uint16(data[i*2:])
	}
	return registers
}

// CoilsToByte 將線圈值轉換為位元組
func CoilsToByte(coils []bool) []byte {
	byteCount := (len(coils) + 7) / 8
	bytes := make([]byte, byteCount)
	for i, coil := range coils {
		if coil {
			bytes[i/8] |= 1 << (i % 8)
		}
	}
	return bytes
}

// ByteToCoils 將位元組轉換為線圈值
func ByteToCoils(data []byte, count int) []bool {
	coils := make([]bool, count)
	for i := 0; i < count; i++ {
		coils[i] = (data[i/8] & (1 << (i % 8))) != 0
	}
	return coils
}

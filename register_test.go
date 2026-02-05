package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterMap_DefaultValues(t *testing.T) {
	rm := DefaultRegisterMap()

	// 測試預設電壓值
	voltage, err := rm.GetScaledValue(40001)
	require.NoError(t, err)
	assert.InDelta(t, 220.0, voltage, 0.01, "預設電壓應為 220V")

	// 測試預設電流值
	current, err := rm.GetScaledValue(40002)
	require.NoError(t, err)
	assert.InDelta(t, 15.50, current, 0.01, "預設電流應為 15.50A")

	// 測試預設頻率值
	freq, err := rm.GetScaledValue(40003)
	require.NoError(t, err)
	assert.InDelta(t, 60.00, freq, 0.01, "預設頻率應為 60Hz")
}

func TestRegisterMap_SetAndGetScaledValue(t *testing.T) {
	rm := DefaultRegisterMap()

	// 設定電壓
	err := rm.SetScaledValue(40001, 230.5)
	require.NoError(t, err)

	// 讀取電壓
	voltage, err := rm.GetScaledValue(40001)
	require.NoError(t, err)
	assert.InDelta(t, 230.5, voltage, 0.1, "電壓應為 230.5V")
}

func TestRegisterMap_Uint32Register(t *testing.T) {
	rm := DefaultRegisterMap()

	// 設定能量值 (uint32)
	err := rm.SetScaledValue(40004, 123456.0)
	require.NoError(t, err)

	// 讀取能量值
	energy, err := rm.GetScaledValue(40004)
	require.NoError(t, err)
	assert.InDelta(t, 123456.0, energy, 1.0, "能量應為 123456 kWh")
}

func TestRegisterMap_HoldingRegisters(t *testing.T) {
	rm := NewRegisterMap(100, 100, 100, 100)

	// 寫入單一暫存器
	err := rm.WriteHoldingRegister(40001, 0x1234)
	require.NoError(t, err)

	// 讀取單一暫存器
	val, err := rm.ReadHoldingRegister(40001)
	require.NoError(t, err)
	assert.Equal(t, uint16(0x1234), val)

	// 寫入多個暫存器
	values := []uint16{0xAAAA, 0xBBBB, 0xCCCC}
	err = rm.WriteHoldingRegisters(40010, values)
	require.NoError(t, err)

	// 讀取多個暫存器
	results, err := rm.ReadHoldingRegisters(40010, 3)
	require.NoError(t, err)
	assert.Equal(t, values, results)
}

func TestRegisterMap_Coils(t *testing.T) {
	rm := NewRegisterMap(100, 100, 100, 100)

	// 寫入單一線圈
	err := rm.WriteCoil(0, true)
	require.NoError(t, err)

	// 讀取單一線圈
	val, err := rm.ReadCoil(0)
	require.NoError(t, err)
	assert.True(t, val)

	// 寫入多個線圈
	coils := []bool{true, false, true, true, false}
	err = rm.WriteCoils(10, coils)
	require.NoError(t, err)

	// 讀取多個線圈
	results, err := rm.ReadCoils(10, 5)
	require.NoError(t, err)
	assert.Equal(t, coils, results)
}

func TestRegisterMap_DiscreteInputs(t *testing.T) {
	rm := NewRegisterMap(100, 100, 100, 100)

	// 設定離散輸入
	err := rm.SetDiscreteInput(5, true)
	require.NoError(t, err)

	// 讀取離散輸入
	val, err := rm.ReadDiscreteInput(5)
	require.NoError(t, err)
	assert.True(t, val)
}

func TestRegisterMap_InputRegisters(t *testing.T) {
	rm := NewRegisterMap(100, 100, 100, 100)

	// 設定輸入暫存器
	err := rm.SetInputRegister(0, 0x5678)
	require.NoError(t, err)

	// 讀取輸入暫存器
	val, err := rm.ReadInputRegister(0)
	require.NoError(t, err)
	assert.Equal(t, uint16(0x5678), val)
}

func TestRegisterMap_OutOfBounds(t *testing.T) {
	rm := NewRegisterMap(10, 10, 10, 10)

	// 測試超出範圍
	_, err := rm.ReadCoil(100)
	assert.Error(t, err)

	_, err = rm.ReadHoldingRegister(50000)
	assert.Error(t, err)
}

func TestRegisterMap_Concurrent(t *testing.T) {
	rm := DefaultRegisterMap()
	done := make(chan bool)

	// 並發讀寫測試
	for i := 0; i < 100; i++ {
		go func(idx int) {
			// 寫入
			rm.SetScaledValue(40001, float64(200+idx))
			// 讀取
			rm.GetScaledValue(40001)
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestRegistersToBytes(t *testing.T) {
	registers := []uint16{0x0102, 0x0304}
	bytes := RegistersToBytes(registers)
	assert.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, bytes)
}

func TestBytesToRegisters(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	registers := BytesToRegisters(data)
	assert.Equal(t, []uint16{0x0102, 0x0304}, registers)
}

func TestCoilsToByte(t *testing.T) {
	coils := []bool{true, false, true, false, false, false, false, true}
	bytes := CoilsToByte(coils)
	assert.Equal(t, []byte{0x85}, bytes) // 10000101 in binary
}

func TestByteToCoils(t *testing.T) {
	data := []byte{0x85}
	coils := ByteToCoils(data, 8)
	expected := []bool{true, false, true, false, false, false, false, true}
	assert.Equal(t, expected, coils)
}

func BenchmarkRegisterMap_SetScaledValue(b *testing.B) {
	rm := DefaultRegisterMap()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rm.SetScaledValue(40001, 220.0)
	}
}

func BenchmarkRegisterMap_GetScaledValue(b *testing.B) {
	rm := DefaultRegisterMap()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rm.GetScaledValue(40001)
	}
}

func BenchmarkRegisterMap_ReadHoldingRegisters(b *testing.B) {
	rm := DefaultRegisterMap()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rm.ReadHoldingRegisters(40001, 10)
	}
}

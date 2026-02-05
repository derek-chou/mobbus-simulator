package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenarioType_String(t *testing.T) {
	tests := []struct {
		scenario ScenarioType
		expected string
	}{
		{ScenarioNormal, "normal"},
		{ScenarioVoltageSag, "voltage_sag"},
		{ScenarioJitter, "jitter"},
		{ScenarioPacketLoss, "packet_loss"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.scenario.String())
		})
	}
}

func TestParseScenarioType(t *testing.T) {
	tests := []struct {
		input    string
		expected ScenarioType
	}{
		{"normal", ScenarioNormal},
		{"voltage_sag", ScenarioVoltageSag},
		{"jitter", ScenarioJitter},
		{"packet_loss", ScenarioPacketLoss},
		{"unknown", ScenarioNormal}, // 預設為 normal
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseScenarioType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetScenarioHandler(t *testing.T) {
	for _, scenarioType := range ListScenarioTypes() {
		handler := GetScenarioHandler(scenarioType)
		require.NotNil(t, handler, "handler for %s should not be nil", scenarioType)
		assert.Equal(t, scenarioType, handler.Type())
	}
}

func TestNormalScenario_Update(t *testing.T) {
	rm := DefaultRegisterMap()
	handler := &NormalScenario{}
	params := ScenarioParams{
		VoltageVariance:   0.005,
		FrequencyVariance: 0.0005,
	}

	// 執行多次更新
	for i := 0; i < 10; i++ {
		handler.Update(rm, params)

		// 檢查電壓在合理範圍內 (220V ±1%)
		voltage, err := rm.GetScaledValue(40001)
		require.NoError(t, err)
		assert.InDelta(t, 220.0, voltage, 220.0*0.01, "電壓應在 220V ±1% 範圍內")

		// 檢查頻率在合理範圍內 (60Hz ±0.1%)
		freq, err := rm.GetScaledValue(40003)
		require.NoError(t, err)
		assert.InDelta(t, 60.0, freq, 60.0*0.001, "頻率應在 60Hz ±0.1% 範圍內")
	}
}

func TestVoltageSagScenario_Update(t *testing.T) {
	rm := DefaultRegisterMap()
	handler := &VoltageSagScenario{}
	params := ScenarioParams{
		Duration:        100 * time.Millisecond,
		VoltageVariance: 0.2, // 降至 80%
	}

	// 執行更新
	handler.Update(rm, params)

	// 檢查電壓是否降低
	voltage, err := rm.GetScaledValue(40001)
	require.NoError(t, err)
	assert.Less(t, voltage, 200.0, "電壓應低於 200V (驟降)")
}

func TestJitterScenario_GetJitterRange(t *testing.T) {
	rm := DefaultRegisterMap()
	handler := &JitterScenario{}
	params := ScenarioParams{
		JitterMin: 100 * time.Millisecond,
		JitterMax: 500 * time.Millisecond,
	}

	handler.Update(rm, params)

	min, max := handler.GetJitterRange()
	assert.Equal(t, 100*time.Millisecond, min)
	assert.Equal(t, 500*time.Millisecond, max)
}

func TestPacketLossScenario_GetLossRate(t *testing.T) {
	rm := DefaultRegisterMap()
	handler := &PacketLossScenario{}
	params := ScenarioParams{
		PacketLossRate: 0.05,
	}

	handler.Update(rm, params)

	rate := handler.GetLossRate()
	assert.Equal(t, 0.05, rate)
}

func TestScenarioEngine(t *testing.T) {
	engine := NewScenarioEngine(1 * time.Second)

	// 預設為 normal
	scenarioType, _ := engine.GetScenario()
	assert.Equal(t, ScenarioNormal, scenarioType)

	// 切換到 voltage_sag
	engine.SetScenario(ScenarioVoltageSag, ScenarioParams{
		Duration:        10 * time.Second,
		VoltageVariance: 0.2,
	})

	scenarioType, params := engine.GetScenario()
	assert.Equal(t, ScenarioVoltageSag, scenarioType)
	assert.Equal(t, 10*time.Second, params.Duration)

	// 重設
	rm := DefaultRegisterMap()
	engine.Reset(rm)

	scenarioType, _ = engine.GetScenario()
	assert.Equal(t, ScenarioNormal, scenarioType)
}

func TestNormalScenario_EnergyAccumulation(t *testing.T) {
	rm := DefaultRegisterMap()
	handler := &NormalScenario{}
	params := ScenarioParams{}

	// 初始能量
	initialEnergy, _ := rm.GetScaledValue(40004)

	// 執行更新
	handler.Update(rm, params)
	time.Sleep(100 * time.Millisecond)
	handler.Update(rm, params)

	// 能量應該增加
	finalEnergy, _ := rm.GetScaledValue(40004)
	assert.GreaterOrEqual(t, finalEnergy, initialEnergy, "能量應該累積")
}

func BenchmarkNormalScenario_Update(b *testing.B) {
	rm := DefaultRegisterMap()
	handler := &NormalScenario{}
	params := ScenarioParams{
		VoltageVariance:   0.005,
		FrequencyVariance: 0.0005,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.Update(rm, params)
	}
}

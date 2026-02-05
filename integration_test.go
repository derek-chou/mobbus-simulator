// +build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/goburrow/modbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSlaveIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	logger, _ := zap.NewDevelopment()
	config := DefaultConfig()
	config.Slaves.Count = 1
	config.Server.Port = 5502 // 使用非特權埠

	// 建立 Slave
	slave := NewSlave(
		nil, // 使用 0.0.0.0
		config.Server.Port,
		config,
		WithLogger(logger),
	)

	// 啟動 Slave
	ctx := context.Background()
	err := slave.Start(ctx)
	require.NoError(t, err)
	defer slave.Stop(ctx)

	// 等待伺服器啟動
	time.Sleep(100 * time.Millisecond)

	// 建立 Modbus 客戶端
	handler := modbus.NewTCPClientHandler("127.0.0.1:5502")
	handler.Timeout = 5 * time.Second
	err = handler.Connect()
	require.NoError(t, err)
	defer handler.Close()

	client := modbus.NewClient(handler)

	// 測試讀取保持暫存器 (FC 03)
	t.Run("ReadHoldingRegisters", func(t *testing.T) {
		// 讀取電壓暫存器 (40001 -> 位址 0)
		results, err := client.ReadHoldingRegisters(0, 1)
		require.NoError(t, err)
		assert.Len(t, results, 2) // 1 暫存器 = 2 bytes

		// 解析電壓值 (縮放因子 10)
		voltage := float64(uint16(results[0])<<8|uint16(results[1])) / 10.0
		t.Logf("讀取電壓: %.1fV", voltage)
		assert.InDelta(t, 220.0, voltage, 10.0, "電壓應接近 220V")
	})

	// 測試寫入單一暫存器 (FC 06)
	t.Run("WriteSingleRegister", func(t *testing.T) {
		// 寫入一個可寫的暫存器
		_, err := client.WriteSingleRegister(100, 0x1234)
		require.NoError(t, err)

		// 讀回驗證
		results, err := client.ReadHoldingRegisters(100, 1)
		require.NoError(t, err)
		value := uint16(results[0])<<8 | uint16(results[1])
		assert.Equal(t, uint16(0x1234), value)
	})

	// 測試讀取多個暫存器
	t.Run("ReadMultipleRegisters", func(t *testing.T) {
		// 讀取多個暫存器 (電壓、電流、頻率)
		results, err := client.ReadHoldingRegisters(0, 3)
		require.NoError(t, err)
		assert.Len(t, results, 6) // 3 暫存器 = 6 bytes

		voltage := float64(uint16(results[0])<<8|uint16(results[1])) / 10.0
		current := float64(uint16(results[2])<<8|uint16(results[3])) / 100.0
		freq := float64(uint16(results[4])<<8|uint16(results[5])) / 100.0

		t.Logf("電壓: %.1fV, 電流: %.2fA, 頻率: %.2fHz", voltage, current, freq)
	})

	// 測試讀取線圈 (FC 01)
	t.Run("ReadCoils", func(t *testing.T) {
		results, err := client.ReadCoils(0, 8)
		require.NoError(t, err)
		assert.Len(t, results, 1) // 8 個線圈 = 1 byte
	})

	// 測試寫入線圈 (FC 05)
	t.Run("WriteSingleCoil", func(t *testing.T) {
		_, err := client.WriteSingleCoil(0, 0xFF00) // ON
		require.NoError(t, err)

		results, err := client.ReadCoils(0, 1)
		require.NoError(t, err)
		assert.Equal(t, byte(1), results[0]&1)
	})
}

func TestEngineIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	logger, _ := zap.NewDevelopment()
	config := DefaultConfig()
	config.Slaves.Count = 3
	config.Server.Port = 5503

	// 建立引擎
	engine := NewEngine(config, logger)

	// 啟動引擎
	ctx := context.Background()
	err := engine.Start(ctx)
	require.NoError(t, err)
	defer engine.Stop(ctx)

	// 等待伺服器啟動
	time.Sleep(200 * time.Millisecond)

	// 檢查狀態
	assert.Equal(t, EngineStateRunning, engine.State())
	stats := engine.Stats()
	assert.Greater(t, stats.ActiveSlaves, 0)

	// 測試套用場景
	err = engine.ApplyScenario(ScenarioVoltageSag)
	require.NoError(t, err)
	assert.Equal(t, ScenarioVoltageSag, engine.GetScenario())

	// 重設場景
	err = engine.ApplyScenario(ScenarioNormal)
	require.NoError(t, err)
	assert.Equal(t, ScenarioNormal, engine.GetScenario())
}

func BenchmarkSlaveConnections(b *testing.B) {
	logger, _ := zap.NewProduction()
	config := DefaultConfig()
	config.Slaves.Count = 1
	config.Server.Port = 5504

	slave := NewSlave(nil, config.Server.Port, config, WithLogger(logger))
	ctx := context.Background()
	slave.Start(ctx)
	defer slave.Stop(ctx)

	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler := modbus.NewTCPClientHandler("127.0.0.1:5504")
		handler.Timeout = 1 * time.Second
		if err := handler.Connect(); err != nil {
			b.Fatal(err)
		}
		client := modbus.NewClient(handler)
		client.ReadHoldingRegisters(0, 10)
		handler.Close()
	}
}

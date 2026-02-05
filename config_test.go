package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, ModbusTCPDefaultPort, cfg.Server.Port)
	assert.Equal(t, 100, cfg.Slaves.Count)
	assert.Equal(t, "normal", cfg.Scenario.DefaultScenario)
	assert.True(t, cfg.Metrics.Enabled)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name: "invalid port - too low",
			modify: func(c *Config) {
				c.Server.Port = 0
			},
			wantErr: true,
		},
		{
			name: "invalid port - too high",
			modify: func(c *Config) {
				c.Server.Port = 70000
			},
			wantErr: true,
		},
		{
			name: "invalid slave count - zero",
			modify: func(c *Config) {
				c.Slaves.Count = 0
			},
			wantErr: true,
		},
		{
			name: "invalid slave count - too high",
			modify: func(c *Config) {
				c.Slaves.Count = 20000
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIPRange_Validate(t *testing.T) {
	tests := []struct {
		name    string
		r       IPRange
		wantErr bool
	}{
		{
			name:    "valid CIDR",
			r:       IPRange{CIDR: "192.168.1.0/24"},
			wantErr: false,
		},
		{
			name:    "valid range",
			r:       IPRange{Start: "192.168.1.1", End: "192.168.1.100"},
			wantErr: false,
		},
		{
			name:    "invalid CIDR",
			r:       IPRange{CIDR: "invalid"},
			wantErr: true,
		},
		{
			name:    "invalid start IP",
			r:       IPRange{Start: "invalid", End: "192.168.1.100"},
			wantErr: true,
		},
		{
			name:    "invalid end IP",
			r:       IPRange{Start: "192.168.1.1", End: "invalid"},
			wantErr: true,
		},
		{
			name:    "missing both",
			r:       IPRange{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.r.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIPRange_Expand_CIDR(t *testing.T) {
	r := IPRange{CIDR: "192.168.1.0/30"}
	ips, err := r.Expand()
	require.NoError(t, err)

	// /30 = 4 IPs, minus network and broadcast = 2 usable
	assert.Len(t, ips, 2)
	assert.Equal(t, "192.168.1.1", ips[0].String())
	assert.Equal(t, "192.168.1.2", ips[1].String())
}

func TestIPRange_Expand_Range(t *testing.T) {
	r := IPRange{Start: "192.168.1.10", End: "192.168.1.15"}
	ips, err := r.Expand()
	require.NoError(t, err)

	assert.Len(t, ips, 6)
	assert.Equal(t, "192.168.1.10", ips[0].String())
	assert.Equal(t, "192.168.1.15", ips[5].String())
}

func TestConfig_ExpandIPRanges(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Network.IPRanges = []IPRange{
		{Start: "192.168.1.1", End: "192.168.1.5"},
		{Start: "192.168.2.1", End: "192.168.2.3"},
	}

	ips, err := cfg.ExpandIPRanges()
	require.NoError(t, err)
	assert.Len(t, ips, 8) // 5 + 3
}

func TestConfig_SaveAndLoad(t *testing.T) {
	// 建立暫存目錄
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.json")

	// 儲存配置
	cfg := DefaultConfig()
	cfg.Slaves.Count = 50
	cfg.Server.Port = 5020

	err := cfg.SaveConfig(configPath)
	require.NoError(t, err)

	// 確認檔案存在
	_, err = os.Stat(configPath)
	require.NoError(t, err)

	// 載入配置
	loadedCfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, cfg.Slaves.Count, loadedCfg.Slaves.Count)
	assert.Equal(t, cfg.Server.Port, loadedCfg.Server.Port)
}

func TestIncIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.1", "192.168.1.2"},
		{"192.168.1.255", "192.168.2.0"},
		{"192.168.255.255", "192.169.0.0"},
		{"10.0.0.1", "10.0.0.2"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ip := net.ParseIP(tt.input).To4()
			incIP(ip)
			assert.Equal(t, tt.expected, ip.String())
		})
	}
}

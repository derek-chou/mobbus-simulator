package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config 全域配置
type Config struct {
	Server   ServerConfig   `json:"server" mapstructure:"server"`
	Network  NetworkConfig  `json:"network" mapstructure:"network"`
	Slaves   SlavesConfig   `json:"slaves" mapstructure:"slaves"`
	Scenario ScenarioConfig `json:"scenario" mapstructure:"scenario"`
	Logging  LoggingConfig  `json:"logging" mapstructure:"logging"`
	Metrics  MetricsConfig  `json:"metrics" mapstructure:"metrics"`
}

// ServerConfig 伺服器配置
type ServerConfig struct {
	Port            int           `json:"port" mapstructure:"port"`
	ReadTimeout     time.Duration `json:"read_timeout" mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout" mapstructure:"write_timeout"`
	MaxConnections  int           `json:"max_connections" mapstructure:"max_connections"`
	GracefulTimeout time.Duration `json:"graceful_timeout" mapstructure:"graceful_timeout"`
}

// NetworkConfig 網路配置
type NetworkConfig struct {
	Interface string    `json:"interface" mapstructure:"interface"`
	IPRanges  []IPRange `json:"ip_ranges" mapstructure:"ip_ranges"`
}

// IPRange IP 範圍
type IPRange struct {
	Start string `json:"start" mapstructure:"start"`
	End   string `json:"end" mapstructure:"end"`
	CIDR  string `json:"cidr" mapstructure:"cidr"`
}

// SlavesConfig Slave 配置
type SlavesConfig struct {
	Count            int                     `json:"count" mapstructure:"count"`
	UnitIDStart      uint8                   `json:"unit_id_start" mapstructure:"unit_id_start"`
	DefaultRegisters []RegisterDefinition    `json:"default_registers" mapstructure:"default_registers"`
}

// RegisterDefinition 暫存器定義
type RegisterDefinition struct {
	Address     uint16   `json:"address" mapstructure:"address"`
	Name        string   `json:"name" mapstructure:"name"`
	DataType    string   `json:"data_type" mapstructure:"data_type"`
	Scale       float64  `json:"scale" mapstructure:"scale"`
	DefaultValue float64 `json:"default_value" mapstructure:"default_value"`
	Unit        string   `json:"unit" mapstructure:"unit"`
	Writable    bool     `json:"writable" mapstructure:"writable"`
}

// ScenarioConfig 場景配置
type ScenarioConfig struct {
	DefaultScenario string                    `json:"default_scenario" mapstructure:"default_scenario"`
	UpdateInterval  time.Duration             `json:"update_interval" mapstructure:"update_interval"`
	Scenarios       map[string]ScenarioParams `json:"scenarios" mapstructure:"scenarios"`
}

// ScenarioParams 場景參數
type ScenarioParams struct {
	Enabled         bool          `json:"enabled" mapstructure:"enabled"`
	Duration        time.Duration `json:"duration" mapstructure:"duration"`
	VoltageVariance float64       `json:"voltage_variance" mapstructure:"voltage_variance"`
	FrequencyVariance float64     `json:"frequency_variance" mapstructure:"frequency_variance"`
	JitterMin       time.Duration `json:"jitter_min" mapstructure:"jitter_min"`
	JitterMax       time.Duration `json:"jitter_max" mapstructure:"jitter_max"`
	PacketLossRate  float64       `json:"packet_loss_rate" mapstructure:"packet_loss_rate"`
}

// LoggingConfig 日誌配置
type LoggingConfig struct {
	Level      string `json:"level" mapstructure:"level"`
	Format     string `json:"format" mapstructure:"format"`
	OutputPath string `json:"output_path" mapstructure:"output_path"`
}

// MetricsConfig 指標配置
type MetricsConfig struct {
	Enabled  bool   `json:"enabled" mapstructure:"enabled"`
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`
	Port     int    `json:"port" mapstructure:"port"`
}

// DefaultConfig 返回預設配置
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:            ModbusTCPDefaultPort,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			MaxConnections:  10000,
			GracefulTimeout: 10 * time.Second,
		},
		Network: NetworkConfig{
			Interface: "eth0",
			IPRanges:  []IPRange{},
		},
		Slaves: SlavesConfig{
			Count:       100,
			UnitIDStart: 1,
			DefaultRegisters: []RegisterDefinition{
				{Address: 40001, Name: "LineVoltage", DataType: "uint16", Scale: 10, DefaultValue: 220.0, Unit: "V", Writable: false},
				{Address: 40002, Name: "LineCurrent", DataType: "uint16", Scale: 100, DefaultValue: 15.50, Unit: "A", Writable: false},
				{Address: 40003, Name: "Frequency", DataType: "uint16", Scale: 100, DefaultValue: 60.00, Unit: "Hz", Writable: false},
				{Address: 40004, Name: "TotalEnergy", DataType: "uint32", Scale: 1, DefaultValue: 0, Unit: "kWh", Writable: false},
				{Address: 40006, Name: "PowerFactor", DataType: "uint16", Scale: 1000, DefaultValue: 0.95, Unit: "", Writable: false},
				{Address: 40007, Name: "ActivePower", DataType: "uint32", Scale: 10, DefaultValue: 3300, Unit: "W", Writable: false},
			},
		},
		Scenario: ScenarioConfig{
			DefaultScenario: "normal",
			UpdateInterval:  1 * time.Second,
			Scenarios: map[string]ScenarioParams{
				"normal": {
					Enabled:           true,
					VoltageVariance:   0.005,  // ±0.5%
					FrequencyVariance: 0.0005, // ±0.05%
				},
				"voltage_sag": {
					Enabled:         true,
					Duration:        10 * time.Second,
					VoltageVariance: 0.20, // 降至 80%
				},
				"jitter": {
					Enabled:   true,
					JitterMin: 100 * time.Millisecond,
					JitterMax: 500 * time.Millisecond,
				},
				"packet_loss": {
					Enabled:        true,
					PacketLossRate: 0.05, // 5% 封包丟失
				},
			},
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			OutputPath: "stdout",
		},
		Metrics: MetricsConfig{
			Enabled:  true,
			Endpoint: "/metrics",
			Port:     9090,
		},
	}
}

// LoadConfig 載入配置檔
func LoadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("json")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/modbussim/")
		viper.AddConfigPath("$HOME/.modbussim/")
	}

	// 環境變數覆蓋
	viper.SetEnvPrefix("MODBUSSIM")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("讀取配置檔失敗: %w", err)
		}
		// 配置檔不存在，使用預設值
	}

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失敗: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置驗證失敗: %w", err)
	}

	return cfg, nil
}

// Validate 驗證配置
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("無效的埠號: %d", c.Server.Port)
	}

	if c.Slaves.Count < 1 {
		return fmt.Errorf("Slave 數量必須大於 0")
	}

	if c.Slaves.Count > 10000 {
		return fmt.Errorf("Slave 數量超過上限 (最大 10000)")
	}

	for _, ipRange := range c.Network.IPRanges {
		if err := ipRange.Validate(); err != nil {
			return fmt.Errorf("IP 範圍驗證失敗: %w", err)
		}
	}

	return nil
}

// Validate 驗證 IP 範圍
func (r *IPRange) Validate() error {
	if r.CIDR != "" {
		_, _, err := net.ParseCIDR(r.CIDR)
		if err != nil {
			return fmt.Errorf("無效的 CIDR: %s", r.CIDR)
		}
		return nil
	}

	if r.Start == "" || r.End == "" {
		return fmt.Errorf("必須指定 Start 和 End 或 CIDR")
	}

	startIP := net.ParseIP(r.Start)
	if startIP == nil {
		return fmt.Errorf("無效的起始 IP: %s", r.Start)
	}

	endIP := net.ParseIP(r.End)
	if endIP == nil {
		return fmt.Errorf("無效的結束 IP: %s", r.End)
	}

	return nil
}

// SaveConfig 儲存配置到檔案
func (c *Config) SaveConfig(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失敗: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("寫入配置檔失敗: %w", err)
	}

	return nil
}

// ExpandIPRanges 展開所有 IP 範圍為 IP 列表
func (c *Config) ExpandIPRanges() ([]net.IP, error) {
	var ips []net.IP

	for _, r := range c.Network.IPRanges {
		rangeIPs, err := r.Expand()
		if err != nil {
			return nil, err
		}
		ips = append(ips, rangeIPs...)
	}

	return ips, nil
}

// Expand 展開 IP 範圍
func (r *IPRange) Expand() ([]net.IP, error) {
	if r.CIDR != "" {
		return expandCIDR(r.CIDR)
	}
	return expandRange(r.Start, r.End)
}

func expandCIDR(cidr string) ([]net.IP, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []net.IP
	for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip) {
		ipCopy := make(net.IP, len(ip))
		copy(ipCopy, ip)
		ips = append(ips, ipCopy)
	}

	// 移除網路位址和廣播位址
	if len(ips) > 2 {
		ips = ips[1 : len(ips)-1]
	}

	return ips, nil
}

func expandRange(start, end string) ([]net.IP, error) {
	startIP := net.ParseIP(start).To4()
	endIP := net.ParseIP(end).To4()

	if startIP == nil || endIP == nil {
		return nil, fmt.Errorf("無效的 IP 範圍: %s - %s", start, end)
	}

	var ips []net.IP
	for ip := startIP; !ip.Equal(endIP); incIP(ip) {
		ipCopy := make(net.IP, len(ip))
		copy(ipCopy, ip)
		ips = append(ips, ipCopy)
	}
	// 包含結束 IP
	ipCopy := make(net.IP, len(endIP))
	copy(ipCopy, endIP)
	ips = append(ips, ipCopy)

	return ips, nil
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

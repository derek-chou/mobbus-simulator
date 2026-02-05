package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	cfgFile   string
	logger    *zap.Logger
	appConfig *Config
)

// rootCmd 根命令
var rootCmd = &cobra.Command{
	Use:   "modbussim",
	Short: "Modbus TCP 壓力測試模擬器",
	Long: `專為能源管理系統 (EMS) 設計的高併發 Modbus TCP 模擬器。
目標單機模擬 1,000+ 個獨立 IP 實體。`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// 初始化日誌
		var err error
		logger, err = initLogger()
		if err != nil {
			return fmt.Errorf("初始化日誌失敗: %w", err)
		}

		// 載入配置 (除了 version 和 help 命令)
		if cmd.Name() != "version" && cmd.Name() != "help" && cmd.Name() != "generate" {
			appConfig, err = LoadConfig(cfgFile)
			if err != nil {
				// 配置載入失敗時使用預設值
				appConfig = DefaultConfig()
				if cfgFile != "" {
					logger.Warn("載入配置檔失敗，使用預設配置", zap.Error(err))
				}
			}
		}
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if logger != nil {
			_ = logger.Sync()
		}
	},
}

// startCmd 啟動命令
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "啟動模擬器",
	Long:  "啟動 Modbus TCP 模擬器，開始監聽連線請求。",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 覆蓋 CLI 參數
		if ip, _ := cmd.Flags().GetString("ip"); ip != "" {
			appConfig.Network.IPRanges = []IPRange{{Start: ip, End: ip}}
		}
		if count, _ := cmd.Flags().GetInt("count"); count > 0 {
			appConfig.Slaves.Count = count
		}
		if port, _ := cmd.Flags().GetInt("port"); port > 0 {
			appConfig.Server.Port = port
		}

		logger.Info("啟動 Modbus 模擬器",
			zap.Int("port", appConfig.Server.Port),
			zap.Int("slaves", appConfig.Slaves.Count),
		)

		// 建立引擎
		engine := NewEngine(appConfig, logger)

		// 設置優雅關閉
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		// 啟動引擎
		if err := engine.Start(ctx); err != nil {
			return fmt.Errorf("啟動引擎失敗: %w", err)
		}

		// 啟動指標收集器
		if appConfig.Metrics.Enabled {
			metrics := NewMetricsCollector(engine, logger)
			if err := metrics.Start(appConfig.Metrics.Endpoint, appConfig.Metrics.Port); err != nil {
				logger.Warn("啟動指標伺服器失敗", zap.Error(err))
			} else {
				logger.Info("指標伺服器已啟動",
					zap.Int("port", appConfig.Metrics.Port),
					zap.String("endpoint", appConfig.Metrics.Endpoint),
				)
			}
		}

		// 等待信號
		sig := <-sigChan
		logger.Info("收到關閉信號", zap.String("signal", sig.String()))

		// 優雅關閉
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), appConfig.Server.GracefulTimeout)
		defer shutdownCancel()

		if err := engine.Stop(shutdownCtx); err != nil {
			logger.Error("關閉引擎失敗", zap.Error(err))
			return err
		}

		logger.Info("模擬器已停止")
		return nil
	},
}

// stopCmd 停止命令
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "停止模擬器",
	Long:  "停止正在運行的 Modbus TCP 模擬器。",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 透過向 PID 發送信號來停止
		pidFile := "/var/run/modbussim.pid"
		if pid, _ := cmd.Flags().GetString("pid-file"); pid != "" {
			pidFile = pid
		}

		data, err := os.ReadFile(pidFile)
		if err != nil {
			return fmt.Errorf("讀取 PID 檔案失敗: %w", err)
		}

		var pid int
		if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
			return fmt.Errorf("解析 PID 失敗: %w", err)
		}

		process, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("找不到程序: %w", err)
		}

		if err := process.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("發送信號失敗: %w", err)
		}

		fmt.Printf("已發送停止信號到 PID %d\n", pid)
		return nil
	},
}

// statusCmd 狀態命令
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看運行狀態",
	Long:  "顯示模擬器的當前運行狀態和統計資訊。",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: 從運行中的實例取得狀態
		fmt.Println("狀態查詢功能尚未實作")
		fmt.Println("請使用 metrics endpoint 查看詳細狀態")
		return nil
	},
}

// networkCmd 網路命令組
var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "網路管理命令",
	Long:  "管理虛擬 IP 配置。",
}

// networkSetupCmd 設置網路
var networkSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "建立虛擬 IP",
	Long:  "在指定的網路介面上建立虛擬 IP 位址。",
	RunE: func(cmd *cobra.Command, args []string) error {
		iface, _ := cmd.Flags().GetString("interface")
		if iface != "" {
			appConfig.Network.Interface = iface
		}

		startIP, _ := cmd.Flags().GetString("start")
		endIP, _ := cmd.Flags().GetString("end")
		cidr, _ := cmd.Flags().GetString("cidr")

		if cidr != "" {
			appConfig.Network.IPRanges = []IPRange{{CIDR: cidr}}
		} else if startIP != "" && endIP != "" {
			appConfig.Network.IPRanges = []IPRange{{Start: startIP, End: endIP}}
		}

		provisioner := NewNetworkProvisioner(appConfig.Network.Interface, logger)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := provisioner.Setup(ctx, appConfig.Network.IPRanges); err != nil {
			return fmt.Errorf("設置網路失敗: %w", err)
		}

		fmt.Println("虛擬 IP 設置完成")
		return nil
	},
}

// networkTeardownCmd 移除網路
var networkTeardownCmd = &cobra.Command{
	Use:   "teardown",
	Short: "移除虛擬 IP",
	Long:  "移除已配置的虛擬 IP 位址。",
	RunE: func(cmd *cobra.Command, args []string) error {
		iface, _ := cmd.Flags().GetString("interface")
		if iface != "" {
			appConfig.Network.Interface = iface
		}

		provisioner := NewNetworkProvisioner(appConfig.Network.Interface, logger)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := provisioner.Teardown(ctx); err != nil {
			return fmt.Errorf("移除網路失敗: %w", err)
		}

		fmt.Println("虛擬 IP 已移除")
		return nil
	},
}

// networkListCmd 列出網路
var networkListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出已配置 IP",
	Long:  "列出目前已配置的虛擬 IP 位址。",
	RunE: func(cmd *cobra.Command, args []string) error {
		iface, _ := cmd.Flags().GetString("interface")
		if iface != "" {
			appConfig.Network.Interface = iface
		}

		provisioner := NewNetworkProvisioner(appConfig.Network.Interface, logger)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		ips, err := provisioner.List(ctx)
		if err != nil {
			return fmt.Errorf("列出 IP 失敗: %w", err)
		}

		if len(ips) == 0 {
			fmt.Println("目前沒有配置虛擬 IP")
			return nil
		}

		fmt.Printf("已配置的虛擬 IP (%d 個):\n", len(ips))
		for _, ip := range ips {
			fmt.Printf("  - %s\n", ip.String())
		}
		return nil
	},
}

// scenarioCmd 場景命令組
var scenarioCmd = &cobra.Command{
	Use:   "scenario",
	Short: "場景管理命令",
	Long:  "管理模擬場景。",
}

// scenarioListCmd 列出場景
var scenarioListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出可用場景",
	Long:  "列出所有可用的模擬場景。",
	Run: func(cmd *cobra.Command, args []string) {
		scenarios := []struct {
			Name        string
			Description string
		}{
			{"normal", "正常波動 (電壓 ±0.5%, 頻率 ±0.05%)"},
			{"voltage_sag", "電壓驟降至 80%"},
			{"jitter", "網路延遲 100-500ms"},
			{"packet_loss", "封包丟失模擬 (5%)"},
		}

		fmt.Println("可用的模擬場景:")
		for _, s := range scenarios {
			fmt.Printf("  %-15s %s\n", s.Name, s.Description)
		}
	},
}

// scenarioApplyCmd 套用場景
var scenarioApplyCmd = &cobra.Command{
	Use:   "apply [scenario]",
	Short: "套用場景",
	Long:  "套用指定的模擬場景。",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scenarioName := args[0]
		duration, _ := cmd.Flags().GetDuration("duration")

		// TODO: 透過 API 或共享記憶體通知運行中的實例
		fmt.Printf("套用場景: %s", scenarioName)
		if duration > 0 {
			fmt.Printf(" (持續 %v)", duration)
		}
		fmt.Println()

		return nil
	},
}

// scenarioResetCmd 重設場景
var scenarioResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "重設為正常模式",
	Long:  "重設模擬器為正常運行模式。",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: 透過 API 或共享記憶體通知運行中的實例
		fmt.Println("重設為正常模式")
		return nil
	},
}

// configCmd 配置命令組
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "配置管理命令",
	Long:  "管理配置檔。",
}

// configValidateCmd 驗證配置
var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "驗證配置檔",
	Long:  "驗證指定的配置檔是否有效。",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig(cfgFile)
		if err != nil {
			return fmt.Errorf("配置驗證失敗: %w", err)
		}

		fmt.Println("配置驗證通過")
		fmt.Printf("  Slaves: %d\n", cfg.Slaves.Count)
		fmt.Printf("  Port: %d\n", cfg.Server.Port)
		fmt.Printf("  Interface: %s\n", cfg.Network.Interface)
		fmt.Printf("  IP Ranges: %d\n", len(cfg.Network.IPRanges))
		return nil
	},
}

// configGenerateCmd 生成配置
var configGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "生成範例配置",
	Long:  "生成範例配置檔。",
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")
		if output == "" {
			output = "config.json"
		}

		cfg := DefaultConfig()

		// 添加範例 IP 範圍
		cfg.Network.IPRanges = []IPRange{
			{Start: "192.168.1.101", End: "192.168.1.200"},
		}

		if err := cfg.SaveConfig(output); err != nil {
			return fmt.Errorf("生成配置失敗: %w", err)
		}

		fmt.Printf("範例配置已生成: %s\n", output)
		return nil
	},
}

// versionCmd 版本命令
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "顯示版本資訊",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("modbussim version %s\n", Version)
		fmt.Printf("  Build: %s\n", BuildTime)
		fmt.Printf("  Commit: %s\n", GitCommit)
	},
}

func init() {
	// 全域 flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "配置檔路徑")

	// start 命令 flags
	startCmd.Flags().StringP("ip", "i", "", "起始 IP 位址")
	startCmd.Flags().IntP("count", "n", 0, "Slave 數量")
	startCmd.Flags().IntP("port", "p", 0, "監聽埠號")

	// stop 命令 flags
	stopCmd.Flags().String("pid-file", "/var/run/modbussim.pid", "PID 檔案路徑")

	// network 命令 flags
	networkSetupCmd.Flags().StringP("interface", "i", "eth0", "網路介面")
	networkSetupCmd.Flags().String("start", "", "起始 IP")
	networkSetupCmd.Flags().String("end", "", "結束 IP")
	networkSetupCmd.Flags().String("cidr", "", "CIDR 表示法")

	networkTeardownCmd.Flags().StringP("interface", "i", "eth0", "網路介面")
	networkListCmd.Flags().StringP("interface", "i", "eth0", "網路介面")

	// scenario 命令 flags
	scenarioApplyCmd.Flags().DurationP("duration", "d", 0, "場景持續時間")

	// config 命令 flags
	configGenerateCmd.Flags().StringP("output", "o", "config.json", "輸出檔案路徑")

	// 組裝命令樹
	networkCmd.AddCommand(networkSetupCmd, networkTeardownCmd, networkListCmd)
	scenarioCmd.AddCommand(scenarioListCmd, scenarioApplyCmd, scenarioResetCmd)
	configCmd.AddCommand(configValidateCmd, configGenerateCmd)

	rootCmd.AddCommand(
		startCmd,
		stopCmd,
		statusCmd,
		networkCmd,
		scenarioCmd,
		configCmd,
		versionCmd,
	)
}

func initLogger() (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	return cfg.Build()
}

// Execute 執行 CLI
func Execute() error {
	return rootCmd.Execute()
}

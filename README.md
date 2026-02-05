# Modbus TCP 壓力測試模擬器

專為能源管理系統 (EMS) 設計的高併發 Modbus TCP 模擬器，目標單機模擬 1,000+ 個獨立 IP 實體。

## 功能特色

- **高併發架構**：支援同時運行數千個 Modbus Slave 實例
- **虛擬 IP 配置**：Linux 平台自動配置虛擬 IP (netlink)
- **場景模擬**：內建多種測試場景
  - `normal` - 正常波動 (電壓 ±0.5%, 頻率 ±0.05%)
  - `voltage_sag` - 電壓驟降至 80%
  - `jitter` - 網路延遲 100-500ms
  - `packet_loss` - 封包丟失模擬 (5%)
- **指標監控**：Prometheus 格式指標端點
- **容器化部署**：支援 Docker 與 docker-compose

## 安裝

### 從原始碼建置

```bash
# 複製專案
git clone https://github.com/derek-chou/mobbus-simulator.git
cd mobbus-simulator

# 建置
make build

# 或直接使用 go build
go build -o build/modbussim .
```

### 使用 Docker

```bash
# 建置映像
make docker

# 或
docker build -t modbussim .
```

## 快速開始

### 基本使用

```bash
# 啟動模擬器 (使用預設配置)
./build/modbussim start

# 指定配置檔
./build/modbussim start -c config.json

# 指定參數啟動
./build/modbussim start --ip 192.168.1.101 --count 100 --port 502
```

### Docker 部署

```bash
# 使用 docker-compose
make docker-up

# 查看日誌
make docker-logs

# 停止
make docker-down
```

## CLI 命令

```
modbussim
├── start              啟動模擬器
│   ├── -c, --config   配置檔路徑
│   ├── -i, --ip       起始 IP 位址
│   ├── -n, --count    Slave 數量
│   └── -p, --port     監聽埠號
├── stop               停止模擬器
├── status             查看運行狀態
├── network
│   ├── setup          建立虛擬 IP
│   ├── teardown       移除虛擬 IP
│   └── list           列出已配置 IP
├── scenario
│   ├── list           列出可用場景
│   ├── apply          套用場景
│   └── reset          重設為正常模式
├── config
│   ├── validate       驗證配置檔
│   └── generate       生成範例配置
└── version            顯示版本資訊
```

## 配置說明

### 配置檔範例 (config.json)

```json
{
  "server": {
    "port": 502,
    "read_timeout": "30s",
    "write_timeout": "30s",
    "max_connections": 10000,
    "graceful_timeout": "10s"
  },
  "network": {
    "interface": "eth0",
    "ip_ranges": [
      {
        "start": "192.168.1.101",
        "end": "192.168.1.200"
      }
    ]
  },
  "slaves": {
    "count": 100,
    "unit_id_start": 1
  },
  "scenario": {
    "default_scenario": "normal",
    "update_interval": "1s"
  },
  "metrics": {
    "enabled": true,
    "endpoint": "/metrics",
    "port": 9090
  }
}
```

### 環境變數

所有配置項目都可以透過環境變數覆蓋，前綴為 `MODBUSSIM_`：

```bash
export MODBUSSIM_SERVER_PORT=5020
export MODBUSSIM_SLAVES_COUNT=500
```

## 暫存器映射

預設的 Holding Registers 映射：

| 位址 | 名稱 | 類型 | 縮放因子 | 預設值 | 單位 |
|------|------|------|----------|--------|------|
| 40001 | LineVoltage | uint16 | ×10 | 220.0 | V |
| 40002 | LineCurrent | uint16 | ×100 | 15.50 | A |
| 40003 | Frequency | uint16 | ×100 | 60.00 | Hz |
| 40004-5 | TotalEnergy | uint32 | ×1 | 0 | kWh |
| 40006 | PowerFactor | uint16 | ×1000 | 0.95 | - |
| 40007-8 | ActivePower | uint32 | ×10 | 3300 | W |

## 指標監控

啟用指標後，可透過 HTTP 端點取得：

```bash
# Prometheus 格式
curl http://localhost:9090/metrics

# JSON 格式
curl -H "Accept: application/json" http://localhost:9090/metrics

# 健康檢查
curl http://localhost:9090/health

# 就緒檢查
curl http://localhost:9090/ready
```

### 可用指標

| 指標名稱 | 類型 | 說明 |
|----------|------|------|
| modbussim_uptime_seconds | gauge | 運行時間 |
| modbussim_slaves_total | gauge | Slave 總數 |
| modbussim_slaves_active | gauge | 活躍 Slave 數 |
| modbussim_requests_total | counter | 請求總數 |
| modbussim_errors_total | counter | 錯誤總數 |
| modbussim_requests_per_second | gauge | 每秒請求數 |
| modbussim_bytes_received_total | counter | 接收位元組數 |
| modbussim_bytes_sent_total | counter | 發送位元組數 |

## 開發

### 建置與測試

```bash
# 安裝依賴
make deps

# 執行測試
make test

# 執行效能測試
make bench

# 產生覆蓋率報告
make coverage

# 程式碼格式化
make fmt

# 靜態分析
make lint
```

### 跨平台建置

```bash
# 建置所有平台
make build-all

# 輸出至 dist/ 目錄
# - modbussim-linux-amd64
# - modbussim-linux-arm64
# - modbussim-darwin-amd64
# - modbussim-darwin-arm64
```

## Docker 部署注意事項

### 網路模式

為支援虛擬 IP aliasing，需使用 host 網路模式：

```yaml
services:
  modbussim:
    network_mode: host
    cap_add:
      - NET_ADMIN
      - NET_RAW
```

### 資源建議

- 每 100 個 Slave 約需 100MB RAM
- 1000 個 Slave 建議至少 2 CPU cores

## 授權條款

MIT License

## 貢獻

歡迎提交 Issue 和 Pull Request。

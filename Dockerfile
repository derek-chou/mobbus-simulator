# 建置階段
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 安裝必要工具
RUN apk add --no-cache git ca-certificates

# 複製 go.mod 和 go.sum
COPY go.mod go.sum ./
RUN go mod download

# 複製源碼
COPY *.go ./

# 建置
ARG VERSION=dev
ARG BUILD_TIME
ARG GIT_COMMIT

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT}" \
    -o modbussim .

# 運行階段
FROM alpine:3.19

WORKDIR /app

# 安裝必要工具
RUN apk add --no-cache ca-certificates tzdata

# 從建置階段複製執行檔
COPY --from=builder /app/modbussim /usr/local/bin/modbussim

# 建立配置目錄
RUN mkdir -p /app/configs

# 複製預設配置
COPY config.json /app/configs/config.json

# 設定環境變數
ENV TZ=Asia/Taipei

# 暴露埠號
EXPOSE 502 9090

# 健康檢查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:9090/health || exit 1

# 啟動命令
ENTRYPOINT ["modbussim"]
CMD ["start", "-c", "/app/configs/config.json"]

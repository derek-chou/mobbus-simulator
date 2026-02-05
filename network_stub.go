//go:build !linux

package main

import (
	"context"
	"fmt"
	"net"

	"go.uber.org/zap"
)

// StubProvisioner 非 Linux 平台的 stub 配置器
type StubProvisioner struct {
	BaseProvisioner
}

func newPlatformProvisioner(interfaceName string, logger *zap.Logger) NetworkProvisioner {
	return &StubProvisioner{
		BaseProvisioner: BaseProvisioner{
			InterfaceName: interfaceName,
			Logger:        logger,
		},
	}
}

// Setup 設置虛擬 IP (stub)
func (p *StubProvisioner) Setup(ctx context.Context, ranges []IPRange) error {
	// 驗證
	if err := p.Validate(ranges); err != nil {
		return err
	}

	// 展開 IP 範圍
	ips, err := p.expandAllRanges(ranges)
	if err != nil {
		return fmt.Errorf("展開 IP 範圍失敗: %w", err)
	}

	p.Logger.Warn("虛擬 IP 配置僅在 Linux 上支援，使用模擬模式",
		zap.String("interface", p.InterfaceName),
		zap.Int("count", len(ips)),
	)

	// 在非 Linux 平台，只記錄 IP 但不實際配置
	p.ConfiguredIPs = ips

	return nil
}

// Teardown 移除虛擬 IP (stub)
func (p *StubProvisioner) Teardown(ctx context.Context) error {
	p.Logger.Warn("虛擬 IP 移除僅在 Linux 上支援，使用模擬模式",
		zap.String("interface", p.InterfaceName),
		zap.Int("count", len(p.ConfiguredIPs)),
	)

	p.ConfiguredIPs = nil
	return nil
}

// List 列出已配置的 IP (stub)
func (p *StubProvisioner) List(ctx context.Context) ([]net.IP, error) {
	// 在非 Linux 平台，返回本地 IP
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("取得本地 IP 失敗: %w", err)
	}

	var ips []net.IP
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ips = append(ips, ipNet.IP)
			}
		}
	}

	// 加入模擬配置的 IP
	ips = append(ips, p.ConfiguredIPs...)

	return ips, nil
}

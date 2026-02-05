package main

import (
	"context"
	"net"

	"go.uber.org/zap"
)

// NetworkProvisioner 網路配置器介面
type NetworkProvisioner interface {
	// Setup 設置虛擬 IP
	Setup(ctx context.Context, ranges []IPRange) error

	// Teardown 移除虛擬 IP
	Teardown(ctx context.Context) error

	// List 列出已配置的 IP
	List(ctx context.Context) ([]net.IP, error)

	// Validate 驗證 IP 範圍
	Validate(ranges []IPRange) error
}

// NewNetworkProvisioner 建立網路配置器
func NewNetworkProvisioner(interfaceName string, logger *zap.Logger) NetworkProvisioner {
	return newPlatformProvisioner(interfaceName, logger)
}

// BaseProvisioner 基礎配置器 (共用邏輯)
type BaseProvisioner struct {
	InterfaceName string
	Logger        *zap.Logger
	ConfiguredIPs []net.IP
}

// Validate 驗證 IP 範圍
func (p *BaseProvisioner) Validate(ranges []IPRange) error {
	for _, r := range ranges {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// expandAllRanges 展開所有 IP 範圍
func (p *BaseProvisioner) expandAllRanges(ranges []IPRange) ([]net.IP, error) {
	var allIPs []net.IP
	for _, r := range ranges {
		ips, err := r.Expand()
		if err != nil {
			return nil, err
		}
		allIPs = append(allIPs, ips...)
	}
	return allIPs, nil
}

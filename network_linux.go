//go:build linux

package main

import (
	"context"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
)

// LinuxProvisioner Linux 網路配置器
type LinuxProvisioner struct {
	BaseProvisioner
	link netlink.Link
}

func newPlatformProvisioner(interfaceName string, logger *zap.Logger) NetworkProvisioner {
	return &LinuxProvisioner{
		BaseProvisioner: BaseProvisioner{
			InterfaceName: interfaceName,
			Logger:        logger,
		},
	}
}

// Setup 設置虛擬 IP (使用 netlink)
func (p *LinuxProvisioner) Setup(ctx context.Context, ranges []IPRange) error {
	// 驗證
	if err := p.Validate(ranges); err != nil {
		return err
	}

	// 取得網路介面
	link, err := netlink.LinkByName(p.InterfaceName)
	if err != nil {
		return fmt.Errorf("找不到網路介面 %s: %w", p.InterfaceName, err)
	}
	p.link = link

	// 展開 IP 範圍
	ips, err := p.expandAllRanges(ranges)
	if err != nil {
		return fmt.Errorf("展開 IP 範圍失敗: %w", err)
	}

	p.Logger.Info("正在設置虛擬 IP",
		zap.String("interface", p.InterfaceName),
		zap.Int("count", len(ips)),
	)

	// 添加 IP
	successCount := 0
	for _, ip := range ips {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		addr := &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(32, 32),
			},
		}

		if err := netlink.AddrAdd(link, addr); err != nil {
			// 如果 IP 已存在，忽略錯誤
			if err.Error() == "file exists" {
				p.Logger.Debug("IP 已存在", zap.String("ip", ip.String()))
				successCount++
				p.ConfiguredIPs = append(p.ConfiguredIPs, ip)
				continue
			}
			p.Logger.Warn("添加 IP 失敗",
				zap.String("ip", ip.String()),
				zap.Error(err),
			)
			continue
		}

		successCount++
		p.ConfiguredIPs = append(p.ConfiguredIPs, ip)
		p.Logger.Debug("已添加 IP", zap.String("ip", ip.String()))
	}

	p.Logger.Info("虛擬 IP 設置完成",
		zap.Int("success", successCount),
		zap.Int("total", len(ips)),
	)

	return nil
}

// Teardown 移除虛擬 IP
func (p *LinuxProvisioner) Teardown(ctx context.Context) error {
	if p.link == nil {
		link, err := netlink.LinkByName(p.InterfaceName)
		if err != nil {
			return fmt.Errorf("找不到網路介面 %s: %w", p.InterfaceName, err)
		}
		p.link = link
	}

	p.Logger.Info("正在移除虛擬 IP",
		zap.String("interface", p.InterfaceName),
		zap.Int("count", len(p.ConfiguredIPs)),
	)

	removedCount := 0
	for _, ip := range p.ConfiguredIPs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		addr := &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(32, 32),
			},
		}

		if err := netlink.AddrDel(p.link, addr); err != nil {
			p.Logger.Warn("移除 IP 失敗",
				zap.String("ip", ip.String()),
				zap.Error(err),
			)
			continue
		}

		removedCount++
		p.Logger.Debug("已移除 IP", zap.String("ip", ip.String()))
	}

	p.ConfiguredIPs = nil

	p.Logger.Info("虛擬 IP 移除完成",
		zap.Int("removed", removedCount),
	)

	return nil
}

// List 列出已配置的 IP
func (p *LinuxProvisioner) List(ctx context.Context) ([]net.IP, error) {
	link, err := netlink.LinkByName(p.InterfaceName)
	if err != nil {
		return nil, fmt.Errorf("找不到網路介面 %s: %w", p.InterfaceName, err)
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return nil, fmt.Errorf("列出 IP 失敗: %w", err)
	}

	var ips []net.IP
	for _, addr := range addrs {
		ips = append(ips, addr.IP)
	}

	return ips, nil
}

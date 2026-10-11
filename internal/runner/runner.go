// Package runner 封装 DDNS 单次更新流程（相当于原脚本的 --cron 模式）。
package runner

import (
	"context"
	"fmt"

	"ddns/internal/cloudflare"
	"ddns/internal/config"
	"ddns/internal/logx"
)

// Result 描述一次 DDNS 更新的结果。
type Result struct {
	PublicIP string
	Action   string // nochange | created | updated
	OldIP    string
	NewIP    string
	RecordID string
	ZoneID   string
}

// DNSClient 是 runner 依赖的 Cloudflare 数据面接口，便于测试注入。
type DNSClient interface {
	FindARecord(ctx context.Context, zoneID, name string) (*cloudflare.DNSRecord, error)
	CreateARecord(ctx context.Context, zoneID, name, content string, ttl int) error
	UpdateARecord(ctx context.Context, zoneID, recordID, name, content string, ttl int) error
	ZoneID(ctx context.Context, zoneName string) (string, error)
}

// PublicIPFn 是公网 IP 探测函数签名，可注入替身以测试。
type PublicIPFn func(ctx context.Context, sources []string) (string, error)

// Run 执行一次完整的 DDNS 更新（使用真实 Cloudflare 客户端与 IP 探测）。
// cfg 必须已经成功 Validate。
func Run(ctx context.Context, cfg *config.Config) (*Result, error) {
	return RunWith(ctx, cfg, cloudflare.NewClient(cfg.CFApiToken), cloudflare.DetectPublicIP)
}

// RunWith 是 Run 的内部实现，允许注入实现已便于单元测试。
func RunWith(ctx context.Context, cfg *config.Config, client DNSClient, detectIP PublicIPFn) (*Result, error) {
	logx.Info("开始 DDNS 更新", "record", cfg.RecordName)

	// 1. 检测公网 IP
	ip, err := detectIP(ctx, cfg.IPSources)
	if err != nil {
		logx.Error("公网 IP 获取失败", "err", err)
		return nil, err
	}
	logx.Info("检测到公网 IP", "ip", ip)

	// 2. 获取 Zone ID
	zoneID, err := client.ZoneID(ctx, cfg.ZoneName)
	if err != nil {
		logx.Error("获取 Zone ID 失败", "err", err, "zone", cfg.ZoneName)
		return nil, err
	}

	// 3. 查询现有 A 记录
	rec, err := client.FindARecord(ctx, zoneID, cfg.RecordName)
	if err != nil {
		logx.Error("查询 DNS 记录失败", "err", err)
		return nil, err
	}

	if rec == nil {
		// 4a. 记录不存在 -> 创建
		if err := client.CreateARecord(ctx, zoneID, cfg.RecordName, ip, cfg.RecordTTL); err != nil {
			logx.Error("创建 DNS 记录失败", "err", err)
			return nil, err
		}
		logx.Info("DNS 记录已创建", "record", cfg.RecordName, "ip", ip)
		return &Result{PublicIP: ip, Action: "created", NewIP: ip, ZoneID: zoneID}, nil
	}

	if rec.Content == ip {
		// 4b. IP 未变化
		logx.Info("IP 未变化，跳过更新", "ip", ip)
		return &Result{PublicIP: ip, Action: "nochange", OldIP: rec.Content, NewIP: ip, RecordID: rec.ID, ZoneID: zoneID}, nil
	}

	// 4c. IP 变化 -> 更新
	if err := client.UpdateARecord(ctx, zoneID, rec.ID, cfg.RecordName, ip, cfg.RecordTTL); err != nil {
		logx.Error("更新 DNS 记录失败", "err", err)
		return nil, err
	}
	logx.Info("DNS 记录已更新", "record", cfg.RecordName, "from", rec.Content, "to", ip)
	return &Result{PublicIP: ip, Action: "updated", OldIP: rec.Content, NewIP: ip, RecordID: rec.ID, ZoneID: zoneID}, nil
}

// Describe 生成人类可读的结果描述。
func (r *Result) Describe() string {
	switch r.Action {
	case "nochange":
		return fmt.Sprintf("✓ IP 未变化，无需更新 (%s)", r.NewIP)
	case "created":
		return fmt.Sprintf("✓ DNS 记录已创建 -> %s", r.NewIP)
	case "updated":
		return fmt.Sprintf("✓ DNS 记录已更新: %s -> %s", r.OldIP, r.NewIP)
	default:
		return "未知结果"
	}
}

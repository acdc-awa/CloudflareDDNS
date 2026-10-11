package main

import (
	"fmt"
	"regexp"
	"strings"

	"ddns/internal/config"
)

// wizard 交互式配置向导，返回新配置；用户中途放弃时返回 nil。
func wizard() *config.Config {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  Cloudflare DDNS - 配置向导")
	fmt.Println("========================================")
	fmt.Println()

	cfg := &config.Config{}

	// API Token
	for {
		v := prompt("Cloudflare API Token（DNS:Edit 权限）", "")
		if v == "" {
			fmt.Println("  Token 不能为空。")
			continue
		}
		if len(v) < 20 {
			fmt.Println("  警告: Token 长度异常，通常为 40 位。确认继续? 输入 yes 或重新输入")
			c := prompt("  确认继续", "no")
			if strings.TrimSpace(c) != "yes" {
				continue
			}
		}
		cfg.CFApiToken = v
		break
	}

	// Zone Name
	for {
		v := prompt("主域名（ZONE_NAME, e.g. example.com）", "")
		if v == "" {
			fmt.Println("  域名不能为空。")
			continue
		}
		if !domainNameRe.MatchString(v) {
			fmt.Println("  域名格式无效。")
			continue
		}
		cfg.ZoneName = v
		break
	}

	// Subdomain prefix
	for {
		v := prompt("二级域名前缀（e.g. ddns -> ddns."+cfg.ZoneName+"）", "ddns")
		if v == "" {
			v = "ddns"
		}
		if !subPrefixRe.MatchString(v) {
			fmt.Println("  前缀格式无效（字母/数字/中划线）。")
			continue
		}
		cfg.RecordName = v + "." + cfg.ZoneName
		break
	}

	// TTL
	for {
		v := prompt("DNS TTL 秒数（1=自动）", "120")
		n := 0
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
			fmt.Println("  TTL 必须是正整数。")
			continue
		}
		if n != 1 && n < 120 {
			fmt.Println("  TTL 无效（1=自动 或 >=120）。")
			continue
		}
		cfg.RecordTTL = n
		break
	}

	// Check interval
	fmt.Println()
	fmt.Println("定时检查间隔（systemd timer）:")
	fmt.Println("  1) 每分钟 (1m)")
	fmt.Println("  2) 每 5 分钟 (5m) [推荐]")
	fmt.Println("  3) 每 30 分钟 (30m)")
	fmt.Println("  4) 自定义")
	interval := chooseInterval("2")
	cfg.CheckInterval = interval

	// IP sources
	fmt.Printf("IP 检测源（逗号分隔 URL）[默认: %s]: ", strings.Join(config.DefaultIPSources, ", "))
	fmt.Println()
	for {
		v := prompt("IP 检测源（空格或逗号分隔，每条为 http URL）", "")
		if v == "" {
			cfg.IPSources = append([]string(nil), config.DefaultIPSources...)
			break
		}
		fields := splitSources(v)
		if len(fields) == 0 {
			fmt.Println("  至少需要一个 IP 源。")
			continue
		}
		cfg.IPSources = fields
		break
	}

	cfg.LogLevel = "info"

	if err := cfg.Validate(); err != nil {
		fmt.Println("配置校验未通过：", err)
		return nil
	}
	return cfg
}

// editWizard 允许在已有配置基础上修改，留空回车保持原值。
func editWizard(cur *config.Config) *config.Config {
	fmt.Println("修改配置（留空回车 = 保持原值）")
	cfg := *cur

	if v := prompt("Cloudflare API Token", mask(cur.CFApiToken, 8, 4)); v != "" && v != mask(cur.CFApiToken, 8, 4) {
		cfg.CFApiToken = v
	}
	if v := prompt("主域名", cur.ZoneName); v != "" {
		if domainNameRe.MatchString(v) {
			cfg.ZoneName = v
		} else {
			fmt.Println("  域名格式无效，保留原值。")
		}
	}
	subPrefix := strings.TrimSuffix(cur.RecordName, "."+cfg.ZoneName)
	if v := prompt("二级域名前缀", subPrefix); v != "" && subPrefixRe.MatchString(v) {
		cfg.RecordName = v + "." + cfg.ZoneName
	}
	ttlDef := fmt.Sprintf("%d", cur.RecordTTL)
	if v := prompt("DNS TTL 秒数（1=自动）", ttlDef); v != "" {
		if n, err := parseTTL(v); err == nil {
			cfg.RecordTTL = n
		} else {
			fmt.Println("  TTL 无效，保留原值。")
		}
	}
	if v := prompt("检查间隔", cur.CheckInterval); v != "" {
		if intervalRe.MatchString(v) {
			cfg.CheckInterval = v
		} else {
			fmt.Println("  间隔格式无效，保留原值。")
		}
	}
	if v := prompt("IP 检测源", strings.Join(cur.IPSources, " ")); v != "" {
		if fields := splitSources(v); len(fields) > 0 {
			cfg.IPSources = fields
		}
	}

	if err := cfg.Validate(); err != nil {
		fmt.Println("配置校验未通过：", err)
		return nil
	}
	return &cfg
}

func chooseInterval(def string) string {
	v := prompt("请选择", def)
	switch strings.TrimSpace(v) {
	case "1":
		return "1m"
	case "2":
		return "5m"
	case "3":
		return "30m"
	case "4":
		c := prompt("输入间隔（如 10m, 1h）", "5m")
		if intervalRe.MatchString(c) {
			return c
		}
		return "5m"
	}
	return "5m"
}

func splitSources(s string) []string {
	repl := strings.NewReplacer(",", " ")
	var out []string
	for _, f := range strings.Fields(repl.Replace(s)) {
		out = append(out, f)
	}
	return out
}

func parseTTL(s string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	if n != 1 && n < 120 {
		return 0, fmt.Errorf("ttl out of range")
	}
	return n, nil
}

var (
	domainNameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$`)
	subPrefixRe  = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	intervalRe   = regexp.MustCompile(`^[0-9]+[smhd]$`)
)

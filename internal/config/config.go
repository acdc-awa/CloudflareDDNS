// Package config 负责 Cloudflare DDNS 的配置模型、校验与默认值。
package config

import (
	"fmt"
	"regexp"
	"strings"
)

// 各配置项的默认值
const (
	DefaultCheckInterval = "5m"
	DefaultRecordTTL     = 120
	DefaultLogLevel      = "info"
)

// 默认公网 IP 检测源
var DefaultIPSources = []string{
	"https://api.ipify.org",
	"https://icanhazip.com",
	"https://ip.sb",
}

// Config 是 DDNS 的完整配置。
type Config struct {
	CFApiToken    string   `json:"cf_api_token"`
	ZoneName      string   `json:"zone_name"`
	RecordName    string   `json:"record_name"`
	RecordTTL     int      `json:"record_ttl"`
	CheckInterval string   `json:"check_interval"`
	IPSources     []string `json:"ip_sources"`
	LogLevel      string   `json:"log_level"`

	sourcePath string // 配置来源路径（内存标记）
}

// SourcePath 返回配置来源文件路径。
func (c *Config) SourcePath() string { return c.sourcePath }

// SetSourcePath 显式设置来源路径。
func (c *Config) SetSourcePath(p string) { c.sourcePath = p }

// ApplyDefaults 填充未设置的可选字段为默认值。
func (c *Config) ApplyDefaults() {
	if c.RecordTTL == 0 {
		c.RecordTTL = DefaultRecordTTL
	}
	if c.CheckInterval == "" {
		c.CheckInterval = DefaultCheckInterval
	}
	if c.LogLevel == "" {
		c.LogLevel = DefaultLogLevel
	}
	if len(c.IPSources) == 0 {
		c.IPSources = append([]string(nil), DefaultIPSources...)
	}
}

// domainRe 匹配合法域名（由字母数字和连字符组成的多级域名）。
var domainRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$`)

// Validate 校验配置，返回第一个错误（含所有信息）。无错误返回 nil。
func (c *Config) Validate() error {
	c.ApplyDefaults()

	if c.CFApiToken == "" {
		return fmt.Errorf("cf_api_token 未设置")
	}
	if len(c.CFApiToken) < 20 || len(c.CFApiToken) > 60 {
		return fmt.Errorf("cf_api_token 长度应为 20-60 位，当前 %d 位", len(c.CFApiToken))
	}

	if c.ZoneName == "" || !domainRe.MatchString(c.ZoneName) {
		return fmt.Errorf("zone_name 格式无效（示例: example.com），当前: %q", c.ZoneName)
	}
	if c.RecordName == "" || !domainRe.MatchString(c.RecordName) {
		return fmt.Errorf("record_name 格式无效（示例: ddns.example.com），当前: %q", c.RecordName)
	}
	if !strings.HasSuffix(c.RecordName, c.ZoneName) {
		return fmt.Errorf("record_name (%s) 必须以 zone_name (%s) 结尾", c.RecordName, c.ZoneName)
	}

	if c.RecordTTL != 1 && c.RecordTTL < 120 {
		return fmt.Errorf("record_ttl 无效（1=自动 或 ≥120），当前: %d", c.RecordTTL)
	}
	if !intervalRe.MatchString(c.CheckInterval) {
		return fmt.Errorf("check_interval 格式无效（示例: 5m, 1h, 30s），当前: %q", c.CheckInterval)
	}
	if c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return fmt.Errorf("log_level 无效（debug/info/warn/error），当前: %q", c.LogLevel)
	}
	return nil
}

// intervalRe 匹配 [0-9]+[smhd] 形式的时间间隔字面量。
var intervalRe = regexp.MustCompile(`^[0-9]+[smhd]$`)

// IntervalDuration 返回 CheckInterval 对应的秒数（供 timer 生成与 readme 参考）。
func (c *Config) IntervalDuration() (int, error) {
	s := c.CheckInterval
	unit := s[len(s)-1]
	num := 0
	for _, r := range s[:len(s)-1] {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid interval %q", s)
		}
		num = num*10 + int(r-'0')
	}
	switch unit {
	case 's':
		return num, nil
	case 'm':
		return num * 60, nil
	case 'h':
		return num * 3600, nil
	case 'd':
		return num * 86400, nil
	}
	return 0, fmt.Errorf("invalid interval unit %q", s)
}

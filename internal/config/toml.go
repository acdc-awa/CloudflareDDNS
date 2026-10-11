// Package config 负责配置的加载、校验、解析与存储。
// 提供零第三方依赖的轻量 TOML（子集）解析实现。
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// parseTOML 解析一个简易 TOML 子集（顶层键值对 + 数组）
// 支持：注释(#)、字符串("...")、整数、布尔、字符串数组([...])
// 不支持：嵌套表、多行字符串、日期等高级特性。
func parseTOML(data []byte) (map[string]string, map[string][]string, error) {
	scalars := make(map[string]string)
	arrays := make(map[string][]string)

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			// 顶层数组也能出现 [ xxx ] 形式，但这里作为 table 头处理则跳过
			if strings.HasSuffix(line, "]") && !strings.Contains(line, "=") {
				// table 头：本实现忽略嵌套表
				continue
			}
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			return nil, nil, fmt.Errorf("行 %d: 无法解析: %q", lineNo, line)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if key == "" {
			return nil, nil, fmt.Errorf("行 %d: 空键", lineNo)
		}

		if strings.HasPrefix(val, "[") {
			if !strings.HasSuffix(val, "]") {
				return nil, nil, fmt.Errorf("行 %d: 数组缺少右括号", lineNo)
			}
			inner := strings.TrimSuffix(strings.TrimPrefix(val, "["), "]")
			inner = strings.TrimSpace(inner)
			if inner == "" {
				arrays[key] = []string{}
				continue
			}
			parts := splitArrayItems(inner)
			for _, p := range parts {
				arrays[key] = append(arrays[key], unquote(p))
			}
			continue
		}

		scalars[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return scalars, arrays, nil
}

// splitArrayItems 将数组内容按逗号分隔（忽略引号内逗号，简化处理）。
func splitArrayItems(s string) []string {
	var items []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ',' && !inQuote:
			items = append(items, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		items = append(items, strings.TrimSpace(cur.String()))
	}
	return items
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return strings.ReplaceAll(s[1:len(s)-1], "\\\"", "\"")
	}
	return s
}

// quote 为字符串加引号并转义。
func quote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}

// encodeTOML 将配置序列化为 TOML 格式。
func encodeTOML(c *Config) string {
	var b strings.Builder
	b.WriteString("# Cloudflare DDNS 配置文件\n")
	b.WriteString("# 由 ddns config 命令管理\n\n")
	b.WriteString(fmt.Sprintf("cf_api_token = %s\n", quote(c.CFApiToken)))
	b.WriteString(fmt.Sprintf("zone_name = %s\n", quote(c.ZoneName)))
	b.WriteString(fmt.Sprintf("record_name = %s\n", quote(c.RecordName)))
	b.WriteString(fmt.Sprintf("record_ttl = %d\n", c.RecordTTL))
	b.WriteString(fmt.Sprintf("check_interval = %s\n", quote(c.CheckInterval)))

	if len(c.IPSources) == 0 {
		b.WriteString("ip_sources = []\n")
	} else {
		var arr []string
		for _, s := range c.IPSources {
			arr = append(arr, quote(s))
		}
		b.WriteString(fmt.Sprintf("ip_sources = [%s]\n", strings.Join(arr, ", ")))
	}
	return b.String()
}

// parseInt 解析整数字面量。
func parseInt(s string) (int64, error) {
	return strconv.ParseInt(strings.Trim(s, "\""), 10, 64)
}

// loadFromTOML 从内存数据填充 Config。
func loadFromTOML(data []byte) (*Config, error) {
	scalars, arrays, err := parseTOML(data)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	var hasToken, hasZone, hasRecord bool

	if v, ok := scalars["cf_api_token"]; ok {
		cfg.CFApiToken = unquote(v)
		hasToken = true
	}
	if v, ok := scalars["zone_name"]; ok {
		cfg.ZoneName = unquote(v)
		hasZone = true
	}
	if v, ok := scalars["record_name"]; ok {
		cfg.RecordName = unquote(v)
		hasRecord = true
	}
	if v, ok := scalars["record_ttl"]; ok {
		n, err := parseInt(v)
		if err != nil {
			return nil, fmt.Errorf("record_ttl: %w", err)
		}
		cfg.RecordTTL = int(n)
	}
	if v, ok := scalars["check_interval"]; ok {
		cfg.CheckInterval = unquote(v)
	}
	if v, ok := scalars["ip_sources"]; ok {
		cfg.IPSources = []string{unquote(v)}
	}
	if v, ok := arrays["ip_sources"]; ok {
		cfg.IPSources = v
	}
	if _, ok := scalars["log_level"]; ok {
		cfg.LogLevel = unquote(scalars["log_level"])
	}

	if !hasToken || !hasZone || !hasRecord {
		return nil, fmt.Errorf("配置缺少必填字段 (cf_api_token/zone_name/record_name)")
	}
	return cfg, nil
}

// Load 从配置文件加载配置。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, err := loadFromTOML(data)
	if err != nil {
		return nil, fmt.Errorf("解析 %s: %w", path, err)
	}
	cfg.sourcePath = path
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save 将配置写入文件（权限 0600）。
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	data := []byte(encodeTOML(c))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	c.sourcePath = path
	return nil
}

func dirOf(path string) string {
	i := lastIndexByte(path, '/')
	if i < 0 {
		i = lastIndexByte(path, '\\')
	}
	if i < 0 {
		return "."
	}
	return path[:i]
}

func lastIndexByte(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}

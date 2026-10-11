package config

import (
	"strings"
	"testing"
)

func TestParseTOMLBasic(t *testing.T) {
	data := []byte(`
# comment
cf_api_token = "abc"
zone_name = "example.com"
record_ttl = 120
check_interval = "5m"
ip_sources = ["https://a", "https://b"]
`)
	scalars, arrays, err := parseTOML(data)
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	if scalars["cf_api_token"] != "\"abc\"" {
		t.Errorf("cf_api_token = %q", scalars["cf_api_token"])
	}
	if scalars["check_interval"] != "\"5m\"" {
		t.Errorf("check_interval = %q", scalars["check_interval"])
	}
	if len(arrays["ip_sources"]) != 2 {
		t.Fatalf("ip_sources len = %d, want 2", len(arrays["ip_sources"]))
	}
}

func TestLoadFromTOML_HappyPath(t *testing.T) {
	data := []byte(`
cf_api_token = "abcdefghijklmnopqrstuvwxyz0123456789abcdef"
zone_name = "example.com"
record_name = "ddns.example.com"
record_ttl = 120
check_interval = "5m"
ip_sources = ["https://api.ipify.org"]
`)
	cfg, err := loadFromTOML(data)
	if err != nil {
		t.Fatalf("loadFromTOML error: %v", err)
	}
	if cfg.ZoneName != "example.com" {
		t.Errorf("zone = %q", cfg.ZoneName)
	}
	if !strings.Contains(strings.Join(cfg.IPSources, ","), "api.ipify.org") {
		t.Errorf("ip_sources = %v", cfg.IPSources)
	}
	// 缺省字段会自动填默认
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("loglevel = %q", cfg.LogLevel)
	}
}

func TestValidate_RejectsBadRecord(t *testing.T) {
	data := []byte(`
cf_api_token = "abcdefghijklmnopqrstuvwxyz0123456789abcdef"
zone_name = "example.com"
record_name = "ddns.other.com"
`)
	cfg, err := loadFromTOML(data)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected validation error for mismatched record/zone")
	}
}

func TestValidate_RejectsBadTTL(t *testing.T) {
	data := []byte(`
cf_api_token = "abcdefghijklmnopqrstuvwxyz0123456789abcdef"
zone_name = "example.com"
record_name = "ddns.example.com"
record_ttl = 50
`)
	cfg, _ := loadFromTOML(data)
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected validation error for ttl=50")
	}
}

func TestIntervalDuration(t *testing.T) {
	c := &Config{CheckInterval: "10m"}
	d, err := c.IntervalDuration()
	if err != nil || d != 600 {
		t.Fatalf("10m -> %d, err=%v", d, err)
	}
	c2 := &Config{CheckInterval: "2h"}
	d2, _ := c2.IntervalDuration()
	if d2 != 7200 {
		t.Fatalf("2h -> %d", d2)
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	orig := &Config{
		CFApiToken:    "abcdefghijklmnopqrstuvwxyz0123456789abcdef",
		ZoneName:      "example.com",
		RecordName:    "ddns.example.com",
		RecordTTL:     300,
		CheckInterval: "30m",
		IPSources:     []string{"https://a.example", "https://b.example"},
		LogLevel:      "info",
	}
	s := encodeTOML(orig)
	cfg, err := loadFromTOML([]byte(s))
	if err != nil {
		t.Fatalf("roundtrip load error: %v\n%s", err, s)
	}
	if cfg.RecordTTL != 300 || cfg.CheckInterval != "30m" || len(cfg.IPSources) != 2 {
		t.Errorf("roundtrip mismatch: %+v", cfg)
	}
}

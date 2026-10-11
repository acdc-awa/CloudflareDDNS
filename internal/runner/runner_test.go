package runner

import (
	"context"
	"errors"
	"testing"

	"ddns/internal/cloudflare"
	"ddns/internal/config"
	"ddns/internal/logx"
)

// mockClient 记录调用并返回可编程结果。
type mockClient struct {
	zoneID       string
	findResult   *cloudflare.DNSRecord
	findErr      error
	createCalled bool
	updateCalled bool
	createdAt    string
	updatedAt    string
}

func (m *mockClient) ZoneID(_ context.Context, _ string) (string, error) { return m.zoneID, nil }
func (m *mockClient) FindARecord(_ context.Context, _, _ string) (*cloudflare.DNSRecord, error) {
	return m.findResult, m.findErr
}
func (m *mockClient) CreateARecord(_ context.Context, _, _ , content string, _ int) error {
	m.createCalled = true
	m.createdAt = content
	return nil
}
func (m *mockClient) UpdateARecord(_ context.Context, _, _, _ , content string, _ int) error {
	m.updateCalled = true
	m.updatedAt = content
	return nil
}

func fakeDetect(ip string) PublicIPFn {
	return func(context.Context, []string) (string, error) { return ip, nil }
}

func cfgFor(t *testing.T) *config.Config {
	t.Helper()
	c := &config.Config{
		CFApiToken:    "abcdefghijklmnopqrstuvwxyz0123456789abcdef",
		ZoneName:      "example.com",
		RecordName:    "ddns.example.com",
		RecordTTL:     120,
		CheckInterval: "5m",
		IPSources:     []string{"https://api.ipify.org"},
		LogLevel:      "info",
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("cfg validate: %v", err)
	}
	return c
}

func TestRun_CreatesWhenMissing(t *testing.T) {
	_ = logx.Init("")
	defer logx.Close()
	mc := &mockClient{zoneID: "z1", findResult: nil}
	cfg := cfgFor(t)
	res, err := RunWith(context.Background(), cfg, mc, fakeDetect("1.2.3.4"))
	if err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if res.Action != "created" || !mc.createCalled {
		t.Errorf("expected created, got %s (create=%v)", res.Action, mc.createCalled)
	}
	if mc.createdAt != "1.2.3.4" {
		t.Errorf("created content = %s", mc.createdAt)
	}
}

func TestRun_NoChange(t *testing.T) {
	_ = logx.Init("")
	defer logx.Close()
	mc := &mockClient{zoneID: "z1", findResult: &cloudflare.DNSRecord{ID: "r1", Content: "5.6.7.8"}}
	cfg := cfgFor(t)
	res, err := RunWith(context.Background(), cfg, mc, fakeDetect("5.6.7.8"))
	if err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if res.Action != "nochange" {
		t.Errorf("expected nochange, got %s", res.Action)
	}
	if mc.createCalled || mc.updateCalled {
		t.Errorf("should not call create/update on nochange")
	}
}

func TestRun_UpdatesWhenChanged(t *testing.T) {
	_ = logx.Init("")
	defer logx.Close()
	mc := &mockClient{zoneID: "z1", findResult: &cloudflare.DNSRecord{ID: "r1", Content: "1.1.1.1"}}
	cfg := cfgFor(t)
	res, err := RunWith(context.Background(), cfg, mc, fakeDetect("9.9.9.9"))
	if err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if res.Action != "updated" || !mc.updateCalled {
		t.Errorf("expected updated, got %s (update=%v)", res.Action, mc.updateCalled)
	}
	if mc.updatedAt != "9.9.9.9" {
		t.Errorf("updated content = %s", mc.updatedAt)
	}
	if res.OldIP != "1.1.1.1" || res.NewIP != "9.9.9.9" {
		t.Errorf("old/new = %s/%s", res.OldIP, res.NewIP)
	}
}

func TestRun_IPDetectionFailure(t *testing.T) {
	_ = logx.Init("")
	defer logx.Close()
	mc := &mockClient{zoneID: "z1"}
	cfg := cfgFor(t)
	fail := func(context.Context, []string) (string, error) { return "", errors.New("no net") }
	if _, err := RunWith(context.Background(), cfg, mc, fail); err == nil {
		t.Fatalf("expected error when IP detection fails")
	}
}

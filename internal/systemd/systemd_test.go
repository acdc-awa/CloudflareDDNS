package systemd

import (
	"strings"
	"testing"
)

func TestServiceUnit(t *testing.T) {
	u := ServiceUnit("/usr/local/bin/ddns", "5m")
	for _, want := range []string{
		"Description=Cloudflare DDNS Updater",
		"Type=oneshot",
		"ExecStart=/usr/local/bin/ddns run-once",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("service unit 缺少 %q:\n%s", want, u)
		}
	}
}

func TestTimerUnit(t *testing.T) {
	u := TimerUnit("10m")
	for _, want := range []string{
		"Description=Cloudflare DDNS Timer",
		"Requires=ddns.service",
		"OnUnitActiveSec=10m",
		"Persistent=true",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("timer unit 缺少 %q:\n%s", want, u)
		}
	}
}

func TestTimerUnitMinutePreset(t *testing.T) {
	u := TimerUnit("1m")
	if !strings.Contains(u, "OnUnitActiveSec=1m") {
		t.Errorf("1m not reflected: %s", u)
	}
}

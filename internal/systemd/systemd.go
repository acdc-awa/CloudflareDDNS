// Package systemd 生成并管理 systemd service/timer 单元文件。
package systemd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ddns/internal/logx"
)

// ServiceName 服务名。
const ServiceName = "ddns"

const timerIntervalDefault = "5m"

// Paths 定义单元文件路径与二进制路径。
type Paths struct {
	BinaryPath string // 如 /usr/local/bin/ddns
	ServiceFile string // 如 /etc/systemd/system/ddns.service
	TimerFile   string // 如 /etc/systemd/system/ddns.timer
}

// DefaultPaths 返回遵循 FHS 的默认路径。
func DefaultPaths(binaryPath string) Paths {
	return Paths{
		BinaryPath: binaryPath,
		ServiceFile: "/etc/systemd/system/ddns.service",
		TimerFile:   "/etc/systemd/system/ddns.timer",
	}
}

// ServiceUnit 生成 service 单元内容。
func ServiceUnit(binaryPath, checkInterval string) string {
	return fmt.Sprintf(`[Unit]
Description=Cloudflare DDNS Updater
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=%s run-once
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, binaryPath)
}

// TimerUnit 生成 timer 单元内容。
func TimerUnit(checkInterval string) string {
	if checkInterval == "" {
		checkInterval = timerIntervalDefault
	}
	return fmt.Sprintf(`[Unit]
Description=Cloudflare DDNS Timer
Requires=ddns.service

[Timer]
OnUnitActiveSec=%s
RandomizedDelaySec=15
Persistent=true

[Install]
WantedBy=timers.target
`, checkInterval)
}

// Install 写入 service/timer 单元并启用 timer。
func Install(p Paths, checkInterval string) error {
	service := ServiceUnit(p.BinaryPath, checkInterval)
	if err := writeUnit(p.ServiceFile, service); err != nil {
		return err
	}
	if err := writeUnit(p.TimerFile, TimerUnit(checkInterval)); err != nil {
		return err
	}

	if err := run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := run("systemctl", "enable", ServiceName+".timer"); err != nil {
		return err
	}
	if err := run("systemctl", "start", ServiceName+".timer"); err != nil {
		return err
	}
	logx.Info("定时器已启用", "interval", checkInterval)
	return nil
}

// WriteOnly 仅写入单元文件而不启用（用于 dry-run/测试或 install 前的校验）。
func WriteOnly(p Paths, checkInterval string) error {
	if err := writeUnit(p.ServiceFile, ServiceUnit(p.BinaryPath, checkInterval)); err != nil {
		return err
	}
	return writeUnit(p.TimerFile, TimerUnit(checkInterval))
}

// Uninstall 停止并禁用 timer，删除单元文件，重载 daemon。
func Uninstall(p Paths) error {
	_ = run("systemctl", "stop", ServiceName+".timer")
	_ = run("systemctl", "disable", ServiceName+".timer")
	_ = os.Remove(p.ServiceFile)
	_ = os.Remove(p.TimerFile)
	if err := run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	logx.Info("定时服务已卸载")
	return nil
}

// IsInstalled 判断 timer 单元文件是否存在。
func IsInstalled(p Paths) bool {
	_, err := os.Stat(p.TimerFile)
	return err == nil
}

// TimerStatus 查询 timer 状态信息。
func TimerStatus(p Paths) (map[string]string, error) {
	if !IsInstalled(p) {
		return map[string]string{"installed": "false"}, nil
	}
	out, err := runOutput("systemctl", "show", ServiceName+".timer", "-p", "ActiveState", "-p", "UnitFileState", "-p", "NextElapseUSecRealtime", "-p", "LastTriggerUSec")
	if err != nil {
		return map[string]string{"installed": "true", "error": err.Error()}, nil
	}
	m := map[string]string{"installed": "true"}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		kv := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(kv) == 2 {
			m[kv[0]] = kv[1]
		}
	}
	return m, nil
}

func writeUnit(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s 失败: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// EnsureRoot 检查是否以 root 运行（安装/卸载需要）。
func EnsureRoot() error {
	if os.Geteuid() != 0 {
		return errors.New("该操作需要 root 权限，请使用 sudo 运行")
	}
	return nil
}

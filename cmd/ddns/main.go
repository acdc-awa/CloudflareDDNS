// ddns — Cloudflare 动态 DNS 更新工具（Go 实现）。
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"ddns/internal/cloudflare"
	"ddns/internal/config"
	"ddns/internal/logx"
	"ddns/internal/runner"
	"ddns/internal/systemd"
)

var version = "0.1.0"

// paths 计算二进制、配置、日志及 systemd 单元路径。
// 可通过环境变量覆盖（便于非标准目录/测试）。
type paths struct {
	ConfigFile  string
	LogFile     string
	BinaryPath  string
	ServiceFile string
	TimerFile   string
}

func defaultPaths() paths {
	return paths{
		ConfigFile:  envWithDefault("DDNS_CONFIG_PATH", "/etc/ddns/config.toml"),
		LogFile:     envWithDefault("DDNS_LOG_PATH", "/var/log/ddns.log"),
		BinaryPath:  envWithDefault("DDNS_BIN_PATH", "/usr/local/bin/ddns"),
		ServiceFile: envWithDefault("DDNS_SERVICE_PATH", "/etc/systemd/system/ddns.service"),
		TimerFile:   envWithDefault("DDNS_TIMER_PATH", "/etc/systemd/system/ddns.timer"),
	}
}

// systemdPaths 将 paths 转为 systemd.Paths。
func (p paths) systemdPaths() systemd.Paths {
	return systemd.Paths{
		BinaryPath:  p.BinaryPath,
		ServiceFile: p.ServiceFile,
		TimerFile:   p.TimerFile,
	}
}

func envWithDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}
	ctx := context.Background()
	p := defaultPaths()

	cmd := os.Args[1]
	switch cmd {
	case "help", "-h", "--help":
		printHelp()
	case "version", "-v", "--version":
		fmt.Println("ddns " + version)
	case "run-once":
		cmdRunOnce(ctx, p)
	case "install":
		cmdInstall(ctx, p)
	case "install-write-only":
		cmdInstallWriteOnly(p)
	case "uninstall":
		cmdUninstall(p)
	case "status":
		cmdStatus(ctx, p)
	case "config":
		cmdConfig(p)
	case "log":
		cmdLog(p)
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", cmd)
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Print(`ddns - Cloudflare 动态 DNS 更新工具

用法:
  ddns install            生成并启用 systemd service+timer（含首次配置向导）
  ddns run-once           静默执行一次 DDNS 更新（由 systemd timer 调用）
  ddns config             交互式配置向导
  ddns status             查看配置、定时器与 DNS 状态
  ddns log                查看最近日志
  ddns uninstall          停止并卸载 systemd 服务与配置
  ddns install-write-only 仅生成单元文件，不启用（供诊断/审查）
  ddns version            显示版本
  ddns help               显示此帮助

路径环境变量覆盖:
  DDNS_CONFIG_PATH  配置文件（默认 /etc/ddns/config.toml）
  DDNS_LOG_PATH     日志文件（默认 /var/log/ddns.log）
  DDNS_BIN_PATH     二进制路径（默认 /usr/local/bin/ddns）
`)
}

// cmdRunOnce 执行单次更新（无交互输出，仅供日志）。
func cmdRunOnce(ctx context.Context, p paths) {
	_ = logx.Init(p.LogFile)
	defer logx.Close()

	cfg, err := config.Load(p.ConfigFile)
	if err != nil {
		logx.Error("配置加载失败", "err", err)
		fmt.Fprintln(os.Stderr, "配置加载失败:", err)
		os.Exit(1)
	}
	res, err := runner.Run(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "更新失败:", err)
		os.Exit(1)
	}
	fmt.Println(res.Describe())
	fmt.Printf("Public IP:   %s\n", res.PublicIP)
}

// cmdStatus 打印状态信息。
func cmdStatus(ctx context.Context, p paths) {
	fmt.Println("========== DDNS 状态 ==========")

	cfg, err := config.Load(p.ConfigFile)
	if err != nil {
		fmt.Println("⚠ 配置不可用:", err)
	} else {
		fmt.Printf("API Token:   %s\n", mask(cfg.CFApiToken, 8, 4))
		fmt.Printf("主域名:      %s\n", cfg.ZoneName)
		fmt.Printf("二级域名:    %s\n", cfg.RecordName)
		fmt.Printf("TTL:         %d\n", cfg.RecordTTL)
		fmt.Printf("检查间隔:    %s\n", cfg.CheckInterval)
		fmt.Printf("IP 源数量:   %d\n", len(cfg.IPSources))
	}
	fmt.Println()

	fmt.Println("--- 定时器状态 ---")
	sp := p.systemdPaths()
	if st, err := systemd.TimerStatus(sp); err == nil && st["installed"] == "true" {
		fmt.Printf("已安装:      %s\n", boolText(true))
		if v, ok := st["ActiveState"]; ok {
			fmt.Printf("ActiveState: %s\n", v)
		}
		if v, ok := st["UnitFileState"]; ok {
			fmt.Printf("UnitFile:    %s\n", v)
		}
		if v, ok := st["NextElapseUSecRealtime"]; ok && v != "" && v != "n/a" {
			fmt.Printf("下次触发:    %s\n", v)
		}
	} else {
		fmt.Println("未安装（使用 ddns install 启用定时刷新）")
	}

	fmt.Println()
	fmt.Println("--- 日志（最近） ---")
	if out, rerr := os.ReadFile(p.LogFile); rerr == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		n := len(lines)
		if n > 20 {
			lines = lines[n-20:]
		}
		for _, l := range lines {
			if l != "" {
				fmt.Println(" ", l)
			}
		}
	} else {
		fmt.Println("  日志文件不存在")
	}
}

// cmdInstall 交互式配置 + 安装 systemd 服务。
func cmdInstall(ctx context.Context, p paths) {
	if err := systemd.EnsureRoot(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cfg, err := config.Load(p.ConfigFile)
	if err != nil {
		fmt.Println("未检测到有效配置，进入配置向导...")
		cfg = wizard()
		if cfg == nil {
			return
		}
		if err := cfg.Save(p.ConfigFile); err != nil {
			fmt.Fprintln(os.Stderr, "保存配置失败:", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("已加载已有配置: %s\n", p.ConfigFile)
		showSummary(cfg)
	}
	_ = ctx

	sp := p.systemdPaths()
	if err := systemd.Install(sp, cfg.CheckInterval); err != nil {
		fmt.Fprintln(os.Stderr, "安装失败:", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("✓ 安装完成")
	fmt.Printf("  配置:       %s\n", p.ConfigFile)
	fmt.Printf("  当前公网IP: %s\n", quickPublicIP(ctx, cfg))
	fmt.Println("  查看状态:   ddns status")
	fmt.Println("  查看日志:   journalctl -u ddns 或 ddns log")
	fmt.Println("  立即测试:   ddns run-once")
}

// cmdInstallWriteOnly 仅写入单元文件（供审查），不要求 root 的 systemctl 操作。
func cmdInstallWriteOnly(p paths) {
	cfg, err := config.Load(p.ConfigFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "请先提供有效配置:", err)
		os.Exit(1)
	}
	sp := p.systemdPaths()
	if err := systemd.WriteOnly(sp, cfg.CheckInterval); err != nil {
		fmt.Fprintln(os.Stderr, "写入失败:", err)
		os.Exit(1)
	}
	fmt.Printf("✓ 单元文件已写入:\n  %s\n  %s\n", sp.ServiceFile, sp.TimerFile)
}

// cmdUninstall 卸载 service/timer 与配置。
func cmdUninstall(p paths) {
	fmt.Println("将执行卸载：停止并删除 ddns.timer、移除配置文件。")
	fmt.Printf("确认? 输入 yes: ")
	var in string
	_, _ = fmt.Scanln(&in)
	if strings.TrimSpace(in) != "yes" {
		fmt.Println("已取消。")
		return
	}
	if err := systemd.EnsureRoot(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sp := p.systemdPaths()
	if err := systemd.Uninstall(sp); err != nil {
		fmt.Fprintln(os.Stderr, "卸载 systemd 失败:", err)
	}
	_ = os.Remove(p.ConfigFile)
	fmt.Println("✓ 卸载完成。配置已删除，日志保留于", p.LogFile)
}

// cmdLog 查看最近日志。
func cmdLog(p paths) {
	if out, err := os.ReadFile(p.LogFile); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		n := len(lines)
		if n > 30 {
			lines = lines[n-30:]
		}
		for _, l := range lines {
			if l != "" {
				fmt.Println(l)
			}
		}
	} else {
		fmt.Println("日志文件不存在:", p.LogFile)
	}
}

// cmdConfig 交互式修改配置。
func cmdConfig(p paths) {
	cfg, err := config.Load(p.ConfigFile)
	var nc *config.Config
	if err != nil {
		fmt.Println("未检测到配置，进入配置向导...")
		nc = wizard()
	} else {
		nc = editWizard(cfg)
	}
	if nc == nil {
		return
	}
	if err := nc.Save(p.ConfigFile); err != nil {
		fmt.Fprintln(os.Stderr, "保存配置失败:", err)
		os.Exit(1)
	}
	fmt.Println("✓ 配置已保存:", p.ConfigFile)
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

func mask(s string, head, tail int) string {
	if len(s) <= head+tail {
		return "****"
	}
	return s[:head] + "..." + s[len(s)-tail:]
}

func boolText(b bool) string {
	if b {
		return "是"
	}
	return "否"
}

func showSummary(c *config.Config) {
	fmt.Println("--- 当前配置 ---")
	fmt.Printf("  主域名:      %s\n", c.ZoneName)
	fmt.Printf("  二级域名:    %s\n", c.RecordName)
	fmt.Printf("  TTL:         %d\n", c.RecordTTL)
	fmt.Printf("  检查间隔:    %s\n", c.CheckInterval)
}

func quickPublicIP(ctx context.Context, c *config.Config) string {
	ip, err := cloudflare.DetectPublicIP(ctx, c.IPSources)
	if err != nil {
		return "无法获取"
	}
	return ip
}

// prompt 读取一行输入。
func prompt(label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	var in string
	_, _ = fmt.Scanln(&in)
	in = strings.TrimSpace(in)
	if in == "" {
		return def
	}
	return in
}

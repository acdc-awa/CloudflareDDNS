#!/usr/bin/env bash
# ==============================================================================
# Cloudflare DDNS 一键部署与管理脚本
# GitHub: https://github.com/acdc-awa/CloudflareDDNS
# ==============================================================================

set -euo pipefail

# ----------------- 配置变量 -----------------
REPO="acdc-awa/CloudflareDDNS"
BIN_NAME="ddns"
INSTALL_DIR="/usr/local/bin"
TARGET_BIN="${INSTALL_DIR}/${BIN_NAME}"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# ----------------- 帮助函数 -----------------
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

# 引导欢迎界面
show_banner() {
    local target_arch="$1"
    echo -e "${BOLD}${CYAN}================================================================${NC}"
    echo -e "${BOLD}${CYAN}           Cloudflare DDNS 自动化部署与配置向导            ${NC}"
    echo -e "${CYAN}   开源仓库: https://github.com/${REPO}${NC}"
    echo -e "${BOLD}${CYAN}================================================================${NC}"
    echo -e "  系统环境: ${GREEN}Linux (${target_arch})${NC}"
    echo -e "  安装目录: ${GREEN}${TARGET_BIN}${NC}"
    echo -e "${CYAN}----------------------------------------------------------------${NC}"
    echo ""
}

# 检查权限
check_root() {
    if [[ "$(id -u)" -ne 0 ]]; then
        log_error "本脚本需要 root 权限以配置 systemd 服务与系统目录。"
        log_info "请使用 sudo 重新运行: sudo bash $0 $*"
        exit 1
    fi
}

# 系统与环境检查
check_system() {
    local os
    os="$(uname -s)"
    if [[ "${os}" != "Linux" ]]; then
        log_error "此 DDNS 工具仅支持 Linux 系统（当前为: ${os}）。"
        exit 1
    fi

    if ! command -v systemctl >/dev/null 2>&1 || [[ ! -d /run/systemd/system ]]; then
        log_error "未检测到运行中的 systemd，该工具依赖 systemd service & timer 托管运行。"
        exit 1
    fi
}

# 架构识别
detect_arch() {
    local arch
    arch="$(uname -m)"
    case "${arch}" in
        x86_64|amd64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            log_error "不支持的 CPU 架构: ${arch}（仅支持 x86_64/amd64 与 aarch64/arm64）"
            exit 1
            ;;
    esac
}

# 下载文件工具
download_file() {
    local url="$1"
    local dest="$2"

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "${url}" -o "${dest}"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "${dest}" "${url}"
    else
        log_error "未找到 curl 或 wget，请先安装其中之一。"
        exit 1
    fi
}

# 版本选择引导
select_version() {
    local specified_version="${1:-}"

    if [[ -n "${specified_version}" ]]; then
        if [[ "${specified_version}" =~ ^[0-9] ]]; then
            specified_version="v${specified_version}"
        fi
        echo "${specified_version}"
        return 0
    fi

    echo -e "${BOLD}请选择要安装的版本：${NC}"
    echo -e "  - 直接按回车将默认安装最新版本: ${GREEN}latest${NC}"
    echo -e "  - 或手动输入特定 Release 版本号 (例如: ${YELLOW}v0.1.0${NC})"
    echo ""

    local chosen_version=""
    if [[ -t 0 ]]; then
        read -r -p "请输入版本号 [默认: latest]: " chosen_version
    elif [[ -e /dev/tty ]]; then
        read -r -p "请输入版本号 [默认: latest]: " chosen_version < /dev/tty
    else
        chosen_version="latest"
    fi

    # 清除前后空白字符
    chosen_version="$(echo "${chosen_version}" | tr -d '[:space:]')"

    if [[ -z "${chosen_version}" || "${chosen_version}" == "latest" ]]; then
        echo "latest"
        return 0
    fi

    # 规范化版本号：如果以数字开头（如 0.1.0），自动补全为 v0.1.0
    if [[ "${chosen_version}" =~ ^[0-9] ]]; then
        chosen_version="v${chosen_version}"
    fi

    echo "${chosen_version}"
}

# 获取二进制文件（仅从 GitHub Releases 下载）
get_binary() {
    local target_arch="$1"
    local version="$2"
    local temp_bin
    temp_bin="$(mktemp /tmp/ddns-bin.XXXXXX)"

    local release_url=""
    if [[ "${version}" == "latest" ]]; then
        release_url="https://github.com/${REPO}/releases/latest/download/ddns-linux-${target_arch}"
    else
        release_url="https://github.com/${REPO}/releases/download/${version}/ddns-linux-${target_arch}"
    fi

    log_info "准备安装版本: ${GREEN}${version}${NC}"
    log_info "正在从 GitHub Release 下载二进制文件: ${release_url} ..."

    if ! download_file "${release_url}" "${temp_bin}"; then
        rm -f "${temp_bin}"
        echo ""
        log_error "下载失败！可能原因："
        echo "  1. GitHub Release 不存在版本 '${version}' 或该 Release 未包含架构 (${target_arch}) 的产物。"
        echo "  2. 服务器无法连通 GitHub 网络（可尝试配置代理或镜像）。"
        echo "  3. 该版本的 GitHub Actions 还在构建中，请稍后重试。"
        exit 1
    fi

    chmod +x "${temp_bin}"
    echo "${temp_bin}"
}

# 执行安装
do_install() {
    local target_ver="${1:-}"

    check_root
    check_system

    local target_arch
    target_arch="$(detect_arch)"

    # 显示欢迎与系统信息面板
    show_banner "${target_arch}"

    # 引导用户选择版本
    local version
    version="$(select_version "${target_ver}")"
    echo ""

    local bin_source
    bin_source="$(get_binary "${target_arch}" "${version}")"

    # 安装到系统路径
    log_info "正在安装二进制文件至 ${TARGET_BIN} ..."
    install -m 755 "${bin_source}" "${TARGET_BIN}"
    rm -f "${bin_source}"

    log_success "二进制文件安装成功。"

    # 触发交互式配置向导及 systemd 注册
    echo ""
    log_info "启动 DDNS 初始化配置向导..."
    echo "--------------------------------------------------"

    # 处理从管道传递 (curl ... | bash) 导致的 stdin 丢失问题
    if [[ -t 0 ]]; then
        "${TARGET_BIN}" install
    elif [[ -e /dev/tty ]]; then
        "${TARGET_BIN}" install < /dev/tty
    else
        log_warn "未检测到交互终端 (TTY)，跳过交互配置向导。"
        log_info "请稍后手动运行以下命令完成配置与服务启动："
        echo -e "${CYAN}sudo ddns install${NC}"
    fi

    echo "--------------------------------------------------"
    log_success "一键部署流程完成！"
    echo ""
    echo -e "常用管理命令："
    echo -e "  查看运行状态:   ${CYAN}ddns status${NC}"
    echo -e "  查看近期日志:   ${CYAN}ddns log${NC}"
    echo -e "  修改配置参数:   ${CYAN}ddns config${NC}"
    echo -e "  手动执行一次:   ${CYAN}ddns run-once${NC}"
    echo -e "  卸载服务与工具: ${CYAN}sudo bash $0 uninstall${NC} 或 ${CYAN}sudo ddns uninstall${NC}"
}

# 执行卸载
do_uninstall() {
    check_root
    log_info "正在停止并卸载 Cloudflare DDNS..."

    if [[ -x "${TARGET_BIN}" ]]; then
        "${TARGET_BIN}" uninstall || true
        rm -f "${TARGET_BIN}"
    fi

    # 清理残留服务文件（如有）
    rm -f /etc/systemd/system/ddns.service /etc/systemd/system/ddns.timer
    systemctl daemon-reload >/dev/null 2>&1 || true

    log_success "卸载完成！"
}

# 执行更新
do_update() {
    local target_ver="${1:-}"

    check_root
    check_system

    local target_arch
    target_arch="$(detect_arch)"

    show_banner "${target_arch}"
    log_info "更新模式："

    local version
    version="$(select_version "${target_ver}")"
    echo ""

    local bin_source
    bin_source="$(get_binary "${target_arch}" "${version}")"

    install -m 755 "${bin_source}" "${TARGET_BIN}"
    rm -f "${bin_source}"

    if systemctl is-active --quiet ddns.timer; then
        log_info "重启 ddns.timer 定时器..."
        systemctl restart ddns.timer
    fi

    log_success "更新成功！当前版本: $("${TARGET_BIN}" version)"
}

# 帮助说明
show_help() {
    cat << EOF
Cloudflare DDNS 一键管理脚本

用法:
  sudo bash deploy.sh [命令] [版本号]

支持命令:
  install [版本]  引导选择版本并安装，启动配置向导与 systemd 定时服务（默认）
  update  [版本]  选择/下载指定版本二进制并就地升级
  uninstall       停止并卸载 systemd 服务与相关文件
  help            显示本帮助信息

一键安装指令 (远程管道):
  bash <(curl -fsSL https://raw.githubusercontent.com/${REPO}/main/deploy.sh)
EOF
}

# ----------------- 命令行解析 -----------------
ACTION="${1:-install}"
ARG_VERSION="${2:-}"

case "${ACTION}" in
    install)
        do_install "${ARG_VERSION}"
        ;;
    uninstall)
        do_uninstall
        ;;
    update)
        do_update "${ARG_VERSION}"
        ;;
    help|--help|-h)
        show_help
        ;;
    v*|[0-9]*)
        # 允许直接执行: sudo bash deploy.sh v0.1.0
        do_install "${ACTION}"
        ;;
    *)
        log_error "未知指令: ${ACTION}"
        show_help
        exit 1
        ;;
esac

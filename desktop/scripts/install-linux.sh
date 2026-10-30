#!/usr/bin/env bash
# CrazyNote 桌面客户端 —— Linux 安装脚本
#
# 用法：
#   ./install.sh              # 安装到 ~/.local/CrazyNote（用户级，无需 root）
#   ./install.sh --system     # 安装到 /opt/CrazyNote（系统级，需要 root）
#   ./install.sh --uninstall  # 卸载用户级安装
#
# 说明：Electron 的 chrome-sandbox 需要 setuid root 才能启用沙箱。
# 用户级安装无法设置 setuid，因此会自动移除该文件并以 --no-sandbox 启动。
# 系统级安装（root）会正确设置 setuid，保留完整沙箱保护。
set -euo pipefail

APP_NAME="crazynote-desktop"
APP_TITLE="CrazyNote"
VERSION="1.0.1"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# 允许从 release/ 或解压后的目录里直接运行
if [ -d "$SCRIPT_DIR/../resources" ] || [ -f "$SCRIPT_DIR/resources/app.asar" ]; then
  SRC_DIR="$SCRIPT_DIR"
elif [ -d "$SCRIPT_DIR/resources" ]; then
  SRC_DIR="$SCRIPT_DIR"
else
  SRC_DIR="$(dirname "$SCRIPT_DIR")"
fi

MODE="user"
for arg in "$@"; do
  case "$arg" in
    --system) MODE="system" ;;
    --uninstall) MODE="uninstall" ;;
    -h|--help)
      sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
      exit 0 ;;
  esac
done

info() { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
warn() { printf '\033[1;33m警告:\033[0m %s\n' "$1"; }
err()  { printf '\033[1;31m错误:\033[0m %s\n' "$1" >&2; }

# ---------------------------------------------------------------- 卸载
if [ "$MODE" = "uninstall" ]; then
  info "卸载用户级安装"
  rm -rf "$HOME/.local/$APP_NAME"
  rm -f "$HOME/.local/share/applications/$APP_NAME.desktop"
  rm -rf "$HOME/.local/share/icons/hicolor"/*/apps/"$APP_NAME".png
  rm -f "$HOME/.local/bin/$APP_NAME"
  info "已卸载"
  exit 0
fi

# ---------------------------------------------------------------- 校验源目录
if [ ! -f "$SRC_DIR/resources/app.asar" ]; then
  err "在 $SRC_DIR 下找不到 resources/app.asar，请在解压后的 CrazyNote 目录内运行本脚本"
  exit 1
fi

# ---------------------------------------------------------------- 安装
if [ "$MODE" = "system" ]; then
  INSTALL_DIR="/opt/$APP_TITLE"
  BIN_DIR="/usr/local/bin"
  ICON_DIR="/usr/share/icons/hicolor"
  APP_DIR="/usr/share/applications"
  NEED_ROOT=1
else
  INSTALL_DIR="$HOME/.local/$APP_NAME"
  BIN_DIR="$HOME/.local/bin"
  ICON_DIR="$HOME/.local/share/icons/hicolor"
  APP_DIR="$HOME/.local/share/applications"
  NEED_ROOT=0
fi

if [ "$NEED_ROOT" = "1" ] && [ "$(id -u)" != "0" ]; then
  err "系统级安装需要 root 权限，请用 sudo 运行"
  exit 1
fi

info "安装目录: $INSTALL_DIR"
rm -rf "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
# 复制除 chrome-sandbox 外的全部内容
cp -a "$SRC_DIR/." "$INSTALL_DIR/"

SANDBOX_ARGS=""
if [ "$(id -u)" = "0" ]; then
  # 有 root：给 chrome-sandbox 设置 setuid，保留沙箱
  if [ -f "$INSTALL_DIR/chrome-sandbox" ]; then
    chown root:root "$INSTALL_DIR/chrome-sandbox" 2>/dev/null || true
    chmod 4755 "$INSTALL_DIR/chrome-sandbox" 2>/dev/null || true
    info "已为 chrome-sandbox 设置 setuid（沙箱保护生效）"
  fi
else
  # 用户级：无法 setuid，移除并以 --no-sandbox 启动
  if [ -f "$INSTALL_DIR/chrome-sandbox" ]; then
    rm -f "$INSTALL_DIR/chrome-sandbox"
    warn "用户级安装无法设置 setuid，已移除 chrome-sandbox 并使用 --no-sandbox 启动"
  fi
  SANDBOX_ARGS="--no-sandbox"
fi

# ---------------------------------------------------------------- 图标
info "安装图标"
for size in 16 24 32 48 64 128 256 512 1024; do
  src="$SCRIPT_DIR/../build/icons/${size}x${size}.png"
  [ -f "$src" ] || src="$SRC_DIR/build/icons/${size}x${size}.png"
  [ -f "$src" ] || continue
  mkdir -p "$ICON_DIR/${size}x${size}/apps"
  cp -f "$src" "$ICON_DIR/${size}x${size}/apps/$APP_NAME.png"
done

# ---------------------------------------------------------------- 桌面启动器
info "创建应用启动器"
cat > "$APP_DIR/$APP_NAME.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=$APP_TITLE
Name[zh_CN]=CrazyNote 笔记
Comment=连接飞牛 NAS 上的 CrazyNote 服务端
Comment[zh_CN]=连接飞牛 NAS 上的 CrazyNote 服务端
Exec=$INSTALL_DIR/$APP_NAME $SANDBOX_ARGS
Icon=$APP_NAME
Terminal=false
Categories=Utility;NoteTaking;
StartupNotify=true
StartupWMClass=$APP_TITLE
Keywords=notes;markdown;note;crazynote;
EOF
chmod +x "$APP_DIR/$APP_NAME.desktop"

# 命令行启动器
mkdir -p "$BIN_DIR"
cat > "$BIN_DIR/$APP_NAME" <<EOF
#!/usr/bin/env bash
exec "$INSTALL_DIR/$APP_NAME" $SANDBOX_ARGS "\$@"
EOF
chmod +x "$BIN_DIR/$APP_NAME"

# 刷新图标缓存（存在才执行）
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database "$APP_DIR" >/dev/null 2>&1 || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
  gtk-update-icon-cache -f -t ~/.local/share/icons/hicolor >/dev/null 2>&1 || true
fi

echo
info "安装完成 (v$VERSION)"
echo "  启动方式:"
echo "    1. 在应用菜单中搜索 '$APP_TITLE'"
echo "    2. 命令行执行: $APP_NAME"
echo "    3. 直接运行: $INSTALL_DIR/$APP_NAME $SANDBOX_ARGS"
echo
echo "  卸载: $0 --uninstall"

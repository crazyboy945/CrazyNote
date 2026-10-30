#!/usr/bin/env bash
# 在飞牛 NAS 上快速部署 crazynote（裸二进制方式）
# 用法: bash deploy.sh <笔记目录> [端口]
#
# 例: bash deploy.sh /vol1/1000/Notes 8899

set -euo pipefail

NOTES_DIR="${1:-/vol1/1000/Notes}"
PORT="${2:-8899}"
INSTALL_DIR="/opt/crazynote"
DATA_DIR="/var/apps/crazynote/etc"

echo "==> CrazyNote 部署"
echo "    笔记目录: ${NOTES_DIR}"
echo "    端口:     ${PORT}"
echo ""

# 检测架构
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64|amd64)  BIN="crazynote-linux-amd64" ;;
  aarch64|arm64) BIN="crazynote-linux-arm64" ;;
  armv7l)        BIN="crazynote-linux-armv7" ;;
  *)
    echo "不支持的架构: ${ARCH}" >&2
    echo "请手动编译: GOOS=linux GOARCH=${ARCH} go build -o crazynote ./cmd/crazynote" >&2
    exit 1
    ;;
esac
echo "==> 检测到架构: ${ARCH}，使用 ${BIN}"

# 定位二进制：优先当前目录，其次 ./dist
SRC=""
for cand in "./${BIN}" "./dist/${BIN}" "./${BIN}.gz"; do
  if [ -f "${cand}" ]; then SRC="${cand}"; break; fi
done
if [ -z "${SRC}" ]; then
  echo "找不到二进制 ${BIN}，请先把文件放到当前目录或 dist/ 下" >&2
  exit 1
fi

echo "==> 安装到 ${INSTALL_DIR}"
mkdir -p "${INSTALL_DIR}" "${DATA_DIR}" "${NOTES_DIR}"
cp "${SRC}" "${INSTALL_DIR}/crazynote"
chmod +x "${INSTALL_DIR}/crazynote"

echo "==> 注册 systemd 服务"
cat > /etc/systemd/system/crazynote.service <<EOF
[Unit]
Description=crazynote - Markdown 笔记服务
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/crazynote -data ${NOTES_DIR} -appdata ${DATA_DIR} -addr 0.0.0.0:${PORT}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable crazynote
systemctl restart crazynote

sleep 2
if systemctl is-active --quiet crazynote; then
  echo ""
  echo "✅ 部署完成"
  echo ""
  echo "   访问地址: http://$(hostname -I 2>/dev/null | awk '{print $1}'):${PORT}"
  echo "   笔记目录: ${NOTES_DIR}"
  echo ""
  echo "   查看初始口令: journalctl -u crazynote -n 20 --no-pager | grep 口令"
  echo "   服务状态:   systemctl status crazynote"
  echo "   查看日志:   journalctl -u crazynote -f"
else
  echo "❌ 启动失败，请检查: journalctl -u crazynote -n 30 --no-pager" >&2
  exit 1
fi

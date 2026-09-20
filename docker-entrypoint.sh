#!/bin/sh
# ──────────────────────────────────────────────────────────────
# ProbeDeck 探针容器入口
#   • 用环境变量生成/更新 config.conf（面板下发的配置会写回同一文件，
#     所以必须放在可持久化的 volume 上）
#   • 不写 AUTO_UPDATE 字段 → 容器内自更新默认关闭
#     （容器升级 = 重新拉镜像，不该由进程自己替换二进制）
#   • 日志直接走 stdout/stderr，docker logs 即可查看
# ──────────────────────────────────────────────────────────────
set -eu

CONFIG_DIR="${CF_PROBE_CONFIG_DIR:-/etc/cf-probe}"
CONFIG_FILE="${CONFIG_DIR}/config.conf"

: "${SERVER_ID:?请设置 SERVER_ID 环境变量（面板里的服务器 ID）}"
: "${SECRET:?请设置 SECRET 环境变量（面板里的服务器密钥）}"
: "${WORKER_URL:?请设置 WORKER_URL 环境变量（面板上报地址，例如 https://example.com/update）}"

mkdir -p "${CONFIG_DIR}"

# ── 生成配置（KV 格式，与 install.sh 写盘格式一致）────────────────
# 已存在的文件保留（面板远程下发的节点/间隔等改动都在里面），
# 仅当三项必填缺失时才初始化——避免重建容器丢失面板侧配置。
if [ ! -s "${CONFIG_FILE}" ]; then
  cat > "${CONFIG_FILE}" <<EOF
SERVER_ID="${SERVER_ID}"
SECRET="${SECRET}"
WORKER_URL="${WORKER_URL}"
REPORT_INTERVAL="${REPORT_INTERVAL:-60}"
COLLECT_INTERVAL="${COLLECT_INTERVAL:-0}"
CONNECTION_MODE="${CONNECTION_MODE:-auto}"
PING_MODE="${PING_MODE:-tcp}"
RESET_DAY="${RESET_DAY:-1}"
INTERFACE="${INTERFACE:-}"
EOF
fi

# 三项必填以环境变量为准（用户在 docker run 改了值就同步进文件）
update_field() {
  key="$1"; value="$2"
  if grep -q "^${key}=" "${CONFIG_FILE}" 2>/dev/null; then
    sed -i "s|^${key}=.*|${key}=\"${value}\"|" "${CONFIG_FILE}"
  else
    printf '%s="%s"\n' "${key}" "${value}" >> "${CONFIG_FILE}"
  fi
}
update_field SERVER_ID  "${SERVER_ID}"
update_field SECRET      "${SECRET}"
update_field WORKER_URL  "${WORKER_URL}"

# 容器内禁用自更新：删掉历史遗留的 AUTO_UPDATE 字段
if grep -q '^AUTO_UPDATE=' "${CONFIG_FILE}" 2>/dev/null; then
  sed -i '/^AUTO_UPDATE=/d' "${CONFIG_FILE}"
fi

DEBUG_FLAG="-debug=0"
if [ "${DEBUG:-0}" = "1" ]; then
  DEBUG_FLAG="-debug=1"
fi

echo "[entrypoint] 使用配置文件 ${CONFIG_FILE}（SERVER_ID=${SERVER_ID}）"
exec /app/cf-probe run -config "${CONFIG_FILE}" ${DEBUG_FLAG}

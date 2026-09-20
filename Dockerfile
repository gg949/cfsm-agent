# syntax=docker/dockerfile:1
# ──────────────────────────────────────────────────────────────
# ProbeDeck Go 探针（cf-probe）Docker 镜像
# 用法：
#   docker run -d --name cf-probe --restart unless-stopped \
#     -e SERVER_ID=<服务器ID> -e SECRET=<密钥> -e WORKER_URL=https://<面板地址>/update \
#     -v /opt/cf-probe:/etc/cf-probe ghcr.io/gg949/cfsm-agent:latest
# 升级 = 重新拉镜像重建容器（容器内禁用自更新，见 entrypoint 说明）。
# ──────────────────────────────────────────────────────────────
FROM alpine:3.21

# ca-certificates：HTTPS 上报/更新检查需要；tzdata：流量重置日按本地时区计算
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# buildx 构建时自动填充
ARG TARGETOS
ARG TARGETARCH

COPY --chmod=755 docker-entrypoint.sh /app/docker-entrypoint.sh
COPY cf-probe-${TARGETOS}-${TARGETARCH} /app/cf-probe

# 配置目录：探针的远程配置下发会写回 config.conf，必须可写 → 建议挂 volume
ENV CF_PROBE_CONFIG_DIR=/etc/cf-probe
VOLUME ["/etc/cf-probe"]

EXPOSE 17986

ENTRYPOINT ["/app/docker-entrypoint.sh"]

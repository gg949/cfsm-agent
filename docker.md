# Docker 部署（Unraid / 群晖 / 任意 Docker 环境）

不想用安装脚本（systemd）的话，可以用 Docker 跑探针，适合 Unraid、群晖 Container Manager、Portainer 等环境。

## 快速开始

```bash
docker run -d \
  --name cf-probe \
  --restart unless-stopped \
  -e SERVER_ID=<服务器ID> \
  -e SECRET=<服务器密钥> \
  -e WORKER_URL=https://<面板地址>/update \
  -v /opt/cf-probe:/etc/cf-probe \
  ghcr.io/gg949/cfsm-agent:latest
```

三项必填环境变量（在面板「服务器」页添加服务器后获取）：

| 变量 | 说明 |
| --- | --- |
| `SERVER_ID` | 服务器 ID |
| `SECRET` | 服务器密钥 |
| `WORKER_URL` | 面板上报地址，例如 `https://probedeck.example.com/update` |

## 可选环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `REPORT_INTERVAL` | `60` | HTTP 模式上报间隔（秒）；WSS 实时模式不受影响 |
| `COLLECT_INTERVAL` | `0` | 采样间隔（秒），`0` = 跟随上报间隔 |
| `CONNECTION_MODE` | `auto` | `auto` 优先 WSS 实时上报，`http` 仅定时 POST |
| `PING_MODE` | `tcp` | `tcp` 或 `icmp` |
| `RESET_DAY` | `1` | 每月流量重置日 1-31，`0` = 不重置 |
| `INTERFACE` | 空 | 指定统计网卡，多个用英文逗号分隔；空 = 自动汇总 |
| `DEBUG` | `0` | `1` 输出调试日志 |

## 数据持久化

`/etc/cf-probe` 存放 `config.conf` 与 `traffic.dat`（流量计数器），**必须挂 volume**，否则重建容器会丢流量统计、且面板下发的节点/间隔配置也会丢。

## 升级

```bash
docker pull ghcr.io/gg949/cfsm-agent:latest
docker stop cf-probe && docker rm cf-probe
# 用原 docker run 命令重建（环境变量不变）
```

容器内**已禁用自动更新**——容器的升级方式就是重新拉镜像，进程自己替换二进制没有意义。

## 卸载

```bash
docker stop cf-probe && docker rm cf-probe
# 可选：docker rmi ghcr.io/gg949/cfsm-agent:latest
# 可选：rm -rf /opt/cf-probe   # 配置和流量计数
```

面板「删除服务器」弹窗的目标系统选 **Docker / Unraid** 会生成同一条命令。Unraid 用户也可以在 Docker 页直接停止并删除 `cf-probe` 容器，再按需删除对应 appdata 目录。

## 查看日志

```bash
docker logs -f cf-probe
```

## 多架构

镜像支持 `linux/amd64` 与 `linux/arm64`（Unraid 多为 amd64；ARM 盒子/部分群晖型号可用）。

## Unraid 模板要点

- Repository：`ghcr.io/gg949/cfsm-agent:latest`
- 必填环境变量：`SERVER_ID` / `SECRET` / `WORKER_URL`
- 挂载：Host Path `/mnt/user/appdata/cf-probe` → Container Path `/etc/cf-probe`（RW）

## 容器图标（Icon URL）

Unraid / 1Panel / Portainer 的 Docker 列表里，容器默认是灰色问号方块。在容器的 **Icon URL** 填下面的地址即可换成项目图标：

| 图标 | Icon URL |
| --- | --- |
| 🛡️ ProbeDeck 盾牌（默认推荐） | `https://raw.githubusercontent.com/gg949/cfsm-agent/main/docker/icon.png` |
| 🐳 Docker 鲸鱼（备选） | `https://raw.githubusercontent.com/gg949/cfsm-agent/main/docker/icon-docker.png` |

**Unraid**：Docker 页 → 点 `cf-probe` 容器 → **Icon URL** → 填表内地址 → Apply。

> 图标只影响 Docker 管理面板的容器列表显示，与 ProbeDeck 面板网站的 favicon 无关，互不影响。

## docker compose

仓库根有 `docker-compose.yml` 示例，改掉三个占位值后 `docker compose up -d` 即可。

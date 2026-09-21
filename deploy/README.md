# 部署说明

目标形态：`/opt/qqbot-course-schedule` + systemd + Caddy 反代（TLS 由 Caddy 提供，机器人只监听 `127.0.0.1`）。

## 一键部署（推荐）

```bash
# 本地准备：填写 config.json（可从 config.example.json 复制）
./deploy/deploy.sh                 # 测试 → 构建 → 上传 → 重启 → 健康检查
./deploy/deploy.sh --skip-tests    # 跳过 go test
./deploy/deploy.sh --caddy         # 同时安装/更新 Caddy 反代，需 CADDY_DOMAIN
HOST=myserver ./deploy/deploy.sh   # 指定 SSH 主机别名（默认 as）
```

可覆盖的环境变量：`HOST`、`REMOTE_DIR`、`SERVICE`、`RUN_USER`、`LOCAL_PORT`、`CADDY_DOMAIN`。

脚本会：

1. `gofmt` 检查 + `go test ./...`；
2. 构建 `linux/amd64` 静态二进制（版本号取 `git describe`）；
3. 首次部署时创建 `qqbot` 用户、`/opt/qqbot-course-schedule/{bin,data}`，上传 `config.json`（600 权限）与 systemd unit；
4. 二进制先传 `/tmp` 再 `install` 原子替换，随后 `systemctl restart`；
5. 轮询 `/healthz`，失败时自动打印 `journalctl` 最近日志。

Caddy 反代只装一次即可（`--caddy` 会覆盖生成 `/etc/caddy/Caddyfile.d/qqbot.caddy` 并 reload）。

## 手动步骤（备用）

```bash
# 构建
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=$(git rev-parse --short HEAD)" \
  -o bin/qqbot-course-schedule ./cmd/bot

# 远端准备（root）
useradd -r -m -d /opt/qqbot-course-schedule -s /sbin/nologin qqbot
mkdir -p /opt/qqbot-course-schedule/{bin,data}

# 上传
scp bin/qqbot-course-schedule <host>:/opt/qqbot-course-schedule/bin/
scp config.json <host>:/opt/qqbot-course-schedule/
ssh <host> 'chown -R qqbot:qqbot /opt/qqbot-course-schedule && chmod 600 /opt/qqbot-course-schedule/config.json'

# systemd
scp deploy/qqbot-course-schedule.service <host>:/etc/systemd/system/
ssh <host> 'systemctl daemon-reload && systemctl enable --now qqbot-course-schedule'

# Caddy
scp deploy/qqbot.caddy <host>:/etc/caddy/Caddyfile.d/qqbot.caddy
ssh <host> 'caddy validate --config /etc/caddy/Caddyfile && systemctl reload caddy'

# 验证
ssh <host> 'curl -s 127.0.0.1:18080/healthz'
curl -s https://<你的域名>/healthz
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://<你的域名>/webhook -d '{}'   # 期望 401
```

## 配置要点

- `bind=127.0.0.1`、`port=18080`：不直接对外，回环监听时端口不受平台 80/443/8080/8443 限制。
- 平台回调填 `https://<你的域名>/webhook`（Caddy 443，符合平台要求）。
- `public_base_url` 填最终对外地址（M2 图片上传用）。

## 运维

```bash
systemctl status qqbot-course-schedule
journalctl -u qqbot-course-schedule -f
systemctl restart qqbot-course-schedule
```

- 数据目录：`/opt/qqbot-course-schedule/data`（SQLite、图片缓存，需可写）
- 备份：`sqlite3 /opt/qqbot-course-schedule/data/course_schedule.sqlite3 ".backup '<目标路径>'"`（M1 起）

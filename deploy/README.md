# 部署说明

以 `/opt/qqbot-course-schedule` + systemd + Caddy 反代为例（实际服务器：Alibaba Cloud Linux 3）。

## 1. 本地构建

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=$(git rev-parse --short HEAD)" \
  -o bin/qqbot-course-schedule ./cmd/bot
```

## 2. 远端准备（root）

```bash
useradd -r -m -d /opt/qqbot-course-schedule -s /sbin/nologin qqbot
mkdir -p /opt/qqbot-course-schedule/{bin,data}
```

## 3. 上传

```bash
scp bin/qqbot-course-schedule <host>:/opt/qqbot-course-schedule/bin/
scp config.json <host>:/opt/qqbot-course-schedule/
ssh <host> 'chown -R qqbot:qqbot /opt/qqbot-course-schedule && chmod 600 /opt/qqbot-course-schedule/config.json'
```

`config.json` 至少填写 `appid`、`secret`；反代场景保持 `bind=127.0.0.1`、`port=18080`，
`public_base_url` 填最终对外地址（M2 图片用）。

## 4. systemd

```bash
scp deploy/qqbot-course-schedule.service <host>:/etc/systemd/system/
ssh <host> 'systemctl daemon-reload && systemctl enable --now qqbot-course-schedule'
ssh <host> 'systemctl status qqbot-course-schedule --no-pager'
ssh <host> 'curl -s 127.0.0.1:18080/healthz'
```

## 5. Caddy

```bash
scp deploy/qqbot.caddy <host>:/etc/caddy/Caddyfile.d/qqbot.caddy
ssh <host> 'caddy validate --config /etc/caddy/Caddyfile && systemctl reload caddy'
curl -s https://<你的域名>/healthz
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://<你的域名>/webhook -d '{}'   # 期望 401
```

## 6. 升级

```bash
scp bin/qqbot-course-schedule <host>:/opt/qqbot-course-schedule/bin/
ssh <host> 'chown qqbot:qqbot /opt/qqbot-course-schedule/bin/qqbot-course-schedule && systemctl restart qqbot-course-schedule'
```

## 7. 运维

```bash
systemctl status qqbot-course-schedule
journalctl -u qqbot-course-schedule -f
```

- 平台回调地址：`https://<你的域名>/webhook`（端口 443 由 Caddy 提供，符合平台 80/443/8080/8443 限制）
- 数据目录：`/opt/qqbot-course-schedule/data`（SQLite、图片缓存，需可写）
- 备份：`sqlite3 /opt/qqbot-course-schedule/data/course_schedule.sqlite3 ".backup '<目标路径>'"`（M1 起）

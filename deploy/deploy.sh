#!/usr/bin/env bash
#
# 一键编译、上传、部署 qqbot-course-schedule
#
# 用法:
#   deploy/deploy.sh                      # 测试 → 构建 → 上传 → 重启 → 健康检查
#   deploy/deploy.sh --skip-tests         # 跳过 go test（快速迭代）
#   deploy/deploy.sh --caddy              # 同时安装/更新 Caddy 反代（需 CADDY_DOMAIN）
#   HOST=myserver deploy/deploy.sh        # 指定 SSH 主机别名
#
# 可覆盖的环境变量:
#   HOST         SSH 主机别名/地址       默认 as
#   REMOTE_DIR   远端目录                默认 /opt/qqbot-course-schedule
#   SERVICE      systemd 服务名          默认 qqbot-course-schedule
#   RUN_USER     运行用户                默认 qqbot
#   LOCAL_PORT   健康检查端口            默认 18080
#   CADDY_DOMAIN Caddy 域名（--caddy 时必填，如 qqbot.example.com）
#
set -euo pipefail

HOST="${HOST:-as}"
REMOTE_DIR="${REMOTE_DIR:-/opt/qqbot-course-schedule}"
SERVICE="${SERVICE:-qqbot-course-schedule}"
RUN_USER="${RUN_USER:-qqbot}"
LOCAL_PORT="${LOCAL_PORT:-18080}"
SKIP_TESTS=0
INSTALL_CADDY=0
CADDY_DOMAIN="${CADDY_DOMAIN:-}"

usage() {
	sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--skip-tests) SKIP_TESTS=1; shift ;;
	--caddy) INSTALL_CADDY=1; shift ;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "未知参数: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

[[ -f config.json ]] || fail "缺少 config.json（可从 config.example.json 复制并填写）"
command -v go >/dev/null || fail "未找到 go"
command -v ssh >/dev/null || fail "未找到 ssh"
command -v scp >/dev/null || fail "未找到 scp"

UNFORMATTED="$(gofmt -l .)"
[[ -z "$UNFORMATTED" ]] || fail "以下文件未通过 gofmt:\n$UNFORMATTED"

if [[ "$SKIP_TESTS" != "1" ]]; then
	log "运行测试"
	go test ./...
fi

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
log "构建 $VERSION (linux/amd64)"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
	-ldflags "-s -w -X main.version=$VERSION" \
	-o bin/qqbot-course-schedule ./cmd/bot

log "准备远端目录 $HOST:$REMOTE_DIR"
ssh "$HOST" "id -u $RUN_USER >/dev/null 2>&1 || useradd -r -m -d $REMOTE_DIR -s /sbin/nologin $RUN_USER
mkdir -p $REMOTE_DIR/bin $REMOTE_DIR/data"

log "上传二进制（先传 /tmp 再原子替换）"
scp -q bin/qqbot-course-schedule "$HOST:/tmp/qqbot-course-schedule.new"
ssh "$HOST" "install -o $RUN_USER -g $RUN_USER -m 755 /tmp/qqbot-course-schedule.new $REMOTE_DIR/bin/qqbot-course-schedule
rm -f /tmp/qqbot-course-schedule.new"

if ! ssh "$HOST" "test -f $REMOTE_DIR/config.json"; then
	log "首次部署：上传 config.json"
	scp -q config.json "$HOST:$REMOTE_DIR/config.json"
	ssh "$HOST" "chown $RUN_USER:$RUN_USER $REMOTE_DIR/config.json && chmod 600 $REMOTE_DIR/config.json"
fi

if ! ssh "$HOST" "systemctl cat $SERVICE >/dev/null 2>&1"; then
	log "安装 systemd unit"
	scp -q deploy/qqbot-course-schedule.service "$HOST:/etc/systemd/system/$SERVICE.service"
	ssh "$HOST" "systemctl daemon-reload"
fi
ssh "$HOST" "systemctl enable $SERVICE >/dev/null 2>&1 || true"

if [[ "$INSTALL_CADDY" == "1" ]]; then
	[[ -n "$CADDY_DOMAIN" ]] || fail "--caddy 需要设置 CADDY_DOMAIN，例如 CADDY_DOMAIN=qqbot.example.com"
	log "更新 Caddy 反代 $CADDY_DOMAIN -> 127.0.0.1:$LOCAL_PORT"
	sed "s/qqbot\.example\.com/$CADDY_DOMAIN/g" deploy/qqbot.caddy >/tmp/qqbot.caddy
	scp -q /tmp/qqbot.caddy "$HOST:/etc/caddy/Caddyfile.d/qqbot.caddy"
	ssh "$HOST" "caddy validate --config /etc/caddy/Caddyfile >/dev/null && systemctl reload caddy"
fi

log "重启服务并做健康检查"
ssh "$HOST" "systemctl restart $SERVICE
for i in \$(seq 1 10); do
  if curl -sf http://127.0.0.1:$LOCAL_PORT/healthz; then echo; exit 0; fi
  sleep 1
done
echo '健康检查失败，最近日志：' >&2
journalctl -u $SERVICE -n 20 --no-pager -o cat >&2
exit 1"

ssh "$HOST" "systemctl is-enabled $SERVICE; systemctl is-active $SERVICE"
log "部署成功：$VERSION"

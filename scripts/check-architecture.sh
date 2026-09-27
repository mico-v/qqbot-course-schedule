#!/usr/bin/env bash
#
# Check the architecture boundaries documented in REFACTOR-GUIDE.md.
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

fail() {
	printf 'ERROR: %b\n' "$*" >&2
	exit 1
}

schedule_deps="$(go list -deps ./internal/schedule)"
if grep -qE 'gin-gonic|modernc.org/sqlite|fogleman/gg|robfig/cron' <<<"$schedule_deps"; then
	fail "架构边界违规：schedule 必须保持纯领域内核"
fi
if grep -q 'internal/admin' <<<"$schedule_deps"; then
	fail "架构边界违规：schedule 不得依赖管理台应用层"
fi

admin_deps="$(go list -deps ./internal/admin)"
if grep -qE 'gin-gonic|modernc.org/sqlite|fogleman/gg|robfig/cron' <<<"$admin_deps"; then
	fail "架构边界违规：admin 不得依赖 HTTP、SQLite 或渲染实现"
fi
if grep -q 'internal/store' <<<"$admin_deps"; then
	fail "架构边界违规：admin 只能依赖 Storage 端口，不得依赖 store 适配器"
fi

for pkg in $(go list ./internal/...); do
	deps="$(go list -deps "$pkg")"
	if grep -q 'gin-gonic' <<<"$deps" && grep -q 'modernc.org/sqlite$' <<<"$deps"; then
		fail "架构边界违规：$pkg 同时依赖 gin 与 sqlite"
	fi
done

server_deps="$(go list -deps ./internal/server)"
if grep -q 'internal/webhook' <<<"$server_deps"; then
	fail "架构边界违规：server 不得依赖 webhook"
fi
if ! grep -q 'internal/admin' <<<"$server_deps"; then
	fail "架构边界违规：server 的管理台路由必须依赖 internal/admin"
fi

stale_admin_methods="$(grep -rnE '^func \(s \*Service\) (ScopeSummaries|PageSchedule|SavePageSchedule|CreateMemberSchedules|WebMemberICS|WebDayOverrides|SetWebDayOverride|DeleteWebDayOverride|PendingMembers|ExportBackup|ImportBackup)\b' --include='*.go' internal/schedule || true)"
[[ -z "$stale_admin_methods" ]] || fail "重构边界违规：管理台方法仍留在 schedule：\n$stale_admin_methods"

stale_admin_types="$(grep -rnE '^type (ScopeSummary|ScopeMemberSummary|WebEvent|PageSchedule|WebEventInput|SavePagePayload|SavePageResult|NewMember|WebDayOverride|BackupFile|BackupMember|ImportResult)\b' --include='*.go' internal/schedule || true)"
[[ -z "$stale_admin_types" ]] || fail "重构边界违规：管理台 DTO 仍留在 schedule：\n$stale_admin_types"

bot_imports="$(grep -rl 'internal/store' --include='*.go' --exclude='*_test.go' internal/bot || true)"
[[ -z "$bot_imports" ]] || fail "架构边界违规：bot 非测试代码不得 import store：\n$bot_imports"

stale_messages="$(grep -rnE '&Message\{|bot\.Message' --include='*.go' internal cmd || true)"
[[ -z "$stale_messages" ]] || fail "重构边界违规：Message 已拆分:\n$stale_messages"

raw_kv="$(grep -rnE '\.Store\.(Get|Set|List|Delete)KV' --include='*.go' internal/bot || true)"
[[ -z "$raw_kv" ]] || fail "重构边界违规：bot 不得直接访问裸 KV:\n$raw_kv"

printf '架构边界检查通过\n'

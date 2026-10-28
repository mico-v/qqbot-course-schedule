# Repository Guidelines

## Scope

- Product scope is QQ group chat only.
- Do not add private-chat, active push, or AI tools.
- Global schedule lookup is allowed only through the explicit `storage_mode=global` setting.
- Completed architecture changes are described in [ARCHITECTURE.md](ARCHITECTURE.md), not as tasks.

## Project Structure

- `cmd/bot/` — process entrypoint.
- `cmd/cardpreview/` — local card preview.
- `internal/app/` — composition, routes, workers, lifecycle.
- `internal/schedule/` — domain, use cases, ports, identity.
- `internal/admin/` — admin/public-editor/transfer application layer.
- `internal/bot/` — chat commands and QQ reply orchestration.
- `internal/server/` — HTTP binding, auth, error mapping.
- `internal/store/` — SQLite v7 adapter and v6 compatibility.
- `internal/webhook/` — signature verification and event dispatch.
- `tests/` — black-box unit/contract/integration/e2e tests.
- `web/` — Vue admin UI and committed `web/dist`.
- `docs/` — official API snapshots only.

## Identity

- Identity is always `(group_openid, member_openid)`.
- `member_openid` is a platform identifier, not a nickname, QQ number, or human-chosen name.
- Two groups may return the same `member_openid`; they remain separate members and schedules.
- `display_name`, `qq`, and optional `union_openid` are member attributes, never identity keys.
- `union_openid` may be used only by the explicit global resolver; it never replaces the group-member primary key.
- New code accepts group coordinates only. Global lookup happens in the schedule application port, not through synthetic identity keys.

## Architecture Rules

- `schedule` must not import gin, SQLite, gg, qqapi, store, or admin.
- `admin` must not import gin, SQLite, render, or store internals.
- `server` must not import webhook or store.
- `bot` production code must not import store.
- `cmd` must not import store directly.
- I/O ports take `context.Context` first.
- HTTP uses the error mapping in `internal/server/errors.go`.
- Do not add generic KV APIs to application layers.
- Run `bash scripts/check-architecture.sh` after changing imports.

## Mandatory Change Workflow

1. Read [ARCHITECTURE.md](ARCHITECTURE.md), [PLAN.md](PLAN.md), and the relevant
   development/connection/deployment docs before editing code.
2. Implement against the documented contract. If the requested behavior conflicts
   with the docs, update the contract first rather than silently diverging.
3. Update every affected document in the same change. If no document changes are
   needed, state why in the commit/PR.
4. Run the relevant checks, including `COVERAGE_STAGE=p2 bash scripts/test-all.sh`
   for Go changes and `RUN_WEB=1 ...` for Web changes.
5. Commit with the required format, then push to `origin/main`.

The complete policy is [CONTRIBUTING.md](CONTRIBUTING.md).

## Commands

```bash
go run ./cmd/bot
go run ./cmd/cardpreview -o /tmp/card.jpg
go test ./internal/... ./tests/...
go test ./internal/schedule -run TestDayoff -v
gofmt -l .
go vet ./cmd/... ./internal/... ./tests/... ./web
COVERAGE_STAGE=p2 bash scripts/test-all.sh
RUN_WEB=1 COVERAGE_STAGE=p2 bash scripts/test-all.sh
```

If dependency downloads fail, set `GOPROXY=https://goproxy.cn,direct`.

## Tests

- Standard library `testing`; no assert framework, golden files, or `-update`.
- Black-box tests belong under `tests/`.
- Source-directory tests must end in `_whitebox_test.go` and genuinely need unexported state.
- Use `httptest` for HTTP/QQ API and temporary directories for SQLite.
- Rendering tests use size and pixel assertions.
- Migration tests must cover v6 → v7, rollback on bad input, and cross-group isolation.

## Code Style

- Format Go with `gofmt`.
- Exported symbols need doc comments.
- Comments and logs are English; user-facing text and Go errors are Chinese.
- No magic numbers.
- Follow existing package/file naming and keep edits scoped.
- Do not add, remove, rename, or expose chat commands unless explicitly requested.

## Configuration And Data

- Never commit `config.json`, `data/`, `bin/`, or `web/node_modules/`.
- Keep `MaxOpenConns(1)`.
- Do not write SQLite tables outside `internal/store`.
- Group schedule writes use revision CAS.

## Commit And PR

- Commit format: `type(scope): 中文描述`.
- Allowed types: `feat/fix/docs/refactor/test/chore/style/ci`.
- CI must pass Go and Web gates.
- Card/WebUI changes should include visual verification.
- A completed code change is not finished until affected docs are updated and the
  commit is pushed.

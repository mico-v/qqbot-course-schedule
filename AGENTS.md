# Repository Guidelines

## Project Structure & Module Organization

- `cmd/bot/` — production entrypoint; `cmd/cardpreview/` renders local card previews.
- `internal/` — Go packages: `schedule` (pure domain core), `bot` (commands/message link),
  `admin` + `server` (web admin), `webhook`, `qqapi`, `render`, `store`, `config`, `timing`.
- `web/` — embedded admin UI; `assets/` — fonts/images; `docs/` — official API snapshots only.
- `deploy/`, `scripts/` — deployment and CI helpers.

Dependencies flow `webhook → bot → schedule`; `store → schedule/admin`. Run
`bash scripts/check-architecture.sh` before touching imports — it enforces the boundaries.

## Build, Test, and Development Commands

- `go build ./...` — build all packages.
- `go run ./cmd/bot` — run locally (reads `config.json`, default `127.0.0.1:18080`, path `/webhook`).
- `go run ./cmd/cardpreview -o /tmp/card.jpg` — preview card layout (`-rank`, `-avatar`).
- `go test ./...` — full suite; narrow with `go test ./internal/schedule/ -run TestDayoff -v`.
- `gofmt -l . && go vet ./...` — formatting and static checks.
- `bash scripts/check-architecture.sh` — enforce package boundaries (also run in CI).

If `go build` hangs, set `go env -w GOPROXY=https://goproxy.cn,direct`.

## Coding Style & Naming Conventions

- Format with `gofmt`; exported symbols need doc comments; no magic numbers.
- Comments and logs are **English**; user-visible text and Go error messages are **Chinese**.
- Keep `schedule` free of `gin`, `sqlite`, `gg`, and `cron`; use its `Storage` interface.
- Follow existing names: packages lowercase, exported `CamelCase`, files `snake_case_test.go`.

## Testing Guidelines

- Standard library `testing` only; no assert frameworks, golden files, or `-update` flags.
- Name tests `TestXxx`; use `httptest` for HTTP and temp dirs for storage.
- Rendering is verified with size assertions and pixel sampling.

## Commit & Pull Request Guidelines

- Commit subject format: `type(scope): 中文描述`, where `type` is one of
  `feat/fix/docs/refactor/test/chore/style/ci` (e.g. `feat(schedule): 实现 /课表 日期解析`).
- PRs must pass CI (`gofmt`, architecture check, `go vet`, `go test ./...`, build) and should
  describe the change, link related issues, and include screenshots for card/WebUI changes.

## Security & Configuration Tips

- Never commit `config.json`, `data/`, or `bin/` (already in `.gitignore`).
- Keep `MaxOpenConns(1)` for SQLite; never write tables directly — go through service layers.

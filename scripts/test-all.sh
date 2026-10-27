#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

coverage_file="${COVERAGE_FILE:-coverage.out}"

if [[ -n "$(gofmt -l .)" ]]; then
	printf 'ERROR: files need gofmt:\n%s\n' "$(gofmt -l .)" >&2
	exit 1
fi
bash scripts/check-architecture.sh
go vet ./cmd/... ./internal/... ./tests/... ./web
go test -race -shuffle=on -count=1 \
	-covermode=atomic -coverpkg=./internal/... \
	-coverprofile="$coverage_file" \
	./cmd/... ./internal/... ./tests/... ./web
bash scripts/check-coverage.sh "$coverage_file"
CGO_ENABLED=0 go build ./...

card_file="${CARD_PREVIEW_FILE:-/tmp/qqbot-cardpreview.jpg}"
go run ./cmd/cardpreview -o "$card_file"
test -s "$card_file"

if [[ "${RUN_WEB:-0}" == "1" ]]; then
	(
		cd web
		npm ci
		npm run lint
		npm run test -- --run
		npm run typecheck
		npm run build
	)
	git diff --exit-code -- web/dist
fi

#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

profile="${1:-coverage.out}"
if [[ ! -s "$profile" ]]; then
	printf 'ERROR: coverage profile %s is missing or empty\n' "$profile" >&2
	exit 1
fi

module="$(go list -m)"
stage="${COVERAGE_STAGE:-p0}"

# P0 starts at the reviewed baseline and can only move upward. Later stages
# raise both the total and package floors as tests migrate under tests/.
case "$stage" in
p0) total_floor=69 ;;
p2) total_floor=75 ;;
p3) total_floor=80 ;;
p5) total_floor=85 ;;
final) total_floor=90 ;;
*)
	printf 'ERROR: unknown COVERAGE_STAGE=%s (want p0|p2|p3|p5|final)\n' "$stage" >&2
	exit 1
	;;
esac

awk -v module="$module" -v stage="$stage" -v total_floor="$total_floor" '
BEGIN {
	floor["internal/admin"] = 54
	floor["internal/bot"] = 68
	floor["internal/config"] = 94
	floor["internal/qqapi"] = 43
	floor["internal/render"] = 71
	floor["internal/schedule"] = 74
	floor["internal/server"] = 76
	floor["internal/store"] = 55
	floor["internal/timing"] = 100
	floor["internal/wakeup"] = 91
	floor["internal/webhook"] = 74
	if (stage == "p2") {
		floor["internal/admin"] = 60
		floor["internal/store"] = 62
	}
	if (stage == "p3") {
		floor["internal/admin"] = 70
		floor["internal/bot"] = 75
		floor["internal/store"] = 75
	}
	if (stage == "p5" || stage == "final") {
		floor["internal/admin"] = 85
		floor["internal/bot"] = 85
		floor["internal/qqapi"] = 80
		floor["internal/render"] = 85
		floor["internal/schedule"] = 90
		floor["internal/server"] = 90
		floor["internal/store"] = 90
		floor["internal/webhook"] = 90
	}
}
NR == 1 { next }
{
	path = $1
	sub(/:[0-9]+\.[0-9]+,[0-9]+\.[0-9]+$/, "", path)
	statements = $2 + 0
	count = $3 + 0
	key = $1 SUBSEP statements
	if (!(key in blockStatements)) {
		blockStatements[key] = statements
		blockPath[key] = path
		blockCount[key] = count
	} else if (count > blockCount[key]) {
		blockCount[key] = count
	}
}
END {
	failed = 0
	for (key in blockStatements) {
		pkg = blockPath[key]
		sub("^" module "/", "", pkg)
		sub("/[^/]+$", "", pkg)
		total[pkg] += blockStatements[key]
		if (blockCount[key] > 0) {
			covered[pkg] += blockStatements[key]
		}
		grandTotal += blockStatements[key]
		if (blockCount[key] > 0) {
			grandCovered += blockStatements[key]
		}
	}
	totalPct = grandTotal ? 100 * grandCovered / grandTotal : 0
	printf "Go total coverage: %.1f%% (floor %.0f%%)\n", totalPct, total_floor
	if (totalPct + 0.0001 < total_floor) {
		printf "ERROR: total coverage %.1f%% is below %.0f%%\n", totalPct, total_floor > "/dev/stderr"
		failed = 1
	}
	for (pkg in floor) {
		pct = total[pkg] ? 100 * covered[pkg] / total[pkg] : 0
		printf "%-24s %5.1f%% (floor %.0f%%)\n", pkg, pct, floor[pkg]
		if (pct + 0.0001 < floor[pkg]) {
			printf "ERROR: %s coverage %.1f%% is below %.0f%%\n", pkg, pct, floor[pkg] > "/dev/stderr"
			failed = 1
		}
	}
	exit failed
}
' "$profile"

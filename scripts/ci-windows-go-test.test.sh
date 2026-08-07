#!/usr/bin/env bash
# Contract tests for scripts/ci-windows-go-test.ps1 (no Windows runner required).
set -euo pipefail

root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
script="$root/scripts/ci-windows-go-test.ps1"

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

[[ -f "$script" ]] || fail "missing $script"

# Must hard-kill the process tree; soft Stop-Process is insufficient on Windows.
grep -q 'taskkill.exe /PID \$ProcessId /T /F' "$script" ||
	grep -q 'taskkill.exe /PID $ProcessId /T /F' "$script" ||
	grep -q 'taskkill.exe /PID' "$script" ||
	fail "expected taskkill process-tree kill"

grep -q 'exit 124' "$script" || fail "expected GNU-timeout-style exit 124 on hard timeout"
grep -q 'WaitForExit' "$script" || fail "expected WaitForExit wall-clock wait"
grep -q 'TimeoutSeconds' "$script" || fail "expected TimeoutSeconds parameter"
grep -q 'GoTestArgs' "$script" || fail "expected GoTestArgs parameter for go test flags"

# Must not reintroduce the retired Windows sandbox wait env as a real dependency.
if grep -q 'WINDOWS_SANDBOX_WAIT_MS' "$script"; then
	fail "wrapper must not depend on retired WINDOWS_SANDBOX_WAIT_MS"
fi

echo "ok: ci-windows-go-test.ps1 contracts"

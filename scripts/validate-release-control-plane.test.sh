#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
"$repo_root/scripts/validate-release-control-plane.sh" "$repo_root"

fixture="$(mktemp -d "${TMPDIR:-/tmp}/reasonix-release-control-test.XXXXXX")"
cleanup() {
	case "$fixture" in */reasonix-release-control-test.*) rm -rf -- "$fixture" ;; *) exit 1 ;; esac
}
trap cleanup EXIT

while IFS= read -r file; do
	mkdir -p "$fixture/$(dirname "$file")"
	cp "$repo_root/$file" "$fixture/$file"
done < <(awk '
	/^required=\(/ { collecting = 1; next }
	collecting && /^\)/ { exit }
	collecting { sub(/^[[:space:]]*/, ""); print }
' "$repo_root/scripts/validate-release-control-plane.sh")
chmod +x "$fixture/scripts/finalize-windows-signed-candidate.sh" "$fixture/scripts/package-windows-desktop.sh"

truncate -s 0 "$fixture/scripts/sign-certum.ps1"
if "$repo_root/scripts/validate-release-control-plane.sh" "$fixture" >"$fixture/missing.log" 2>&1; then
	echo "incomplete release control checkout unexpectedly passed" >&2
	exit 1
fi
grep -Fq 'missing release control file: scripts/sign-certum.ps1' "$fixture/missing.log"

echo "release control preflight tests: PASS"

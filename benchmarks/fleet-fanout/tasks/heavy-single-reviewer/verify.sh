#!/usr/bin/env bash
set -e
# The answer needs a three-step derivation per module and a comparison across
# ten of them. Grepping the constants returns thirty numbers and settles
# nothing, so both arms have to do the same per-subject reasoning; what differs
# is whether one run holds all ten or ten runs hold one each.
test -f ANSWER.md
grep -qE '^[[:space:]]*module=.*ledger[[:space:]]*$' ANSWER.md
grep -qE '^[[:space:]]*effective=34[[:space:]]*$' ANSWER.md

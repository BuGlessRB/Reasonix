#!/usr/bin/env bash
set -e
# Both arms answer the same question over the same twelve files, so the grader
# is identical: what differs is whether the middle item maps over the subjects
# or handles all of them in one run.
test -f ANSWER.md
grep -qE '^[[:space:]]*missing=.*h07(\.py)?[[:space:]]*$' ANSWER.md

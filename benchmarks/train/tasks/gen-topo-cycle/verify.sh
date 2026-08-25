#!/bin/bash
set -e

# Run from the directory that contains the agent's topo.py.
cd "$(dirname "$0")"

python3 - <<'PYEOF'
import signal
import sys

import topo


class _Timeout(Exception):
    pass


def _alarm_handler(signum, frame):
    raise _Timeout("topological_sort did not return within the time budget")


signal.signal(signal.SIGALRM, _alarm_handler)


def run_with_timeout(fn, *args, **kwargs):
    signal.alarm(5)
    try:
        return fn(*args, **kwargs)
    finally:
        signal.alarm(0)


def is_topo_order(n, edges, order):
    if not isinstance(order, list) or len(order) != n:
        return "expected a list of %d nodes, got %r" % (n, order)
    if sorted(order) != list(range(n)):
        return "order %r is not a permutation of 0..%d" % (order, n - 1)
    pos = {node: i for i, node in enumerate(order)}
    for u, v in edges:
        if pos[u] >= pos[v]:
            return "edge (%d, %d) violated: %d must come before %d" % (u, v, u, v)
    return None


failures = []

# Acyclic graphs must yield a valid topological order.
CASES = [
    (5, [(0, 1), (0, 2), (1, 3), (2, 3), (3, 4)]),
    (4, [(1, 3)]),
    (3, [(2, 1), (1, 0)]),
    (1, []),
    (6, [(0, 2), (1, 2), (2, 4), (3, 4), (4, 5)]),
]
for n, edges in CASES:
    try:
        order = run_with_timeout(topo.topological_sort, n, edges)
    except _Timeout:
        failures.append("acyclic case n=%d: topological_sort did not return within 5s" % n)
        continue
    except Exception as exc:
        failures.append("acyclic case n=%d, edges=%r raised %r" % (n, edges, exc))
        continue
    err = is_topo_order(n, edges, order)
    if err is not None:
        failures.append("acyclic case n=%d, edges=%r: %s" % (n, edges, err))

# A cyclic graph must be reported via ValueError, not an endless loop.
try:
    run_with_timeout(topo.topological_sort, 3, [(0, 1), (1, 2), (2, 0)])
    failures.append("cyclic graph: expected ValueError, but topological_sort returned a result")
except ValueError:
    pass
except _Timeout:
    failures.append("cyclic graph: topological_sort looped forever instead of reporting the cycle")
except Exception as exc:
    failures.append("cyclic graph: raised %r instead of ValueError" % (exc,))

if failures:
    for msg in failures:
        print("FAIL:", msg)
    sys.exit(1)

print("ALL CHECKS PASSED")
PYEOF

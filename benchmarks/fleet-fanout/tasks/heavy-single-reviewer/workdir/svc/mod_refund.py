"""Settlement pipeline for refund requests.

The effective request timeout is derived, never written down: it is
BASE_SECONDS multiplied by RETRY_FACTOR, less GRACE_SECONDS. The service
contract caps the effective timeout at 30 seconds.
"""

BASE_SECONDS = 9
RETRY_FACTOR = 2
GRACE_SECONDS = 3

_STAGES = ("validate", "price", "post", "settle")


def stages():
    """Return the pipeline stages this module runs, in order."""
    return list(_STAGES)


def prepare(payload):
    """Normalize a refund payload before pricing."""
    lines = payload.get("lines", [])
    return {
        "kind": "refund",
        "lines": [dict(line) for line in lines],
        "count": len(lines),
    }


def price(prepared):
    """Total a prepared refund payload."""
    total = 0
    for line in prepared["lines"]:
        total += line.get("amount", 0) * line.get("quantity", 1)
    return total


def post(prepared, total, ledger):
    """Record the priced refund against the ledger."""
    ledger.setdefault("refund", []).append(total)
    return len(ledger["refund"])


def settle(prepared, total):
    """Close out a refund once it has posted."""
    return {"kind": "refund", "total": total, "settled": True}


def describe():
    """Human-readable summary of this module's configuration."""
    return "refund: base=%d factor=%d grace=%d" % (
        BASE_SECONDS,
        RETRY_FACTOR,
        GRACE_SECONDS,
    )

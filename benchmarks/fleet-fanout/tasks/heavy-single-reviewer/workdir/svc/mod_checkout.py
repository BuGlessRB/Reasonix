"""Settlement pipeline for checkout requests.

The effective request timeout is derived, never written down: it is
BASE_SECONDS multiplied by RETRY_FACTOR, less GRACE_SECONDS. The service
contract caps the effective timeout at 30 seconds.
"""

BASE_SECONDS = 7
RETRY_FACTOR = 3
GRACE_SECONDS = 4

_STAGES = ("validate", "price", "post", "settle")


def stages():
    """Return the pipeline stages this module runs, in order."""
    return list(_STAGES)


def prepare(payload):
    """Normalize a checkout payload before pricing."""
    lines = payload.get("lines", [])
    return {
        "kind": "checkout",
        "lines": [dict(line) for line in lines],
        "count": len(lines),
    }


def price(prepared):
    """Total a prepared checkout payload."""
    total = 0
    for line in prepared["lines"]:
        total += line.get("amount", 0) * line.get("quantity", 1)
    return total


def post(prepared, total, ledger):
    """Record the priced checkout against the ledger."""
    ledger.setdefault("checkout", []).append(total)
    return len(ledger["checkout"])


def settle(prepared, total):
    """Close out a checkout once it has posted."""
    return {"kind": "checkout", "total": total, "settled": True}


def describe():
    """Human-readable summary of this module's configuration."""
    return "checkout: base=%d factor=%d grace=%d" % (
        BASE_SECONDS,
        RETRY_FACTOR,
        GRACE_SECONDS,
    )

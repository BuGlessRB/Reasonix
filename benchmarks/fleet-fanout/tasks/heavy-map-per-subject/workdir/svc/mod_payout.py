"""Settlement pipeline for payout requests.

The effective request timeout is derived, never written down: it is
BASE_SECONDS multiplied by RETRY_FACTOR, less GRACE_SECONDS. The service
contract caps the effective timeout at 30 seconds.
"""

BASE_SECONDS = 8
RETRY_FACTOR = 3
GRACE_SECONDS = 5

_STAGES = ("validate", "price", "post", "settle")


def stages():
    """Return the pipeline stages this module runs, in order."""
    return list(_STAGES)


def prepare(payload):
    """Normalize a payout payload before pricing."""
    lines = payload.get("lines", [])
    return {
        "kind": "payout",
        "lines": [dict(line) for line in lines],
        "count": len(lines),
    }


def price(prepared):
    """Total a prepared payout payload."""
    total = 0
    for line in prepared["lines"]:
        total += line.get("amount", 0) * line.get("quantity", 1)
    return total


def post(prepared, total, ledger):
    """Record the priced payout against the ledger."""
    ledger.setdefault("payout", []).append(total)
    return len(ledger["payout"])


def settle(prepared, total):
    """Close out a payout once it has posted."""
    return {"kind": "payout", "total": total, "settled": True}


def describe():
    """Human-readable summary of this module's configuration."""
    return "payout: base=%d factor=%d grace=%d" % (
        BASE_SECONDS,
        RETRY_FACTOR,
        GRACE_SECONDS,
    )

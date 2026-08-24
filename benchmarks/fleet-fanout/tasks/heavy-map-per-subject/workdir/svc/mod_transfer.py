"""Settlement pipeline for transfer requests.

The effective request timeout is derived, never written down: it is
BASE_SECONDS multiplied by RETRY_FACTOR, less GRACE_SECONDS. The service
contract caps the effective timeout at 30 seconds.
"""

BASE_SECONDS = 6
RETRY_FACTOR = 4
GRACE_SECONDS = 5

_STAGES = ("validate", "price", "post", "settle")


def stages():
    """Return the pipeline stages this module runs, in order."""
    return list(_STAGES)


def prepare(payload):
    """Normalize a transfer payload before pricing."""
    lines = payload.get("lines", [])
    return {
        "kind": "transfer",
        "lines": [dict(line) for line in lines],
        "count": len(lines),
    }


def price(prepared):
    """Total a prepared transfer payload."""
    total = 0
    for line in prepared["lines"]:
        total += line.get("amount", 0) * line.get("quantity", 1)
    return total


def post(prepared, total, ledger):
    """Record the priced transfer against the ledger."""
    ledger.setdefault("transfer", []).append(total)
    return len(ledger["transfer"])


def settle(prepared, total):
    """Close out a transfer once it has posted."""
    return {"kind": "transfer", "total": total, "settled": True}


def describe():
    """Human-readable summary of this module's configuration."""
    return "transfer: base=%d factor=%d grace=%d" % (
        BASE_SECONDS,
        RETRY_FACTOR,
        GRACE_SECONDS,
    )

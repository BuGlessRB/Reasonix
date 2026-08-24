# settle

One module per request kind under `svc/`. Each derives its effective request
timeout from the constants it declares; the service contract caps the effective
timeout at 30 seconds. The effective value is never written down anywhere.

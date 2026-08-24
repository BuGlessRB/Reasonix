package main

import (
	"reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/surface"
	"reasonix/internal/telemetry"
)

// windowTelemetry starts this surface's reporter and returns the decorator that
// counts a pane's turns. The window offers the launch ping and the aggregate
// counters as two consents, so each is honored alone: counters declined still
// pings, the ping declined still counts. Mode is "on" only once the config has
// said so — Enabled still refuses a development build, CI, or DO_NOT_TRACK.
func windowTelemetry(cfg *config.Config) func(event.Sink) event.Sink {
	undecorated := func(sink event.Sink) event.Sink { return sink }
	if !cfg.DesktopMetrics() && !cfg.DesktopTelemetry() {
		return undecorated
	}
	reporter := telemetry.Start(telemetry.Options{
		Mode:         "on",
		Version:      version,
		Surface:      surface.Desktop,
		HomeDir:      config.ReasonixHomeDir(),
		Proxy:        cfg.NetworkProxySpec(),
		Interactive:  true,
		Language:     cfg.Language,
		SuppressPing: !cfg.DesktopTelemetry(),
	})
	if !cfg.DesktopMetrics() {
		return undecorated
	}
	return reporter.Wrap
}

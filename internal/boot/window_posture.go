// window_posture.go — the Ask/Auto/YOLO posture a window opens in.
package boot

import (
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/surface"
)

// withWindowPosture hands back the controller in the posture its config names.
// That posture is configuration, not something each shell repeats for itself:
// the Wails shell read it and set it, the Electron one never did, so one config
// opened two different postures depending on the binary. A terminal frontend
// states its own on the command line and is left alone here.
func withWindowPosture(ctrl *control.Controller, cfg *config.Config, src surface.Surface) *control.Controller {
	if ctrl == nil || cfg == nil || src != surface.Desktop {
		return ctrl
	}
	// An unset config normalises to Ask, so this loosens nothing unasked for.
	if mode, ok := control.ParseToolApprovalMode(cfg.DesktopDefaultToolApprovalMode()); ok {
		ctrl.SetToolApprovalMode(mode)
	}
	return ctrl
}

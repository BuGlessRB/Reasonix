package serve

import (
	"net/http"

	"reasonix/internal/control"
)

func scopeView(ctl control.SessionAPI) control.CapabilityScope { return ctl.CapabilityScope() }

// capabilityScopeHandler answers the scope bar on its own, so a surface can
// name its project before either listing has loaded.
func (s *Server) capabilityScopeHandler(w http.ResponseWriter, r *http.Request) {
	writeJSONCached(w, r, scopeView(s.ctl()))
}

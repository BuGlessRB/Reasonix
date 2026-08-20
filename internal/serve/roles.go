// roles.go — which model takes which job.
package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"reasonix/internal/config"
)

// roleFields maps a wire name onto the AgentConfig field that already decides
// the behaviour. A role with no field behind it would be a switch that changes
// nothing, so a name only appears here once the kernel reads it.
var roleFields = map[string]func(*config.Config) *string{
	"planner":  func(c *config.Config) *string { return &c.Agent.PlannerModel },
	"subagent": func(c *config.Config) *string { return &c.Agent.SubagentModel },
	"guardian": func(c *config.Config) *string { return &c.Agent.GuardianModel },
	"vision":   func(c *config.Config) *string { return &c.Agent.VisionModel },
}

func (s *Server) registerRoleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /roles", s.roles)
	mux.HandleFunc("POST /roles", s.setRole)
}

// An empty ref is the default and means "this job rides the main model". It is
// a real value rather than a missing one, so clearing a role sends "".
func (s *Server) roles(w http.ResponseWriter, _ *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make(map[string]string, len(roleFields))
	for name, field := range roleFields {
		out[name] = strings.TrimSpace(*field(cfg))
	}
	writeJSON(w, out)
}

// setRole persists one assignment and rebuilds, because boot reads every role
// model while assembling the runtime. It shares the provider-edit grant: both
// write the configuration of the machine running the kernel, which is the one
// thing a server reachable over a network must not let a client do.
func (s *Server) setRole(w http.ResponseWriter, r *http.Request) {
	if !s.grants.providerEdit {
		http.Error(w, "role editing is not enabled on this server", http.StatusForbidden)
		return
	}
	var body struct {
		Role string `json:"role"`
		Ref  string `json:"ref"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		http.Error(w, "invalid role request", http.StatusBadRequest)
		return
	}
	field, ok := roleFields[strings.TrimSpace(body.Role)]
	if !ok {
		http.Error(w, fmt.Sprintf("unknown role %q", body.Role), http.StatusBadRequest)
		return
	}
	ref := strings.TrimSpace(body.Ref)
	if ref != "" {
		cfg, err := config.Load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Naming a model that does not resolve would strand the role on the next
		// build, and the failure would surface as a broken turn rather than here.
		if !cfg.ModelRefSelectable(ref, s.ctl().ProviderCatalog()) {
			http.Error(w, fmt.Sprintf("no configured model matches %q", ref), http.StatusBadRequest)
			return
		}
	}
	edit := config.LoadForEdit(config.UserConfigPath())
	*field(edit) = ref
	if err := edit.SaveTo(config.UserConfigPath()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.rebuildInPlace(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// rebuildInPlace reassembles the runtime on the model it is already using, so a
// setting boot only reads while assembling reaches it. Rebuilding refuses
// mid-turn for the same reason a model switch does: the running loop would be
// swapped underneath.
func (s *Server) rebuildInPlace(ctx context.Context) error {
	ref := currentModelRef(s.ctl())
	if ref == "" {
		return fmt.Errorf("no current model to rebuild on")
	}
	return s.switchModel(ctx, ref)
}

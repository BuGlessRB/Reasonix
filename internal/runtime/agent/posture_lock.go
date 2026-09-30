package agent

// CodePostureToolNotAllowed identifies a call refused because the run's
// posture offers no such tool.
const CodePostureToolNotAllowed = "posture.tool_not_allowed"

// LockPosture fixes the gate and asker installed now: later SetGate and
// SetAsker calls change nothing, and a call to a tool the registry does not
// hold is refused with CodePostureToolNotAllowed. It cannot be undone.
func (a *Agent) LockPosture() { a.svc.postureLocked.Store(true) }

func (s *agentServices) setGate(g Gate) {
	if !s.postureLocked.Load() {
		s.gate = g
	}
}

func (s *agentServices) setAsker(as Asker) {
	if !s.postureLocked.Load() {
		s.asker = as
	}
}

// computer.go — reading and operating another application on this machine.
package permission

// IsComputerTool reports whether a tool reads or operates another application.
func IsComputerTool(toolName string) bool { return groupOf(toolName) == computerGroup }

// decideComputer answers a call on another application, named by its bundle
// id. Input there reaches whatever that application can, so only a rule naming
// the application answers it — never a glob, a bare grant or auto.
func (p Policy) decideComputer(toolName, subject string) (Decision, bool) {
	if !IsComputerTool(toolName) {
		return 0, false
	}
	switch {
	case matchAny(p.Deny, toolName, subject):
		return Deny, true
	case matchAnyExact(p.SessionAllow, toolName, subject), matchAnyExact(p.Allow, toolName, subject):
		return Allow, true
	case p.Mode == Deny:
		return Deny, true
	}
	return Ask, true
}

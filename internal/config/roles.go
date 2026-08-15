// roles.go — the model refs a role assignment can name.
package config

// roleModelRefs lists every model a role points at. Both the fallback repair and
// the referenced-provider scan walk this set, and they have to walk the same one:
// a role missing from either keeps a ref alive that no provider can resolve.
func (c *Config) roleModelRefs() []string {
	if c == nil {
		return nil
	}
	return []string{c.Agent.PlannerModel, c.Agent.SubagentModel, c.Agent.VisionModel}
}

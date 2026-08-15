package control

import (
	"strings"

	"reasonix/internal/command"
	"reasonix/internal/config"
	"reasonix/internal/i18n"
	"reasonix/internal/pluginpkg"
	"reasonix/internal/skill"
)

// SubmitSlashCommands is the built-in surface a frontend gets for free by
// routing raw input through Submit: the verbs submitCommandOrTurnReady and
// managementNotice dispatch, plus the two serve intercepts. The chat TUI's
// catalogue is larger because it implements commands of its own — listing
// those here would offer a window a command nothing answers.
func SubmitSlashCommands() []SlashItem {
	return []SlashItem{
		{Label: "/compact", Insert: "/compact ", Hint: i18n.M.CmdCompact},
		{Label: "/context", Insert: "/context", Hint: i18n.M.CmdContext},
		{Label: "/new", Insert: "/new", Hint: i18n.M.CmdNew},
		{Label: "/clear", Insert: "/clear", Hint: i18n.M.CmdClear},
		{Label: "/rewind", Insert: "/rewind", Hint: i18n.M.CmdRewind},
		{Label: "/tree", Insert: "/tree", Hint: i18n.M.CmdTree},
		{Label: "/branch", Insert: "/branch ", Hint: i18n.M.CmdBranch},
		{Label: "/switch", Insert: "/switch ", Hint: i18n.M.CmdSwitchBranch},
		{Label: "/model", Insert: "/model ", Hint: i18n.M.CmdModel, Descend: true},
		{Label: "/provider", Insert: "/provider ", Hint: i18n.M.CmdProvider, Descend: true},
		{Label: "/effort", Insert: "/effort ", Hint: i18n.M.CmdEffort, Descend: true},
		{Label: "/goal", Insert: "/goal ", Hint: i18n.M.CmdGoal, Descend: true},
		{Label: "/memory", Insert: "/memory ", Hint: i18n.M.CmdMemory, Descend: true},
		{Label: "/remember", Insert: "/remember ", Hint: i18n.M.CmdRemember},
		{Label: "/skills", Insert: "/skills ", Hint: i18n.M.CmdSkill, Descend: true},
		{Label: "/plugins", Insert: "/plugins ", Hint: i18n.M.CmdPlugins, Descend: true},
		{Label: "/hooks", Insert: "/hooks ", Hint: i18n.M.CmdHooks, Descend: true},
		{Label: "/mcp", Insert: "/mcp ", Hint: i18n.M.CmdMcp, Descend: true},
		{Label: "/docs", Insert: "/docs ", Hint: i18n.M.CmdDocs},
		{Label: "/migrate", Insert: "/migrate", Hint: i18n.M.CmdMigrate},
		{Label: "/reload-cmd", Insert: "/reload-cmd", Hint: i18n.M.CmdReloadCmd},
	}
}

// CompletionData assembles what the composer's menu needs from the live
// session. Frontends read it from the controller rather than gathering it
// themselves, so the same session answers every one of them identically.
func (c *Controller) CompletionData() CompletionData {
	commands := c.Commands()
	skills := c.SlashSkills()
	d := CompletionData{
		ArgData: ArgData{
			Skills:          skills,
			DisabledSkills:  c.DisabledSkills(),
			ConfiguredMCP:   c.ConfiguredMCPNames(),
			DisconnectedMCP: c.DisconnectedMCPNames(),
			ModelRefs:       configuredModelRefs(),
			CurrentModel:    c.ModelRef(),
			ProviderNames:   configuredProviderNames(),
			PluginNames:     installedPluginNames(),
		},
		Names:         c.completionNames(commands, skills),
		WorkspaceRoot: c.WorkspaceRoot(),
	}
	d.CurrentProvider, _, _ = strings.Cut(d.CurrentModel, "/")
	d.MemoryRefs, d.MemoryArchives = MemoryCompletionData(c.Memory())
	if h := c.Host(); h != nil {
		d.ServerNames = h.ServerNames()
		d.Resources = h.Resources()
	}
	return d
}

// completionNames orders the catalogue the way Submit resolves it: a custom
// command beats a skill of the same name, and both beat nothing — so the menu's
// answer and the kernel's are the same answer.
func (c *Controller) completionNames(commands []command.Command, skills []skill.Skill) []SlashItem {
	docs := "/" + ResolvedBuiltinSlashName(DocsSlashName, commands, skills)
	items := make([]SlashItem, 0, len(commands)+len(skills)+len(SubmitSlashCommands()))
	for _, b := range SubmitSlashCommands() {
		if b.Label == "/docs" {
			b.Insert = docs + strings.TrimPrefix(b.Insert, b.Label)
			b.Label = docs
		}
		b.Kind = "builtin"
		items = append(items, b)
	}
	seen := map[string]bool{}
	for _, cmd := range commands {
		if cmd.Hidden || seen[cmd.Name] {
			continue
		}
		seen[cmd.Name] = true
		items = append(items, SlashItem{
			Label: "/" + cmd.Name, Insert: "/" + cmd.Name + " ",
			Hint: sourcedHint(cmd.Plugin, cmd.Description), Kind: "command",
		})
	}
	docsOwner := ResolveSlashCommandOwner(DocsSlashName, commands, skills)
	for _, sk := range skills {
		name := sk.SlashName()
		if name == "" || seen[name] || (docsOwner == SlashOwnerCustom && name == DocsSlashName) {
			continue
		}
		seen[name] = true
		kind := "skill"
		if sk.RunAs == skill.RunSubagent {
			kind = "subagent"
		}
		items = append(items, SlashItem{
			Label: "/" + name, Insert: "/" + name + " ",
			Hint: sourcedHint(sk.Plugin, sk.Description), Kind: kind,
		})
	}
	if h := c.Host(); h != nil {
		for _, p := range h.Prompts() {
			if seen[p.Name] {
				continue
			}
			seen[p.Name] = true
			items = append(items, SlashItem{
				Label: "/" + p.Name, Insert: "/" + p.Name + " ",
				Hint: p.Description, Kind: "prompt",
			})
		}
	}
	return items
}

// Where a command came from matters as much as what it does once plugins can
// install them: two rows named the same differ only by their source.
func sourcedHint(plugin, description string) string {
	if plugin == "" {
		return description
	}
	if description == "" {
		return "plugin " + plugin
	}
	return "plugin " + plugin + " · " + description
}

func configuredModelRefs() []string {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	var out []string
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if !p.Configured() {
			continue
		}
		for _, model := range p.ChatModelList() {
			out = append(out, p.Name+"/"+model)
		}
	}
	return out
}

func configuredProviderNames() []string {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	var out []string
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if p.Configured() {
			out = append(out, p.Name)
		}
	}
	return out
}

func installedPluginNames() []string {
	names, err := pluginpkg.InstalledNames(config.ReasonixHomeDir())
	if err != nil {
		return nil
	}
	return names
}

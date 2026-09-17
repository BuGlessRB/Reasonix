package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"reasonix/internal/browser"
	"reasonix/internal/tool"
)

func init() {
	tool.RegisterBuiltin(browserOpen{})
	tool.RegisterBuiltin(browserRead{})
	tool.RegisterBuiltin(browserAct{})
}

const (
	browserSnapshotLines = 250
	browserMaxLines      = 1000
	browserChangeLines   = 80
)

// BrowserTools binds the browser tools to one agent's browser session.
func BrowserTools(session *browser.Session) []tool.Tool {
	return []tool.Tool{browserOpen{session: session}, browserRead{session: session}, browserAct{session: session}}
}

// BrowserBound reports whether t is a browser tool with a browser session behind it.
func BrowserBound(t tool.Tool) bool {
	switch b := t.(type) {
	case browserOpen:
		return b.session != nil
	case browserRead:
		return b.session != nil
	case browserAct:
		return b.session != nil
	}
	return false
}

// withOrigin writes the host's account of which site a call concerns over any
// origin the model sent, or removes it when there is none.
func withOrigin(args json.RawMessage, origin string) json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(args, &fields) != nil || fields == nil {
		fields = map[string]json.RawMessage{}
	}
	delete(fields, "origin")
	if origin != "" {
		fields["origin"], _ = json.Marshal(origin)
	}
	out, err := json.Marshal(fields)
	if err != nil {
		return args
	}
	return out
}

func tabArg(args json.RawMessage) string {
	var p struct {
		Tab string `json:"tab"`
	}
	_ = json.Unmarshal(args, &p)
	return p.Tab
}

var errNoBrowser = &browser.Failure{Code: browser.CodeEngineMissing, Detail: "no browser is available in this runtime"}

type browserOpen struct{ session *browser.Session }

func (browserOpen) Name() string { return "browser_open" }

func (browserOpen) Description() string {
	return "Load a page in your browser and return its snapshot: one element per line, with [eN] refs for browser_act. " +
		"url is http(s), about:blank or a workspace file. Without url, tab switches to that tab; close closes it."
}

func (browserOpen) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"},"tab":{"type":"string","description":"t1, t2...; default active"},"new_tab":{"type":"boolean"},"close":{"type":"boolean"}}}`)
}

// PermissionArgs names the site being loaded, or the one a tab switch or close
// leaves the agent on.
func (b browserOpen) PermissionArgs(_ context.Context, args json.RawMessage) json.RawMessage {
	var p struct {
		URL string `json:"url"`
		Tab string `json:"tab"`
	}
	_ = json.Unmarshal(args, &p)
	if p.URL != "" {
		return withOrigin(args, browser.OriginOf(p.URL))
	}
	if b.session == nil {
		return withOrigin(args, "")
	}
	return withOrigin(args, b.session.Origin(p.Tab))
}

func (browserOpen) ReadOnly() bool                                   { return false }
func (browserOpen) Sequential(context.Context, json.RawMessage) bool { return true }

func (b browserOpen) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		URL    string `json:"url"`
		Tab    string `json:"tab"`
		NewTab bool   `json:"new_tab"`
		Close  bool   `json:"close"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if b.session == nil {
		return "", errNoBrowser
	}
	switch {
	case p.Close:
		if err := b.session.CloseTab(ctx, p.Tab); err != nil {
			return "", err
		}
		return renderTabs(b.session.Tabs()), nil
	case p.URL == "":
		if p.Tab == "" {
			return "", &browser.Failure{Code: browser.CodeBadStep, Detail: "give a url to load, or a tab to switch to"}
		}
		if _, err := b.session.Switch(p.Tab); err != nil {
			return "", err
		}
	default:
		if _, err := b.session.Open(ctx, p.URL, p.Tab, p.NewTab); err != nil {
			if browser.CodeOf(err) != browser.CodeNavigationTimeout {
				return "", err
			}
			out, _ := b.snapshot(ctx)
			return out, err
		}
	}
	return b.snapshot(ctx)
}

func (b browserOpen) snapshot(ctx context.Context) (string, error) {
	snap, err := b.session.Snapshot(ctx, "", "")
	if err != nil {
		return "", err
	}
	out := renderSnapshot(snap, 0, browserSnapshotLines)
	if tabs := b.session.Tabs(); len(tabs) > 1 {
		out += "\n" + renderTabs(tabs)
	}
	return out, nil
}

type browserRead struct{ session *browser.Session }

func (browserRead) Name() string { return "browser_read" }

func (browserRead) Description() string {
	return "Read a browser page: snapshot (default; page with offset/limit, or ref for one subtree), " +
		"screenshot (browser_act x/y use its pixels), logs (errors since you last read them), tabs."
}

func (browserRead) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"what":{"type":"string","enum":["snapshot","screenshot","logs","tabs"]},"tab":{"type":"string"},"ref":{"type":"string"},"offset":{"type":"integer"},"limit":{"type":"integer"}}}`)
}

// PermissionArgs names the site the tab being read shows.
func (b browserRead) PermissionArgs(_ context.Context, args json.RawMessage) json.RawMessage {
	if b.session == nil {
		return withOrigin(args, "")
	}
	return withOrigin(args, b.session.Origin(tabArg(args)))
}

func (browserRead) ReadOnly() bool                                   { return true }
func (browserRead) Sequential(context.Context, json.RawMessage) bool { return true }

func (b browserRead) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	text, _, err := b.ExecuteWithImages(ctx, args)
	return text, err
}

func (b browserRead) ExecuteWithImages(ctx context.Context, args json.RawMessage) (string, []string, error) {
	var p struct {
		What   string `json:"what"`
		Tab    string `json:"tab"`
		Ref    string `json:"ref"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", nil, fmt.Errorf("invalid args: %w", err)
	}
	if b.session == nil {
		return "", nil, errNoBrowser
	}
	switch p.What {
	case "", "snapshot":
		snap, err := b.session.Snapshot(ctx, p.Tab, p.Ref)
		if err != nil {
			return "", nil, err
		}
		limit := browserSnapshotLines
		if p.Limit > 0 {
			limit = min(p.Limit, browserMaxLines)
		}
		return renderSnapshot(snap, max(p.Offset, 0), limit), nil, nil
	case "screenshot":
		url, info, err := b.session.Screenshot(ctx, p.Tab)
		if err != nil {
			return "", nil, err
		}
		return tabLine(info) + "\n[image: screenshot]", []string{url}, nil
	case "logs":
		entries, info, err := b.session.Logs(p.Tab)
		if err != nil {
			return "", nil, err
		}
		if len(entries) == 0 {
			return tabLine(info) + "\nNothing new reported.", nil, nil
		}
		return tabLine(info) + "\n" + renderLogs(entries), nil, nil
	case "tabs":
		return renderTabs(b.session.Tabs()), nil, nil
	}
	return "", nil, &browser.Failure{Code: browser.CodeBadStep, Detail: fmt.Sprintf("what=%q is not one of snapshot, screenshot, logs, tabs", p.What)}
}

type browserAct struct{ session *browser.Session }

func (browserAct) Name() string { return "browser_act" }

func (browserAct) Description() string {
	return "Operate a browser page with real input. Steps run in order and stop at the first failure; target by ref, or x/y from a screenshot. " +
		"Returns each step's result, page errors and what changed. fill replaces a value, type inserts text; press takes keys like Enter or Control+a; " +
		"select sets a <select>; dialog answers alert/confirm/prompt."
}

func (browserAct) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"tab":{"type":"string"},"steps":{"type":"array","items":{"type":"object","properties":{"action":{"type":"string","enum":["click","double_click","hover","fill","type","press","select","scroll","wait","wait_for","dialog","back"]},"ref":{"type":"string"},"text":{"type":"string"},"key":{"type":"string"},"values":{"type":"array","items":{"type":"string"}},"x":{"type":"number"},"y":{"type":"number"},"delta_y":{"type":"number"},"ms":{"type":"integer"},"accept":{"type":"boolean"}},"required":["action"]}}},"required":["steps"]}`)
}

// PermissionArgs names the site the steps will operate.
func (b browserAct) PermissionArgs(_ context.Context, args json.RawMessage) json.RawMessage {
	if b.session == nil {
		return withOrigin(args, "")
	}
	return withOrigin(args, b.session.Origin(tabArg(args)))
}

func (browserAct) ReadOnly() bool                                   { return false }
func (browserAct) Sequential(context.Context, json.RawMessage) bool { return true }

func (b browserAct) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Tab   string         `json:"tab"`
		Steps []browser.Step `json:"steps"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if b.session == nil {
		return "", errNoBrowser
	}
	if len(p.Steps) == 0 {
		return "", &browser.Failure{Code: browser.CodeBadStep, Detail: "steps is empty"}
	}
	res, stepErr := b.session.Act(ctx, p.Tab, p.Steps)
	var out strings.Builder
	fmt.Fprintf(&out, "Completed %d of %d step(s).\n", res.Done, len(p.Steps))
	for i, note := range res.Notes {
		fmt.Fprintf(&out, "  %d. %s\n", i+1, note)
	}
	if stepErr != nil && res.FailedAt >= 0 {
		fmt.Fprintf(&out, "Step %d failed: %v\n", res.FailedAt+1, stepErr)
	}
	if len(res.Logs) > 0 {
		out.WriteString("The page reported:\n" + renderLogs(res.Logs))
	}
	if len(res.Opened) > 0 {
		fmt.Fprintf(&out, "Opened tab(s): %s (now active)\n", strings.Join(res.Opened, ", "))
	}
	if res.Tab.ID == "" {
		return out.String(), stepErr
	}
	out.WriteString(tabLine(res.Tab) + "\n")
	if res.Dialog != nil {
		fmt.Fprintf(&out, "A %s dialog is open: %q. Answer it with a dialog step.\n", res.Dialog.Type, res.Dialog.Message)
		return out.String(), stepErr
	}
	snap, changes, err := b.session.Refresh(ctx, res.Tab.ID)
	if err != nil {
		fmt.Fprintf(&out, "The page could not be read afterwards: %v\n", err)
		return out.String(), stepErr
	}
	out.WriteString(renderChanges(snap, changes))
	return out.String(), stepErr
}

func tabLine(info browser.TabInfo) string {
	title := info.Title
	if title == "" {
		title = "(untitled)"
	}
	return fmt.Sprintf("Tab %s: %s — %s", info.ID, title, info.URL)
}

func renderSnapshot(snap browser.Snapshot, offset, limit int) string {
	var b strings.Builder
	b.WriteString(tabLine(snap.Tab) + "\n")
	if snap.Dialog != nil {
		fmt.Fprintf(&b, "A %s dialog is open: %q. Answer it with a browser_act dialog step.\n", snap.Dialog.Type, snap.Dialog.Message)
	}
	total := len(snap.Lines)
	if total == 0 {
		b.WriteString("The page shows nothing.\n")
		return b.String()
	}
	start := min(offset, total)
	end := min(start+limit, total)
	if start > 0 || end < total {
		fmt.Fprintf(&b, "Lines %d-%d of %d.", start+1, end, total)
		if end < total {
			fmt.Fprintf(&b, " Continue with offset=%d.", end)
		}
		b.WriteString("\n")
	}
	for _, line := range snap.Lines[start:end] {
		b.WriteString(line + "\n")
	}
	return b.String()
}

func renderChanges(snap browser.Snapshot, c browser.Changes) string {
	if c.NewDocument {
		return "New page:\n" + strings.TrimPrefix(renderSnapshot(snap, 0, browserSnapshotLines), tabLine(snap.Tab)+"\n")
	}
	if len(c.Added) == 0 && len(c.Removed) == 0 {
		return "The page's accessibility tree did not change.\n"
	}
	var b strings.Builder
	b.WriteString("Changes since your last snapshot (+ appeared, - gone):\n")
	shown := 0
	for _, group := range []struct {
		mark  string
		lines []string
	}{{"+ ", c.Added}, {"- ", c.Removed}} {
		for _, line := range group.lines {
			if shown == browserChangeLines {
				fmt.Fprintf(&b, "... %d more changed line(s); read a snapshot for the whole page\n", len(c.Added)+len(c.Removed)-shown)
				return b.String()
			}
			b.WriteString(group.mark + strings.TrimLeft(line, " ") + "\n")
			shown++
		}
	}
	return b.String()
}

func renderLogs(entries []browser.LogEntry) string {
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "  [%s] %s: %s\n", e.Level, e.Kind, e.Text)
	}
	return b.String()
}

func renderTabs(tabs []browser.TabInfo) string {
	if len(tabs) == 0 {
		return "No tabs are open.\n"
	}
	var b strings.Builder
	b.WriteString("Tabs:\n")
	for _, t := range tabs {
		mark := " "
		if t.Active {
			mark = "*"
		}
		fmt.Fprintf(&b, "%s %s\n", mark, tabLine(t))
	}
	return b.String()
}

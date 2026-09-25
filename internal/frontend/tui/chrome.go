package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"reasonix/internal/frontend/termrender"
)

const (
	spinEvery    = 100 * time.Millisecond
	welcomeWidth = 64
	gaugeCells   = 8
)

var spinFrames = []string{"·", "✢", "✳", "✶", "✻", "✽", "✻", "✶", "✳", "✢"}

type spinMsg struct{}

func tickSpin() tea.Cmd {
	return tea.Tick(spinEvery, func(time.Time) tea.Msg { return spinMsg{} })
}

// greet prints the welcome card before anything else reaches the scrollback;
// it asks for the status itself because the first frame has not been drawn.
func (m *model) greet() tea.Cmd {
	return func() tea.Msg {
		s, _ := m.client.Status(m.ctx)
		return tea.Println(welcomeCard(m.opts, s.ModelRef, m.width))()
	}
}

func welcomeCard(opts Options, modelRef string, width int) string {
	w := min(width-2, welcomeWidth)
	title := termrender.Accent("◆ ") + termrender.Bold("Reasonix")
	if opts.Version != "" {
		title += termrender.Dim("  " + opts.Version)
	}
	if w < 30 {
		return title
	}
	inner := w - 4
	rows := []string{title, ""}
	if modelRef != "" {
		rows = append(rows, termrender.Dim("model  ")+oneLine(modelName(modelRef), inner-7))
	}
	dir := opts.Workspace
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if dir != "" {
		rows = append(rows, termrender.Dim("dir    ")+clipHead(homeRel(dir), inner-7))
	}
	rows = append(rows, "", termrender.Dim(oneLine("/ commands · @ files · ! shell · shift+tab mode", inner)))
	bar := strings.Repeat("─", w-2)
	out := []string{termrender.Accent("╭" + bar + "╮")}
	for _, r := range rows {
		out = append(out, termrender.Accent("│")+" "+termrender.PadRight(r, inner)+" "+termrender.Accent("│"))
	}
	return strings.Join(append(out, termrender.Accent("╰"+bar+"╯")), "\n")
}

// modelName drops the provider half of a ref that only repeats the model.
func modelName(ref string) string {
	if p, name, ok := strings.Cut(ref, "/"); ok && p == name {
		return name
	}
	return ref
}

func homeRel(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return p
}

// clipHead keeps the end of s, where a path says the most.
func clipHead(s string, width int) string {
	r := []rune(s)
	if width < 2 || len(r) <= width {
		return s
	}
	return "…" + string(r[len(r)-width+1:])
}

// noteRunning starts the spinner on the frame a turn begins.
func (m *model) noteRunning(was bool) tea.Cmd {
	if was || !m.tr.Running {
		return nil
	}
	m.runSince = time.Now()
	if m.spinning {
		return nil
	}
	m.spinning = true
	return tickSpin()
}

func (m *model) onSpin() tea.Cmd {
	if !m.tr.Running {
		m.spinning = false
		return nil
	}
	return tickSpin()
}

// spinnerLine says what the running turn is doing and for how long.
func (m *model) spinnerLine() string {
	since := time.Since(m.runSince)
	frame := spinFrames[int(since/spinEvery)%len(spinFrames)]
	return "  " + termrender.Accent(frame) + " " + termrender.Accent(m.activity()+"…") +
		termrender.Dim(fmt.Sprintf("  %ds · esc to interrupt", int(since.Seconds())))
}

func (m *model) activity() string {
	for _, it := range slices.Backward(m.tr.Items) {
		switch {
		case it.Kind == ItemTool && it.Running:
			return "Running " + termrender.ToolDisplayName(it.Tool.Name)
		case it.Kind == ItemSay && !it.Done && strings.TrimSpace(it.Text) != "":
			return "Writing"
		case it.Kind == ItemSay && !it.Done && it.Reasoning != "":
			return "Thinking"
		case it.Kind == ItemUser && !it.Pending:
			return "Working"
		}
	}
	return "Working"
}

func (m *model) rule() string {
	line := strings.Repeat("─", max(m.width-1, 1))
	if m.shell {
		return termrender.Yellow(line)
	}
	return termrender.Dim(line)
}

func modeBadge(mode string) string {
	c := termrender.ActiveTheme().Info
	switch mode {
	case "auto":
		c = termrender.ActiveTheme().Success
	case "yolo":
		c = termrender.ActiveTheme().Danger
	}
	return termrender.ThemeFg(c, "● "+mode)
}

// gauge draws how full the context window is, turning warn then red as it
// nears the point the kernel folds history.
func gauge(used, window int) string {
	frac := min(float64(used)/float64(window), 1)
	on := int(frac*gaugeCells + 0.5)
	bar := strings.Repeat("▰", on) + strings.Repeat("▱", gaugeCells-on)
	paint := termrender.Dim
	switch {
	case frac >= 0.8:
		paint = termrender.Red
	case frac >= 0.5:
		paint = termrender.Yellow
	}
	return paint(bar) + termrender.Dim(" "+tokens(used)+"/"+tokens(window))
}

// keyHints draws key/meaning pairs with the key standing out.
func keyHints(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, termrender.Accent(termrender.Bold(pairs[i]))+" "+termrender.Dim(pairs[i+1]))
	}
	return strings.Join(parts, termrender.Dim("  ·  "))
}

// card frames a prompt that is waiting on the user, so it reads as a question
// rather than as more of the transcript.
func card(lines []string, width int, edge func(string) string) []string {
	w := min(width-2, 88)
	if w < 20 {
		return lines
	}
	inner := w - 4
	bar := strings.Repeat("─", w-2)
	out := []string{"  " + edge("╭"+bar+"╮")}
	for _, l := range lines {
		out = append(out, "  "+edge("│")+" "+termrender.PadRight(clipVisible(l, inner), inner)+" "+edge("│"))
	}
	return append(out, "  "+edge("╰"+bar+"╯"))
}

func clipVisible(s string, width int) string {
	return ansi.Truncate(s, width, "…")
}

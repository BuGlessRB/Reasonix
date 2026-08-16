// desktop_config.go — the desktop-only preference block.
package config

// DesktopConfig controls desktop-only UI preferences. It is intentionally
// separate from top-level language and [ui] so desktop choices do not affect CLI
// language, terminal colours, or provider-visible prompt/request data.
type DesktopConfig struct {
	Language    string `toml:"language"`     // auto|en|zh; empty/auto = browser/OS auto-detect
	Currency    string `toml:"currency"`     // legacy display currency; migrated to [billing].display_currency
	LayoutStyle string `toml:"layout_style"` // classic|workbench|creation; desktop layout style
	Theme       string `toml:"theme"`        // auto|dark|light; empty resolves to auto
	ThemeStyle  string `toml:"theme_style"`  // graphite|aurora|slate|carbon|nocturne|amber and legacy aliases
	ThemePack   string `toml:"theme_pack"`   // installed pack id; empty is the default appearance
	// Welcomed records that the opening sequence has played. It lives in the
	// config rather than browser storage because clearing a cache is not the
	// same as meeting the app for the first time.
	Welcomed bool `toml:"welcomed"`
	// SurfaceSlots is where the user put an extension's surface, keyed
	// "<pluginId>:<surfaceId>". It outranks the extension's own suggestion.
	SurfaceSlots            map[string]string `toml:"surface_slots"`
	TerminalTheme           string            `toml:"terminal_theme"`             // auto|dark|light; auto follows the desktop app theme
	ExternalOpener          string            `toml:"external_opener"`            // preferred installed app used by the desktop Open control
	CloseBehavior           string            `toml:"close_behavior"`             // quit|background; desktop window close behavior
	DisplayMode             string            `toml:"display_mode"`               // standard|compact (legacy "minimal" maps to compact); transcript display mode
	StatusBarStyle          string            `toml:"status_bar_style"`           // icon|text; desktop status bar metric labels
	StatusBarItems          []string          `toml:"status_bar_items"`           // ordered visible desktop status bar items
	DefaultToolApprovalMode string            `toml:"default_tool_approval_mode"` // ask|auto|yolo; defaults to auto for newly-created desktop sessions
	CheckUpdates            *bool             `toml:"check_updates"`              // startup update checks; nil keeps the default enabled
	UpdateChannel           string            `toml:"update_channel"`             // legacy: read for compatibility, never written back
	Telemetry               *bool             `toml:"telemetry"`                  // anonymous launch ping plus scrubbed next-launch native crash diagnostics; nil keeps the default enabled
	Metrics                 *bool             `toml:"metrics"`                    // aggregate desktop metrics (anonymous signal/bucket counts, including lifecycle health; no content); nil keeps the default enabled
	ProviderAccess          []string          `toml:"provider_access"`            // desktop-only list of provider entries shown in Settings > Model > Access
	ExpandThinking          bool              `toml:"expand_thinking"`            // deprecated compatibility alias: true maps to auto
	ReasoningDisplayMode    string            `toml:"reasoning_display_mode"`
	ConversationWidth       string            `toml:"conversation_width"` // standard|full; max transcript width; empty = standard
	PinnedVersion           string            `toml:"pinned_version"`     // release pinned by a rollback; empty follows the channel
	// MaxPanes caps concurrently driven sessions. Each pane owns plugin
	// subprocesses, so the ceiling is real work; 0 keeps the default.
	MaxPanes   int              `toml:"max_panes"`
	Appearance AppearanceConfig `toml:"appearance"`
}

// DesktopMaxPanes resolves the pane ceiling: the configured value clamped to a
// range one process can plausibly hold, or the default when unset.
func (c *Config) DesktopMaxPanes(fallback int) int {
	if c == nil || c.Desktop.MaxPanes <= 0 {
		return fallback
	}
	return min(c.Desktop.MaxPanes, 32)
}

// AppearanceConfig is what the user set for themselves, on top of whichever
// theme pack is active: how large it all is, what it is set in, and their own
// picture. A pack ships a palette; these are the reader's own eyes and desk.
// Zero means "unset" throughout, so an untouched config resolves to the
// stylesheet's own defaults rather than to a number written here.
type AppearanceConfig struct {
	Zoom      float64         `toml:"zoom"`      // whole-interface scale, 0.8..1.6; 0 = 1.0
	ReadSize  float64         `toml:"read_size"` // transcript body size in px; 0 = the stylesheet's
	FontUI    string          `toml:"font_ui"`   // CSS font-family list for the interface
	FontMono  string          `toml:"font_mono"` // CSS font-family list for code and output
	Wallpaper WallpaperConfig `toml:"wallpaper"`
}

// WallpaperConfig is the user's own background image. File is a name inside
// the appearance directory, not a path: the bytes live where the app can find
// them after the picture the user picked has moved or been deleted.
type WallpaperConfig struct {
	File    string  `toml:"file"`
	Opacity float64 `toml:"opacity"` // at rest, 0..1
	Dim     float64 `toml:"dim"`     // scrim of page colour over it, 0..1
	FocusX  float64 `toml:"focus_x"` // 0..1, the point that survives cropping
	FocusY  float64 `toml:"focus_y"` // 0..1
}

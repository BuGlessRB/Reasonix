// Package theme reads installed theme packs. A pack is data — named colours
// for a light and a dark scheme — with no code, stylesheet, or script in it,
// so anything that can write a JSON file can author one and no frontend has to
// trust what it loads.
package theme

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"reasonix/internal/config"
)

// The packs that ship. Only their manifests travel — a pack is colours, and
// the previews and backgrounds the old shell also carried are megabytes that
// no reader here looks at.
//
//go:embed builtin/*.json
var builtin embed.FS

const (
	manifestName = "theme.json"
	dirName      = "themes"
	// schemaVersion is the only shape this reader understands. A pack that
	// declares a newer one is skipped rather than half-read: a theme is
	// cosmetic, and guessing at unknown fields is worse than staying default.
	schemaVersion = 1
)

// Pack is one installed theme. Tokens are keyed by scheme ("light"/"dark")
// and then by the pack's own colour names; a frontend maps those onto its own
// variables, because only it knows what each surface in its layout is called.
type Pack struct {
	ID          string                       `json:"id"`
	Name        string                       `json:"name"`
	Author      string                       `json:"author,omitempty"`
	Description string                       `json:"description,omitempty"`
	Tokens      map[string]map[string]string `json:"tokens"`
}

type manifest struct {
	SchemaVersion int                          `json:"schemaVersion"`
	ID            string                       `json:"id"`
	Name          string                       `json:"name"`
	Author        string                       `json:"author"`
	Description   string                       `json:"description"`
	Tokens        map[string]map[string]string `json:"tokens"`
}

// Dir is where user-installed packs live: one directory per pack, each with a
// theme.json. It is under the memory root so a pack an agent writes lands
// beside the rest of what the user's agent has authored.
func Dir() string {
	return filepath.Join(config.MemoryUserDir(), dirName)
}

// List returns every readable pack, shipped and installed, sorted by id. A
// missing directory is not an error — it means nothing is installed. An
// unreadable or malformed pack is skipped so one bad file cannot hide the
// rest. An installed pack shadows a shipped one of the same id: the user's
// copy of a palette is the one they meant.
func List() []Pack {
	byID := map[string]Pack{}
	for _, pack := range listBuiltin() {
		byID[pack.ID] = pack
	}
	if entries, err := os.ReadDir(Dir()); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			pack, err := load(filepath.Join(Dir(), e.Name(), manifestName), e.Name())
			if err != nil {
				continue
			}
			byID[pack.ID] = pack
		}
	}
	out := make([]Pack, 0, len(byID))
	for _, pack := range byID {
		out = append(out, pack)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func listBuiltin() []Pack {
	entries, err := fs.ReadDir(builtin, "builtin")
	if err != nil {
		return nil
	}
	var out []Pack
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".json")
		raw, err := builtin.ReadFile("builtin/" + e.Name())
		if err != nil {
			continue
		}
		pack, err := decode(raw, id)
		if err != nil {
			continue
		}
		out = append(out, pack)
	}
	return out
}

// Load returns one pack by id.
func Load(id string) (Pack, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Pack{}, fmt.Errorf("theme: empty id")
	}
	if id != filepath.Base(id) || strings.Contains(id, string(filepath.Separator)) {
		return Pack{}, fmt.Errorf("theme: %q is not a pack id", id)
	}
	if pack, err := load(filepath.Join(Dir(), id, manifestName), id); err == nil {
		return pack, nil
	}
	raw, err := builtin.ReadFile("builtin/" + id + ".json")
	if err != nil {
		return Pack{}, fmt.Errorf("theme: no pack %q", id)
	}
	return decode(raw, id)
}

func load(path, dirID string) (Pack, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Pack{}, err
	}
	return decode(raw, dirID)
}

func decode(raw []byte, dirID string) (Pack, error) {
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Pack{}, fmt.Errorf("theme %s: %w", dirID, err)
	}
	if m.SchemaVersion != schemaVersion {
		return Pack{}, fmt.Errorf("theme %s: unsupported schemaVersion %d", dirID, m.SchemaVersion)
	}
	// The directory name is the address the user activates by, so it wins over
	// whatever the manifest claims its id is.
	id := dirID
	name := strings.TrimSpace(m.Name)
	if name == "" {
		name = id
	}
	tokens := map[string]map[string]string{}
	for _, scheme := range []string{"light", "dark"} {
		if len(m.Tokens[scheme]) == 0 {
			return Pack{}, fmt.Errorf("theme %s: no %s tokens", id, scheme)
		}
		copied := make(map[string]string, len(m.Tokens[scheme]))
		for k, v := range m.Tokens[scheme] {
			if isColour(v) {
				copied[k] = v
			}
		}
		if len(copied) == 0 {
			return Pack{}, fmt.Errorf("theme %s: no usable %s colours", id, scheme)
		}
		tokens[scheme] = copied
	}
	return Pack{ID: id, Name: name, Author: strings.TrimSpace(m.Author), Description: strings.TrimSpace(m.Description), Tokens: tokens}, nil
}

// isColour keeps a token only when it is a plain hex colour. The value is
// interpolated straight into a stylesheet, so anything that could carry a
// url(), an expression, or a closing brace is dropped rather than escaped.
func isColour(v string) bool {
	v = strings.TrimSpace(v)
	if len(v) != 4 && len(v) != 7 && len(v) != 9 {
		return false
	}
	if v[0] != '#' {
		return false
	}
	for _, r := range v[1:] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

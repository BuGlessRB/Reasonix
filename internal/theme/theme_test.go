package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func writePack(t *testing.T, root, id, body string) {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const minimal = `{"schemaVersion":1,"id":"x","name":"Dusk","author":"me",
 "tokens":{"light":{"bg":"#FFFFFF","fg":"#111111"},"dark":{"bg":"#000000","fg":"#EEEEEE"}}}`

// The pack directory is the address a user activates by, so it is what the
// reader reports — a manifest claiming someone else's id cannot shadow them.
func TestLoadUsesTheDirectoryNameAsID(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	writePack(t, Dir(), "dusk", minimal)

	pack, err := Load("dusk")
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "dusk" || pack.Name != "Dusk" {
		t.Fatalf("pack = %+v, want the directory id and manifest name", pack)
	}
	if pack.Tokens["dark"]["bg"] != "#000000" {
		t.Fatalf("dark tokens = %+v", pack.Tokens["dark"])
	}
}

// A token value is interpolated into a stylesheet by whoever renders it, so
// anything that is not a plain hex colour is dropped here rather than escaped
// downstream — a pack cannot smuggle a url(), an expression, or a brace.
func TestLoadKeepsOnlyPlainHexColours(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	writePack(t, Dir(), "sneaky", `{"schemaVersion":1,"name":"Sneaky","tokens":{
	  "light":{"bg":"#FFFFFF","evil":"url(http://x/y.png)","brace":"#fff}body{display:none","expr":"var(--x)"},
	  "dark":{"bg":"#000000"}}}`)

	pack, err := Load("sneaky")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pack.Tokens["light"]["bg"]; !ok {
		t.Fatal("the plain colour was dropped")
	}
	for _, key := range []string{"evil", "brace", "expr"} {
		if v, ok := pack.Tokens["light"][key]; ok {
			t.Fatalf("token %q survived as %q", key, v)
		}
	}
}

// A cosmetic file is not worth guessing at: a pack from a newer schema is
// skipped, and one bad pack must not hide the ones that are fine.
func TestListSkipsUnreadablePacksWithoutHidingTheRest(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	writePack(t, Dir(), "good", minimal)
	writePack(t, Dir(), "future", `{"schemaVersion":99,"name":"Future","tokens":{"light":{"bg":"#fff"},"dark":{"bg":"#000"}}}`)
	writePack(t, Dir(), "broken", `not json`)
	writePack(t, Dir(), "empty-dark", `{"schemaVersion":1,"name":"Half","tokens":{"light":{"bg":"#fff"}}}`)

	packs := List()
	if len(packs) != 1 || packs[0].ID != "good" {
		t.Fatalf("List = %+v, want only the readable pack", packs)
	}
}

func TestListWithNothingInstalledIsEmptyNotAnError(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	if packs := List(); len(packs) != 0 {
		t.Fatalf("List = %+v, want empty", packs)
	}
}

func TestLoadRejectsAPathInsteadOfAnID(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	for _, id := range []string{"../escape", "a/b", ""} {
		if _, err := Load(id); err == nil {
			t.Fatalf("Load(%q) succeeded, want a rejection", id)
		}
	}
}

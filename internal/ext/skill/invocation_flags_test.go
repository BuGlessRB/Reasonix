package skill

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

func flagStore(t *testing.T) *Store {
	t.Helper()
	home := testenv.TempDir(t)
	writeSkill(t, home, ".claude/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: ship it to prod\ndisable-model-invocation: true\n---\nDEPLOY BODY")
	writeSkill(t, home, ".claude/skills/legacyctx/SKILL.md", "---\nname: legacyctx\ndescription: legacy context\nuser-invocable: false\nargument-hint: \"[env]\"\n---\nCTX BODY")
	writeSkill(t, home, ".claude/skills/plain/SKILL.md", "---\nname: plain\ndescription: ordinary\n---\nPLAIN BODY")
	return New(Options{HomeDir: home, DisableBuiltins: true})
}

func TestFrontmatterInvocationFlagsParse(t *testing.T) {
	store := flagStore(t)
	deploy, _ := store.Read("deploy")
	if !deploy.DisableModelInvocation || deploy.DisableUserInvocation {
		t.Fatalf("deploy flags = %+v", deploy)
	}
	bg, _ := store.Read("legacyctx")
	if bg.DisableModelInvocation || !bg.DisableUserInvocation || bg.ArgumentHint != "[env]" {
		t.Fatalf("legacyctx flags = %+v", bg)
	}
	plain, _ := store.Read("plain")
	if plain.DisableModelInvocation || plain.DisableUserInvocation || plain.ArgumentHint != "" {
		t.Fatalf("plain flags = %+v", plain)
	}
}

func TestDisableModelInvocationLeavesModelListing(t *testing.T) {
	store := flagStore(t)
	index := IndexBlock(store.List())
	if strings.Contains(index, "deploy") {
		t.Fatalf("disable-model-invocation skill leaked into the model listing:\n%s", index)
	}
	if !strings.Contains(index, "legacyctx") || !strings.Contains(index, "plain") {
		t.Fatalf("model-invocable skills missing:\n%s", index)
	}
	if got := ModelInvocable(store.List()); len(got) != 2 {
		t.Fatalf("ModelInvocable = %d skills, want 2", len(got))
	}
}

func TestModelToolsRefuseDisabledSkillWithTypedError(t *testing.T) {
	store := flagStore(t)
	for name, tl := range map[string]interface {
		Execute(context.Context, json.RawMessage) (string, error)
	}{
		"run_skill":  NewRunSkillTool(store, nil),
		"read_skill": NewReadSkillTool(store),
	} {
		_, err := tl.Execute(context.Background(), json.RawMessage(`{"name":"deploy"}`))
		if !errors.Is(err, ErrModelInvocationDisabled) {
			t.Fatalf("%s error = %v, want ErrModelInvocationDisabled", name, err)
		}
	}
	out, err := NewRunSkillTool(store, nil).Execute(context.Background(), json.RawMessage(`{"name":"legacyctx"}`))
	if err != nil || !strings.Contains(out, "CTX BODY") {
		t.Fatalf("model-only skill must stay invocable: %q %v", out, err)
	}
}

func TestDisableUserInvocationLeavesSlashSurface(t *testing.T) {
	store := flagStore(t)
	var names []string
	for _, sk := range store.SlashList() {
		names = append(names, sk.SlashName())
	}
	if strings.Join(names, ",") != "deploy,plain" {
		t.Fatalf("SlashList = %v, want deploy and plain only", names)
	}
	if _, ok := store.ReadSlash("legacyctx"); ok {
		t.Fatal("/legacyctx must not resolve when user-invocable is false")
	}
	if _, ok := store.ReadSlash("deploy"); !ok {
		t.Fatal("/deploy must stay reachable by the user")
	}
	if _, ok := store.Read("legacyctx"); !ok {
		t.Fatal("the model-side registry must still hold legacyctx")
	}
}

package workspaceid

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

func sharesRepo(a, b string) bool {
	repo := RepoDir(a)
	return repo != "" && repo == RepoDir(b)
}

func TestKeyPathWhenNotARepository(t *testing.T) {
	dir := testenv.TempDir(t)
	key := Key(dir)
	if !strings.HasPrefix(key, PathPrefix) {
		t.Fatalf("Key(%q) = %q, want a %s key", dir, key, PathPrefix)
	}
	if RepoDir(dir) != "" {
		t.Fatalf("RepoDir(%q) = %q, want empty", dir, RepoDir(dir))
	}
}

func TestKeyRepoForRootAndSubdirectory(t *testing.T) {
	repo := testenv.TempDir(t)
	mkdir(t, filepath.Join(repo, ".git"))
	sub := filepath.Join(repo, "internal", "agent")
	mkdir(t, sub)

	rootKey := Key(repo)
	if !strings.HasPrefix(rootKey, RepoPrefix) {
		t.Fatalf("Key(repo) = %q, want a %s key", rootKey, RepoPrefix)
	}
	if got := Key(sub); got != rootKey {
		t.Fatalf("Key(subdir) = %q, want the repository key %q", got, rootKey)
	}
	info := Describe(sub)
	if info.CheckoutRoot != canonical(repo) || info.Prefix != filepath.Join("internal", "agent") {
		t.Fatalf("checkout ownership lost selected subdirectory: %+v", info)
	}
}

// A linked worktree must land on the same key as the tree it was cut from, or
// a switch flipped in the main tree would silently not apply in the worktree.
func TestKeyLinkedWorktreeSharesTheRepositoryKey(t *testing.T) {
	base := testenv.TempDir(t)
	repo := filepath.Join(base, "repo")
	gitDir := filepath.Join(repo, ".git")
	mkdir(t, gitDir)
	mkdir(t, filepath.Join(gitDir, "worktrees", "studio"))

	tree := filepath.Join(base, "trees", "studio")
	mkdir(t, tree)
	writeFile(t, filepath.Join(tree, ".git"),
		"gitdir: "+filepath.Join(gitDir, "worktrees", "studio")+"\n")

	if got, want := Key(tree), Key(repo); got != want {
		t.Fatalf("worktree key = %q, main tree key = %q, want equal", got, want)
	}
	if !sharesRepo(tree, repo) {
		t.Fatal("sharesRepo(worktree, repo) = false, want true")
	}
}

func TestKeyRelativeGitdirPointer(t *testing.T) {
	base := testenv.TempDir(t)
	repo := filepath.Join(base, "repo")
	mkdir(t, filepath.Join(repo, ".git", "worktrees", "wt"))
	tree := filepath.Join(base, "wt")
	mkdir(t, tree)
	writeFile(t, filepath.Join(tree, ".git"), "gitdir: ../repo/.git/worktrees/wt\n")

	if got, want := Key(tree), Key(repo); got != want {
		t.Fatalf("relative-pointer worktree key = %q, want %q", got, want)
	}
}

// A submodule is its own repository: its gitdir has no worktrees element, so it
// must not converge onto the superproject's key.
func TestKeySubmoduleStaysItsOwnRepository(t *testing.T) {
	base := testenv.TempDir(t)
	super := filepath.Join(base, "super")
	mkdir(t, filepath.Join(super, ".git", "modules", "vendor"))
	sub := filepath.Join(super, "vendor")
	mkdir(t, sub)
	writeFile(t, filepath.Join(sub, ".git"), "gitdir: ../.git/modules/vendor\n")

	if Key(sub) == Key(super) {
		t.Fatalf("submodule shares the superproject key %q", Key(sub))
	}
	if !strings.HasPrefix(Key(sub), RepoPrefix) {
		t.Fatalf("Key(submodule) = %q, want a %s key", Key(sub), RepoPrefix)
	}
}

// Two plain folders are two projects. Walking up for a repository must not
// collapse them onto the drive the walk ended on.
func TestKeyDistinctPerFolderOutsideRepositories(t *testing.T) {
	a, b := testenv.TempDir(t), testenv.TempDir(t)
	if Key(a) == Key(b) {
		t.Fatalf("two plain folders share key %q", Key(a))
	}
	if Key(a) != Key(a+string(filepath.Separator)) {
		t.Fatal("Key is not stable across spellings of one folder")
	}
}

func TestKeyEmptyRoot(t *testing.T) {
	if got := Key("  "); got != "" {
		t.Fatalf("Key(blank) = %q, want empty", got)
	}
}

func TestSharesRepoIsFalseOutsideRepositories(t *testing.T) {
	a, b := testenv.TempDir(t), testenv.TempDir(t)
	if sharesRepo(a, b) {
		t.Fatal("SharesRepo on two plain folders = true, want false")
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// A repository that is moved or renamed must keep its key: the identity is its
// first commit, not the path its git dir happens to sit at.
func TestKeySurvivesRepositoryRename(t *testing.T) {
	requireGit(t)
	base := testenv.TempDir(t)
	repo := filepath.Join(base, "before")
	initGitRepo(t, repo)
	before := Key(repo)
	if !strings.HasPrefix(before, RepoPrefix) {
		t.Fatalf("Key(repo) = %q, want a %s key", before, RepoPrefix)
	}
	moved := filepath.Join(base, "after")
	if err := os.Rename(repo, moved); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got := Key(moved); got != before {
		t.Fatalf("key after rename = %q, want the pre-rename key %q", got, before)
	}
}

// Packing the objects (git gc) must not change the key either.
func TestKeyStableAcrossGitGc(t *testing.T) {
	requireGit(t)
	repo := filepath.Join(testenv.TempDir(t), "repo")
	initGitRepo(t, repo)
	before := Key(repo)
	runGit(t, repo, "gc", "-q")
	if got := Key(repo); got != before {
		t.Fatalf("key after gc = %q, want %q", got, before)
	}
}

// A linked worktree cut from the repo lands on the same key as the main tree.
func TestKeyRealWorktreeSharesRepositoryKey(t *testing.T) {
	requireGit(t)
	base := testenv.TempDir(t)
	repo := filepath.Join(base, "repo")
	initGitRepo(t, repo)
	runGit(t, repo, "branch", "feature")
	tree := filepath.Join(base, "tree")
	runGit(t, repo, "worktree", "add", "-q", tree, "feature")
	if got, want := Key(tree), Key(repo); got != want {
		t.Fatalf("worktree key = %q, main tree key = %q, want equal", got, want)
	}
}

// The legacy key is the pre-move-stable git-dir digest, kept so an override
// written by an older build still resolves; it must differ from the current key.
func TestDescribeReportsLegacyRepoKey(t *testing.T) {
	requireGit(t)
	repo := filepath.Join(testenv.TempDir(t), "repo")
	initGitRepo(t, repo)
	info := Describe(repo)
	if info.Key == "" || info.LegacyRepoKey == "" {
		t.Fatalf("Describe = %+v, want both keys set", info)
	}
	if info.Key == info.LegacyRepoKey {
		t.Fatalf("Key and LegacyRepoKey both = %q, want the new key to differ", info.Key)
	}
	if !strings.HasPrefix(info.LegacyRepoKey, RepoPrefix) {
		t.Fatalf("LegacyRepoKey = %q, want a %s key", info.LegacyRepoKey, RepoPrefix)
	}
}

// A repository with no commits yet has no first commit, so it keys by path and
// the next commit moves it onto the stable key.
func TestKeyEmptyRepositoryFallsBackToGitDir(t *testing.T) {
	requireGit(t)
	repo := filepath.Join(testenv.TempDir(t), "repo")
	mkdir(t, repo)
	runGit(t, repo, "init", "-q", "-b", "main")
	info := Describe(repo)
	if info.Key != info.LegacyRepoKey {
		t.Fatalf("Key = %q, LegacyRepoKey = %q, want the fallback", info.Key, info.LegacyRepoKey)
	}
}

// A shallow clone's grafted boundary moves when the clone is deepened, so the
// key must not be derived from it.
func TestShallowCloneKeysByPath(t *testing.T) {
	requireGit(t)
	repo := filepath.Join(testenv.TempDir(t), "repo")
	initGitRepo(t, repo)
	gitDir := filepath.Join(repo, ".git")
	head, err := exec.Command("git", "--git-dir="+gitDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	writeFile(t, filepath.Join(gitDir, "shallow"), string(head))
	info := Describe(repo)
	if info.Key != info.LegacyRepoKey {
		t.Fatalf("Key = %q, LegacyRepoKey = %q, want the shallow fallback", info.Key, info.LegacyRepoKey)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("git repository fixtures are exercised on POSIX in this suite")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	mkdir(t, dir)
	runGit(t, dir, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, "f.txt"), "hi\n")
	runGit(t, dir, "add", "f.txt")
	runGit(t, dir, "-c", "user.email=t@example.com", "-c", "user.name=test", "commit", "-q", "-m", "init")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

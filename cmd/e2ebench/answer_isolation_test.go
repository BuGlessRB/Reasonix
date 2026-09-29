package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/safety/sandbox"
)

func TestMemoryBenchRequiresReadSandbox(t *testing.T) {
	tasks, err := loadTasks("../../benchmarks/memorybench")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) == 0 {
		t.Fatal("MemoryBench corpus is empty")
	}
	for _, task := range tasks {
		if task.answerRoot == "" {
			t.Fatalf("%s can run without the answer boundary", task.ID)
		}
	}
	err = validateAnswerIsolation(tasks)
	if sandbox.Available() && err != nil {
		t.Fatal(err)
	}
	if !sandbox.Available() && err == nil {
		t.Fatal("MemoryBench accepted a host with no OS read sandbox")
	}
	if sandbox.Available() {
		previous := benchStateBase
		benchStateBase = func() (string, error) { return filepath.Join(tasks[0].answerRoot, "cache"), nil }
		t.Cleanup(func() { benchStateBase = previous })
		if err := validateAnswerIsolation(tasks); err == nil {
			t.Fatal("MemoryBench accepted a state root inside the forbidden checkout")
		}
	}
}

func TestMemoryBenchAnswerIsolationAtSandboxBoundary(t *testing.T) {
	if !sandbox.Available() {
		t.Skip("this host has no OS read sandbox")
	}
	const suite = "../../benchmarks/memorybench"
	tasks, err := loadTasks(suite)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) == 0 || tasks[0].answerRoot == "" {
		t.Fatal("MemoryBench answer material was not identified")
	}
	if err := validateAnswerIsolation(tasks); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := stageAnswerIsolation(tasks[0].answerRoot, work); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("[sandbox]\nbash = \"off\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REASONIX_HOME", home)
	cfg, err := config.LoadForRoot(work)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BashMode() != "enforce" {
		t.Fatalf("sandbox mode = %q, want enforce even when the user config disables it", cfg.BashMode())
	}
	visible := filepath.Join(work, "visible.txt")
	if err := os.WriteFile(visible, []byte("visible-control"), 0o600); err != nil {
		t.Fatal(err)
	}
	answer := filepath.Join(suite, "tasks", "mb-contradiction", "memory", "project", "package-manager.md")
	answer, err = filepath.Abs(answer)
	if err != nil {
		t.Fatal(err)
	}
	spec := sandbox.Spec{Mode: cfg.BashMode(), WriteRoots: []string{work}, ForbidReadRoots: cfg.ForbidReadRootsForRoot(work)}
	read := func(path string) (string, error) {
		t.Helper()
		argv, wrapped := sandbox.CommandArgs(spec, []string{"cat", path})
		if !wrapped {
			t.Fatal("OS sandbox did not wrap the command")
		}
		out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
		return string(out), err
	}
	if got, err := read(visible); err != nil || !strings.Contains(got, "visible-control") {
		t.Fatalf("control file read = %q, %v", got, err)
	}
	if got, err := read(answer); err == nil || strings.Contains(got, "npm install") {
		t.Fatalf("answer file remained readable through the sandbox: %q, %v", got, err)
	}
}

func TestAnswerIsolationRefusesUnsafeWorkdirAndExistingConfig(t *testing.T) {
	const answerRoot = "../../benchmarks/memorybench/tasks"
	if err := stageAnswerIsolation(answerRoot, filepath.Dir(answerRoot)); err == nil {
		t.Fatal("workdir inside the checkout was accepted")
	}
	work := t.TempDir()
	path := filepath.Join(work, "reasonix.toml")
	if err := os.WriteFile(path, []byte("# task fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := stageAnswerIsolation(answerRoot, work); err == nil {
		t.Fatal("existing task config was overwritten")
	}
	if data, _ := os.ReadFile(path); string(data) != "# task fixture\n" {
		t.Fatalf("task config changed: %q", data)
	}
}

func TestRunTaskDropsIsolationConfigBeforeGrading(t *testing.T) {
	requireShellStub(t)
	suite := t.TempDir()
	taskDir := filepath.Join(suite, "tasks", "demo")
	if err := os.MkdirAll(filepath.Join(taskDir, "memory", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	verify := "#!/usr/bin/env bash\nset -e\ntest ! -e reasonix.toml\ngrep -q '^ok$' answer.txt\n"
	if err := os.WriteFile(filepath.Join(taskDir, "verify.sh"), []byte(verify), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "fake-agent")
	script := `#!/usr/bin/env bash
set -e
test -f reasonix.toml
printf 'ok\n' > answer.txt
while [ "$#" -gt 0 ]; do
  if [ "$1" = --metrics ]; then
    printf '{"complete":true}\n' > "$2"
    break
  fi
  shift
done
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	tk := task{ID: "demo", Prompt: "write the answer", TimeoutSec: 30, dir: taskDir, answerRoot: filepath.Join(suite, "tasks")}
	r := runTask(suiteConfig{bin: bin}, tk)
	if !r.Passed {
		t.Fatalf("isolated task failed grading: %+v", r)
	}
}

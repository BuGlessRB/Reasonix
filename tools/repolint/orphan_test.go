package main

import (
	"strings"
	"testing"
)

func orphanFindings(t *testing.T, files map[string]string) []Finding {
	t.Helper()
	scan := newOrphanScan()
	for rel, body := range files {
		src := parseBytes(rel, []byte(body))
		if src == nil {
			t.Fatalf("parse %s", rel)
		}
		scan.observe(src)
	}
	return scan.findings()
}

func names(findings []Finding) string {
	var out []string
	for _, f := range findings {
		out = append(out, f.Msg)
	}
	return strings.Join(out, "\n")
}

func TestOrphanReportsExportedKernelFuncWithNoCaller(t *testing.T) {
	got := orphanFindings(t, map[string]string{
		"internal/repair/config.go": "package repair\nfunc RecordHealthyConfig(v string) error { return nil }\n",
	})
	if !strings.Contains(names(got), "RecordHealthyConfig") {
		t.Fatalf("uncalled exported kernel func not reported: %q", names(got))
	}
}

// A doc comment naming the function is what makes an orphan look wired; it is
// not a caller and must not silence the finding.
func TestOrphanIgnoresCommentMentions(t *testing.T) {
	got := orphanFindings(t, map[string]string{
		"internal/repair/config.go": "package repair\nfunc RecordHealthyConfig(v string) error { return nil }\n",
		"internal/config/lkg.go":    "package config\n\n// Written by repair.RecordHealthyConfig after a successful boot.\nfunc Path() string { return \"\" }\n",
	})
	if !strings.Contains(names(got), "RecordHealthyConfig") {
		t.Fatal("a comment mention silenced the finding")
	}
}

// Tests are what keep an orphan green; they are not evidence of a caller.
func TestOrphanIgnoresTestCallers(t *testing.T) {
	got := orphanFindings(t, map[string]string{
		"internal/repair/config.go":      "package repair\nfunc RecordHealthyConfig(v string) error { return nil }\n",
		"internal/repair/config_test.go": "package repair\nfunc TestX(t *T) { RecordHealthyConfig(\"v\") }\n",
	})
	if !strings.Contains(names(got), "RecordHealthyConfig") {
		t.Fatal("a test caller silenced the finding")
	}
}

// The desktop module is a real caller of internal/; a scan that stopped at one
// module would call its APIs dead.
func TestOrphanCountsCallersInOtherModules(t *testing.T) {
	got := orphanFindings(t, map[string]string{
		"internal/repair/config.go": "package repair\nfunc RecordHealthyConfig(v string) error { return nil }\n",
		"desktop/next/main.go":      "package main\nfunc main() { repair.RecordHealthyConfig(\"1\") }\n",
	})
	if strings.Contains(names(got), "RecordHealthyConfig") {
		t.Fatalf("a desktop-module caller was not counted: %q", names(got))
	}
}

func TestOrphanSkipsUnexportedAndNonKernel(t *testing.T) {
	got := orphanFindings(t, map[string]string{
		"internal/repair/config.go": "package repair\nfunc recordHealthy(v string) error { return nil }\n",
		"cmd/reasonix/main.go":      "package main\nfunc Unused() {}\n",
	})
	if n := names(got); n != "" {
		t.Fatalf("reported outside the kernel export surface: %q", n)
	}
}

func TestOrphanSkipsTestSupportPackages(t *testing.T) {
	got := orphanFindings(t, map[string]string{
		"internal/agent/testutil/fake.go": "package testutil\nfunc NewFake() int { return 0 }\n",
		"internal/testenv/home.go":        "package testenv\nfunc IsolateUserState() {}\n",
		"internal/remote/sshtest/s.go":    "package sshtest\nfunc Start() {}\n",
	})
	if n := names(got); n != "" {
		t.Fatalf("reported a test-support package: %q", n)
	}
}

func TestOrphanExemptsStandardInterfaceMethods(t *testing.T) {
	got := orphanFindings(t, map[string]string{
		"internal/x/e.go": "package x\ntype E struct{}\nfunc (E) Error() string { return \"\" }\nfunc (E) Close() error { return nil }\n",
	})
	if n := names(got); n != "" {
		t.Fatalf("reported a standard interface method: %q", n)
	}
}

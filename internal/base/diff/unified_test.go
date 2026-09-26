package diff

import "testing"

const sampleGitDiff = "diff --git a/x.go b/x.go\n" +
	"index 112294521..f9fdd2c37 100644\n" +
	"--- a/x.go\n" +
	"+++ b/x.go\n" +
	"@@ -1 +1 @@\n" +
	"-old\n" +
	"+new\n"

func TestIsUnifiedDiff(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"git-style whole diff", sampleGitDiff, true},
		{"leading blank lines are skipped", "\n\n" + sampleGitDiff, true},
		{"--- style header", "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n", true},
		{"empty", "", false},
		{"blank only", "\n \n", false},
		{"plain prose", "Replaced all 15 em dashes in README.md.\n", false},
		{"mixed output", "building...\n" + sampleGitDiff, false},
		{"stat only, no hunk", "diff --git a/x b/x\n x | 2 +-\n 1 file changed, 1 insertion(+), 1 deletion(-)\n", false},
		{"hunk but no changed line", "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n same\n", false},
		{"no hunk", "diff --git a/x b/x\n--- a/x\n+++ b/x\n-old\n+new\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUnifiedDiff(tc.in); got != tc.want {
				t.Fatalf("IsUnifiedDiff = %v, want %v", got, tc.want)
			}
		})
	}
}

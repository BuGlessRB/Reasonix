// Command reasonix is a config- and plugin-driven coding agent CLI.
package main

import (
	"os"
	"runtime/debug"

	"reasonix/internal/contract/config"
	"reasonix/internal/frontend/cli"
	"reasonix/internal/platform/crashreport"

	// Blank imports wire compile-time built-ins into their registries.
	_ "reasonix/internal/model/anthropic"
	_ "reasonix/internal/model/openai"
	_ "reasonix/internal/model/responses"
	_ "reasonix/internal/tools/builtin"
)

// Build identity injected via -ldflags (see Makefile). version remains the
// single-line contract for `reasonix --version`; gitCommit/buildTimeUTC feed
// `reasonix version --verbose` / `--json` without embedding config paths.
var (
	version      = "dev"
	gitCommit    = ""
	buildTimeUTC = ""
)

// runCLI is the CLI entry; tests may stub it. Production routes through
// RunWithBuildInfo so ldflags metadata is available to version --verbose/--json.
var runCLI = func(args []string, buildVersion string) int {
	return cli.RunWithBuildInfo(args, cli.BuildInfo{
		Version:      buildVersion,
		GitCommit:    gitCommit,
		BuildTimeUTC: buildTimeUTC,
	})
}

func main() {
	os.Exit(runWithCrashCapture(os.Args[1:], version))
}

func runWithCrashCapture(args []string, buildVersion string) (exitCode int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = crashreport.CapturePanic(config.ReasonixHomeDir(), buildVersion, recovered, debug.Stack())
			panic(recovered)
		}
	}()
	return runCLI(args, buildVersion)
}

package shellsafe

import "strings"

// ArgvCarriesInlineCode reports whether an argv runs a program written in the
// call itself — `python -c`, `node -e`, `bash -c`, `deno eval`. Permission and
// evidence must agree on it, so it lives here rather than once in each, where
// the two copies had already drifted over the shells and smaller interpreters.
func ArgvCarriesInlineCode(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	args := argv[1:]
	switch ExecutableBase(argv[0]) {
	case "bash", "dash", "fish", "ksh", "sh", "zsh":
		return hasShellCommandFlag(args)
	case "powershell", "pwsh":
		return hasInlineFlag(args, "-c", "-command", "-e", "-enc", "-encodedcommand")
	case "cmd":
		return hasInlineFlag(args, "/c", "/k")
	case "node", "bun":
		return hasInlineFlag(args, "-e", "--eval", "-p", "--print")
	case "deno":
		return hasInlineFlag(args, "eval")
	case "python", "python3", "py", "pypy", "pypy3":
		return hasInlineFlag(args, "-c")
	case "perl", "ruby", "lua", "luajit", "r", "rscript", "osascript":
		return hasInlineFlag(args, "-e")
	case "php":
		return hasInlineFlag(args, "-r")
	}
	return false
}

// ExecutableBase strips directory and .exe so a path and a bare name classify
// the same. It is exported because every caller of the rules here needs it.
func ExecutableBase(command string) string {
	if i := strings.LastIndexAny(command, `/\`); i >= 0 {
		command = command[i+1:]
	}
	return strings.TrimSuffix(strings.ToLower(command), ".exe")
}

// hasShellCommandFlag reports a POSIX shell's -c, including bundled forms such
// as `sh -lc`. A bare `--` ends option parsing, so nothing after it counts.
func hasShellCommandFlag(args []string) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		switch {
		case lower == "--":
			return false
		case lower == "--command":
			return true
		case strings.HasPrefix(lower, "-") && !strings.HasPrefix(lower, "--") && strings.Contains(lower[1:], "c"):
			return true
		}
	}
	return false
}

func hasInlineFlag(args []string, candidates ...string) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		for _, candidate := range candidates {
			if inlineFlagMatches(lower, candidate) {
				return true
			}
		}
	}
	return false
}

// inlineFlagMatches accepts the spellings a flag actually takes: exact, an
// attached value (--eval=…, -enc:…), and a bundled short form (-pe).
func inlineFlagMatches(arg, candidate string) bool {
	switch {
	case arg == candidate:
		return true
	case strings.HasPrefix(candidate, "--"):
		return strings.HasPrefix(arg, candidate+"=") || strings.HasPrefix(arg, candidate+":")
	case strings.HasPrefix(candidate, "/"):
		return len(candidate) == 2 && strings.HasPrefix(arg, candidate)
	case strings.HasPrefix(candidate, "-") && len(candidate) == 2:
		return strings.HasPrefix(arg, candidate) && !strings.HasPrefix(arg, "--")
	case strings.HasPrefix(candidate, "-"):
		return strings.HasPrefix(arg, candidate+":")
	}
	return false
}

package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"golang.org/x/term"

	"reasonix/internal/base/i18n"
	"reasonix/internal/contract/config"
	"reasonix/internal/frontend/serve"
	"reasonix/internal/frontend/termrender"
	"reasonix/internal/frontend/tui"
	"reasonix/internal/session/control"
	"reasonix/internal/state/sessionstore"
)

// tuiBase is the route prefix of the one runtime a terminal session drives.
// The host name is never resolved: the in-process transport answers it.
const tuiBase = "http://reasonix.local/rt/r1"

// runTUI starts the terminal UI on a kernel in this process. The UI reaches
// the kernel through the same routes Studio uses, over a transport that opens
// no port, so there is nothing on the machine to authenticate against.
func runTUI(args []string, version string) int {
	defer closeCLIUsageCatalogs()
	fs := pflag.NewFlagSet("tui", pflag.ContinueOnError)
	model := fs.String("model", "", "provider name (default: config default_model)")
	preset := fs.String("preset", "balanced", "agent execution setting: light | balanced | delivery")
	dir := fs.String("dir", "", "change to this directory first (project root)")
	inline := fs.Bool("inline", false, "write the conversation into the terminal's scrollback instead of taking the full screen")
	permissionMode := fs.String("permission-mode", "ask", "permission mode: manual | ask | auto | acceptEdits | dontAsk | plan | bypassPermissions | danger-full-access")
	cont := registerContinueFlag(fs)
	resume := fs.StringP("resume", "r", "", "resume by session file path, session ID, or machine session ID; bare -r picks one (takes precedence over --continue)")
	fs.Lookup("resume").NoOptDefVal = resumePickerSentinel
	if code, ok := parseCommandFlags(fs, normalizeOptionalResumeArg(args)); !ok {
		return code
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "reasonix tui needs an interactive terminal; use `reasonix run` for scripts")
		return 2
	}
	profile, err := parseRuntimeProfile(*preset)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 2
	}
	permissions, err := parsePermissionMode(*permissionMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 2
	}
	workspaceRoot, err := workspaceRootForDir(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	resumePath, err := tuiResumePath(workspaceRoot, *resume, *cont)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	termrender.ConfigureThemeFromConfigForTTYOutput()
	restoreLog := routeLogsAwayFromTerminal()
	defer restoreLog()

	ctx := context.Background()
	leases := control.NewSessionLeaseKeeper()
	defer leases.Release()
	var resumed *sessionstore.Session
	if resumePath != "" {
		if err := leases.Rebind(resumePath); err != nil {
			if errors.Is(err, sessionstore.ErrSessionLeaseHeld) {
				err = errors.New(control.SessionInUseMessage(err) + "; " + control.SessionLeaseCloseHint)
			}
			fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
			return 1
		}
		if resumed, err = loadResumableSession(resumePath); err != nil {
			fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
			return 1
		}
	}
	bc := serve.NewBroadcaster()
	cfg, _ := config.Load()
	ctrl, err := setupProfileWithOverrides(ctx, *model, 0, false, withNotifications(bc, cfg), profile, cliBuildOverrides{
		Version: version, WorkspaceRoot: workspaceRoot, OnSessionRecovered: cliSessionRecoveredHandler(leases),
		PermissionAllow: permissions.allow,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	defer ctrl.Close()
	SetTaskJobKiller(ctrlKillerAdapter{ctrl})
	if resumed != nil {
		_ = ctrl.Resume(resumed, resumePath)
	}
	ctrl.EnsureSessionPath()
	if err := rebindCLIControllerAuthority(leases, ctrl); err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, control.SessionInUseMessage(err)+"; "+control.SessionLeaseCloseHint)
		return 1
	}
	if fs.Changed("permission-mode") {
		ctrl.SetToolApprovalMode(permissions.approval)
		if permissions.plan {
			ctrl.SetPlanMode(true)
		}
	}
	serveCfg := config.ServeConfig{AuthMode: "none"}
	hub := serve.NewHub(serve.HubOptions{Serve: serveCfg})
	defer hub.Shutdown()
	adoptFirstPane(hub, ctrl, bc, bc, serveCfg, leases)

	err = tui.Run(ctx, tui.Options{
		Client:      &tui.Client{HTTP: hub.InProcessClient(), Base: tuiBase},
		Prompt:      strings.Join(fs.Args(), " "),
		Restore:     resumed != nil,
		PickSession: *resume == resumePickerSentinel,
		Inline:      *inline,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	return 0
}

func tuiResumePath(workspaceRoot, resume string, cont bool) (string, error) {
	sessionDir := resolveCLISessionDirFor(workspaceRoot)
	if q := strings.TrimSpace(resume); q != "" {
		return resolveSessionQuery(sessionDir, q)
	}
	if !cont {
		return "", nil
	}
	reclaimCLIRecoveryBranches(sessionDir)
	session, ok := mostRecentSession(sessionDir)
	if !ok {
		return "", errors.New(i18n.M.NoSessionToResume)
	}
	return session.Path, nil
}

// routeLogsAwayFromTerminal sends the kernel's logging to a file for as long
// as the UI owns the screen: a line written to stderr lands in the middle of
// the frame the UI is drawing.
func routeLogsAwayFromTerminal() func() {
	prev := slog.Default()
	path := filepath.Join(config.ReasonixHomeDir(), "logs", "tui.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		slog.SetDefault(slog.New(slog.DiscardHandler))
		return func() { slog.SetDefault(prev) }
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		slog.SetDefault(slog.New(slog.DiscardHandler))
		return func() { slog.SetDefault(prev) }
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, nil)))
	return func() {
		slog.SetDefault(prev)
		_ = f.Close()
	}
}

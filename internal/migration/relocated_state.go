package migration

import (
	"fmt"
	"os"
	"path/filepath"

	"reasonix/internal/config"
	"reasonix/internal/event"
)

// AdoptRelocatedStateEntries brings across what an earlier relocation left in
// the home root. The wallpaper and theme pack a config still names are the
// visible half; the repair backups an update rolls back to are the half nobody
// notices until they need it.
func AdoptRelocatedStateEntries(sink event.Sink) []string {
	// Same rule the automatic importers follow: a run that redirected its roots
	// for this process did not ask for the production install to be moved into
	// it. A relocation lives in config and is the user's standing choice.
	if config.IsolatedHomeDir() != "" || config.IsolatedStateDir() != "" {
		return nil
	}
	state, home := config.MemoryUserDir(), config.ReasonixHomeDir()
	if state == "" || home == "" || samePath(state, home) {
		return nil
	}
	var adopted []string
	for _, name := range config.StateRootEntriesEarlyMovesLeft {
		src := filepath.Join(home, name)
		if !isDir(src) {
			continue
		}
		// Only what the new root does not have. A directory already there is
		// this install's, and copying over it would resurrect what was deleted.
		if isDir(filepath.Join(state, name)) {
			continue
		}
		n, err := copyMissingTree(src, filepath.Join(state, name))
		if err != nil {
			sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
				Text: fmt.Sprintf("could not bring %s across from the previous storage location: %v", name, err)})
			continue
		}
		if n > 0 {
			adopted = append(adopted, name)
		}
	}
	if len(adopted) > 0 {
		sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
			Text: fmt.Sprintf("brought %v across from the previous storage location", adopted)})
	}
	return adopted
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

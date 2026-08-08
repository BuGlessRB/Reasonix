// Package winappid gives every Reasonix desktop process and shortcut one
// explicit Windows AppUserModelID (AUMID). Without one, Windows keys taskbar
// grouping on the full executable path: the pinned launcher
// (Reasonix.exe / reasonix-launcher.exe) and the versioned desktop it spawns
// (versions/<v>/reasonix-desktop.exe) live at different paths, so Explorer
// shows two taskbar buttons for one app.
//
// The desktop process registers the ID on itself (SetProcessID), and the
// launcher stamps the same ID onto the pinned taskbar shortcut before the
// desktop starts (EnsureShortcutIDs), so the first click after pinning already
// converges onto a single button. The ID is a stable constant: it must never
// include the version, or pinned buttons would break on update.
package winappid

// ID is the AppUserModelID shared by the desktop process and every pinned
// Reasonix launcher shortcut.
const ID = "Reasonix.Desktop"

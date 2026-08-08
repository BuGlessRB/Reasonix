//go:build windows

package winappid

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Property-system identifiers for System.AppUserModel.ID on shell links.
var (
	// pkeyAppUserModelID is PKEY_AppUserModelID, the property Explorer's
	// taskbar grouping keys on.
	pkeyAppUserModelID = propertyKey{
		fmtid: windows.GUID{
			Data1: 0x9F4C2855,
			Data2: 0x9F79,
			Data3: 0x4B39,
			Data4: [8]byte{0xA8, 0xD0, 0xE1, 0xD4, 0x2D, 0xE1, 0xD5, 0xF3},
		},
		pid: 5,
	}
	// iidIPropertyStore is IID_IPropertyStore.
	iidIPropertyStore = windows.GUID{
		Data1: 0x886D8EEB,
		Data2: 0x8CF2,
		Data3: 0x4446,
		Data4: [8]byte{0x8D, 0x02, 0xCD, 0xBA, 0x1D, 0xBD, 0xCF, 0x99},
	}
)

const (
	coInitApartmentThreaded = 0x2 // COINIT_APARTMENTTHREADED
	stgmReadWrite           = 0x2 // STGM_READWRITE
	vtLpwstr                = 31  // VT_LPWSTR

	shcneUpdateItem = 0x00002000 // SHCNE_UPDATEITEM
	shcnfPathW      = 0x0005     // SHCNF_PATHW
)

// pinnedLauncherNames are the taskbar shortcut names the user can pin for this
// product: the portable alias (Reasonix.exe), the stable launcher
// (reasonix-launcher.exe), and the versioned desktop binary. Explorer names a
// pinned shortcut after its target executable.
var pinnedLauncherNames = []string{
	"Reasonix.exe.lnk",
	"reasonix-launcher.exe.lnk",
	"reasonix-desktop.exe.lnk",
}

var (
	// windowsPinnedTaskbarDir and windowsShortcutSetter are seams for tests.
	windowsPinnedTaskbarDir = pinnedTaskbarDir
	windowsShortcutSetter   = setShortcutID
)

// SetProcessID registers ID as this process's AppUserModelID so Explorer
// groups its windows with any shortcut carrying the same ID. It must run
// before the process registers its first window; after that the identity is
// locked in for the lifetime of the process.
func SetProcessID() error {
	idPtr, err := windows.UTF16PtrFromString(ID)
	if err != nil {
		return err
	}
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("SetCurrentProcessExplicitAppUserModelID")
	hr, _, _ := proc.Call(uintptr(unsafe.Pointer(idPtr)))
	if hr != 0 {
		return fmt.Errorf("set explicit AppUserModelID: hr=0x%08x", uint32(hr))
	}
	return nil
}

// EnsureShortcutIDs stamps ID onto every Reasonix taskbar-pinned shortcut.
// The launcher calls this before spawning the desktop (and the desktop again
// before its first window), so the pinned button and the running window share
// one identity and never split into two taskbar icons. Best effort: when
// nothing is pinned there is nothing to do, and a failed property write is
// reported but never blocks launching.
func EnsureShortcutIDs() error {
	taskbar, err := windowsPinnedTaskbarDir()
	if err != nil {
		// Nothing pinned yet: the user has not asked for a taskbar button.
		return nil
	}
	var out error
	for _, name := range pinnedLauncherNames {
		path := filepath.Join(taskbar, name)
		if err := windowsShortcutSetter(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			out = errors.Join(out, fmt.Errorf("%s: %w", path, err))
		}
	}
	return out
}

// pinnedTaskbarDir resolves the per-user folder holding taskbar-pinned
// shortcuts (%APPDATA%\Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar).
func pinnedTaskbarDir() (string, error) {
	pinned, err := windows.KnownFolderPath(windows.FOLDERID_UserPinned, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", err
	}
	return filepath.Join(pinned, "TaskBar"), nil
}

// setShortcutID writes ID into the shortcut's System.AppUserModel.ID property
// via the shell's IPropertyStore and asks Explorer to refresh the item. Only
// regular files are touched; directories and dangling links are skipped.
func setShortcutID(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, coInitApartmentThreaded); err != nil && err != syscall.Errno(1) {
		// S_FALSE (1) merely means the apartment is already initialized.
		return err
	}
	defer windows.CoUninitialize()

	store, err := openPropertyStore(path)
	if err != nil {
		return err
	}
	defer store.Release()

	idPtr, err := windows.UTF16PtrFromString(ID)
	if err != nil {
		return err
	}
	value := propVariant{vt: vtLpwstr, pwszVal: idPtr}
	if hr := store.SetValue(&pkeyAppUserModelID, &value); hr != 0 {
		return fmt.Errorf("set AppUserModelID property: hr=0x%08x", uint32(hr))
	}
	if hr := store.Commit(); hr != 0 {
		return fmt.Errorf("commit AppUserModelID property: hr=0x%08x", uint32(hr))
	}
	notifyShortcutChanged(path)
	return nil
}

// openPropertyStore opens the shell's property store for a shortcut path.
// STGM_READWRITE is tried first; some files are only readable (e.g. held by
// another handle), and the shell property handler can still persist a write
// opened read-only.
func openPropertyStore(path string) (*iPropertyStore, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("SHGetPropertyStoreFromParsingName")
	var lastHR uintptr
	for _, mode := range []uint32{stgmReadWrite, 0 /* STGM_READ */} {
		var store *iPropertyStore
		hr, _, _ := proc.Call(
			uintptr(unsafe.Pointer(pathPtr)),
			0, // pbc: no bind context
			uintptr(mode),
			uintptr(unsafe.Pointer(&iidIPropertyStore)),
			uintptr(unsafe.Pointer(&store)),
		)
		lastHR = hr
		if hr == 0 && store != nil {
			return store, nil
		}
	}
	return nil, fmt.Errorf("open shortcut property store: hr=0x%08x", uint32(lastHR))
}

// notifyShortcutChanged invalidates one shortcut in Explorer's cache so the
// new property is picked up without a full association flush.
func notifyShortcutChanged(path string) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify")
	_, _, _ = proc.Call(shcneUpdateItem, shcnfPathW, uintptr(unsafe.Pointer(pathPtr)), 0)
}

// iPropertyStore is a minimal raw view of the COM IPropertyStore interface.
// The only methods used are SetValue and Commit; QueryInterface/AddRef exist
// only to keep the vtable layout honest.
type iPropertyStore struct {
	vtbl *iPropertyStoreVtbl
}

type iPropertyStoreVtbl struct {
	queryInterface uintptr
	addRef         uintptr
	release        uintptr
	getCount       uintptr
	getAt          uintptr
	getValue       uintptr
	setValue       uintptr
	commit         uintptr
}

// Release drops one reference. The store is owned by this thread's apartment
// and must be released before CoUninitialize.
func (s *iPropertyStore) Release() {
	syscall.SyscallN(s.vtbl.release, uintptr(unsafe.Pointer(s)))
}

// SetValue sets one property on the shortcut.
func (s *iPropertyStore) SetValue(key *propertyKey, value *propVariant) (hr uintptr) {
	r1, _, _ := syscall.SyscallN(s.vtbl.setValue, uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(value)))
	return r1
}

// Commit persists the pending property changes to the shortcut file.
func (s *iPropertyStore) Commit() (hr uintptr) {
	r1, _, _ := syscall.SyscallN(s.vtbl.commit, uintptr(unsafe.Pointer(s)))
	return r1
}

// propertyKey mirrors the PROPERTYKEY structure (fmtid + property id).
type propertyKey struct {
	fmtid windows.GUID
	pid   uint32
}

// propVariant mirrors the leading fields of PROPVARIANT, enough for the
// VT_LPWSTR values this package writes; the remaining union fields overlap
// pwszVal and are never touched.
type propVariant struct {
	vt         uint16
	wReserved1 uint16
	wReserved2 uint16
	wReserved3 uint16
	pwszVal    *uint16
}

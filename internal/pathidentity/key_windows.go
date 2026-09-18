//go:build windows

package pathidentity

import (
	"encoding/binary"
	"errors"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func platformIdentityKey(path string) (string, error) {
	return windowsIdentityKeyBy(path, directoryCaseInsensitive)
}

func windowsIdentityKeyBy(path string, caseForDirectory func(string) (bool, bool, error)) (string, error) {
	path = stripExtendedPrefix(path)
	volume := filepath.VolumeName(path)
	if volume == "" {
		return "", errors.New("windows path has no volume")
	}
	current := volume + string(filepath.Separator)
	identity := strings.ToLower(current)
	caseInsensitive := true
	rest := strings.TrimLeft(path[len(volume):], `\/`)
	for _, component := range strings.FieldsFunc(rest, func(r rune) bool { return r == '\\' || r == '/' }) {
		insensitive, exists, err := caseForDirectory(current)
		if err != nil {
			return "", err
		}
		if exists {
			caseInsensitive = insensitive
		}
		identityComponent := component
		if caseInsensitive {
			identityComponent = strings.ToLower(component)
		}
		identity = filepath.Join(identity, identityComponent)
		current = filepath.Join(current, component)
	}
	return filepath.Clean(identity), nil
}

func directoryCaseInsensitive(path string) (insensitive, exists bool, err error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, false, err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return false, false, nil
		}
		return false, false, err
	}
	defer windows.CloseHandle(handle)
	var info [4]byte
	err = windows.GetFileInformationByHandleEx(handle, windows.FileCaseSensitiveInfo, &info[0], uint32(len(info)))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_SUPPORTED) {
		return true, true, nil
	}
	if err != nil {
		return false, false, err
	}
	return binary.LittleEndian.Uint32(info[:])&windows.FILE_CS_FLAG_CASE_SENSITIVE_DIR == 0, true, nil
}

func stripExtendedPrefix(path string) string {
	if strings.HasPrefix(strings.ToUpper(path), `\\?\UNC\`) {
		return `\\` + path[len(`\\?\UNC\`):]
	}
	return strings.TrimPrefix(path, `\\?\`)
}

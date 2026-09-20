//go:build windows

package winaclresidue

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func installLegacyDeny(t *testing.T, path, userSID string) {
	t.Helper()
	if err := icacls(path, "/deny", "*"+userSID+":(RX)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = icacls(path, "/remove:d", "*"+userSID, "/C") })
}

// writeMarker records a deny for path under a marker owned by pid. Our own PID
// stands for a crashed predecessor after PID reuse; the parent test runner's
// PID stands for a live owner.
func writeMarker(t *testing.T, pid int, path string) string {
	t.Helper()
	if err := os.MkdirAll(markerDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(markerDir(), strconv.Itoa(pid)+"-credential-test.txt")
	if err := os.WriteFile(marker, []byte("deny\t"+path+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return marker
}

func TestRepairLegacyCredentialDenyRemovesExactCurrentUserACE(t *testing.T) {
	t.Setenv("TEMP", t.TempDir())
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("KEY=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	userSID, err := currentProcessUserSIDString()
	if err != nil {
		t.Fatal(err)
	}
	installLegacyDeny(t, path, userSID)
	legacy, other, err := currentUserDenyACECounts(path, userSID)
	if err != nil || legacy != 1 || other != 0 {
		t.Fatalf("deny counts before repair = legacy:%d other:%d err:%v", legacy, other, err)
	}
	marker := writeMarker(t, os.Getpid(), path)

	if err := RepairLegacyCredentialDeny(path); err != nil {
		t.Fatal(err)
	}
	legacy, other, err = currentUserDenyACECounts(path, userSID)
	if err != nil || legacy != 0 || other != 0 {
		t.Fatalf("deny counts after repair = legacy:%d other:%d err:%v", legacy, other, err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "KEY=value\n" {
		t.Fatalf("credential after repair = %q, %v", data, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("stale marker survived repair: %v", err)
	}
	installLegacyDeny(t, path, userSID)
	if err := RepairLegacyCredentialDeny(path); err == nil {
		t.Fatal("repair reused a consumed stale marker")
	}
}

func TestRepairLegacyCredentialDenyMatchesFileAcrossPathAliases(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("KEY=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	shortBuffer := make([]uint16, 32768)
	n, err := windows.GetShortPathName(pathUTF16, &shortBuffer[0], uint32(len(shortBuffer)))
	if err != nil || n == 0 || n >= uint32(len(shortBuffer)) {
		t.Skipf("8.3 path aliases unavailable: %v", err)
	}
	alias := windows.UTF16ToString(shortBuffer[:n])
	if strings.EqualFold(filepath.Clean(alias), filepath.Clean(path)) {
		t.Skip("fixture path has no distinct 8.3 alias")
	}
	userSID, err := currentProcessUserSIDString()
	if err != nil {
		t.Fatal(err)
	}
	installLegacyDeny(t, path, userSID)
	writeMarker(t, os.Getpid(), alias)
	if err := RepairLegacyCredentialDeny(path); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "KEY=value\n" {
		t.Fatalf("credential after aliased repair = %q, %v", data, err)
	}
}

func TestRepairLegacyCredentialDenyPreservesUnattributedAndLiveACL(t *testing.T) {
	for _, source := range []string{"missing marker", "live marker", "wrong path"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("TEMP", t.TempDir())
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte("KEY=value\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			userSID, err := currentProcessUserSIDString()
			if err != nil {
				t.Fatal(err)
			}
			installLegacyDeny(t, path, userSID)
			switch source {
			case "live marker":
				writeMarker(t, os.Getppid(), path)
			case "wrong path":
				writeMarker(t, os.Getpid(), path+"-other")
			}
			before, err := pathDACLSDDL(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := RepairLegacyCredentialDeny(path); err == nil {
				t.Fatal("repair accepted an unattributed or live deny")
			}
			after, err := pathDACLSDDL(path)
			if err != nil || after != before {
				t.Fatalf("DACL changed: before %s, after %s, err %v", before, after, err)
			}
		})
	}
}

func TestRepairLegacyCredentialDenyLeavesOrdinaryACLAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("KEY=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := pathDACLSDDL(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := RepairLegacyCredentialDeny(path); err != nil {
		t.Fatal(err)
	}
	after, err := pathDACLSDDL(path)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("ordinary DACL changed:\nbefore %s\nafter  %s", before, after)
	}
}

func TestRepairLegacyCredentialDenyRefusesMixedCurrentUserDenyACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("KEY=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	userSID, err := currentProcessUserSIDString()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString(fmt.Sprintf(
		"D:(D;;0x%x;;;%s)(D;;0x2;;;%s)(A;;FA;;;%s)",
		uint32(legacyCredentialDenyMask), userSID, userSID, userSID,
	))
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(sd)
	legacy, other, err := currentUserDenyACECounts(path, userSID)
	if err != nil || legacy != 1 || other == 0 {
		t.Fatalf("deny counts before repair = legacy:%d other:%d err:%v", legacy, other, err)
	}
	before, err := pathDACLSDDL(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := RepairLegacyCredentialDeny(path); err == nil {
		t.Fatal("RepairLegacyCredentialDeny accepted mixed current-user deny ACL")
	}
	after, err := pathDACLSDDL(path)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("mixed DACL changed:\nbefore %s\nafter  %s", before, after)
	}
}

func pathDACLSDDL(path string) (string, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		return "", err
	}
	return sd.String(), nil
}

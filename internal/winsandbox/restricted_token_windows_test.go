//go:build windows

package winsandbox

import (
	"errors"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsRestrictedTokenDefaultDACLAllowsSessionIPC(t *testing.T) {
	capabilitySID, err := windows.StringToSid(deriveCapabilitySID(capabilitySessionTemp, `c:\session-temp`))
	if err != nil {
		t.Fatal(err)
	}
	token, err := createWriteRestrictedPrimaryToken(false, []restrictedCapability{{
		purpose: capabilitySessionTemp,
		sid:     capabilitySID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer token.Close()

	current := windows.GetCurrentProcessToken()
	logonSID, err := currentProcessLogonSID(current)
	if err != nil {
		t.Fatal(err)
	}
	for name, sid := range map[string]*windows.SID{
		"logon":      logonSID,
		"capability": capabilitySID,
	} {
		if !tokenDefaultDACLGrantsForTest(t, token, sid, windows.ACCESS_MASK(fileAllAccess)) {
			t.Fatalf("restricted token default DACL does not grant full access to %s SID", name)
		}
	}
}

func tokenDefaultDACLGrantsForTest(t *testing.T, token windows.Token, sid *windows.SID, mask windows.ACCESS_MASK) bool {
	t.Helper()
	var needed uint32
	if err := windows.GetTokenInformation(token, windows.TokenDefaultDacl, nil, 0, &needed); err != nil && !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		t.Fatal(err)
	}
	buffer := make([]byte, needed)
	if err := windows.GetTokenInformation(token, windows.TokenDefaultDacl, &buffer[0], uint32(len(buffer)), &needed); err != nil {
		t.Fatal(err)
	}
	acl := (*tokenDefaultDACL)(unsafe.Pointer(&buffer[0])).DefaultDACL
	if acl == nil {
		return false
	}
	for index := range uint32(acl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, index, &ace); err != nil {
			t.Fatal(err)
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask&mask != mask {
			continue
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if windows.EqualSid(aceSID, sid) {
			runtime.KeepAlive(buffer)
			return true
		}
	}
	runtime.KeepAlive(buffer)
	return false
}

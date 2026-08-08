package winappid

import (
	"reflect"
	"testing"
)

func TestIDStable(t *testing.T) {
	const want = "Reasonix.Desktop"
	if ID != want {
		t.Fatalf("ID = %q; want %q", ID, want)
	}
}

func TestPinnedLauncherNames(t *testing.T) {
	want := []string{"Reasonix.exe.lnk", "reasonix-launcher.exe.lnk", "reasonix-desktop.exe.lnk"}
	if !reflect.DeepEqual(pinnedLauncherNames, want) {
		t.Fatalf("pinnedLauncherNames = %v; want %v", pinnedLauncherNames, want)
	}
}

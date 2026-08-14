package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func makeArchive(t *testing.T, headers []tar.Header, bodies [][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for i := range headers {
		header := headers[i]
		if err := tw.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if i < len(bodies) {
			if _, err := tw.Write(bodies[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func regularMember(name string, body []byte) tar.Header {
	return tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
}

func completeRelease() ([]tar.Header, [][]byte) {
	return []tar.Header{
			regularMember("reasonix-desktop", []byte("desktop")),
			regularMember("reasonix-guard", []byte("guard")),
			regularMember("reasonix", []byte("cli")),
		}, [][]byte{
			[]byte("desktop"), []byte("guard"), []byte("cli"),
		}
}

// Every member is resolved by basename, so an archive carrying two paths that
// end in the same name is ambiguous about which one gets published.
func TestExtractReleaseUnitRejectsAmbiguousMembers(t *testing.T) {
	headers, bodies := completeRelease()
	if got, err := ExtractReleaseUnit(makeArchive(t, headers, bodies)); err != nil ||
		string(got["reasonix-desktop"]) != "desktop" {
		t.Fatalf("complete release extraction = %v, %q", err, got["reasonix-desktop"])
	}

	duplicateHeaders := append(append([]tar.Header(nil), headers...), regularMember("nested/reasonix", []byte("duplicate")))
	duplicateBodies := append(append([][]byte(nil), bodies...), []byte("duplicate"))
	if _, err := ExtractReleaseUnit(makeArchive(t, duplicateHeaders, duplicateBodies)); err == nil ||
		!strings.Contains(err.Error(), "appears more than once") {
		t.Fatalf("duplicate release member error = %v", err)
	}

	nonRegular := append([]tar.Header(nil), headers...)
	nonRegular[1] = tar.Header{Name: "reasonix-guard", Typeflag: tar.TypeSymlink, Linkname: "outside"}
	nonRegularBodies := [][]byte{bodies[0], nil, bodies[2]}
	if _, err := ExtractReleaseUnit(makeArchive(t, nonRegular, nonRegularBodies)); err == nil ||
		!strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("non-regular release member error = %v", err)
	}
}

// A missing member must fail before anything is published: the pointer swap
// would otherwise make a version directory current that has no binary in it.
func TestExtractReleaseUnitRequiresEveryMember(t *testing.T) {
	headers, bodies := completeRelease()
	for i, missing := range releaseUnitMembers {
		short := append([]tar.Header(nil), headers[:i]...)
		short = append(short, headers[i+1:]...)
		shortBodies := append([][]byte(nil), bodies[:i]...)
		shortBodies = append(shortBodies, bodies[i+1:]...)
		_, err := ExtractReleaseUnit(makeArchive(t, short, shortBodies))
		if err == nil || !strings.Contains(err.Error(), missing) {
			t.Fatalf("archive without %s: err = %v, want it named", missing, err)
		}
	}
}

package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// releaseUnitMembers are the binaries a Linux tarball must carry. reasonix-guard
// is still packaged for v1.18-v1.19 updaters reading these archives; v1.20+
// extracts it and never persists it again.
var releaseUnitMembers = []string{"reasonix-desktop", "reasonix-guard", "reasonix"}

// ExtractReleaseUnit reads the release binaries out of a verified .tar.gz. It
// requires every member and rejects duplicates: a partial extraction would
// publish a version directory that is missing a binary, which the pointer swap
// would then make current.
func ExtractReleaseUnit(targz []byte) (map[string][]byte, error) {
	want := make(map[string]struct{}, len(releaseUnitMembers))
	for _, name := range releaseUnitMembers {
		want[name] = struct{}{}
	}
	found := make(map[string][]byte, len(want))
	gz, err := gzip.NewReader(bytes.NewReader(targz))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		name := path.Base(strings.TrimSpace(h.Name))
		if _, ok := want[name]; !ok {
			continue
		}
		if h.Typeflag != tar.TypeReg || h.Size < 0 {
			return nil, fmt.Errorf("update: release member %q is not a regular file", name)
		}
		if _, duplicate := found[name]; duplicate {
			return nil, fmt.Errorf("update: release member %q appears more than once", name)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		found[name] = body
	}
	for _, name := range releaseUnitMembers {
		if _, ok := found[name]; !ok {
			return nil, fmt.Errorf("update: release member %q not found in archive", name)
		}
	}
	return found, nil
}

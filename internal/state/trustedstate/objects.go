package trustedstate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Digest names an object by content: "sha256:" followed by 64 hex digits.
type Digest string

const digestPrefix = "sha256:"

// DigestOf is the name an object with these bytes has in any store.
func DigestOf(data []byte) Digest {
	sum := sha256.Sum256(data)
	return Digest(digestPrefix + hex.EncodeToString(sum[:]))
}

func (d Digest) hex() (string, bool) {
	h, ok := strings.CutPrefix(string(d), digestPrefix)
	if !ok || len(h) != sha256.Size*2 {
		return "", false
	}
	if _, err := hex.DecodeString(h); err != nil {
		return "", false
	}
	return h, true
}

func (s *Store) objectPath(d Digest) (string, error) {
	h, ok := d.hex()
	if !ok {
		return "", fmt.Errorf("%w: malformed digest %q", ErrTampered, d)
	}
	return filepath.Join(s.root, "objects", h[:2], h), nil
}

// PutObject stores data and returns its digest. Storing bytes that are already
// present is a no-op, except that an existing file whose bytes do not match its
// name is reported rather than silently trusted.
func (s *Store) PutObject(data []byte) (Digest, error) {
	d := DigestOf(data)
	path, err := s.objectPath(d)
	if err != nil {
		return "", err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if !bytes.Equal(existing, data) {
			return "", fmt.Errorf("%w: object %s does not hold the bytes it is named for", ErrTampered, d)
		}
		return d, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: %w", ErrUnwritable, err)
	}
	if err := writeFileAtomic(path, data); err != nil {
		return "", err
	}
	return d, nil
}

// Object returns the bytes named by d after checking they still hash to d.
func (s *Store) Object(d Digest) ([]byte, error) {
	path, err := s.objectPath(d)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: object %s", ErrNotFound, d)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	if DigestOf(data) != d {
		return nil, fmt.Errorf("%w: object %s does not hold the bytes it is named for", ErrTampered, d)
	}
	return data, nil
}

// writeFileAtomic writes through a temporary sibling and renames it into place,
// so a reader sees the previous file or the complete new one, never a prefix.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("%w: %w", ErrUnwritable, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnwritable, err)
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	serr := tmp.Sync()
	cerr := tmp.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("%w: %w", ErrUnwritable, err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("%w: %w", ErrUnwritable, err)
	}
	return nil
}

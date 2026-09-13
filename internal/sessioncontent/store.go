// Package sessioncontent stores immutable session payloads by content digest.
// It deliberately owns bytes only: session ordering, authorization and model
// projection remain responsibilities of their respective services.
package sessioncontent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

const (
	copyBufferBytes = 1 << 20
	// MaxReadRange bounds one allocation, not the size of an object or session.
	MaxReadRange = 8 << 20
)

// Metadata describes how callers display or interpret content. None of these
// fields participate in the storage path; identical bytes are deduplicated.
type Metadata struct {
	MediaType string
	Name      string
}

// Ref is the durable, path-independent identity of one immutable object.
type Ref struct {
	Digest    string `json:"digest"`
	Bytes     int64  `json:"bytes"`
	MediaType string `json:"mediaType,omitempty"`
	Name      string `json:"name,omitempty"`
}

// Store is a process-independent content-addressed object store. Publishing is
// no-overwrite: concurrent writers of the same digest converge on one object.
type Store struct {
	root string
}

func New(root string) *Store { return &Store{root: root} }

func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

// Put streams r into a private temporary file, fsyncs it, then atomically
// publishes the file under its SHA-256 digest. A returned Ref always names a
// complete object. Cancellation never publishes the partial temporary file.
func (s *Store) Put(ctx context.Context, r io.Reader, meta Metadata) (Ref, error) {
	if s == nil || s.root == "" {
		return Ref{}, errors.New("session content store unavailable")
	}
	if r == nil {
		return Ref{}, errors.New("session content reader is nil")
	}
	if err := ctx.Err(); err != nil {
		return Ref{}, err
	}
	tmpDir := filepath.Join(s.root, ".tmp")
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return Ref{}, fmt.Errorf("create session content temp directory: %w", err)
	}
	tmp, err := os.CreateTemp(tmpDir, "content-*.tmp")
	if err != nil {
		return Ref{}, fmt.Errorf("create session content temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
	}()

	digest := sha256.New()
	n, err := copyWithContext(ctx, io.MultiWriter(tmp, digest), r)
	if err != nil {
		return Ref{}, fmt.Errorf("stage session content: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return Ref{}, fmt.Errorf("fsync session content: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		return Ref{}, fmt.Errorf("protect session content: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return Ref{}, fmt.Errorf("close session content: %w", err)
	}
	closed = true

	ref := Ref{
		Digest:    hex.EncodeToString(digest.Sum(nil)),
		Bytes:     n,
		MediaType: meta.MediaType,
		Name:      meta.Name,
	}
	dest := s.objectPath(ref.Digest)
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return Ref{}, fmt.Errorf("create session content object directory: %w", err)
	}
	if err := os.Link(tmpPath, dest); err != nil {
		if !os.IsExist(err) {
			return Ref{}, fmt.Errorf("publish session content %s: %w", ref.Digest, err)
		}
		if verifyErr := s.verify(ctx, ref); verifyErr != nil {
			return Ref{}, fmt.Errorf("existing session content %s is invalid: %w", ref.Digest, verifyErr)
		}
		return ref, nil
	}
	_ = syncParent(filepath.Dir(dest))
	return ref, nil
}

// Open validates the complete immutable object before returning it positioned
// at byte zero. This favors integrity over trusting a mutable local filesystem.
func (s *Store) Open(ctx context.Context, ref Ref) (*os.File, error) {
	if err := validateRef(ref); err != nil {
		return nil, err
	}
	f, err := s.openRaw(ref)
	if err != nil {
		return nil, err
	}
	if err := verifyOpenFile(ctx, f, ref); err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("rewind session content %s: %w", ref.Digest, err)
	}
	return f, nil
}

// Stat validates the object and returns its caller-owned display metadata.
func (s *Store) Stat(ctx context.Context, ref Ref) (Ref, error) {
	f, err := s.Open(ctx, ref)
	if err != nil {
		return Ref{}, err
	}
	if err := f.Close(); err != nil {
		return Ref{}, fmt.Errorf("close session content %s: %w", ref.Digest, err)
	}
	return ref, nil
}

// ReadRange reads exactly length bytes starting at offset after validating the
// object. The per-call allocation is bounded independently of object size.
func (s *Store) ReadRange(ctx context.Context, ref Ref, offset, length int64) ([]byte, error) {
	if offset < 0 || length < 0 {
		return nil, errors.New("session content range must be non-negative")
	}
	if length > MaxReadRange {
		return nil, fmt.Errorf("session content range %d exceeds per-read budget %d", length, MaxReadRange)
	}
	if offset > ref.Bytes || length > ref.Bytes-offset {
		return nil, fmt.Errorf("session content range [%d,%d) exceeds object size %d", offset, offset+length, ref.Bytes)
	}
	f, err := s.Open(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek session content %s: %w", ref.Digest, err)
	}
	buf := make([]byte, int(length))
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, fmt.Errorf("read session content %s range: %w", ref.Digest, err)
	}
	return buf, nil
}

func (s *Store) openRaw(ref Ref) (*os.File, error) {
	if s == nil || s.root == "" {
		return nil, errors.New("session content store unavailable")
	}
	path := s.objectPath(ref.Digest)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("stat session content %s: %w", ref.Digest, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("session content %s is not a regular file", ref.Digest)
	}
	if info.Size() != ref.Bytes {
		return nil, fmt.Errorf("session content %s size is %d, expected %d", ref.Digest, info.Size(), ref.Bytes)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session content %s: %w", ref.Digest, err)
	}
	return f, nil
}

func (s *Store) verify(ctx context.Context, ref Ref) error {
	f, err := s.openRaw(ref)
	if err != nil {
		return err
	}
	defer f.Close()
	return verifyOpenFile(ctx, f, ref)
}

func verifyOpenFile(ctx context.Context, f *os.File, ref Ref) error {
	digest := sha256.New()
	if _, err := copyWithContext(ctx, digest, f); err != nil {
		return fmt.Errorf("verify session content %s: %w", ref.Digest, err)
	}
	got := hex.EncodeToString(digest.Sum(nil))
	if got != ref.Digest {
		return fmt.Errorf("session content %s failed SHA-256 verification: got %s", ref.Digest, got)
	}
	return nil
}

func validateRef(ref Ref) error {
	if ref.Bytes < 0 {
		return errors.New("session content byte size must be non-negative")
	}
	if len(ref.Digest) != sha256.Size*2 {
		return fmt.Errorf("invalid session content digest %q", ref.Digest)
	}
	for _, c := range ref.Digest {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return fmt.Errorf("invalid session content digest %q", ref.Digest)
		}
	}
	return nil
}

func (s *Store) objectPath(digest string) string {
	if len(digest) < 4 {
		return filepath.Join(s.root, "objects", digest)
	}
	return filepath.Join(s.root, "objects", digest[:2], digest[2:4], digest)
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, copyBufferBytes)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			wn, writeErr := dst.Write(buf[:n])
			written += int64(wn)
			if writeErr != nil {
				return written, writeErr
			}
			if wn != n {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
}

func syncParent(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Sync(); err != nil && runtime.GOOS != "windows" && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) && !errors.Is(err, syscall.ENOSYS) {
		return err
	}
	return nil
}

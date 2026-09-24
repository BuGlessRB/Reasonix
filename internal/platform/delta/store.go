package delta

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// ErrChunkMismatch is a stored chunk whose bytes are not the ones its name
// promises: corrupted in storage, altered in transit, or served for another.
var ErrChunkMismatch = errors.New("delta: chunk does not match its hash")

// HashOf is a chunk's name: the lowercase hex SHA-256 of its plain bytes.
func HashOf(plain []byte) string {
	sum := sha256.Sum256(plain)
	return hex.EncodeToString(sum[:])
}

// ObjectName is where a chunk is stored, relative to the chunk store.
func ObjectName(hash string) string { return hash + ".zst" }

var (
	encOnce sync.Once
	enc     *zstd.Encoder
	decOnce sync.Once
	dec     *zstd.Decoder
)

// Compress is a chunk as the store holds it.
func Compress(plain []byte) []byte {
	encOnce.Do(func() {
		enc, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression), zstd.WithEncoderConcurrency(1))
	})
	return enc.EncodeAll(plain, nil)
}

// Decompress reads a stored chunk back and checks it is the chunk named hash.
// Output is capped at the largest chunk the format allows.
func Decompress(stored []byte, hash string) ([]byte, error) {
	decOnce.Do(func() {
		dec, _ = zstd.NewReader(nil, zstd.WithDecoderMaxMemory(maxChunk), zstd.WithDecoderConcurrency(1))
	})
	plain, err := dec.DecodeAll(stored, make([]byte, 0, avgChunk))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrChunkMismatch, hash, err)
	}
	if HashOf(plain) != hash {
		return nil, fmt.Errorf("%w: %s", ErrChunkMismatch, hash)
	}
	return plain, nil
}

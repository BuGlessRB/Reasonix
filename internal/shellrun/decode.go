package shellrun

import (
	"unicode/utf8"

	fileenc "reasonix/internal/fileutil/encoding"
)

// decodeShellOutput reads a child's bytes as text. A Windows console tool
// answers in the machine's code page rather than UTF-8, and the bytes were kept
// as a Go string unchanged: "FIND: 参数格式不正确" reached the model as U+FFFD once
// JSON coerced them, so four failed calls never said why they failed.
func decodeShellOutput(b []byte) string {
	if len(b) == 0 || utf8.Valid(trimEdgeRunes(b)) {
		return string(b)
	}
	return string(fileenc.DecodeToUTF8(b))
}

// trimEdgeRunes drops a character the buffer cut in half at either end. The tail
// keeps the last N bytes and so starts mid-character; without this, UTF-8 output
// that was merely truncated would be re-read as the code page and mangled.
func trimEdgeRunes(b []byte) []byte {
	for len(b) > 0 && !utf8.RuneStart(b[0]) {
		b = b[1:]
	}
	return fileenc.TrimPartialRune(b)
}

package serve

import (
	"net/http"
	"strconv"

	"reasonix/internal/control"
)

// maxCompleteLine caps the line a client may ask about. The composer sends one
// prompt line per keystroke; anything past this is not a line being edited.
const maxCompleteLine = 8 << 10

// complete answers the composer's menu. The grammar lives in control.Complete,
// so this frontend's "@" and "/" mean what the terminal's mean. Offsets cross
// here in UTF-16 code units, which is how a browser indexes the string it sent:
// the kernel counts bytes, and a Chinese prompt would splice at the wrong
// place.
func (s *Server) complete(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	line := q.Get("line")
	if len(line) > maxCompleteLine {
		http.Error(w, "line too long", http.StatusRequestEntityTooLarge)
		return
	}
	cursor := len(line)
	if v := q.Get("cursor"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			http.Error(w, "cursor must be a number", http.StatusBadRequest)
			return
		}
		cursor = byteOffset(line, n)
	}
	data := s.ctl().CompletionData()
	// A client of this server completes inside the workspace and nowhere else,
	// which is the same boundary SubmitHTTP resolves its references within.
	data.Scoped = true
	out := control.Complete(line, cursor, data)
	out.From = utf16Offset(line, out.From)
	out.To = utf16Offset(line, out.To)
	writeJSON(w, out)
}

// utf16Offset converts a byte offset into s to the UTF-16 code-unit offset a
// JavaScript client indexes the same string by.
func utf16Offset(s string, b int) int {
	if b <= 0 {
		return 0
	}
	if b > len(s) {
		b = len(s)
	}
	n := 0
	for _, r := range s[:b] {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// byteOffset is utf16Offset reversed: the byte index of a client's caret.
func byteOffset(s string, u16 int) int {
	if u16 <= 0 {
		return 0
	}
	n := 0
	for i, r := range s {
		if n >= u16 {
			return i
		}
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return len(s)
}

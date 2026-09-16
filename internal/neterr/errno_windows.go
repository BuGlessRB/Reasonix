package neterr

import "syscall"

// syscall.ECONNRESET and friends exist on Windows only as APPLICATION_ERROR
// placeholders, so errors.Is against them never matches a real socket failure.
// Winsock's own numbering carries the identities the runtime returns; Go
// declares two of them and wsaeconnrefused fills in the third.
const wsaeconnrefused = syscall.Errno(10061)

var (
	resetErrors   = []error{syscall.WSAECONNRESET, syscall.WSAECONNABORTED, syscall.ERROR_BROKEN_PIPE}
	refusedErrors = []error{wsaeconnrefused}
)

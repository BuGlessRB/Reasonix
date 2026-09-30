// Package feedback sends a person's report to the maintainers and follows what
// becomes of it. One service sits behind every frontend: it validates and
// redacts locally, submits with an idempotency key so a retry never files
// twice, keeps the install identity and receipts on disk, and reads the
// caller's own reports back. Nothing here reaches a model request.
package feedback

// Package market reads the Reasonix community registry and installs what it
// lists through install_source. It is a source, never a trust root:
//
//   - The registry is read-only here, over https to one fixed host, with a
//     timeout, a body cap and no redirects. Its rows are untrusted data.
//   - Only an approved version is installable, and only when its row carries a
//     content digest a reviewer bound to it. The digest reaches install_source
//     as expectDigest, which refuses material that differs before writing.
//   - Every install is the ordinary two-phase plan and apply. Apply must echo
//     the planId of a plan it was shown, so a source that expands into several
//     skills is listed to the person before any of them lands.
//   - What was installed from here is recorded in a ledger under the Reasonix
//     home, read back only to say "installed" — and only while the recorded
//     targets still exist.
package market

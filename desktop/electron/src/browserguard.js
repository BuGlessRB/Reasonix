"use strict";

// What a page in the agent's browser may become. The agent's own requests pass
// the kernel's policy first; this is the window's answer for everything else a
// page can reach — a link, a redirect, a script, the person typing an address.
// The kernel's own origin is never one of them: its routes act on the session.
function guestNavigationAllowed(raw, kernelOrigin) {
  let url;
  try {
    url = new URL(String(raw));
  } catch {
    return false;
  }
  if (url.origin === kernelOrigin) return false;
  return url.protocol === "http:" || url.protocol === "https:" || url.href === "about:blank";
}

// What the person may type into the address bar: an address with a scheme, or
// a bare host that is read as https.
function typedAddress(raw) {
  const text = String(raw || "").trim();
  if (!text) return "";
  if (/^[a-z][a-z0-9+.-]*:/i.test(text)) return text;
  return "https://" + text;
}

module.exports = { guestNavigationAllowed, typedAddress };

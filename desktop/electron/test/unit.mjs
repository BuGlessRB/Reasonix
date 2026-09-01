import { test } from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import path from "node:path";
import os from "node:os";

const require = createRequire(import.meta.url);
const { parse, readActs } = require("../src/host.js");
const { contextTemplate } = require("../src/editmenu.js");
const { externalTarget } = require("../src/links.js");

const TOKEN = "a".repeat(64);
const line = (over) => JSON.stringify({ version: 1, origin: "http://127.0.0.1:8080", token: TOKEN, ...over });

test("the handshake is accepted only when every field accounts for itself", () => {
  assert.deepEqual(parse(line()), { origin: "http://127.0.0.1:8080", token: TOKEN });

  const refused = [
    ["not JSON at all", "hello"],
    ["a version this shell does not know", line({ version: 2 })],
    ["no version at all", JSON.stringify({ origin: "http://127.0.0.1:8080", token: TOKEN })],
    ["a scheme this shell does not load", line({ origin: "https://127.0.0.1:8080" })],
    ["an address that is not this machine", line({ origin: "http://10.0.0.5:8080" })],
    ["a name a resolver answers for", line({ origin: "http://localhost:8080" })],
    ["no port", line({ origin: "http://127.0.0.1" })],
    ["a credential too short to be one", line({ token: "short" })],
    ["no credential", JSON.stringify({ version: 1, origin: "http://127.0.0.1:8080" })],
  ];
  for (const [why, raw] of refused) {
    assert.throws(() => parse(raw), undefined, `accepted ${why}`);
  }
});

test("no refusal repeats the line it refused", () => {
  // The credential is in that line; a message carrying it would put it into
  // whatever collects this process's logs.
  for (const raw of [line({ version: 9 }), line({ origin: "https://127.0.0.1:1" }), "not json"]) {
    try {
      parse(raw);
      assert.fail("expected a refusal");
    } catch (err) {
      assert.ok(!err.message.includes(TOKEN), `the refusal carried the credential: ${err.message}`);
    }
  }
});

test("the context menu offers nothing where nothing can be edited", () => {
  assert.deepEqual(contextTemplate({ isEditable: false, selectionText: "", editFlags: {} }), []);
  assert.ok(contextTemplate({ isEditable: true, selectionText: "", editFlags: {} }).length > 0);
  assert.ok(contextTemplate({ isEditable: false, selectionText: "picked", editFlags: {} }).length > 0);
});

test("the context menu mirrors what the page says is possible", () => {
  const flags = { canUndo: true, canRedo: false, canCut: true, canCopy: true, canPaste: false, canSelectAll: true };
  const items = contextTemplate({ isEditable: true, selectionText: "x", editFlags: flags });
  const byRole = Object.fromEntries(items.filter((i) => i.role).map((i) => [i.role, i.enabled]));
  assert.deepEqual(byRole, {
    undo: true, redo: false, cut: true, copy: true, paste: false, selectAll: true,
  });
});

test("only http and https ever reach the platform opener", () => {
  assert.equal(externalTarget("https://example.test/a"), "https://example.test/a");
  assert.equal(externalTarget("http://example.test/"), "http://example.test/");
  for (const raw of [
    "file:///etc/passwd",
    "javascript:alert(1)",
    "data:text/html,<script>1</script>",
    "mailto:someone@example.test",
    "vscode://open",
    "not a url",
    "",
    null,
  ]) {
    assert.equal(externalTarget(raw), null, `let through ${String(raw)}`);
  }
});

const { StudioHost } = require("../src/hostclient.js");
const http = await import("node:http");

// The client is main's own reach into the kernel, so it has to present what the
// boundary asks for: this launch's credential on every request, and this
// listener's origin on anything that writes.
test("the host client presents the credential and names its own origin", async () => {
  const seen = [];
  const server = http.createServer((req, res) => {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      seen.push({ method: req.method, path: req.url, cookie: req.headers.cookie, origin: req.headers.origin, body });
      res.writeHead(200, { "content-type": "application/json" });
      res.end(JSON.stringify({ icon: true, live: true, closeToTray: false }));
    });
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  const origin = `http://127.0.0.1:${server.address().port}`;
  const client = new StudioHost(origin, "the-launch-credential");

  const prefs = await client.trayPrefs();
  assert.deepEqual(prefs, { icon: true, live: true, closeToTray: false });
  await client.setTrayPrefs(true, true);
  server.close();

  assert.equal(seen[0].method, "GET");
  assert.equal(seen[0].cookie, "reasonix_token=the-launch-credential");
  // A read carries no origin, which is what a top-level navigation looks like
  // and what the gate admits; only the write has to name the listener.
  assert.equal(seen[1].method, "PUT");
  assert.equal(seen[1].cookie, "reasonix_token=the-launch-credential");
  assert.equal(seen[1].origin, origin);
  assert.deepEqual(JSON.parse(seen[1].body), { icon: true, closeToTray: true });
});

// A kernel that has already gone answers null rather than throwing: every
// caller of this is a surface that must keep working while the app shuts down.
test("an unreachable kernel is an answer, not a crash", async () => {
  const dead = new StudioHost("http://127.0.0.1:1", "x");
  assert.equal(await dead.trayPrefs(), null);
  assert.equal(await dead.trayState(), null);
});

const { hostBinary, pageDir } = require("../src/layout.js");

// Packaged, both live in resources/ beside app.asar. Reading them from inside
// it is the failure this pins: a child process cannot be spawned out of an
// archive, and the kernel serves the SPA off the filesystem.
test("the kernel and the page are found in both layouts", () => {
  const dev = { packaged: false, resourcesPath: "/res", dirname: path.join("/repo", "electron", "src"), platform: "linux" };
  assert.equal(hostBinary(dev), path.join("/repo", "electron", "bin", "reasonix-studio-host"));
  assert.equal(pageDir(dev), path.join("/repo", "frontend-next", "dist"));

  const packed = { ...dev, packaged: true };
  assert.equal(hostBinary(packed), path.join("/res", "bin", "reasonix-studio-host"));
  assert.equal(pageDir(packed), path.join("/res", "frontend-next", "dist"));
  for (const p of [hostBinary(packed), pageDir(packed)]) {
    assert.doesNotMatch(p, /app\.asar/, "resolved into the archive");
  }
});

test("Windows gets the suffix spawn needs, and an override wins over both", () => {
  const win = { packaged: true, resourcesPath: "/res", dirname: "/d/src", platform: "win32" };
  assert.equal(hostBinary(win), path.join("/res", "bin", "reasonix-studio-host.exe"));
  assert.equal(hostBinary({ ...win, env: { REASONIX_STUDIO_HOST: "/custom/kernel" } }), "/custom/kernel");
  assert.equal(pageDir({ ...win, env: { REASONIX_STUDIO_PAGE: "/custom/page" } }), "/custom/page");
});

const { appIcon, iconFile } = require("../src/appicon.js");
const fsSync = require("node:fs");

// The window icon is the one the taskbar draws, and an unnamed one is Electron's
// own — which is what shipped: BrowserWindow carried no icon at all.
test("every platform that draws the window icon is given one that exists", () => {
  for (const platform of ["win32", "linux", "freebsd"]) {
    const opts = appIcon(platform);
    assert.ok(opts.icon, `${platform} was given no icon`);
    assert.ok(fsSync.existsSync(opts.icon), `${platform} names a file that is not there: ${opts.icon}`);
  }
  // macOS reads the bundle instead, so the key is absent rather than wrong.
  assert.deepEqual(appIcon("darwin"), {});
  assert.equal(iconFile("darwin"), null);
});

// Windows draws the taskbar icon from the sizes inside an .ico; a PNG there is
// scaled from one bitmap and shows it.
test("Windows is given the format it reads the small sizes from", () => {
  assert.equal(iconFile("win32"), "icon.ico");
  assert.equal(iconFile("linux"), "icon.png");
});

const { profileFor } = require("../src/instance.js");

// Two homes are two Studios, and the profile is what carries that into the
// platform's own lock. Sharing one would make the second launch look like a
// duplicate of the first.
test("each instance gets a profile of its own", () => {
  // Built rather than written out: on Windows path.join answers in backslashes,
  // and a POSIX literal made the prefix check fail there for the separator.
  const base = path.join(os.tmpdir(), "profiles");
  assert.notEqual(profileFor(base, "io.reasonix.studio.aaaa"), profileFor(base, "io.reasonix.studio.bbbb"));
  assert.ok(profileFor(base, "io.reasonix.studio.aaaa").startsWith(base));
  // An identity nobody could work out leaves the default alone rather than
  // inventing a profile that no second launch would agree on.
  assert.equal(profileFor(base, ""), base);
});

// A program that cannot be started emits "error" and never "exit". Waiting for
// the handshake instead spent the full timeout and then blamed the kernel for
// saying nothing, which is where the launch failure on Windows hid: go build
// had written a binary Node could not spawn.
test("a kernel that cannot be started says so, rather than going quiet", async () => {
  const { start } = require("../src/host.js");
  const { ready } = start(path.join(os.tmpdir(), "reasonix-no-such-kernel-b3f1"), []);
  await assert.rejects(ready, (err) => {
    assert.match(err.message, /could not be started/);
    assert.doesNotMatch(err.message, /sent no handshake/);
    return true;
  });
});

// A fake child: only the stdout half readActs touches, and a way to push bytes
// through it in whatever chunks the test wants -- which is the point, since the
// bug this guards is a line split across two of them.
function fakeChild() {
  const listeners = [];
  return {
    stdout: { setEncoding() {}, on: (_e, fn) => listeners.push(fn) },
    push: (chunk) => listeners.forEach((fn) => fn(chunk)),
  };
}

test("an act arriving in the handshake's own chunk is not lost", () => {
  const child = fakeChild();
  const seen = [];
  // What the pipe handed the handshake reader after it took its first line.
  readActs(child, '{"act":"quit"}\n', (act) => seen.push(act));
  assert.deepEqual(seen, ["quit"]);
});

test("acts are read a line at a time, however the pipe flushed them", () => {
  const child = fakeChild();
  const seen = [];
  readActs(child, "", (act) => seen.push(act));

  child.push('{"act":"relaunch"}\n{"act":');
  assert.deepEqual(seen, ["relaunch"], "a half line is not an act");
  child.push('"quit"}\n');
  assert.deepEqual(seen, ["relaunch", "quit"]);
});

test("a line this process cannot read is one it does not act on", () => {
  const child = fakeChild();
  const seen = [];
  readActs(child, "", (act) => seen.push(act));

  // Nothing here is an act, and none of it may stop the next line from being
  // one: a kernel that logged to the wrong stream must not end the handover.
  child.push("not json at all\n");
  child.push('{"version":1}\n');
  child.push('{"act":42}\n');
  child.push("\n");
  assert.deepEqual(seen, []);
  child.push('{"act":"quit"}\n');
  assert.deepEqual(seen, ["quit"]);
});

test("a child writing without newlines cannot grow this process", () => {
  const child = fakeChild();
  const seen = [];
  readActs(child, "", (act) => seen.push(act));

  child.push("x".repeat(8192));
  // Dropped rather than held, and the next real line still reads: the buffer
  // is a line assembler, not a log.
  child.push('{"act":"quit"}\n');
  assert.deepEqual(seen, ["quit"]);
});

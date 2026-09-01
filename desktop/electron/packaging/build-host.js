"use strict";
const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");

// The kernel the shell spawns, built into the directory electron-builder.yml
// lists as an extra resource.
//
// On macOS it has to be one universal binary rather than the host's own
// architecture. --universal packages an x64 app and an arm64 app and merges
// them, and @electron/universal reads each Mach-O it finds in both: already
// universal on both sides is accepted and left alone, differing slices are
// lipo'd together, and byte-identical thin slices are refused outright. A
// single-architecture kernel copied into both legs is that third case.
const OUT = path.join(__dirname, "..", "bin");
const PKG = "../../cmd/reasonix-studio-host";
const CWD = path.join(__dirname, "..");

function go(args, env) {
  const run = spawnSync("go", args, { cwd: CWD, stdio: "inherit", env: { ...process.env, ...env } });
  if (run.status !== 0) {
    console.error(`go ${args.join(" ")} failed`);
    process.exit(run.status ?? 1);
  }
}

if (process.platform !== "darwin") {
  // go build names the binary after the package and adds .exe where it belongs.
  go(["build", "-o", "bin/", PKG]);
  console.log("built reasonix-studio-host");
  process.exit(0);
}

// CGO_ENABLED=0 because one of these two is always a cross-compile, and the
// host is pure Go — studio.yml holds it to that.
const slices = ["amd64", "arm64"].map((arch) => {
  const out = path.join(OUT, `reasonix-studio-host-${arch}`);
  go(["build", "-o", out, PKG], { CGO_ENABLED: "0", GOOS: "darwin", GOARCH: arch });
  return out;
});

const universal = path.join(OUT, "reasonix-studio-host");
const merged = spawnSync("lipo", ["-create", "-output", universal, ...slices], { stdio: "inherit" });
if (merged.status !== 0) {
  console.error("lipo failed");
  process.exit(merged.status ?? 1);
}
for (const slice of slices) {
  fs.rmSync(slice, { force: true });
}
console.log("built reasonix-studio-host (universal)");

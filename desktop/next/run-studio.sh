#!/usr/bin/env bash
# Build Studio and launch it the way macOS requires: from inside an .app bundle.
#
# A bare binary run from a terminal draws its window fine, so the difference is
# invisible until you open a native panel — NSOpenPanel takes focus and closes
# again in the same beat, and "Add a folder…" looks broken. LaunchServices only
# treats a process as a real GUI app when it lives in a bundle with an
# Info.plist, so that is what this builds.
#
# The bundle is self-contained on purpose: main.go looks for the frontend next
# to the executable first (see assetsFS), so a symlink there frees the launch
# from whatever working directory `open` happens to use — it uses "/".
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
desktop=$(cd "$here/.." && pwd)
app=${STUDIO_APP:-/tmp/ReasonixStudio.app}

if [ "$(uname -s)" != "Darwin" ]; then
  echo "run-studio.sh is the macOS launch path; elsewhere just run the binary" >&2
  exit 1
fi

# emsdk's clang shadows the toolchain and breaks cgo with an -arch error.
PATH=$(echo "$PATH" | tr ':' '\n' | grep -v emsdk | paste -sd: -)
export PATH

echo "==> frontend"
(cd "$desktop/frontend-next" && pnpm build >/dev/null)

echo "==> shell"
(cd "$desktop" && go build -tags "desktop production" -o "$app.bin" ./next)

echo "==> bundle"
rm -rf "$app"
mkdir -p "$app/Contents/MacOS"
mv "$app.bin" "$app/Contents/MacOS/ReasonixStudio"
ln -s "$desktop/frontend-next" "$app/Contents/MacOS/frontend-next"
cat > "$app/Contents/Info.plist" <<'PLIST'
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>CFBundlePackageType</key><string>APPL</string>
    <key>CFBundleName</key><string>Reasonix Studio</string>
    <key>CFBundleExecutable</key><string>ReasonixStudio</string>
    <key>CFBundleIdentifier</key><string>io.reasonix.studio.dev</string>
    <key>CFBundleVersion</key><string>0.0.0</string>
    <key>CFBundleShortVersionString</key><string>0.0.0</string>
    <key>LSMinimumSystemVersion</key><string>10.13.0</string>
    <key>NSLocalNetworkUsageDescription</key>
    <string>Reasonix opens workspaces on other machines over SSH, including ones on this network.</string>
    <key>NSHighResolutionCapable</key><string>true</string>
  </dict>
</plist>
PLIST
# Signed with a real identity where one exists, ad-hoc otherwise. This is not
# about Gatekeeper — an ad-hoc signature's hash changes with every build, so
# macOS treats each one as a new app and the Local Network grant a LAN host
# needs is asked for again, or silently missing, every time. A development
# certificate keeps the identity stable across rebuilds.
identity=${STUDIO_SIGN_IDENTITY:-}
if [ -z "$identity" ]; then
  identity=$(security find-identity -v -p codesigning 2>/dev/null |
    grep -o '"Apple Development: [^"]*"' | head -1 | tr -d '"')
fi
if [ -n "$identity" ]; then
  codesign --force --sign "$identity" "$app" >/dev/null 2>&1 ||
    codesign --force --sign - "$app" >/dev/null 2>&1
else
  codesign --force --sign - "$app" >/dev/null 2>&1
fi

pkill -f "$app/Contents/MacOS/ReasonixStudio" 2>/dev/null || true
open "$app"
echo "==> $app"

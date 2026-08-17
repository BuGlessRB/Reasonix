#!/usr/bin/env bash
# Build Reasonix Studio (desktop/next + desktop/frontend-next) for one platform.
#
# Studio has no wails.json yet, so this drives `go build` directly rather than
# the Wails CLI. That costs the NSIS installer and the versioninfo stamp the
# desktop line gets for free (see docs/STUDIO_RELEASE.md), which is why the
# Windows artifact here is a portable archive and not an updater-installable
# installer. The Wails build tags are still required: without them the shell
# opens a "will not build without the correct build tags" dialog at runtime
# instead of failing to compile, so a missing tag ships silently.
#
# frontendAssets() resolves the SPA next to the executable first, so every
# archive carries frontend-next/dist as a sibling of the binary. Nothing is
# embedded; a binary shipped on its own cannot draw its UI.
#
# Usage:
#   studio-build.sh --shell-only            compile the shell for the host (CI smoke)
#   studio-build.sh <os/arch> <version>     full artifact into dist/
#
#   e.g. studio-build.sh windows/amd64 v0.1.0
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APPNAME="ReasonixStudio"
BINNAME="reasonix-studio"
# Wails v2 refuses to run without these; production also drops the devtools.
TAGS="desktop production"

# make_zip archives a staging directory's contents. Git for Windows ships no
# `zip`, and this has to work both on a windows runner and on a maintainer's
# machine, so the tool is whichever of the three is present rather than assumed.
make_zip() {
	local out="$1" src="$2"
	rm -f "$out"
	if command -v 7z >/dev/null 2>&1; then
		(cd "$src" && 7z a -tzip -mx=9 "$out" . >/dev/null)
	elif command -v zip >/dev/null 2>&1; then
		(cd "$src" && zip -qr "$out" .)
	elif command -v powershell >/dev/null 2>&1; then
		powershell -NoProfile -Command \
			"Compress-Archive -Path '$src/*' -DestinationPath '$out' -CompressionLevel Optimal"
	else
		echo "no zip tool found (need 7z, zip, or powershell)" >&2
		return 1
	fi
	[ -f "$out" ] || {
		echo "zip produced no $out" >&2
		return 1
	}
}

build_shell() {
	local goos="$1" goarch="$2" version="$3" out="$4"
	local ldflags="-s -w -X main.version=$version"
	[ "$goos" = windows ] && ldflags="$ldflags -H windowsgui"
	local tags="$TAGS"
	# WebKitGTK 4.1: 4.0 (libwebkit2gtk-4.0.so.37) is gone on Ubuntu 24.04+ and
	# Fedora 40+, while 4.1 ships from Ubuntu 22.04 onward.
	[ "$goos" = linux ] && tags="$TAGS webkit2_41"
	echo "==> go build $goos/$goarch"
	(cd "$ROOT/desktop" && GOOS="$goos" GOARCH="$goarch" go build \
		-trimpath -tags "$tags" -ldflags "$ldflags" -o "$out" ./next)
}

# CI smoke: the host's own platform, no frontend, no packaging. This is the
# check that a kernel change did not break the shell's compile.
if [ "${1:-}" = "--shell-only" ]; then
	goos=$(cd "$ROOT/desktop" && go env GOOS)
	goarch=$(cd "$ROOT/desktop" && go env GOARCH)
	out="$ROOT/dist/$BINNAME-smoke"
	[ "$goos" = windows ] && out="$out.exe"
	mkdir -p "$ROOT/dist"
	build_shell "$goos" "$goarch" dev "$out"
	echo "==> ok: $out"
	exit 0
fi

PLATFORM="${1:?usage: studio-build.sh <os/arch> <version> | --shell-only}"
VERSION="${2:?usage: studio-build.sh <os/arch> <version> | --shell-only}"
os="${PLATFORM%/*}"
arch="${PLATFORM#*/}"

echo "==> frontend"
(cd "$ROOT/desktop/frontend-next" && pnpm install --frozen-lockfile && pnpm build)
[ -f "$ROOT/desktop/frontend-next/dist/index.html" ] || {
	echo "frontend build produced no dist/index.html" >&2
	exit 1
}

staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT
mkdir -p "$ROOT/dist"

case "$os" in
windows)
	build_shell windows "$arch" "$VERSION" "$staging/$BINNAME.exe"
	mkdir -p "$staging/frontend-next"
	cp -R "$ROOT/desktop/frontend-next/dist" "$staging/frontend-next/dist"
	make_zip "$ROOT/dist/${APPNAME}-windows-${arch}.zip" "$staging"
	;;
darwin)
	# LaunchServices only treats a process as a GUI app inside a bundle with an
	# Info.plist; a bare binary opens NSOpenPanel and loses focus in the same
	# beat, which reads as a broken "Add a folder…". Same reason as run-studio.sh.
	app="$staging/${APPNAME}.app"
	mkdir -p "$app/Contents/MacOS/frontend-next" "$app/Contents/Resources"
	if [ "$arch" = universal ]; then
		build_shell darwin amd64 "$VERSION" "$staging/amd64"
		build_shell darwin arm64 "$VERSION" "$staging/arm64"
		lipo -create "$staging/amd64" "$staging/arm64" -output "$app/Contents/MacOS/$APPNAME"
		rm -f "$staging/amd64" "$staging/arm64"
	else
		build_shell darwin "$arch" "$VERSION" "$app/Contents/MacOS/$APPNAME"
	fi
	cp -R "$ROOT/desktop/frontend-next/dist" "$app/Contents/MacOS/frontend-next/dist"
	cp "$ROOT/desktop/next/appicon.icns" "$app/Contents/Resources/appicon.icns"
	# CFBundleVersion must be strictly numeric, so a -preview.N tag keeps its
	# full form in ldflags and contributes only its base here.
	numver="${VERSION#v}"
	numver="${numver%%-*}"
	sed -e "s/@VERSION@/${VERSION#v}/g" -e "s/@NUMVER@/$numver/g" \
		"$ROOT/desktop/next/Info.plist.in" >"$app/Contents/Info.plist"
	# Ad-hoc signature only. Studio releases from the studio branch, and the
	# production SignPath/Apple policies are bound to main-v2, so this is NOT
	# notarized: users clear the quarantine attribute (docs/STUDIO_RELEASE.md).
	codesign --force --deep -s - "$app"
	ditto -c -k --keepParent "$app" "$ROOT/dist/${APPNAME}-darwin-${arch}.zip"
	;;
linux)
	build_shell linux "$arch" "$VERSION" "$staging/$BINNAME"
	mkdir -p "$staging/frontend-next"
	cp -R "$ROOT/desktop/frontend-next/dist" "$staging/frontend-next/dist"
	tar -czf "$ROOT/dist/${APPNAME}-linux-${arch}.tar.gz" -C "$staging" .
	;;
*)
	echo "unsupported os: $os" >&2
	exit 1
	;;
esac

echo "==> packaged into dist/:"
ls -la "$ROOT/dist"

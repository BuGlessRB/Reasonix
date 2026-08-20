#!/usr/bin/env bash
# Build Reasonix Studio (desktop/next + desktop/frontend-next) for one platform.
#
# Studio has no wails.json, so this drives `go build` directly rather than the
# Wails CLI, and packages the NSIS installer itself (see docs/STUDIO_RELEASE.md).
# What is still missing without it is the versioninfo stamp and a windows/arm64
# .syso. The Wails build tags are required either way: without them the shell
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
# The oldest macOS each slice runs on, measured rather than assumed: 10.13 is as
# far back as this source links, and Apple Silicon did not exist before 11.0.
# Info.plist gets the lower of the two, because that is who can open the bundle.
MACOS_MIN_AMD64="10.13"
MACOS_MIN_ARM64="11.0"
MACOS_MIN="$MACOS_MIN_AMD64"
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
		# Git Bash hands out /d/… and /tmp/…, and PowerShell resolves neither —
		# it reads the first as a path relative to the current drive. cygpath is
		# what that shell ships for this, and a CI runner never reaches the
		# powershell branch, so nothing else depends on the POSIX spelling.
		local winsrc="$src" winout="$out"
		if command -v cygpath >/dev/null 2>&1; then
			winsrc=$(cygpath -w "$src")
			winout=$(cygpath -w "$out")
		fi
		powershell -NoProfile -Command \
			"Compress-Archive -Path '$winsrc\\*' -DestinationPath '$winout' -CompressionLevel Optimal"
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
	if [ "$goos" != darwin ]; then
		(cd "$ROOT/desktop" && GOOS="$goos" GOARCH="$goarch" go build \
			-trimpath -tags "$tags" -ldflags "$ldflags" -o "$out" ./next)
		return
	fi
	# Left alone, clang targets the *builder's* OS, so a release built on a
	# current machine refuses to load on every older one — and Info.plist's claim
	# is not what dyld reads. The minimum has to reach the Objective-C compiler
	# too, or Wails' own objects keep the builder's version and warn at link.
	local min="$MACOS_MIN_ARM64" archflag=""
	if [ "$goarch" = amd64 ]; then
		min="$MACOS_MIN_AMD64"
		# Cross-compiling the Intel slice on Apple Silicon: without -arch the
		# host compiler builds arm64 objects and the link fails on every symbol.
		archflag=" -arch x86_64"
	fi
	(
		cd "$ROOT/desktop"
		export GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=1
		export MACOSX_DEPLOYMENT_TARGET="$min"
		export CGO_CFLAGS="-mmacosx-version-min=$min$archflag"
		export CGO_LDFLAGS="-mmacosx-version-min=$min$archflag"
		[ -n "$archflag" ] && export CC="clang$archflag"
		go build -trimpath -tags "$tags" -ldflags "$ldflags" -o "$out" ./next
	)
}

# build_nsis turns the Windows payload into an installer. The portable zip stays:
# it is what a machine without administrator rights can still run.
#
# The WebView2 bootstrapper is fetched rather than vendored — Microsoft publishes
# one evergreen URL, and the desktop line's copy is a Wails build artifact that
# is not in the repository. A fetch that fails drops the bundled runtime instead
# of failing the release; Windows 11 already ships it.
build_nsis() {
	local arch="$1" version="$2" src="$3"
	command -v makensis >/dev/null 2>&1 || {
		echo "==> skipping installer: makensis is not installed" >&2
		return 0
	}
	# The payload sits under the repo rather than /tmp: MSYS maps /tmp through the
	# user profile and hands makensis an 8.3 short path it cannot open, while a
	# path under $ROOT converts to a plain drive path.
	local payload="$ROOT/dist/nsis-payload-$arch"
	rm -rf "$payload"
	mkdir -p "$payload"
	cp "$src/$BINNAME.exe" "$payload/$BINNAME.exe"
	cp -R "$src/frontend-next" "$payload/frontend-next"
	cp "$ROOT/desktop/next/build/windows/appicon.ico" "$payload/appicon.ico"

	local webview2="$payload/MicrosoftEdgeWebview2Setup.exe" webview2_def=()
	if curl --fail --location --silent --show-error --connect-timeout 30 --max-time 120 \
		--output "$webview2" "https://go.microsoft.com/fwlink/p/?LinkId=2124703" &&
		[ -s "$webview2" ]; then
		# A flag, not a path: File resolves it under PAYLOAD like every other
		# member, and NSIS reads a bare forward-slash path as a wildcard.
		webview2_def=("-DWEBVIEW2=1")
	else
		echo "==> WebView2 bootstrapper unavailable; installer will not bundle it" >&2
		rm -f "$webview2"
	fi

	# NSIS wants a numeric VIProductVersion, so a -preview.N tag contributes only
	# its base here while the shown VERSION keeps its full form.
	local numver="${version#v}"
	numver="${numver%%-*}"
	echo "==> makensis ${APPNAME}-windows-${arch}-installer.exe"
	# The .nsi is UTF-8 with no BOM, which NSIS 3 reads as ANSI and then rejects
	# at its first non-ASCII byte. Naming the encoding fixes that without relying
	# on a byte order mark surviving an editor.
	makensis -INPUTCHARSET UTF8 -V2 \
		"-DVERSION=$numver" \
		"-DPAYLOAD=$payload" \
		"-DOUTFILE=$ROOT/dist/${APPNAME}-windows-${arch}-installer.exe" \
		"${webview2_def[@]}" \
		"$ROOT/desktop/next/build/windows/installer/studio.nsi"
	rm -rf "$payload"
	[ -s "$ROOT/dist/${APPNAME}-windows-${arch}-installer.exe" ] || {
		echo "makensis produced no installer" >&2
		return 1
	}
}

# build_deb packages the Linux payload for dpkg. The .deb is what makes Studio
# self-updating on Linux: the versioned layout stages single files, and Studio's
# release is a binary plus the SPA tree, which dpkg installs and the shared
# apply path does not. nfpm builds it without dpkg-deb, so this runs anywhere.
build_deb() {
	local arch="$1" version="$2" src="$3"
	command -v nfpm >/dev/null 2>&1 || {
		echo "==> skipping .deb: nfpm is not installed" >&2
		return 0
	}
	local payload="$ROOT/dist/deb-payload"
	rm -rf "$payload"
	mkdir -p "$payload/frontend-next"
	cp "$src/$BINNAME" "$payload/$BINNAME"
	cp -R "$ROOT/desktop/frontend-next/dist" "$payload/frontend-next/dist"
	# Same helper source as the desktop line, built with Studio's package name so
	# it will only ever install Studio's own .deb.
	echo "==> go build $BINNAME-update-helper"
	(cd "$ROOT/desktop" && GOOS=linux GOARCH="$arch" go build -trimpath \
		-ldflags "-s -w -X main.packageName=reasonix-studio" \
		-o "$payload/$BINNAME-update-helper" ./cmd/update-helper)
	# Debian orders a prerelease below its stable: 2.0.0~preview.1 < 2.0.0.
	local body="${version#v}" deb_version
	if [ "$body" != "${body%%-*}" ]; then
		local pre="${body#*-}"
		deb_version="${body%%-*}~${pre//-/.}"
	else
		deb_version="$body"
	fi
	echo "==> nfpm package $deb_version ($arch)"
	(cd "$ROOT" && DEB_VERSION="$deb_version" DEB_ARCH="$arch" \
		nfpm package --config desktop/next/build/linux/nfpm.yaml --packager deb \
		--target "$ROOT/dist/${APPNAME}-linux-${arch}.deb")
	rm -rf "$payload"
}

# sign_app signs with a Developer ID when the keychain holds one, and ad-hoc
# otherwise so a machine with no certificate still produces a runnable bundle.
# Notarization follows only when credentials are present: a Developer ID
# signature alone still meets Gatekeeper's first refusal, but it leaves the
# user a right-click → Open, which an ad-hoc signature does not.
sign_app() {
	local app="$1" staging="$2"
	local identity
	identity="$(security find-identity -v -p codesigning 2>/dev/null |
		awk -F'"' '/Developer ID Application/{print $2; exit}')"
	if [ -z "$identity" ]; then
		echo "==> codesign (ad-hoc): no Developer ID in the keychain"
		codesign --force --deep -s - "$app"
		return
	fi
	echo "==> codesign (Developer ID): $identity"
	codesign --force --deep --timestamp --options runtime \
		--entitlements "$ROOT/desktop/next/entitlements.plist" \
		-s "$identity" "$app"
	codesign --verify --strict --deep "$app" ||
		{ echo "signed bundle does not verify" >&2; return 1; }

	# notarytool takes an archive, not a bundle, so the app is zipped only to be
	# submitted. Stapling writes the ticket into the bundle itself.
	ditto -c -k --keepParent "$app" "$staging/notarize.zip"
	notarize "$staging/notarize.zip" && xcrun stapler staple "$app"
	rm -f "$staging/notarize.zip"
}

# notarize submits one signed artifact to Apple and waits for the verdict. The
# caller staples afterwards, because what gets stapled is not always what gets
# submitted: an app is sent as a zip but the ticket belongs in the bundle.
# Without credentials this is skipped rather than fatal — a local build still
# produces a signed artifact.
notarize() {
	local path="$1" args=()
	if [ -n "${APPLE_API_KEY_PATH:-}" ] && [ -n "${APPLE_API_KEY_ID:-}" ] && [ -n "${APPLE_API_ISSUER_ID:-}" ]; then
		args=(--key "$APPLE_API_KEY_PATH" --key-id "$APPLE_API_KEY_ID" --issuer "$APPLE_API_ISSUER_ID")
	elif [ -n "${NOTARY_PROFILE:-}" ]; then
		args=(--keychain-profile "$NOTARY_PROFILE")
	else
		echo "==> not notarized: no notarytool credentials in the environment" >&2
		return 1
	fi
	echo "==> notarytool submit $(basename "$path")"
	xcrun notarytool submit "$path" "${args[@]}" --wait
}

# build_dmg wraps the bundle in the disk image macOS users expect to drag from.
# The zip stays: it is what the updater downloads, and what survives a browser
# that unpacks archives on arrival.
build_dmg() {
	local arch="$1" app="$2"
	command -v hdiutil >/dev/null 2>&1 || {
		echo "==> skipping .dmg: hdiutil is unavailable" >&2
		return 0
	}
	local out="$ROOT/dist/${APPNAME}-darwin-${arch}.dmg"
	local stage
	stage=$(mktemp -d)
	cp -R "$app" "$stage/"
	# The Applications symlink is the whole gesture: without it the window shows
	# an app with nowhere to drop it.
	ln -s /Applications "$stage/Applications"
	rm -f "$out"
	echo "==> hdiutil ${APPNAME}-darwin-${arch}.dmg"
	hdiutil create -quiet -volname "$APPNAME" -srcfolder "$stage" \
		-ov -format UDZO "$out"
	rm -rf "$stage"
	# The image is what the user downloads, so it carries quarantine and is what
	# Gatekeeper reads first. Signing the bundle inside does not sign the
	# container, and a ticket stapled to the app does not travel on the image.
	local identity
	identity="$(security find-identity -v -p codesigning 2>/dev/null |
		awk -F'"' '/Developer ID Application/{print $2; exit}')"
	if [ -n "$identity" ]; then
		codesign --force --timestamp -s "$identity" "$out"
		notarize "$out" && xcrun stapler staple "$out"
	fi
	[ -s "$out" ] || {
		echo "hdiutil produced no disk image" >&2
		return 1
	}
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
	build_nsis "$arch" "$VERSION" "$staging"
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
		-e "s/@MACOSMIN@/$MACOS_MIN/g" \
		"$ROOT/desktop/next/Info.plist.in" >"$app/Contents/Info.plist"
	sign_app "$app" "$staging"
	ditto -c -k --keepParent "$app" "$ROOT/dist/${APPNAME}-darwin-${arch}.zip"
	build_dmg "$arch" "$app"
	;;
linux)
	build_shell linux "$arch" "$VERSION" "$staging/$BINNAME"
	mkdir -p "$staging/frontend-next"
	cp -R "$ROOT/desktop/frontend-next/dist" "$staging/frontend-next/dist"
	tar -czf "$ROOT/dist/${APPNAME}-linux-${arch}.tar.gz" -C "$staging" .
	build_deb "$arch" "$VERSION" "$staging"
	;;
*)
	echo "unsupported os: $os" >&2
	exit 1
	;;
esac

echo "==> packaged into dist/:"
ls -la "$ROOT/dist"

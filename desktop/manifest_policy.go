package main

import (
	"fmt"
	"strings"

	"reasonix/desktop/internal/update"
	"reasonix/internal/installlayout"
)

// manifest_policy.go decides whether a fetched latest.json is one of our
// releases. Separate from the transport that fetched it: what counts as
// official changes with the packaging, not with the network.

type requiredDesktopAsset struct {
	group    string
	key      string
	filename string
}

var (
	requiredDesktopUpdaterAssets = []requiredDesktopAsset{
		{group: "platforms", key: "darwin-arm64", filename: "Reasonix-darwin-arm64.zip"},
		{group: "platforms", key: "darwin-amd64", filename: "Reasonix-darwin-amd64.zip"},
		{group: "platforms", key: "windows-amd64", filename: "Reasonix-windows-amd64-installer.exe"},
		{group: "platforms", key: "windows-arm64", filename: "Reasonix-windows-arm64-installer.exe"},
		{group: "platforms", key: "linux-amd64", filename: "Reasonix-linux-amd64.tar.gz"},
		{group: "native_packages", key: "linux-amd64", filename: "Reasonix-linux-amd64.deb"},
	}
	requiredDesktopDownloadAssets = []requiredDesktopAsset{
		{group: "downloads", key: "Reasonix-darwin-universal.dmg", filename: "Reasonix-darwin-universal.dmg"},
		{group: "downloads", key: "Reasonix-windows-amd64.zip", filename: "Reasonix-windows-amd64.zip"},
	}
)

// validateManifestChannel rejects every prerelease. The selected value remains
// in the signature for compatibility with existing callers.
func validateManifestChannel(selected string, m *update.Manifest) error {
	_ = selected
	if !stableDesktopVersionRE.MatchString(m.Version) {
		return fmt.Errorf("official manifest has invalid release version %q", m.Version)
	}
	return nil
}

func desktopReleaseTag(_ string, version string) string {
	return "desktop-" + version
}

func desktopAssetBases(selected, version string, allowLegacyPreview bool) []string {
	_ = selected
	_ = allowLegacyPreview
	tag := desktopReleaseTag(selected, version)
	return []string{
		fmt.Sprintf("%s/%s/", r2Base, tag),
		fmt.Sprintf("https://github.com/esengine/DeepSeek-Reasonix/releases/download/%s/", tag),
		fmt.Sprintf("https://github.com/esengine/DeepSeek-Reasonix/releases/download/%s/", version),
	}
}

func validateManifestAsset(selected, version, filename string, asset update.Asset, allowLegacyPreview bool) (string, error) {
	base := ""
	for _, candidate := range desktopAssetBases(selected, version, allowLegacyPreview) {
		if asset.URL == candidate+filename {
			base = candidate
			break
		}
	}
	if base == "" {
		return "", fmt.Errorf("asset URL %q is not the official %s path for %s", asset.URL, normalizeUpdateChannel(selected), filename)
	}
	if asset.Sig != asset.URL+".minisig" {
		return "", fmt.Errorf("asset signature URL %q does not match %q", asset.Sig, asset.URL+".minisig")
	}
	if asset.Size <= 0 || asset.Size > maxDesktopReleaseAssetSize {
		return "", fmt.Errorf("asset %s has invalid size %d", filename, asset.Size)
	}
	if !sha256RE.MatchString(asset.SHA256) {
		return "", fmt.Errorf("asset %s has invalid SHA-256 %q", filename, asset.SHA256)
	}
	if err := validateAssetInstallLayout(asset.InstallLayout); err != nil {
		return "", err
	}
	return base, nil
}

// validateAssetInstallLayout accepts the pre-v1.20 empty layout (flat install)
// and the v1.20+ versioned-v1 layout. Unknown values must fail closed so a new
// client never partially installs an unrecognized package shape.
func validateAssetInstallLayout(layout string) error {
	switch strings.TrimSpace(layout) {
	case "", installlayout.InstallLayoutVersionedV1:
		return nil
	default:
		return fmt.Errorf("unsupported install_layout %q (keeping current version)", layout)
	}
}

func validateDesktopManifest(selected string, m *update.Manifest) error {
	selected = normalizeUpdateChannel(selected)
	if err := validateManifestChannel(selected, m); err != nil {
		return err
	}
	if m.DownloadPage != manifestDownloadPageURL {
		return fmt.Errorf("%s manifest has invalid download page %q", selected, m.DownloadPage)
	}
	// Older public manifests predate the two website-only download assets. Keep
	// accepting their six signed updater artifacts so an upgrade to the first
	// single-channel release does not strand existing users. Once downloads is
	// present it is a new-format manifest and all eight assets are mandatory.
	legacyManifest := m.Downloads == nil
	requiredAssets := append([]requiredDesktopAsset(nil), requiredDesktopUpdaterAssets...)
	if !legacyManifest {
		requiredAssets = append(requiredAssets, requiredDesktopDownloadAssets...)
	}
	base := ""
	for _, required := range requiredAssets {
		var assets map[string]update.Asset
		switch required.group {
		case "platforms":
			assets = m.Platforms
		case "native_packages":
			assets = m.NativePackages
		case "downloads":
			assets = m.Downloads
		default:
			return fmt.Errorf("unsupported manifest asset group %q", required.group)
		}
		asset, ok := assets[required.key]
		if !ok {
			return fmt.Errorf("%s manifest has no %s asset for %s", selected, required.group, required.key)
		}
		assetBase, err := validateManifestAsset(selected, m.Version, required.filename, asset, legacyManifest)
		if err != nil {
			return fmt.Errorf("%s %s asset: %w", required.group, required.key, err)
		}
		if base != "" && assetBase != base {
			return fmt.Errorf("%s manifest mixes asset bases %q and %q", selected, base, assetBase)
		}
		base = assetBase
	}
	return nil
}

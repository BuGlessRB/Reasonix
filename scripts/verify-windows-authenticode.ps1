# Reads the Authenticode chain back off the Windows artifacts that will actually
# ship. A signature the release job believes it applied but that is not on the
# bytes in dist/ is the one failure the signing requests cannot catch about
# themselves, so every artifact is checked from disk rather than trusted from
# the order the steps ran in.
param(
    [Parameter(Mandatory = $true)]
    [string]$PayloadDirectory,

    [Parameter(Mandatory = $true)]
    [string]$InstallerPath,

    [Parameter(Mandatory = $true)]
    [string]$PortableArchivePath,

    [switch]$RequireTrusted
)

$ErrorActionPreference = "Stop"

# The executables Studio ships of its own: the window, and the kernel it spawns.
# Everything else the bundle carries belongs to Chromium or to electron-builder
# -- resources/elevate.exe is the one that is also a PE file -- and arrives from
# its own publisher, so this verifies what we signed, not what we shipped.
#
# Keyed by the path inside the bundle; the flat signing payload carries the leaf
# name, because SignPath receives a directory rather than a tree.
$signedExecutables = [ordered]@{
    "Reasonix Studio.exe"                    = "Reasonix Studio.exe"
    "resources/bin/reasonix-studio-host.exe" = "reasonix-studio-host.exe"
}

function Assert-AuthenticodeSignature {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "Signed Windows artifact is missing: $Path"
    }
    $signature = Get-AuthenticodeSignature -LiteralPath $Path
    if ($null -eq $signature.SignerCertificate -or $signature.SignatureType -eq "None") {
        throw "Authenticode signature is missing: $Path"
    }
    if ($RequireTrusted -and $signature.Status -ne "Valid") {
        throw "Authenticode signature is not trusted for $Path`: $($signature.Status) $($signature.StatusMessage)"
    }
    Write-Host "Authenticode $($signature.Status): $Path"
}

# The payload is what came back from stage one. Every executable Studio signs
# has to be in it: one arriving unsigned is the failure that reaches a user's
# disk after the installer around it has already been trusted.
$payloadPaths = @{}
foreach ($leaf in $signedExecutables.Values) {
    $path = Join-Path $PayloadDirectory $leaf
    Assert-AuthenticodeSignature -Path $path
    $payloadPaths[$leaf] = $path
}

Assert-AuthenticodeSignature -Path $InstallerPath

$extractRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("reasonix-authenticode-" + [guid]::NewGuid().ToString("N"))
try {
    Expand-Archive -LiteralPath $PortableArchivePath -DestinationPath $extractRoot

    # The archive is packed from the bundle after it comes back signed, so its
    # copies have to be the same bytes. Checking the signature alone would pass
    # an archive packed from an earlier build that happened to be signed too.
    foreach ($entry in $signedExecutables.GetEnumerator()) {
        $portablePath = Join-Path $extractRoot ($entry.Key -replace "/", [System.IO.Path]::DirectorySeparatorChar)
        Assert-AuthenticodeSignature -Path $portablePath
        $portableHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $portablePath).Hash
        $payloadHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $payloadPaths[$entry.Value]).Hash
        if ($portableHash -ne $payloadHash) {
            throw "Portable $($entry.Key) does not match the signed payload"
        }
    }
}
finally {
    if (Test-Path -LiteralPath $extractRoot) {
        Remove-Item -LiteralPath $extractRoot -Recurse -Force
    }
}

Write-Host "Windows Authenticode release contract verified."

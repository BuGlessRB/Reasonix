# Reads the Authenticode chain back off the Windows artifacts that will actually
# ship. A signature the release job believes it applied but that is not on the
# bytes in dist/ is the one failure the signing requests cannot catch about
# themselves, so all three artifacts are checked from disk rather than trusted
# from the order the steps ran in.
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

# Studio ships one executable of its own. Anything else a bundle carries —
# MicrosoftEdgeWebview2Setup.exe is the one — belongs to its own publisher and
# arrives already signed, so this verifies what we signed, not what we shipped.
$payloadExecutable = "reasonix-studio.exe"

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

$payloadFiles = @(Get-ChildItem -LiteralPath $PayloadDirectory -File -Filter "*.exe")
if ($payloadFiles.Count -ne 1) {
    throw "Payload must contain exactly one executable, found $($payloadFiles.Count): $($payloadFiles.Name -join ', ')"
}
$payloadPath = Join-Path $PayloadDirectory $payloadExecutable
Assert-AuthenticodeSignature -Path $payloadPath
Assert-AuthenticodeSignature -Path $InstallerPath

$extractRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("reasonix-authenticode-" + [guid]::NewGuid().ToString("N"))
try {
    Expand-Archive -LiteralPath $PortableArchivePath -DestinationPath $extractRoot

    $portableFiles = @(Get-ChildItem -LiteralPath $extractRoot -Recurse -File -Filter "*.exe")
    if ($portableFiles.Count -ne 1) {
        throw "Portable archive must contain exactly one executable, found $($portableFiles.Count): $($portableFiles.Name -join ', ')"
    }

    # The archive is packed from the payload after it comes back signed, so its
    # copy has to be the same bytes. Checking the signature alone would pass an
    # archive packed from an earlier build that happened to be signed too.
    $portablePath = Join-Path $extractRoot $payloadExecutable
    Assert-AuthenticodeSignature -Path $portablePath
    $portableHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $portablePath).Hash
    $payloadHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $payloadPath).Hash
    if ($portableHash -ne $payloadHash) {
        throw "Portable $payloadExecutable does not match the signed payload"
    }
}
finally {
    if (Test-Path -LiteralPath $extractRoot) {
        Remove-Item -LiteralPath $extractRoot -Recurse -Force
    }
}

Write-Host "Windows Authenticode release contract verified."

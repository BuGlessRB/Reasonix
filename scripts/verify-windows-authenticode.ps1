# Reads the Authenticode chain back off the Windows artifacts that will actually
# ship. A signature the release job believes it applied but that is not on the
# bytes in dist/ is the one failure the signing requests cannot catch about
# themselves, so every artifact is checked from disk rather than trusted from
# the order the steps ran in. Given only the payload, it checks the payload.
[CmdletBinding(DefaultParameterSetName = "Payload")]
param(
    [Parameter(Mandatory = $true)]
    [string]$PayloadDirectory,

    [Parameter(Mandatory = $true, ParameterSetName = "Release")]
    [string]$InstallerPath,

    [Parameter(Mandatory = $true, ParameterSetName = "Release")]
    [string]$PortableArchivePath,

    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[0-9a-fA-F]{40}$')]
    [string]$ExpectedThumbprint,

    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$ExpectedSubject
)

$ErrorActionPreference = "Stop"

# The executables Studio builds itself: the window, the kernel it spawns, and
# the computer-use helper. The release signs these and the installer, nothing
# else. Electron's DLLs, electron-builder's resources/elevate.exe and the NSIS
# uninstaller the installer writes ship with whatever signature their builder
# gave them, which for most of them is none.
#
# Keyed by the path inside the bundle; the flat signing payload carries the leaf.
$signedExecutables = [ordered]@{
    "Reasonix Studio.exe"                        = "Reasonix Studio.exe"
    "resources/bin/reasonix-studio-host.exe"     = "reasonix-studio-host.exe"
    "resources/bin/reasonix-computer-helper.exe" = "reasonix-computer-helper.exe"
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
    if ($signature.Status -ne "Valid") {
        throw "Authenticode signature is not trusted for $Path`: $($signature.Status) $($signature.StatusMessage)"
    }
    $certificate = $signature.SignerCertificate
    if ($certificate.Thumbprint -ne $ExpectedThumbprint.ToUpperInvariant()) {
        throw "Unexpected signer thumbprint $($certificate.Thumbprint): $Path"
    }
    if ($certificate.Subject -cne $ExpectedSubject) {
        throw "Unexpected signer subject '$($certificate.Subject)': $Path"
    }
    # The thumbprint pins one certificate and so its issuer; a Valid status
    # already means that issuer chains to a trusted root.
    if ([string]::IsNullOrWhiteSpace($certificate.Issuer)) {
        throw "Signer certificate has no issuer: $Path"
    }
    Write-Host "Signer: $($certificate.Subject) / issuer: $($certificate.Issuer)"
    if ($null -eq $signature.TimeStamperCertificate) {
        throw "Authenticode timestamp is missing: $Path"
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

if ($PSCmdlet.ParameterSetName -eq "Payload") {
    Write-Host "Windows Authenticode payload verified."
    exit 0
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

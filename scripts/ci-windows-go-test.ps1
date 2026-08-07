#Requires -Version 5.1
# Run `go test` under a hard wall-clock budget and kill the entire process
# tree if the budget is exceeded.
#
# Why this exists
# ---------------
# GitHub Actions step `timeout-minutes` is a soft cancel on Windows. When a
# package test leaves a child (PowerShell, helper, stuck pipe) that does not
# exit on Ctrl+C / job cancel, the step stays `in_progress` for tens of
# minutes even after the configured 10-minute budget. That pinned main-v2
# push CI during the v1.21.2 notes window and stranded release candidates.
#
# Go's `-timeout` is per package only, so it cannot bound the whole suite
# wall-clock either. This wrapper is the process-tree ceiling: after
# TimeoutSeconds it runs `taskkill /T /F` on the go.exe root, which is the
# reliable Windows way to reap the tree.
#
# Exit codes
# ----------
# - go test's exit code on normal completion
# - 124 on hard timeout (same convention as GNU coreutils `timeout`)

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateRange(1, 86400)]
    [int]$TimeoutSeconds,

    # Pass go-test flags as an explicit array so PowerShell does not treat
    # `-p` / `-timeout=...` as parameters of this wrapper script.
    # Example: -GoTestArgs @('-p','4','-timeout=3m','./...')
    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string[]]$GoTestArgs
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function ConvertTo-ProcessArgumentString {
    param(
        [Parameter(Mandatory = $true)]
        [AllowEmptyCollection()]
        [string[]]$Arguments
    )

    $parts = foreach ($arg in $Arguments) {
        if ($null -eq $arg) {
            continue
        }
        if ($arg -match '[\s"]') {
            '"' + ($arg -replace '(\\*)"', '$1$1\"') + '"'
        }
        else {
            $arg
        }
    }
    return ($parts -join ' ')
}

function Stop-ProcessTree {
    param(
        [Parameter(Mandatory = $true)]
        [int]$ProcessId
    )

    # /T = tree, /F = force. taskkill is the durable Windows process-tree API;
    # Stop-Process alone leaves grandchildren that keep the Actions step open.
    & taskkill.exe /PID $ProcessId /T /F 2>$null | Out-Null
    Start-Sleep -Milliseconds 500
}

$go = Get-Command go -ErrorAction Stop
$argList = @('test') + $GoTestArgs
$argString = ConvertTo-ProcessArgumentString -Arguments $argList

$startInfo = New-Object System.Diagnostics.ProcessStartInfo
$startInfo.FileName = $go.Source
$startInfo.Arguments = $argString
$startInfo.WorkingDirectory = (Get-Location).Path
$startInfo.UseShellExecute = $false
$startInfo.RedirectStandardOutput = $false
$startInfo.RedirectStandardError = $false
$startInfo.CreateNoWindow = $true

$proc = New-Object System.Diagnostics.Process
$proc.StartInfo = $startInfo
[void]$proc.Start()

$budgetMs = $TimeoutSeconds * 1000
if ($proc.WaitForExit($budgetMs)) {
    exit $proc.ExitCode
}

Write-Host ("::error::go test exceeded hard wall-clock budget of {0}s (pid={1}); killing process tree" -f $TimeoutSeconds, $proc.Id)
Stop-ProcessTree -ProcessId $proc.Id

# Wait briefly for the tree to disappear so the Actions step can close.
$graceMs = 15000
if (-not $proc.WaitForExit($graceMs)) {
    try {
        $proc.Kill()
    }
    catch {
        # Best-effort; taskkill already ran.
    }
    [void]$proc.WaitForExit(5000)
}

exit 124

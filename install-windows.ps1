param(
    [ValidateSet('chrome', 'chromium', 'edge', 'brave')]
    [string]$Browser = 'chrome',
    [string]$Db,
    [string]$ExtensionId,
    [switch]$DryRun,
    [switch]$Uninstall
)
$ErrorActionPreference = 'Stop'
$pagevaultArchitecture = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$pagevaultTarget = switch ($pagevaultArchitecture.ToUpperInvariant()) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "Unsupported architecture: $pagevaultArchitecture" }
}
$pagevaultExecutable = Join-Path $PSScriptRoot "bin/windows-$pagevaultTarget/pagevault-host.exe"
if (-not (Test-Path -LiteralPath $pagevaultExecutable -PathType Leaf)) {
    throw 'Release binary not found. Extract the complete release or run: go run ./cmd/release'
}
$pagevaultArguments = @('install', '--browser', $Browser, '--extension-manifest', (Join-Path $PSScriptRoot 'extension/manifest.json'))
if ($Db) { $pagevaultArguments += @('--db', $Db) }
if ($ExtensionId) { $pagevaultArguments += @('--extension-id', $ExtensionId) }
if ($DryRun) { $pagevaultArguments += '--dry-run' }
if ($Uninstall) { $pagevaultArguments += '--uninstall' }
& $pagevaultExecutable @pagevaultArguments
exit $LASTEXITCODE

param([Parameter(Mandatory = $true)][string]$Version)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root "dist"
$expected = @(
    "project-brain-$Version-windows-amd64.exe",
    "project-brain-$Version-linux-amd64",
    "project-brain-$Version-darwin-amd64",
    "project-brain-$Version-darwin-arm64"
)

foreach ($name in $expected) {
    $binary = Join-Path $dist $name
    if (-not (Test-Path -LiteralPath $binary)) { throw "Missing release binary: $name" }
    if (-not (Test-Path -LiteralPath "$binary.sha256")) { throw "Missing checksum: $name.sha256" }
}

Write-Output "Release layout verified for $Version"

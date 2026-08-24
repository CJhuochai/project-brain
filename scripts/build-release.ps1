param([string]$Version = "1.0.0")

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root "dist"
New-Item -ItemType Directory -Force -Path $dist | Out-Null
$binary = Join-Path $dist "project-brain-$Version-windows-amd64.exe"
Push-Location $root
try {
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $binary .\cmd\project-brain
    Get-FileHash -Algorithm SHA256 $binary | ForEach-Object { "$($_.Hash.ToLowerInvariant())  $([IO.Path]::GetFileName($binary))" } | Set-Content -Encoding utf8 ("$binary.sha256")
} finally {
    Pop-Location
}

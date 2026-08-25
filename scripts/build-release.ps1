param([Parameter(Mandatory = $true)][string]$Version)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root "dist"
New-Item -ItemType Directory -Force -Path $dist | Out-Null
$hadGOOS = Test-Path Env:GOOS
$hadGOARCH = Test-Path Env:GOARCH
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
Push-Location $root
try {
    foreach ($target in @(
        @{ GOOS = "windows"; GOARCH = "amd64"; Suffix = ".exe" },
        @{ GOOS = "linux"; GOARCH = "amd64"; Suffix = "" },
        @{ GOOS = "darwin"; GOARCH = "amd64"; Suffix = "" },
        @{ GOOS = "darwin"; GOARCH = "arm64"; Suffix = "" }
    )) {
        $binary = Join-Path $dist "project-brain-$Version-$($target.GOOS)-$($target.GOARCH)$($target.Suffix)"
        $env:GOOS = $target.GOOS
        $env:GOARCH = $target.GOARCH
        go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $binary .\cmd\project-brain
        Get-FileHash -Algorithm SHA256 $binary | ForEach-Object { "$($_.Hash.ToLowerInvariant())  $([IO.Path]::GetFileName($binary))" } | Set-Content -Encoding utf8 ("$binary.sha256")
    }
} finally {
    if ($hadGOOS) { $env:GOOS = $previousGOOS } else { Remove-Item Env:GOOS -ErrorAction SilentlyContinue }
    if ($hadGOARCH) { $env:GOARCH = $previousGOARCH } else { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue }
    Pop-Location
}

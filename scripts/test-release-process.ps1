param([string]$Version = "1.2.0")

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$workflow = Join-Path $root ".github\workflows\release.yml"
$changelog = Join-Path $root "CHANGELOG.md"

if (-not (Test-Path -LiteralPath $workflow)) { throw "Missing tag-triggered release workflow" }
if (-not (Test-Path -LiteralPath $changelog)) { throw "Missing changelog" }

$workflowText = Get-Content -Raw -LiteralPath $workflow
foreach ($required in @('v*', "go test ./... -count=1", "go vet ./...", "build-release.ps1", "test-release-layout.ps1", "gh release create")) {
    if (-not $workflowText.Contains($required)) { throw "Release workflow missing: $required" }
}

$changelogText = Get-Content -Raw -LiteralPath $changelog
if (-not $changelogText.Contains("[$Version]")) { throw "Changelog missing version $Version" }

foreach ($readme in @("README.md", "README.en.md")) {
    $text = Get-Content -Raw -LiteralPath (Join-Path $root $readme)
    if (-not $text.Contains("releases/latest")) { throw "$readme must link to the latest release" }
    if ($text.Contains("project-brain-1.0.0-")) { throw "$readme still hard-codes 1.0.0 binary names" }
}

Write-Output "Release process verified for $Version"

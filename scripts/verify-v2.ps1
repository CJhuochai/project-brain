param(
    [Parameter(Mandatory = $true)][string]$Executable,
    [Parameter(Mandatory = $true)][string]$OutputDirectory,
    [string]$Workspace = "E:\BasisProject"
)

$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
$repositories = @((& $Executable discover $Workspace) | ConvertFrom-Json)
if ($LASTEXITCODE -ne 0) { throw "discover failed" }
function RepositoryState {
    @($repositories | ForEach-Object {
        [pscustomobject]@{ path = $_.path; status = @(& git -C $_.path status --porcelain); refs = @(& git -C $_.path for-each-ref --format='%(refname):%(objectname)') }
    }) | ConvertTo-Json -Depth 5 -Compress
}
$before = RepositoryState
$timer = [Diagnostics.Stopwatch]::StartNew()
$status = $null
for ($attempt = 0; $attempt -lt 60; $attempt++) {
    $status = (& $Executable status $Workspace) | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0) { throw "status failed" }
    if ($status.freshness -eq "up_to_date") { break }
}
if ($status.freshness -ne "up_to_date") { throw "Initial refresh did not complete within 30 minutes" }
$timer.Stop()
$status | ConvertTo-Json -Depth 10 | Set-Content -Encoding utf8 (Join-Path $OutputDirectory "status.json")
$measurements = @()
foreach ($query in @("VisitActivityController", "ApprovalApplication", "访校活动", "groupSubmit")) {
    $watch = [Diagnostics.Stopwatch]::StartNew()
    $search = (& $Executable search $Workspace $query --limit 10) | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0) { throw "search failed: $query" }
    $watch.Stop()
    $search | ConvertTo-Json -Depth 12 | Set-Content -Encoding utf8 (Join-Path $OutputDirectory "$query.search.json")
    $measurements += [pscustomobject]@{ query = $query; elapsed_ms = $watch.ElapsedMilliseconds; evidence_count = @($search.evidence).Count; first = @($search.evidence)[0]; coverage = $search.coverage }
}
$symbol = @($measurements[0].first)[0]
if (!$symbol.id) { throw "No exact controller identity found" }
foreach ($operation in @("context", "flow", "impact")) {
    $response = (& $Executable $operation $Workspace $symbol.id --repository $symbol.repository --limit 10) | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0) { throw "$operation failed" }
    $response | ConvertTo-Json -Depth 25 | Set-Content -Encoding utf8 (Join-Path $OutputDirectory "$operation.json")
}
$contracts = (& $Executable contracts $Workspace "" --limit 100) | ConvertFrom-Json
if ($LASTEXITCODE -ne 0) { throw "contracts failed" }
$contracts | ConvertTo-Json -Depth 20 | Set-Content -Encoding utf8 (Join-Path $OutputDirectory "contracts.json")
$mcpRequests = @(
    @{ jsonrpc = "2.0"; id = 1; method = "initialize" },
    @{ jsonrpc = "2.0"; method = "notifications/initialized" },
    @{ jsonrpc = "2.0"; id = 2; method = "tools/list" },
    @{ jsonrpc = "2.0"; id = 3; method = "tools/call"; params = @{ name = "analyze_requirement"; arguments = @{ text = "调整 VisitActivityController 的访校活动报名截止规则"; snapshot_id = $status.snapshot_id } } }
) | ForEach-Object { $_ | ConvertTo-Json -Depth 8 -Compress }
$mcp = @($mcpRequests | & $Executable mcp $Workspace)
if ($LASTEXITCODE -ne 0 -or $mcp.Count -ne 3) { throw "MCP exchange failed" }
$mcp | Set-Content -Encoding utf8 (Join-Path $OutputDirectory "mcp.jsonl")
$requirement = ($mcp[-1] | ConvertFrom-Json).result.content[0].text | ConvertFrom-Json
if (!$requirement.evidence) { throw "requirement returned no evidence" }
$after = RepositoryState
if ($before -ne $after) { throw "Business repository status or refs changed" }
[pscustomobject]@{
    repositories = $repositories.Count
    index_elapsed_ms = $timer.ElapsedMilliseconds
    snapshot_id = $status.snapshot_id
    files = ($status.repositories | Measure-Object file_count -Sum).Sum
    diagnostics = ($status.repositories | Measure-Object diagnostic_count -Sum).Sum
    queries = $measurements
    contracts_returned = @($contracts.contracts).Count
    contracts_coverage = $contracts.coverage
    requirement_first_repository = @($requirement.repositories)[0]
    requirement_coverage = $requirement.coverage
    business_repository_state_unchanged = $true
} | ConvertTo-Json -Depth 12 | Tee-Object -FilePath (Join-Path $OutputDirectory "summary.json")

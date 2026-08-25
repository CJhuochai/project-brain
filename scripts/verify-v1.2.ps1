param(
    [Parameter(Mandatory = $true)][string]$Executable,
    [string]$Workspace = "E:\BasisProject"
)

function Get-RepositoryState {
    Get-ChildItem -LiteralPath $Workspace -Directory -Force -Recurse -Filter .git |
        ForEach-Object {
            $repository = $_.Parent.FullName
            [pscustomobject]@{ Path = $repository; State = (& git -C $repository status --porcelain) -join "`n" }
        } | Sort-Object Path | ConvertTo-Json -Compress
}

$before = Get-RepositoryState
$measurement = Measure-Command { $statusText = & $Executable status $Workspace }
$status = $statusText | ConvertFrom-Json
$jobs = 1..3 | ForEach-Object {
    Start-Job -ScriptBlock { param($path, $root) & $path search $root AdmitService } -ArgumentList $Executable, $Workspace
}
$responses = $jobs | ForEach-Object { Receive-Job -Job $_ -Wait -AutoRemoveJob | ConvertFrom-Json }
$after = Get-RepositoryState
if ($before -ne $after) { throw "检测到业务仓库状态发生变化" }

$snapshots = @($responses | ForEach-Object { $_.snapshot_id })
$workspaceDir = Join-Path $env:LOCALAPPDATA "ProjectBrain\workspaces"
$sizes = Get-ChildItem -LiteralPath $workspaceDir -Recurse -Filter "snapshot-*.sqlite" -ErrorAction SilentlyContinue |
    Measure-Object -Property Length -Sum
[pscustomobject]@{
    status_elapsed_ms = [math]::Round($measurement.TotalMilliseconds, 0)
    snapshot_ids = $snapshots
    unique_snapshot_ids = @($snapshots | Select-Object -Unique)
    concurrent_consistent = (@($snapshots | Select-Object -Unique).Count -eq 1)
    active_snapshot = $status.snapshot_id
    indexed_files = $status.indexed_files
    changed_files = $status.changed_files
    snapshot_bytes = $sizes.Sum
    business_repository_write = $false
} | ConvertTo-Json -Depth 4

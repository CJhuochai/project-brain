param(
    [Parameter(Mandatory = $true)][string]$Workspace,
    [Parameter(Mandatory = $true)][scriptblock]$Action
)

function Get-RepositoryState {
    Get-ChildItem -LiteralPath $Workspace -Directory -Force -Recurse -Filter .git |
        ForEach-Object {
            $repository = $_.Parent.FullName
            [pscustomobject]@{ Path = $repository; State = (& git -C $repository status --porcelain) -join "`n" }
        } | Sort-Object Path | ConvertTo-Json -Compress
}

$before = Get-RepositoryState
& $Action
$after = Get-RepositoryState
if ($before -ne $after) { throw "检测到业务仓库状态发生变化" }

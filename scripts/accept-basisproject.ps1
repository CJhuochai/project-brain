param(
    [Parameter(Mandatory = $true)][string]$Executable,
    [string]$Workspace = "E:\BasisProject"
)

$outputs = @(& "$PSScriptRoot\verify-readonly.ps1" -Workspace $Workspace -Action {
    & $Executable discover $Workspace
    & $Executable index $Workspace
    & $Executable status $Workspace
})
if ($outputs.Count -ne 3) { throw "验收命令输出数量异常：$($outputs.Count)" }
$discover = $outputs[0] | ConvertFrom-Json
$index = $outputs[1] | ConvertFrom-Json
$status = $outputs[2] | ConvertFrom-Json

[pscustomobject]@{
    repositories = @($discover).Count
    indexed_repositories = $index.repositories
    indexed_files = $index.indexed_files
    changed_files = $index.changed_files
    status_repositories = @($status).Count
} | ConvertTo-Json

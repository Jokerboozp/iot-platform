[CmdletBinding()]
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [object[]]$Arguments
)

if ($env:OS -ne "Windows_NT") {
    throw "此脚本只能在 Windows PowerShell 上运行。"
}

$target = Join-Path $PSScriptRoot "package-offline.ps1"
& $target @Arguments
exit $LASTEXITCODE

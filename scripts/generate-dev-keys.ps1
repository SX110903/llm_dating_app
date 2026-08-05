[CmdletBinding()]
param(
    [string]$OutputDirectory = (Join-Path $PSScriptRoot "..\secrets\dev")
)

$ErrorActionPreference = "Stop"
$outputPath = [System.IO.Path]::GetFullPath($OutputDirectory)
$generator = Join-Path $PSScriptRoot "generate_dev_keys.go"
& go run $generator -output $outputPath
if ($LASTEXITCODE -ne 0) {
    throw "No se pudieron generar las claves RSA de desarrollo."
}

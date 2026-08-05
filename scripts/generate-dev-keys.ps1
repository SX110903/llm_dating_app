[CmdletBinding()]
param(
    [string]$OutputDirectory = (Join-Path $PSScriptRoot "..\secrets\dev")
)

$ErrorActionPreference = "Stop"
$outputPath = [System.IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Force -Path $outputPath | Out-Null

function ConvertTo-Pem {
    param(
        [Parameter(Mandatory)] [string]$Label,
        [Parameter(Mandatory)] [byte[]]$Bytes
    )

    $base64 = [Convert]::ToBase64String($Bytes, [Base64FormattingOptions]::InsertLineBreaks)
    return "-----BEGIN $Label-----`n$base64`n-----END $Label-----`n"
}

$rsa = [System.Security.Cryptography.RSA]::Create(3072)
try {
    $utf8 = [System.Text.UTF8Encoding]::new($false)
    $privatePem = ConvertTo-Pem -Label "PRIVATE KEY" -Bytes $rsa.ExportPkcs8PrivateKey()
    $publicPem = ConvertTo-Pem -Label "PUBLIC KEY" -Bytes $rsa.ExportSubjectPublicKeyInfo()
    [System.IO.File]::WriteAllText((Join-Path $outputPath "jwt_private.pem"), $privatePem, $utf8)
    [System.IO.File]::WriteAllText((Join-Path $outputPath "jwt_public.pem"), $publicPem, $utf8)
}
finally {
    $rsa.Dispose()
}

Write-Host "Claves RSA de desarrollo creadas en $outputPath"


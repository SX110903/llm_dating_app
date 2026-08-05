[CmdletBinding()]
param([switch]$Force)

$ErrorActionPreference = "Stop"
$root = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$envPath = Join-Path $root ".env"

if ((Test-Path -LiteralPath $envPath) -and -not $Force) {
    throw ".env ya existe. Usa -Force solo si quieres reemplazarlo."
}

& (Join-Path $PSScriptRoot "generate-dev-keys.ps1")

function New-Secret {
    $bytes = New-Object byte[] 32
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    return [Convert]::ToBase64String($bytes).TrimEnd("=").Replace("+", "-").Replace("/", "_")
}

$contents = @"
APP_ENV=development
HTTP_ADDR=:8080
LOG_LEVEL=info
CORS_ALLOWED_ORIGINS=http://localhost:8080
POSTGRES_DB=llmatch
POSTGRES_USER=llmatch_admin
POSTGRES_ADMIN_PASSWORD=$(New-Secret)
APP_DB_PASSWORD=$(New-Secret)
MIGRATOR_DB_PASSWORD=$(New-Secret)
REDIS_PASSWORD=$(New-Secret)
JWT_PRIVATE_KEY_PATH=./secrets/dev/jwt_private.pem
JWT_PUBLIC_KEY_PATH=./secrets/dev/jwt_public.pem
"@

[System.IO.File]::WriteAllText($envPath, $contents, [System.Text.UTF8Encoding]::new($false))
Write-Host "Entorno de desarrollo creado en $envPath (ignorado por Git)."


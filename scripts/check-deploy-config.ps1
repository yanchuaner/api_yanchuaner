param(
  [string]$EnvFile = (Join-Path $PSScriptRoot "..\deploy\.env")
)

$ErrorActionPreference = "Stop"
$composeFile = Join-Path $PSScriptRoot "..\deploy\compose.yaml"

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
  throw "Docker CLI is not installed or not available on PATH."
}
if (-not (Test-Path -LiteralPath $EnvFile)) {
  throw "Missing $EnvFile. Run scripts/generate-deploy-env.ps1 first."
}

$forbidden = Select-String -LiteralPath $EnvFile -Pattern 'replace-with|CHANGE_ME|=123456$'
if ($forbidden) {
  throw "The environment file still contains placeholder or unsafe values."
}

docker compose --env-file $EnvFile -f $composeFile config --quiet
if ($LASTEXITCODE -ne 0) {
  throw "Docker Compose configuration validation failed."
}

Write-Host "Deployment configuration is valid. Container health checks require a running Docker engine."

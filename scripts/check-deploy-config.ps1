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

$values = @{}
Get-Content -LiteralPath $EnvFile | ForEach-Object {
  if ($_ -match '^([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
    $values[$matches[1]] = $matches[2].Trim().Trim('"').Trim("'")
  }
}
$oauthClientIds = @(
  $values["YANCHUANER_OAUTH_CLIENT_ID"],
  $values["YANCHUANER_AI_OAUTH_CLIENT_ID"],
  $values["YANCHUANER_AI_WEB_OAUTH_CLIENT_ID"]
)
if ($oauthClientIds.Where({ [string]::IsNullOrWhiteSpace($_) }).Count -gt 0 -or
    ($oauthClientIds | Select-Object -Unique).Count -ne $oauthClientIds.Count) {
  throw "API, Open WebUI, and autonomous AI Web must use three distinct OAuth client IDs."
}
$isolatedSecrets = @(
  $values["YANCHUANER_OAUTH_CLIENT_SECRET"],
  $values["YANCHUANER_AI_OAUTH_CLIENT_SECRET"],
  $values["YANCHUANER_AI_WEB_OAUTH_CLIENT_SECRET"],
  $values["YANCHUANER_SUBJECT_EXCHANGE_CLIENT_SECRET"]
)
if ($isolatedSecrets.Where({ [string]::IsNullOrWhiteSpace($_) -or $_.Length -lt 32 }).Count -gt 0 -or
    ($isolatedSecrets | Select-Object -Unique).Count -ne $isolatedSecrets.Count) {
  throw "OAuth clients and YanCore subject exchange must use distinct secrets of at least 32 characters."
}

docker compose --env-file $EnvFile -f $composeFile config --quiet
if ($LASTEXITCODE -ne 0) {
  throw "Docker Compose configuration validation failed."
}

Write-Host "Deployment configuration is valid. Container health checks require a running Docker engine."

param(
  [string]$WebRepo = (Join-Path $PSScriptRoot "..\..\web_yanchuaner"),
  [string]$AiRepo = (Join-Path $PSScriptRoot "..\..\ai_yanchuaner")
)

$ErrorActionPreference = "Stop"
$apiRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$apiEnv = Join-Path $apiRoot "deploy\.env"
$webEnv = Join-Path $WebRepo ".env"
$aiEnv = Join-Path $AiRepo ".env"

function New-UrlSafeSecret([int]$ByteCount = 32) {
  $bytes = New-Object byte[] $ByteCount
  [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
  return [Convert]::ToBase64String($bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_')
}

function New-RsaPrivateKeyBase64() {
  $rsa = [System.Security.Cryptography.RSA]::Create(2048)
  try {
    return [Convert]::ToBase64String($rsa.ExportPkcs8PrivateKey())
  } finally {
    $rsa.Dispose()
  }
}

function Read-DotEnv([string]$Path) {
  $values = @{}
  if (-not (Test-Path -LiteralPath $Path)) { return $values }
  Get-Content -LiteralPath $Path | ForEach-Object {
    if ($_ -match '^([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
      $values[$matches[1]] = $matches[2].Trim().Trim('"').Trim("'")
    }
  }
  return $values
}

function Set-DotEnvValues([string]$Path, [hashtable]$Values) {
  if (-not (Test-Path -LiteralPath $Path)) {
    [System.IO.File]::WriteAllText($Path, "", [System.Text.UTF8Encoding]::new($false))
  }
  $lines = [System.Collections.Generic.List[string]]::new()
  Get-Content -LiteralPath $Path | ForEach-Object { $lines.Add($_) }
  foreach ($name in $Values.Keys) {
    $replacement = "$name=$($Values[$name])"
    $index = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
      if ($lines[$i] -match "^$([regex]::Escape($name))=") {
        $index = $i
        break
      }
    }
    if ($index -ge 0) {
      $lines[$index] = $replacement
    } else {
      $lines.Add($replacement)
    }
  }
  $content = ($lines -join [Environment]::NewLine).TrimEnd() + [Environment]::NewLine
  [System.IO.File]::WriteAllText($Path, $content, [System.Text.UTF8Encoding]::new($false))
}

if (-not (Test-Path -LiteralPath $apiEnv)) {
  & (Join-Path $PSScriptRoot "generate-deploy-env.ps1")
}
if (-not (Test-Path -LiteralPath $webEnv)) {
  throw "Missing main-site environment file: $webEnv"
}
if (-not (Test-Path -LiteralPath $aiEnv)) {
  throw "Missing AI environment file: $aiEnv"
}

$apiValues = Read-DotEnv $apiEnv
$webValues = Read-DotEnv $webEnv
$oauthSecret = $apiValues["YANCHUANER_OAUTH_CLIENT_SECRET"]
if ([string]::IsNullOrWhiteSpace($oauthSecret)) { $oauthSecret = New-UrlSafeSecret 48 }
$aiOAuthSecret = $apiValues["YANCHUANER_AI_OAUTH_CLIENT_SECRET"]
if ([string]::IsNullOrWhiteSpace($aiOAuthSecret)) { $aiOAuthSecret = New-UrlSafeSecret 48 }
$subjectExchangeSecret = $apiValues["YANCHUANER_SUBJECT_EXCHANGE_CLIENT_SECRET"]
if ([string]::IsNullOrWhiteSpace($subjectExchangeSecret)) { $subjectExchangeSecret = New-UrlSafeSecret 48 }
$oauthSigningKey = $webValues["YANCHUANER_OAUTH_SIGNING_KEY"]
if ([string]::IsNullOrWhiteSpace($oauthSigningKey)) { $oauthSigningKey = New-RsaPrivateKeyBase64 }
$rootPassword = $apiValues["NEW_API_ROOT_PASSWORD"]
if ([string]::IsNullOrWhiteSpace($rootPassword)) { $rootPassword = New-UrlSafeSecret 36 }
$newUserInitialQuota = $apiValues["NEW_USER_INITIAL_QUOTA"]
if ([string]::IsNullOrWhiteSpace($newUserInitialQuota)) { $newUserInitialQuota = "500000" }
$openWebUiServiceQuota = $apiValues["OPENWEBUI_SERVICE_QUOTA"]
if ([string]::IsNullOrWhiteSpace($openWebUiServiceQuota)) { $openWebUiServiceQuota = "10000000" }

Set-DotEnvValues $apiEnv @{
  NEW_API_PUBLIC_URL = "http://localhost:3101"
  NEW_API_SESSION_COOKIE_SECURE = "false"
  NEW_API_SESSION_COOKIE_TRUSTED_URL = ""
  NEW_API_ROOT_USERNAME = "yanchuaner"
  NEW_API_ROOT_PASSWORD = $rootPassword
  NEW_USER_INITIAL_QUOTA = $newUserInitialQuota
  OPENWEBUI_SERVICE_QUOTA = $openWebUiServiceQuota
  YANCHUANER_OAUTH_CLIENT_ID = "api-yanchuaner"
  YANCHUANER_OAUTH_CLIENT_SECRET = $oauthSecret
  YANCHUANER_AI_OAUTH_CLIENT_ID = "ai-yanchuaner"
  YANCHUANER_AI_OAUTH_CLIENT_SECRET = $aiOAuthSecret
  YANCHUANER_SUBJECT_EXCHANGE_ENABLED = "false"
  YANCHUANER_SUBJECT_EXCHANGE_CLIENT_ID = "ai-yancore-bff"
  YANCHUANER_SUBJECT_EXCHANGE_CLIENT_SECRET = $subjectExchangeSecret
  YANCHUANER_SUBJECT_EXCHANGE_USERINFO_URL = "http://host.docker.internal:3000/api/oauth/userinfo"
  YANCHUANER_SUBJECT_EXCHANGE_ALLOW_INSECURE_HTTP = "true"
  WEB_MAIN_PUBLIC_URL = "http://localhost:3000"
  WEB_MAIN_INTERNAL_URL = "http://host.docker.internal:3000"
  LITELLM_PUBLIC_URL = "http://127.0.0.1:4000"
  LITELLM_INTERNAL_URL = "http://litellm-gateway:4000"
  AI_CORE_NETWORK = "yanchuaner-ai-core"
  REDIS_HOST_PORT = "6380"
}

$apiValues = Read-DotEnv $apiEnv
$redisPassword = $apiValues["REDIS_PASSWORD"]
if ([string]::IsNullOrWhiteSpace($redisPassword)) {
  throw "REDIS_PASSWORD is missing from $apiEnv"
}

Set-DotEnvValues $webEnv @{
  REDIS_URL = "redis://:$redisPassword@127.0.0.1:6380/1"
  YANCHUANER_OAUTH_CLIENT_ID = "api-yanchuaner"
  YANCHUANER_OAUTH_CLIENT_SECRET = $oauthSecret
  YANCHUANER_OAUTH_REDIRECT_URI = "http://localhost:3101/oauth/yanchuaner"
  YANCHUANER_OAUTH_ISSUER = "http://localhost:3000"
  YANCHUANER_OAUTH_INTERNAL_URL = "http://host.docker.internal:3000"
  YANCHUANER_OAUTH_SIGNING_KEY = $oauthSigningKey
  YANCHUANER_AI_OAUTH_CLIENT_ID = "ai-yanchuaner"
  YANCHUANER_AI_OAUTH_CLIENT_SECRET = $aiOAuthSecret
  YANCHUANER_AI_OAUTH_REDIRECT_URI = "http://localhost:3001/oauth/oidc/callback"
  AI_WORKSPACE_URL = "http://localhost:3001"
  API_PLATFORM_URL = "http://localhost:3101"
  NEXT_PUBLIC_AI_WORKSPACE_URL = "http://localhost:3001"
  NEXT_PUBLIC_API_PLATFORM_URL = "http://localhost:3101"
  NEXT_PUBLIC_LAB_URL = "http://localhost:3100"
}

$aiValues = Read-DotEnv $aiEnv
$aiWebSessionSecret = $aiValues["AI_WEB_SESSION_SECRET"]
if ([string]::IsNullOrWhiteSpace($aiWebSessionSecret)) { $aiWebSessionSecret = New-UrlSafeSecret 48 }
$openWebUiApiKey = $aiValues["OPENWEBUI_API_KEY"]
if ([string]::IsNullOrWhiteSpace($openWebUiApiKey)) {
  $openWebUiApiKey = $aiValues["OPENWEBUI_LITELLM_KEY"]
}
if ([string]::IsNullOrWhiteSpace($openWebUiApiKey)) {
  throw "OPENWEBUI_LITELLM_KEY is required as the one-time migration key"
}

Set-DotEnvValues $aiEnv @{
  AI_CORE_NETWORK = "yanchuaner-ai-core"
  AI_WEB_HOST_PORT = "3002"
  AI_WEB_PUBLIC_URL = "http://localhost:3002"
  AI_WEB_SESSION_SECRET = $aiWebSessionSecret
  AI_WEB_ALLOW_INSECURE_INTERNAL_HTTP = "true"
  API_GATEWAY_BASE_URL = "http://api-gateway:3000/v1"
  YANCORE_API_BASE_URL = "http://api-gateway:3000"
  YANCORE_OIDC_ISSUER = "http://localhost:3000"
  YANCORE_OIDC_DISCOVERY_URL = "http://host.docker.internal:3000/api/oauth/.well-known/openid-configuration"
  YANCORE_OIDC_CLIENT_ID = "ai-yanchuaner"
  YANCORE_OIDC_CLIENT_SECRET = $aiOAuthSecret
  YANCORE_SUBJECT_EXCHANGE_CLIENT_ID = "ai-yancore-bff"
  YANCORE_SUBJECT_EXCHANGE_CLIENT_SECRET = $subjectExchangeSecret
  OPENWEBUI_API_KEY = $openWebUiApiKey
  OPENWEBUI_IMAGE_API_KEY = $openWebUiApiKey
  OPENWEBUI_HOST_PORT = "3001"
  OPENWEBUI_URL = "http://localhost:3001"
  OPENWEBUI_CORS_ALLOW_ORIGIN = "http://localhost:3001"
  YANCHUANER_AI_OAUTH_CLIENT_ID = "ai-yanchuaner"
  YANCHUANER_AI_OAUTH_CLIENT_SECRET = $aiOAuthSecret
  YANCHUANER_AI_OAUTH_REDIRECT_URI = "http://localhost:3001/oauth/oidc/callback"
  YANCHUANER_OIDC_DISCOVERY_URL = "http://host.docker.internal:3000/api/oauth/.well-known/openid-configuration"
  OPENWEBUI_OAUTH_AUTO_REDIRECT = "True"
}

docker network inspect yanchuaner-ai-core *> $null
if ($LASTEXITCODE -ne 0) {
  docker network create yanchuaner-ai-core | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "Unable to create yanchuaner-ai-core network" }
}

Write-Output "Integrated environment prepared for web_yanchuaner, ai_yanchuaner, and api_yanchuaner."
Write-Output "Secrets were written only to ignored local .env files."

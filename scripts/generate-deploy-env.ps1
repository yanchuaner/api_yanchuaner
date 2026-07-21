param(
  [string]$OutputPath = (Join-Path $PSScriptRoot "..\deploy\.env")
)

$ErrorActionPreference = "Stop"

if (Test-Path -LiteralPath $OutputPath) {
  throw "Refusing to overwrite existing file: $OutputPath"
}

function New-UrlSafeSecret([int]$ByteCount = 32) {
  $bytes = New-Object byte[] $ByteCount
  [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
  return [Convert]::ToBase64String($bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_')
}

$content = @"
NEW_API_IMAGE_TAG=v1.0.0-rc.21-yc.1
NEW_API_HOST_PORT=3101
NEW_API_PUBLIC_URL=https://api.yanchuaner.cn
NEW_API_SESSION_COOKIE_SECURE=true
NEW_API_SESSION_COOKIE_TRUSTED_URL=https://api.yanchuaner.cn
YANCHUANER_HASHED_KEYS_ENABLED=true
YANCHUANER_QUOTA_LEDGER_ENABLED=true
YANCHUANER_CAMPAIGN_FUNDING_ENABLED=false
YANCHUANER_VIRTUAL_KEY_POLICY_ENABLED=false
YANCHUANER_VIRTUAL_KEY_DEFAULT_RPM=60
YANCHUANER_VIRTUAL_KEY_DEFAULT_TPM=100000
YANCHUANER_VIRTUAL_KEY_DEFAULT_CONCURRENCY=2
YANCHUANER_SUBJECT_GRANTS_ENABLED=false
AI_CORE_NETWORK=yanchuaner-ai-core
REDIS_HOST_PORT=6380

CONTROL_DB_PASSWORD=$(New-UrlSafeSecret)
GATEWAY_DB_PASSWORD=$(New-UrlSafeSecret)
REDIS_PASSWORD=$(New-UrlSafeSecret)

NEW_API_SESSION_SECRET=$(New-UrlSafeSecret 48)
NEW_API_CRYPTO_SECRET=$(New-UrlSafeSecret 48)
YANCHUANER_SUBJECT_SIGNING_SECRET=$(New-UrlSafeSecret 48)
YANCHUANER_SUBJECT_EXCHANGE_ENABLED=false
YANCHUANER_SUBJECT_EXCHANGE_CLIENT_ID=ai-yancore-bff
YANCHUANER_SUBJECT_EXCHANGE_CLIENT_SECRET=$(New-UrlSafeSecret 48)
YANCHUANER_SUBJECT_EXCHANGE_USERINFO_URL=https://yanchuaner.cn/api/oauth/userinfo
YANCHUANER_SUBJECT_EXCHANGE_ALLOW_INSECURE_HTTP=false
YANCHUANER_AI_WEB_SESSION_QUOTA=50000
YANCHUANER_AI_WEB_MODELS=gpt-4.1-mini,deepseek-chat

NEW_API_ROOT_USERNAME=yanchuaner
NEW_API_ROOT_PASSWORD=$(New-UrlSafeSecret 36)
NEW_API_ROOT_USER_ID=
NEW_API_ROOT_ACCESS_TOKEN=
NEW_USER_INITIAL_QUOTA=500000
OPENWEBUI_SERVICE_QUOTA=10000000

YANCHUANER_OAUTH_CLIENT_ID=api-yanchuaner
YANCHUANER_OAUTH_CLIENT_SECRET=$(New-UrlSafeSecret 48)
YANCHUANER_AI_OAUTH_CLIENT_ID=ai-yanchuaner
YANCHUANER_AI_OAUTH_CLIENT_SECRET=$(New-UrlSafeSecret 48)
YANCHUANER_AI_WEB_OAUTH_CLIENT_ID=ai-web-yanchuaner
YANCHUANER_AI_WEB_OAUTH_CLIENT_SECRET=$(New-UrlSafeSecret 48)
WEB_MAIN_PUBLIC_URL=https://yanchuaner.cn
WEB_MAIN_INTERNAL_URL=https://yanchuaner.cn

LITELLM_PUBLIC_URL=http://127.0.0.1:4000
LITELLM_INTERNAL_URL=http://litellm-gateway:4000

LITELLM_MASTER_KEY=sk-$(New-UrlSafeSecret 36)
LITELLM_SALT_KEY=sk-$(New-UrlSafeSecret 36)
LITELLM_UI_USERNAME=yanchuaner
LITELLM_UI_PASSWORD=$(New-UrlSafeSecret 36)
LITELLM_NEW_API_KEY=
"@

$directory = Split-Path -Parent $OutputPath
New-Item -ItemType Directory -Force -Path $directory | Out-Null
[System.IO.File]::WriteAllText($OutputPath, $content, [System.Text.UTF8Encoding]::new($false))
Write-Host "Created $OutputPath. Keep this file out of Git and back it up securely."

param(
  [string]$WebRepo = (Join-Path $PSScriptRoot "..\..\web_yanchuaner"),
  [string]$AiRepo = (Join-Path $PSScriptRoot "..\..\ai_yanchuaner")
)

$ErrorActionPreference = "Stop"
$apiRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$apiEnvPath = Join-Path $apiRoot "deploy\.env"
$apiCompose = Join-Path $apiRoot "deploy\compose.yaml"
$aiCompose = Join-Path $AiRepo "docker-compose.yml"
$aiEnvPath = Join-Path $AiRepo ".env"

function Read-DotEnv([string]$Path) {
  $values = @{}
  Get-Content -LiteralPath $Path | ForEach-Object {
    if ($_ -match '^([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
      $values[$matches[1]] = $matches[2].Trim().Trim('"').Trim("'")
    }
  }
  return $values
}

function Set-DotEnvValues([string]$Path, [hashtable]$Values) {
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
    if ($index -ge 0) { $lines[$index] = $replacement } else { $lines.Add($replacement) }
  }
  $content = ($lines -join [Environment]::NewLine).TrimEnd() + [Environment]::NewLine
  [System.IO.File]::WriteAllText($Path, $content, [System.Text.UTF8Encoding]::new($false))
}

function Wait-Http([string]$Uri, [int]$Attempts = 90) {
  for ($attempt = 1; $attempt -le $Attempts; $attempt++) {
    try {
      $response = Invoke-WebRequest -UseBasicParsing -Uri $Uri -TimeoutSec 5
      if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500) { return }
    } catch {}
    Start-Sleep -Seconds 2
  }
  throw "Service did not become ready: $Uri"
}

function Invoke-NewApiJson(
  [string]$Method,
  [string]$Path,
  [object]$Body = $null,
  [Microsoft.PowerShell.Commands.WebRequestSession]$Session = $null
) {
  $params = @{
    Method = $Method
    Uri = "http://127.0.0.1:3101$Path"
    ContentType = "application/json"
    TimeoutSec = 30
  }
  if ($null -ne $Body) { $params.Body = ($Body | ConvertTo-Json -Depth 12 -Compress) }
  if ($null -ne $Session) { $params.WebSession = $Session }
  if (-not [string]::IsNullOrWhiteSpace($script:newApiUserId)) {
    $params.Headers = @{ "New-Api-User" = $script:newApiUserId }
  }
  return Invoke-RestMethod @params
}

& (Join-Path $PSScriptRoot "prepare-integrated-env.ps1") -WebRepo $WebRepo -AiRepo $AiRepo
$apiEnv = Read-DotEnv $apiEnvPath
$aiEnv = Read-DotEnv $aiEnvPath
$newUserInitialQuota = 0
$openWebUiServiceQuota = 0
if (-not [int]::TryParse($apiEnv["NEW_USER_INITIAL_QUOTA"], [ref]$newUserInitialQuota) -or $newUserInitialQuota -le 0) {
  throw "NEW_USER_INITIAL_QUOTA must be a positive integer"
}
if (-not [int]::TryParse($apiEnv["OPENWEBUI_SERVICE_QUOTA"], [ref]$openWebUiServiceQuota) -or $openWebUiServiceQuota -le 0) {
  throw "OPENWEBUI_SERVICE_QUOTA must be a positive integer"
}

docker compose --project-directory $AiRepo -f $aiCompose up -d db litellm
if ($LASTEXITCODE -ne 0) { throw "Unable to start LiteLLM data plane" }
Wait-Http "http://127.0.0.1:4000/health/liveliness" 60

docker compose --env-file $apiEnvPath -f $apiCompose up -d --build control-db redis new-api
if ($LASTEXITCODE -ne 0) { throw "Unable to start New API control plane" }
Wait-Http "http://127.0.0.1:3101/api/status" 120

$setup = Invoke-NewApiJson "GET" "/api/setup"
if (-not $setup.data.status) {
  $setupResult = Invoke-NewApiJson "POST" "/api/setup" @{
    username = $apiEnv["NEW_API_ROOT_USERNAME"]
    password = $apiEnv["NEW_API_ROOT_PASSWORD"]
    confirmPassword = $apiEnv["NEW_API_ROOT_PASSWORD"]
    SelfUseModeEnabled = $false
    DemoSiteEnabled = $false
  }
  if (-not $setupResult.success) { throw "New API setup failed: $($setupResult.message)" }
}

$session = [Microsoft.PowerShell.Commands.WebRequestSession]::new()
$login = Invoke-NewApiJson "POST" "/api/user/login" @{
  username = $apiEnv["NEW_API_ROOT_USERNAME"]
  password = $apiEnv["NEW_API_ROOT_PASSWORD"]
} $session
if (-not $login.success) { throw "New API root login failed" }
$script:newApiUserId = [string]$login.data.id

$lockedOptions = [ordered]@{
  SystemName = "燕中 API"
  "theme.frontend" = "default"
  RegisterEnabled = $true
  PasswordRegisterEnabled = $false
  GitHubOAuthEnabled = $false
  LinuxDOOAuthEnabled = $false
  TelegramOAuthEnabled = $false
  WeChatAuthEnabled = $false
  QuotaForNewUser = $newUserInitialQuota
  QuotaForInviter = 0
  QuotaForInvitee = 0
  SelfUseModeEnabled = $false
  DemoSiteEnabled = $false
  ServerAddress = $apiEnv["NEW_API_PUBLIC_URL"]
}
foreach ($entry in $lockedOptions.GetEnumerator()) {
  $result = Invoke-NewApiJson "PUT" "/api/option/" @{ key = $entry.Key; value = $entry.Value } $session
  if (-not $result.success) { throw "Failed to set New API option $($entry.Key): $($result.message)" }
}

$providers = Invoke-NewApiJson "GET" "/api/custom-oauth-provider/" $null $session
$oauthProvider = @($providers.data) | Where-Object { $_.slug -eq "yanchuaner" } | Select-Object -First 1
$oauthPayload = @{
  name = "燕中校友"
  slug = "yanchuaner"
  icon = "school"
  enabled = $true
  client_id = $apiEnv["YANCHUANER_OAUTH_CLIENT_ID"]
  client_secret = $apiEnv["YANCHUANER_OAUTH_CLIENT_SECRET"]
  authorization_endpoint = "$($apiEnv['WEB_MAIN_PUBLIC_URL'])/api/oauth/authorize"
  token_endpoint = "$($apiEnv['WEB_MAIN_INTERNAL_URL'])/api/oauth/token"
  user_info_endpoint = "$($apiEnv['WEB_MAIN_INTERNAL_URL'])/api/oauth/userinfo"
  scopes = "openid profile email"
  user_id_field = "sub"
  username_field = "preferred_username"
  display_name_field = "name"
  email_field = "email"
  well_known = ""
  auth_style = 1
  access_policy = '{"logic":"and","conditions":[{"field":"role","op":"in","value":["alumni","admin"]}]}'
  access_denied_message = "仅限已完成身份认证的燕中校友使用。"
}
if ($oauthProvider) {
  $oauthResult = Invoke-NewApiJson "PUT" "/api/custom-oauth-provider/$($oauthProvider.id)" $oauthPayload $session
} else {
  $oauthResult = Invoke-NewApiJson "POST" "/api/custom-oauth-provider/" $oauthPayload $session
}
if (-not $oauthResult.success) { throw "Failed to configure Yanchuaner OAuth: $($oauthResult.message)" }

$liteLlmAdminHeaders = @{ Authorization = "Bearer $($aiEnv['LITELLM_MASTER_KEY'])" }
$modelsResponse = Invoke-RestMethod -Uri "$($apiEnv['LITELLM_PUBLIC_URL'])/v1/models" -Headers $liteLlmAdminHeaders -TimeoutSec 30
$upstreamModels = @($modelsResponse.data | ForEach-Object { $_.id } | Sort-Object -Unique)
if ($upstreamModels.Count -eq 0) { throw "LiteLLM exposes no models" }

$liteLlmKey = $apiEnv["LITELLM_NEW_API_KEY"]
$liteLlmKeyValid = $false
if (-not [string]::IsNullOrWhiteSpace($liteLlmKey)) {
  try {
    Invoke-RestMethod -Uri "$($apiEnv['LITELLM_PUBLIC_URL'])/v1/models" -Headers @{ Authorization = "Bearer $liteLlmKey" } -TimeoutSec 10 | Out-Null
    $liteLlmKeyValid = $true
  } catch {}
}
if (-not $liteLlmKeyValid) {
  $keyResponse = Invoke-RestMethod -Method Post -Uri "$($apiEnv['LITELLM_PUBLIC_URL'])/key/generate" `
    -Headers $liteLlmAdminHeaders -ContentType "application/json" `
    -Body (@{
      key_alias = "api-yanchuaner-upstream"
      models = $upstreamModels
      rpm_limit = 180
      tpm_limit = 300000
      metadata = @{ owner = "api_yanchuaner"; purpose = "New API upstream routing" }
    } | ConvertTo-Json -Depth 8 -Compress) -TimeoutSec 30
  $liteLlmKey = $keyResponse.key
  if ([string]::IsNullOrWhiteSpace($liteLlmKey)) { throw "LiteLLM did not return a virtual key" }
  Set-DotEnvValues $apiEnvPath @{ LITELLM_NEW_API_KEY = $liteLlmKey }
}

$publicModels = @("deepseek-chat", "deepseek-reasoner", "gpt-image-2")
$modelMapping = @{
  "deepseek-chat" = "deepseek/deepseek-v4-flash"
  "deepseek-reasoner" = "deepseek/deepseek-v4-pro"
  "gpt-image-2" = "gpt-image-2"
} | ConvertTo-Json -Compress
$channels = Invoke-NewApiJson "GET" "/api/channel/?p=1&page_size=100" $null $session
$channel = @($channels.data.items) | Where-Object { $_.name -eq "燕中 AI 数据面" } | Select-Object -First 1
$channelPayload = @{
  type = 1
  key = $liteLlmKey
  name = "燕中 AI 数据面"
  weight = 100
  base_url = $apiEnv["LITELLM_INTERNAL_URL"]
  models = ($publicModels -join ",")
  group = "default"
  model_mapping = $modelMapping
  auto_ban = 1
  priority = 0
  other = ""
  other_info = ""
  setting = ""
  settings = ""
}
if ($channel) {
  $channelPayload.id = $channel.id
  $channelResult = Invoke-NewApiJson "PUT" "/api/channel/" $channelPayload $session
  $channelId = $channel.id
} else {
  $channelPayload.status = 1
  $channelResult = Invoke-NewApiJson "POST" "/api/channel/" @{
    mode = "single"
    multi_key_mode = "random"
    batch_add_set_key_prefix_2_name = $false
    channel = $channelPayload
  } $session
  $channels = Invoke-NewApiJson "GET" "/api/channel/?p=1&page_size=100" $null $session
  $channelId = (@($channels.data.items) | Where-Object { $_.name -eq "燕中 AI 数据面" } | Select-Object -First 1).id
}
if (-not $channelResult.success -or -not $channelId) { throw "Failed to configure New API channel: $($channelResult.message)" }

$channelTest = Invoke-NewApiJson "GET" "/api/channel/test/$channelId" $null $session
if (-not $channelTest.success) { throw "New API channel test failed: $($channelTest.message)" }

$tokens = Invoke-NewApiJson "GET" "/api/token/?p=1&page_size=100" $null $session
$serviceToken = @($tokens.data.items) | Where-Object { $_.name -eq "open-webui-service" } | Select-Object -First 1
if (-not $serviceToken) {
  $tokenResult = Invoke-NewApiJson "POST" "/api/token/" @{
    name = "open-webui-service"
    expired_time = -1
    unlimited_quota = $false
    remain_quota = $openWebUiServiceQuota
    model_limits_enabled = $true
    model_limits = ($publicModels -join ",")
    group = "default"
  } $session
  if (-not $tokenResult.success) { throw "Failed to create Open WebUI service token: $($tokenResult.message)" }
  $tokens = Invoke-NewApiJson "GET" "/api/token/?p=1&page_size=100" $null $session
  $serviceToken = @($tokens.data.items) | Where-Object { $_.name -eq "open-webui-service" } | Select-Object -First 1
} elseif ($serviceToken.unlimited_quota) {
  $tokenResult = Invoke-NewApiJson "PUT" "/api/token/" @{
    id = $serviceToken.id
    name = $serviceToken.name
    expired_time = $serviceToken.expired_time
    unlimited_quota = $false
    remain_quota = $openWebUiServiceQuota
    model_limits_enabled = $true
    model_limits = ($publicModels -join ",")
    group = "default"
    allow_ips = ""
    cross_group_retry = $false
  } $session
  if (-not $tokenResult.success) { throw "Failed to cap the Open WebUI service token: $($tokenResult.message)" }
}
$tokenKey = Invoke-NewApiJson "POST" "/api/token/$($serviceToken.id)/key" @{} $session
$openWebUiKey = "sk-$($tokenKey.data.key)"
Set-DotEnvValues $aiEnvPath @{
  OPENWEBUI_API_KEY = $openWebUiKey
  OPENWEBUI_IMAGE_API_KEY = $openWebUiKey
}

docker compose --project-directory $AiRepo -f $aiCompose up -d --force-recreate open-webui
if ($LASTEXITCODE -ne 0) { throw "Unable to start Open WebUI" }
Wait-Http "http://127.0.0.1:3001/health" 90
& (Join-Path $AiRepo "scripts\sync-openwebui-api-config.ps1")

$oidcDiscovery = Invoke-RestMethod -Uri "$($apiEnv['WEB_MAIN_PUBLIC_URL'])/api/oauth/.well-known/openid-configuration" -TimeoutSec 30
if (-not (@($oidcDiscovery.id_token_signing_alg_values_supported) -contains "RS256")) {
  throw "Main-site OIDC discovery does not advertise RS256"
}
$oidcJwks = Invoke-RestMethod -Uri "$($apiEnv['WEB_MAIN_PUBLIC_URL'])/api/oauth/jwks" -TimeoutSec 30
if (@($oidcJwks.keys).Count -lt 1 -or $oidcJwks.keys[0].kty -ne "RSA") {
  throw "Main-site OIDC JWKS does not expose an RSA public key"
}
$openWebUiConfig = Invoke-RestMethod -Uri "http://127.0.0.1:3001/api/config" -TimeoutSec 30
if (-not ($openWebUiConfig.oauth.providers.PSObject.Properties.Name -contains "oidc")) {
  throw "Open WebUI does not expose the Yanchuaner OIDC provider"
}

$apiHeaders = @{ Authorization = "Bearer $openWebUiKey"; "Content-Type" = "application/json" }
$apiModels = Invoke-RestMethod -Uri "http://127.0.0.1:3101/v1/models" -Headers $apiHeaders -TimeoutSec 30
if (-not (@($apiModels.data.id) -contains "deepseek-chat")) { throw "New API model list is missing deepseek-chat" }
$chatContent = ""
for ($attempt = 1; $attempt -le 3; $attempt++) {
  try {
    $chat = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:3101/v1/chat/completions" `
      -Headers $apiHeaders -Body (@{
        model = "deepseek-chat"
        messages = @(@{ role = "user"; content = "请只回复：燕中 API 链路通过" })
        max_tokens = 80
        stream = $false
      } | ConvertTo-Json -Depth 8 -Compress) -TimeoutSec 90
    $chatContent = [string]$chat.choices[0].message.content
    if (-not [string]::IsNullOrWhiteSpace($chatContent)) { break }
  } catch {
    if ($attempt -eq 3) { throw }
  }
  Start-Sleep -Seconds 2
}
if ([string]::IsNullOrWhiteSpace($chatContent)) {
  throw "End-to-end model response was empty"
}

Write-Output "Integrated stack is ready."
Write-Output "Main site: http://localhost:3000"
Write-Output "API platform: http://localhost:3101"
Write-Output "AI workspace: http://localhost:3001"
Write-Output "LiteLLM admin: http://localhost:4000/ui"
Write-Output "Verified model path: New API -> LiteLLM -> deepseek-chat"
Write-Output "Verified identity path: Main site -> OAuth/OIDC -> New API and Open WebUI"

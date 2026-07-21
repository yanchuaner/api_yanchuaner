param(
  [string]$BaseUrl = "http://127.0.0.1:3101",
  [string]$EnvFile = (Join-Path $PSScriptRoot "..\deploy\.env"),
  [string]$UserId = "",
  [string]$AccessToken = "",
  [string]$Model = "deepseek-chat",
  [switch]$KeepToken
)

$ErrorActionPreference = "Stop"

function Read-DotEnv([string]$Path) {
  $values = @{}
  Get-Content -LiteralPath $Path | ForEach-Object {
    if ($_ -match '^([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
      $values[$matches[1]] = $matches[2].Trim().Trim('"').Trim("'")
    }
  }
  return $values
}

function Invoke-ControlPlaneJson([string]$Method, [string]$Path, [object]$Body = $null) {
  $parameters = @{
    Method = $Method
    Uri = "$BaseUrl$Path"
    Headers = $script:headers
    ContentType = "application/json"
    TimeoutSec = 30
  }
  if ($null -ne $Body) {
    $parameters.Body = $Body | ConvertTo-Json -Depth 12 -Compress
  }
  return Invoke-RestMethod @parameters
}

if ([string]::IsNullOrWhiteSpace($UserId) -or [string]::IsNullOrWhiteSpace($AccessToken)) {
  if (-not (Test-Path -LiteralPath $EnvFile)) {
    throw "Missing $EnvFile. Generate the local environment or pass UserId and AccessToken."
  }
  $envValues = Read-DotEnv $EnvFile
  $UserId = $envValues["NEW_API_ROOT_USER_ID"]
  $AccessToken = $envValues["NEW_API_ROOT_ACCESS_TOKEN"]
}
if ([string]::IsNullOrWhiteSpace($UserId) -or [string]::IsNullOrWhiteSpace($AccessToken)) {
  throw "Run scripts/bootstrap-integrated-stack.ps1 once or pass UserId and AccessToken."
}

$script:headers = @{
  Authorization = "Bearer $AccessToken"
  "New-Api-User" = $UserId
}
$status = Invoke-ControlPlaneJson "GET" "/api/status"
if (-not $status.data.yanchuaner_hashed_keys_enabled -or -not $status.data.yancore_virtual_key_policy_enabled) {
  throw "Hashed keys and YanCore virtual key policy must both be enabled in the local stack."
}

$tokenId = 0
try {
  $created = Invoke-ControlPlaneJson "POST" "/api/token/" @{
    name = "policy-acceptance-$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
    expired_time = -1
    remain_quota = 500000
    unlimited_quota = $false
    model_limits_enabled = $true
    model_limits = $Model
    allow_ips = "127.0.0.1"
    group = "default"
    cross_group_retry = $false
    yancore_policy = @{
      max_rpm = 6
      max_tpm = 12000
      max_concurrency = 1
    }
  }
  if (-not $created.success -or -not $created.data.token.id -or -not $created.data.policy.id) {
    throw "Virtual key creation did not return a token and policy."
  }
  $tokenId = [int]$created.data.token.id

  $initial = Invoke-ControlPlaneJson "GET" "/api/yancore/virtual-key-policies/$tokenId"
  if (-not $initial.success -or $initial.data.version -ne 1) {
    throw "Initial policy revision is invalid."
  }

  $updated = Invoke-ControlPlaneJson "PUT" "/api/yancore/virtual-key-policies/$tokenId" @{
    max_rpm = 5
    max_tpm = 10000
    max_concurrency = 1
    reason = "local stage 2D acceptance"
    token = @{
      name = "policy-acceptance-updated-$tokenId"
      remain_quota = 490000
      expired_time = -1
      models = @($Model)
      allow_ips = "127.0.0.1`n10.0.0.0/8"
      group = "default"
      cross_group_retry = $false
    }
  }
  if (-not $updated.success -or $updated.data.version -ne 2 -or $updated.data.max_rpm -ne 5) {
    throw "Atomic policy update did not produce revision 2."
  }

  $revisions = Invoke-ControlPlaneJson "GET" "/api/yancore/virtual-key-policies/$tokenId/revisions"
  if (-not $revisions.success -or @($revisions.data).Count -ne 2) {
    throw "Expected exactly two policy revisions."
  }
  if ($revisions.data[0].reason -ne "local stage 2D acceptance") {
    throw "Latest policy revision reason is missing."
  }

  $bypassBody = @{
    id = $tokenId
    name = "legacy-bypass"
    remain_quota = 999999
    unlimited_quota = $false
    expired_time = -1
    model_limits_enabled = $true
    model_limits = $Model
  } | ConvertTo-Json -Depth 8 -Compress
  $bypass = Invoke-WebRequest -Method Put -Uri "$BaseUrl/api/token/" -Headers $script:headers `
    -ContentType "application/json" -Body $bypassBody -SkipHttpErrorCheck -TimeoutSec 30
  if ($bypass.StatusCode -ne 409) {
    throw "Legacy token update bypass returned HTTP $($bypass.StatusCode), expected 409."
  }

  Write-Output "Stage 2D virtual key policy acceptance passed for token ID $tokenId at revision 2."
} finally {
  if ($tokenId -gt 0 -and -not $KeepToken) {
    try {
      Invoke-ControlPlaneJson "DELETE" "/api/token/$tokenId" | Out-Null
    } catch {
      Write-Warning "Unable to clean up acceptance token ID $tokenId."
    }
  }
}

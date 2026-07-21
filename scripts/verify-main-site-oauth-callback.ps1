param(
  [string]$MainBaseUrl = "http://localhost:3000",
  [string]$ApiBaseUrl = "http://localhost:3201",
  [string]$ClientId = "api-yanchuaner",
  [string]$AlumniUsername = "acceptance-alumni",
  [string]$AdminUsername = "acceptance-admin",
  [string]$AcceptancePassword = "AcceptancePass!2026",
  [switch]$AllowLocalMutation
)

$ErrorActionPreference = "Stop"

function Assert-LocalHttpUrl([string]$Value, [string]$Name) {
  try {
    $uri = [Uri]$Value
  } catch {
    throw "$Name must be an absolute localhost URL."
  }
  if (-not $uri.IsAbsoluteUri -or $uri.Scheme -ne "http") {
    throw "$Name must use HTTP in the isolated local environment."
  }
  if ($uri.Host -notin @("localhost", "127.0.0.1", "::1")) {
    throw "$Name must target localhost; remote OAuth mutation is intentionally unsupported."
  }
  return $uri.GetLeftPart([UriPartial]::Authority).TrimEnd("/")
}

function Invoke-Json(
  [string]$Method,
  [string]$Uri,
  [object]$Body = $null,
  [Microsoft.PowerShell.Commands.WebRequestSession]$Session = $null,
  [hashtable]$Headers = @{}
) {
  $parameters = @{
    Method = $Method
    Uri = $Uri
    Headers = $Headers
    SkipHttpErrorCheck = $true
    TimeoutSec = 30
  }
  if ($null -ne $Session) { $parameters.WebSession = $Session }
  if ($null -ne $Body) {
    $parameters.ContentType = "application/json"
    $parameters.Body = $Body | ConvertTo-Json -Depth 8 -Compress
  }
  return Invoke-RestMethod @parameters
}

function Invoke-AuthorizationRedirect(
  [string]$Uri,
  [Microsoft.PowerShell.Commands.WebRequestSession]$Session
) {
  $handler = [System.Net.Http.HttpClientHandler]::new()
  $handler.AllowAutoRedirect = $false
  $handler.CookieContainer = $Session.Cookies
  $client = [System.Net.Http.HttpClient]::new($handler)
  $response = $null
  try {
    $response = $client.GetAsync($Uri).GetAwaiter().GetResult()
    if ([int]$response.StatusCode -notin @(302, 303, 307, 308)) {
      throw "Main-site authorization returned HTTP $([int]$response.StatusCode), expected a callback redirect."
    }
    $location = $response.Headers.Location
    if ($null -eq $location) { throw "Main-site authorization did not return a callback location." }
    if (-not $location.IsAbsoluteUri) {
      $location = [Uri]::new([Uri]$MainBaseUrl, $location)
    }
    return $location
  } finally {
    if ($null -ne $response) { $response.Dispose() }
    $client.Dispose()
    $handler.Dispose()
  }
}

function Invoke-MainSiteLogin(
  [string]$Username,
  [Microsoft.PowerShell.Commands.WebRequestSession]$Session
) {
  $login = Invoke-Json "POST" "$MainBaseUrl/api/auth/login" @{
    username = $Username
    password = $AcceptancePassword
  } $Session @{ Origin = $MainBaseUrl }
  if (-not $login.success) {
    throw "Main-site acceptance login failed for $Username."
  }
}

function Invoke-NewApiOAuthLogin([string]$Username) {
  $mainSession = [Microsoft.PowerShell.Commands.WebRequestSession]::new()
  $apiSession = [Microsoft.PowerShell.Commands.WebRequestSession]::new()
  Invoke-MainSiteLogin $Username $mainSession

  $stateResult = Invoke-Json "GET" "$ApiBaseUrl/api/oauth/state" $null $apiSession
  if (-not $stateResult.success -or [string]::IsNullOrWhiteSpace($stateResult.data)) {
    throw "New API did not issue an OAuth state."
  }
  $state = [string]$stateResult.data
  $callbackUrl = "$ApiBaseUrl/oauth/yanchuaner"
  $authorizeUrl = "$MainBaseUrl/api/oauth/authorize" +
    "?response_type=code" +
    "&client_id=$([Uri]::EscapeDataString($ClientId))" +
    "&redirect_uri=$([Uri]::EscapeDataString($callbackUrl))" +
    "&scope=$([Uri]::EscapeDataString('openid profile email'))" +
    "&state=$([Uri]::EscapeDataString($state))"

  $location = Invoke-AuthorizationRedirect $authorizeUrl $mainSession
  $expectedCallback = [Uri]$callbackUrl
  if ($location.GetLeftPart([UriPartial]::Path) -ne $expectedCallback.GetLeftPart([UriPartial]::Path)) {
    throw "Main-site authorization returned an unexpected callback target."
  }
  $query = [System.Web.HttpUtility]::ParseQueryString($location.Query)
  if ($query["state"] -ne $state) { throw "OAuth callback state does not match the New API session." }
  if (-not [string]::IsNullOrWhiteSpace($query["error"])) {
    throw "Main-site authorization rejected the acceptance identity: $($query['error'])."
  }
  $code = $query["code"]
  if ([string]::IsNullOrWhiteSpace($code)) { throw "OAuth callback did not contain an authorization code." }

  $callback = "$ApiBaseUrl/api/oauth/yanchuaner" +
    "?code=$([Uri]::EscapeDataString($code))" +
    "&state=$([Uri]::EscapeDataString($state))"
  $result = Invoke-Json "GET" $callback $null $apiSession
  if (-not $result.success) {
    throw "New API callback failed for $Username."
  }
  return $result.data
}

if (-not $AllowLocalMutation) {
  throw "Pass -AllowLocalMutation to confirm that this script may create users and audit records in an isolated local database."
}

$MainBaseUrl = Assert-LocalHttpUrl $MainBaseUrl "MainBaseUrl"
$ApiBaseUrl = Assert-LocalHttpUrl $ApiBaseUrl "ApiBaseUrl"

$status = Invoke-Json "GET" "$ApiBaseUrl/api/status"
if (-not $status.success) { throw "New API status endpoint is unavailable." }
if ($status.data.password_login_enabled -ne $false -or $status.data.password_register_enabled -ne $false) {
  throw "New API password login and registration must both be disabled before OAuth acceptance."
}
if ($status.data.register_enabled -ne $true) {
  throw "New API registration must remain enabled for first-time OAuth provisioning."
}
$provider = @($status.data.custom_oauth_providers) |
  Where-Object { $_.slug -eq "yanchuaner" } |
  Select-Object -First 1
if ($null -eq $provider -or $provider.client_id -ne $ClientId) {
  throw "The public yanchuaner OAuth provider is missing or uses a different client ID."
}
if ($provider.authorization_endpoint.TrimEnd("/") -ne "$MainBaseUrl/api/oauth/authorize") {
  throw "The public yanchuaner OAuth provider does not use the expected main-site authorization endpoint."
}

$firstAlumni = Invoke-NewApiOAuthLogin $AlumniUsername
$secondAlumni = Invoke-NewApiOAuthLogin $AlumniUsername
if ($firstAlumni.id -ne $secondAlumni.id) {
  throw "Repeated alumni login created or selected a different New API user."
}
if ($firstAlumni.role -ne 1 -or $secondAlumni.role -ne 1) {
  throw "Verified alumni did not map to the common-user role."
}

$admin = Invoke-NewApiOAuthLogin $AdminUsername
if ($admin.role -ne 100) {
  throw "Trusted main-site administrator did not map to the New API root role."
}

$passwordLogin = Invoke-Json "POST" "$ApiBaseUrl/api/user/login" @{
  username = $AlumniUsername
  password = $AcceptancePassword
}
if ($passwordLogin.success -ne $false) { throw "New API local password login is still available." }

$passwordRegistration = Invoke-Json "POST" "$ApiBaseUrl/api/user/register" @{
  username = "acceptance-local-registration"
  password = $AcceptancePassword
}
if ($passwordRegistration.success -ne $false) { throw "New API local password registration is still available." }

Write-Output "New API OAuth callback acceptance passed: alumni user $($firstAlumni.id) was reused, administrator role synchronization passed, and local password entry points were rejected."

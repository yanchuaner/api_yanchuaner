param(
  [string]$Baseline = "v1.0.0-rc.21"
)

$ErrorActionPreference = "Stop"

$repoRoot = (& git rev-parse --show-toplevel).Trim()
if (-not $repoRoot) {
  throw "Run this script inside the api_yanchuaner Git repository."
}

$baselineCommit = (& git rev-parse "$Baseline^{commit}").Trim()
if ($LASTEXITCODE -ne 0 -or -not $baselineCommit) {
  throw "Cannot resolve baseline $Baseline."
}

$baselineFiles = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
& git ls-tree -r --name-only $baselineCommit | ForEach-Object {
  [void]$baselineFiles.Add($_)
}

$trackedAndNewFiles = @(& git ls-files --cached --others --exclude-standard) | Sort-Object -Unique
$results = foreach ($path in $trackedAndNewFiles) {
  if (-not $baselineFiles.Contains($path)) {
    [PSCustomObject]@{
      Path = $path
      MechanicalClass = "local-addition-review-required"
      Baseline = $baselineCommit
    }
    continue
  }

  & git diff --quiet $baselineCommit -- $path
  $class = if ($LASTEXITCODE -eq 0) { "retained-upstream" } else { "modified-upstream" }
  [PSCustomObject]@{
    Path = $path
    MechanicalClass = $class
    Baseline = $baselineCommit
  }
}

$results | Sort-Object MechanicalClass, Path | Format-Table -AutoSize

Write-Host ""
Write-Host "Summary:"
$results | Group-Object MechanicalClass | Sort-Object Name | Select-Object Name, Count | Format-Table -AutoSize

Write-Host ""
Write-Host "Dependency manifests: go.mod, go.sum, web/package.json, web/bun.lock"
Write-Host "License inventories: LICENSE, NOTICE, THIRD-PARTY-LICENSES.md, THIRD_PARTY_NOTICES.md"
Write-Host "Manual classification: docs/yanchuaner/copyright-matrix.md"

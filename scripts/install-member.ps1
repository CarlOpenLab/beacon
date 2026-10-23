# Install Beacon's member daemon and register it as a logon task.
#
#   irm https://raw.githubusercontent.com/CarlOpenLab/beacon/main/scripts/install-member.ps1 -OutFile install.ps1
#   ./install.ps1 -Relay wss://relay.example/relay/agent -DeviceId <id> -DeviceSecret <secret>
#
# Run it again to update: the binary is replaced and the task restarted.
param(
  [string]$Relay = "",
  [string]$DeviceId = "",
  [string]$DeviceSecret = "",
  [string]$MachineName = "",
  [string]$RelayCA = "",
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$Rest
)

$ErrorActionPreference = "Stop"
$repo = "CarlOpenLab/beacon"

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  "AMD64" { "amd64" }
  "ARM64" { "arm64" }
  default { throw "unsupported architecture $env:PROCESSOR_ARCHITECTURE" }
}

$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
$version = $release.tag_name.TrimStart("v")
$asset = "beacon-cli_${version}_windows_${arch}.zip"
$url = "https://github.com/$repo/releases/download/$($release.tag_name)/$asset"

$dest = Join-Path $env:LOCALAPPDATA "Beacon\bin"
New-Item -ItemType Directory -Force -Path $dest | Out-Null
$zip = Join-Path $env:TEMP $asset
Write-Host "downloading $($release.tag_name) (windows/$arch)…"
Invoke-WebRequest -Uri $url -OutFile $zip
Expand-Archive -Path $zip -DestinationPath $dest -Force
Remove-Item $zip -Force

$tfd = Join-Path $dest "tfd.exe"
Write-Host "installed $tfd"

$flags = @()
if ($Relay) { $flags += @("-relay", $Relay) }
if ($DeviceId) { $flags += @("-device-id", $DeviceId) }
if ($DeviceSecret) { $flags += @("-device-secret", $DeviceSecret) }
if ($MachineName) { $flags += @("-machine-name", $MachineName) }
if ($RelayCA) { $flags += @("-relay-ca", $RelayCA) }

& $tfd service install @flags
if ($LASTEXITCODE -ne 0) { throw "tfd service install failed" }

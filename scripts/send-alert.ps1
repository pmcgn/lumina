<#
.SYNOPSIS
  Sends an urgent alert to a running Lumina (overlay on all connected dashboards).

.DESCRIPTION
  Calls POST /api/alerts (see docs/alerts.md). With -Confirm the overlay gets a
  confirm button; pressing it makes Lumina call -CallbackUrl. The default callback is
  the recorder of the demo services (scripts/demo.ps1); list what it received with
  -ShowCallbacks.

.PARAMETER Text
  Message to show. Long texts are shown smaller (max 600 characters).

.PARAMETER Color
  red or orange.

.PARAMETER Seconds
  Time until the overlay disappears on its own.

.PARAMETER Confirm
  Show a confirm button (sends -CallbackUrl as callbackUrl).

.PARAMETER CallbackUrl
  URL that Lumina calls (POST) when the button is pressed.

.PARAMETER Dismiss
  Id of an alert to remove early (without calling its callback).

.PARAMETER ShowCallbacks
  Lists the confirmations the demo services received.

.EXAMPLE
  .\scripts\send-alert.ps1 -Text 'Water leak in the basement' -Color red -Seconds 60 -Confirm
.EXAMPLE
  .\scripts\send-alert.ps1 -Text 'Washing machine is done' -Color orange -Seconds 20
#>
[CmdletBinding(DefaultParameterSetName = 'Send')]
param(
  [Parameter(ParameterSetName = 'Send')][string]$Text = 'Water leak in the basement!',
  [Parameter(ParameterSetName = 'Send')][ValidateSet('red', 'orange')][string]$Color = 'red',
  [Parameter(ParameterSetName = 'Send')][ValidateRange(1, 86400)][int]$Seconds = 30,
  [Parameter(ParameterSetName = 'Send')][switch]$Confirm,
  [Parameter(ParameterSetName = 'Send')][string]$CallbackUrl = 'http://localhost:18080/alert-callback',
  [Parameter(ParameterSetName = 'Dismiss', Mandatory)][string]$Dismiss,
  [Parameter(ParameterSetName = 'Callbacks', Mandatory)][switch]$ShowCallbacks,
  [string]$Lumina = 'http://localhost:8080'
)

$ErrorActionPreference = 'Stop'

if ($ShowCallbacks) {
  Invoke-RestMethod 'http://localhost:18080/alert-callback' | ConvertTo-Json -Depth 5
  return
}

if ($Dismiss) {
  Invoke-RestMethod -Method Delete -Uri "$Lumina/api/alerts/$Dismiss"
  Write-Host "alert $Dismiss removed"
  return
}

$body = @{ text = $Text; color = $Color; durationSeconds = $Seconds }
if ($Confirm) { $body.callbackUrl = $CallbackUrl }

try {
  $res = Invoke-RestMethod -Method Post -Uri "$Lumina/api/alerts" -ContentType 'application/json; charset=utf-8' `
    -Body ([Text.Encoding]::UTF8.GetBytes(($body | ConvertTo-Json)))
  Write-Host "alert $($res.id) shown until $($res.expiresAt)"
} catch {
  # PowerShell 7 puts the response body into ErrorDetails, Windows PowerShell does not
  $detail = if ($_.ErrorDetails.Message) { $_.ErrorDetails.Message } else { $_.Exception.Message }
  try { $detail = ($detail | ConvertFrom-Json).error } catch { }
  throw "Lumina rejected the alert: $detail"
}

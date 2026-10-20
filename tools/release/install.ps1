# Installs or updates Aego on Windows: downloads the installer of the latest release, checks it and runs it silently.
# Usage: irm https://aego.akarpov.ru/install.ps1 | iex
# Nightly: $env:AEGO_CHANNEL = "nightly"; irm https://aego.akarpov.ru/install.ps1 | iex
$ErrorActionPreference = "Stop"
$api = if ($env:AEGO_API) { $env:AEGO_API } else { "https://git.akarpov.ru/api/v1/repos/sanspie/aego" }
$which = switch ($env:AEGO_CHANNEL) {
    "nightly" { "tags/nightly" }
    { $_ -like "v*" } { "tags/$($env:AEGO_CHANNEL)" }
    default { "latest" }
}
$release = Invoke-RestMethod "$api/releases/$which"
$setup = $release.assets | Where-Object { $_.name -like "*-windows-amd64-setup.exe" } | Select-Object -First 1
if (-not $setup) { throw "release $which has no Windows installer" }
$sum = $release.assets | Where-Object { $_.name -eq "$($setup.name).sha256" } | Select-Object -First 1
$file = Join-Path $env:TEMP $setup.name
Write-Host "Downloading $($setup.name)"
Invoke-WebRequest $setup.browser_download_url -OutFile $file -UseBasicParsing
if ($sum) {
    $want = ((Invoke-WebRequest $sum.browser_download_url -UseBasicParsing).Content -split '\s+')[0]
    $got = (Get-FileHash $file -Algorithm SHA256).Hash
    if ($want -ne $got) { Remove-Item $file; throw "checksum mismatch, refusing to install" }
}
Write-Host "Installing"
Start-Process $file -ArgumentList "/S" -Wait
Remove-Item $file
Write-Host "Aego is installed: open it from the Start menu, or run 'aego' in a new terminal."

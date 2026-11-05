# Installs or updates Aego on Windows with a checksum-verified silent installer; defaults to latest, while AEGO_CHANNEL selects nightly or a v-prefixed version.
$ErrorActionPreference = "Stop"
$api = if ($env:AEGO_API) { $env:AEGO_API } else { "https://git.akarpov.ru/api/v1/repos/sanspie/aego" }
$which = switch ($env:AEGO_CHANNEL) {
    "nightly" { "tags/nightly" }
    { $_ -like "v*" } { "tags/$($env:AEGO_CHANNEL)" }
    default { "latest" }
}
try { $release = Invoke-RestMethod "$api/releases/$which" } catch { $release = $null }
if (-not $release -and $which -eq "latest") {
    Write-Host "No stable release yet; installing the nightly build."
    $which = "tags/nightly"
    try { $release = Invoke-RestMethod "$api/releases/$which" } catch { $release = $null }
}
if (-not $release) { throw "No Aego release is published yet; build it from source: https://aego.akarpov.ru/install.html#build-from-source" }
$setup = $release.assets | Where-Object { $_.name -like "*-windows-amd64-setup.zip" } | Select-Object -First 1
if (-not $setup) { throw "release $which has no Windows installer" }
$sums = $release.assets | Where-Object { $_.name -eq "SHA256SUMS.txt" } | Select-Object -First 1
if (-not $sums) { throw "release $which has no SHA256SUMS.txt" }
$dir = Join-Path $env:TEMP ("aego-setup-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $dir | Out-Null
$zip = Join-Path $dir $setup.name
$line = ((Invoke-WebRequest ($sums.browser_download_url + "?t=" + [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()) -UseBasicParsing).Content -split "`n") | Where-Object { $_ -match [regex]::Escape($setup.name) + '\s*$' } | Select-Object -First 1
if (-not $line) { throw "SHA256SUMS.txt does not list $($setup.name)" }
$want = ($line -split '\s+')[0]
Write-Host "Downloading $($setup.name)"
Invoke-WebRequest ($setup.browser_download_url + "?v=" + $want) -OutFile $zip -UseBasicParsing
$got = (Get-FileHash $zip -Algorithm SHA256).Hash
if ($want -ne $got) { Remove-Item -Recurse $dir; throw "checksum mismatch, refusing to install" }
Expand-Archive $zip -DestinationPath $dir
$exe = Get-ChildItem $dir -Filter "*-setup.exe" | Select-Object -First 1
Write-Host "Installing"
Start-Process $exe.FullName -ArgumentList "/S" -Wait
Remove-Item -Recurse $dir
Write-Host "Aego is installed: open it from the Start menu, or run 'aego' in a new terminal."

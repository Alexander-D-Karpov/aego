$ErrorActionPreference = 'Stop'

# Exercise the real bootstrap script without network, registry or a real EXE.
$fixture = Join-Path ([System.IO.Path]::GetTempPath()) ('aego-install-test-' + [guid]::NewGuid())
[System.IO.Directory]::CreateDirectory($fixture) | Out-Null
$oldTemp = $env:TEMP
$oldApi = $env:AEGO_API
$digest = 'a' * 64
$global:AegoTestInstallerExit = 0
$global:AegoTestInstallerCalls = 0

function Invoke-RestMethod([string]$Uri) {
    if ($Uri -notlike 'https://fixture.invalid/*') { throw "Unexpected release API: $Uri" }
    return [pscustomobject]@{ assets = @(
        [pscustomobject]@{ name = 'aego-test-windows-amd64-setup.zip'; browser_download_url = 'https://fixture.invalid/aego-test-windows-amd64-setup.zip' },
        [pscustomobject]@{ name = 'SHA256SUMS.txt'; browser_download_url = 'https://fixture.invalid/SHA256SUMS.txt' }
    ) }
}
function Invoke-WebRequest([string]$Uri, [string]$OutFile, [switch]$UseBasicParsing) {
    if ($Uri -like 'https://fixture.invalid/SHA256SUMS.txt*') {
        return [pscustomobject]@{ Content = "$digest  aego-test-windows-amd64-setup.zip`n" }
    }
    if ($Uri -notlike 'https://fixture.invalid/aego-test-windows-amd64-setup.zip*' -or -not $OutFile) {
        throw "Unexpected download: $Uri"
    }
    [System.IO.File]::WriteAllText($OutFile, 'synthetic archive')
}
function Get-FileHash([string]$Path, [string]$Algorithm) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf) -or $Algorithm -ne 'SHA256') { throw 'Unexpected hash request' }
    return [pscustomobject]@{ Hash = $digest }
}
function Expand-Archive([string]$Path, [string]$DestinationPath) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw 'Synthetic archive missing' }
    [System.IO.File]::WriteAllText((Join-Path $DestinationPath 'aego-test-setup.exe'), 'synthetic executable')
}
function Start-Process([string]$FilePath, [string]$ArgumentList, [switch]$Wait, [switch]$PassThru) {
    if (-not (Test-Path -LiteralPath $FilePath -PathType Leaf) -or $ArgumentList -ne '/S' -or -not $Wait) {
        throw 'Unexpected installer invocation'
    }
    $global:AegoTestInstallerCalls++
    if ($PassThru -and $null -ne $global:AegoTestInstallerExit) {
        return [pscustomobject]@{ ExitCode = $global:AegoTestInstallerExit }
    }
}

try {
    $env:TEMP = $fixture
    $env:AEGO_API = 'https://fixture.invalid'
    foreach ($code in @(0, 17, $null)) {
        $global:AegoTestInstallerExit = $code
        $caught = $null
        try { & (Join-Path $PSScriptRoot '../install.ps1') 6>&1 | Out-Null } catch { $caught = $_ }
        if ($null -ne $code -and $code -eq 0 -and $null -ne $caught) { throw "Successful installer was rejected: $caught" }
        if (($null -eq $code -or $code -ne 0) -and $null -eq $caught) { throw 'A failed installer was reported as successful.' }
        if ($null -ne $code -and $code -ne 0 -and $caught.ToString() -notmatch '17') { throw "Installer failure lost its exit code: $caught" }
        if ($null -eq $code -and $caught.ToString() -notmatch 'process status') { throw "Missing process status was not reported: $caught" }
    }
    if ($global:AegoTestInstallerCalls -ne 3) { throw 'Unexpected number of installer calls.' }
    Write-Host 'Windows bootstrap exit-status tests passed (synthetic installer).'
} finally {
    $env:TEMP = $oldTemp
    $env:AEGO_API = $oldApi
    Remove-Variable -Scope Global -Name AegoTestInstallerExit, AegoTestInstallerCalls -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $fixture) { Remove-Item -LiteralPath $fixture -Recurse -Force }
}

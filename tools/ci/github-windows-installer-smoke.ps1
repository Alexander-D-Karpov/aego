$ErrorActionPreference = 'Stop'

# The installer kills processes by executable name and changes HKCU. Never run
# it on a developer machine or a self-hosted runner.
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:GITHUB_SERVER_URL -ne 'https://github.com') {
    throw 'Installer smoke is restricted to an ephemeral GitHub-hosted runner.'
}
if (Get-Process aego-editor -ErrorAction SilentlyContinue) { throw 'An editor is already running.' }
$install = Join-Path $env:RUNNER_TEMP 'aego-ci-install'
if ($install.Contains(' ')) { throw 'NSIS /D requires an unquoted install path; runner temp contains spaces.' }
$registry = @(
    'HKCU:\Software\Aego',
    'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Aego',
    'HKCU:\Software\Classes\Aego.Project',
    'HKCU:\Software\Classes\Aego.Scene',
    'HKCU:\Software\Classes\Aego.Prefab',
    'HKCU:\Software\Classes\Directory\shell\Aego',
    'HKCU:\Software\Classes\Directory\Background\shell\Aego',
    'HKCU:\Software\Classes\.aego',
    'HKCU:\Software\Classes\.aescene',
    'HKCU:\Software\Classes\.aeprefab'
)
foreach ($key in $registry) { if (Test-Path $key) { throw "Refusing to overwrite existing key: $key" } }
$startMenu = Join-Path ([Environment]::GetFolderPath('Programs')) 'Aego'
$desktop = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Aego.lnk'
if ((Test-Path $install) -or (Test-Path $startMenu) -or (Test-Path $desktop)) {
    throw 'Install directory or shortcuts already exist.'
}
. (Join-Path $PSScriptRoot '../release/windows/user-path.ps1')
function Get-UserPathState {
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment')
    if ($null -eq $key) {
        return [pscustomobject]@{ Present = $false; Value = ''; Kind = [Microsoft.Win32.RegistryValueKind]::ExpandString }
    }
    try { return Read-AegoUserPath $key } finally { $key.Dispose() }
}
function Describe-UserPathState($State) {
    # Log structural metadata only: never disclose the user's PATH entries.
    return "present=$($State.Present), kind=$($State.Kind), length=$($State.Value.Length), segments=$(@($State.Value.Split([char]';')).Count)"
}
function Assert-SameUserPath($Before, $After, [string]$Message) {
    if ($Before.Present -ne $After.Present -or ($Before.Present -and ($Before.Kind -ne $After.Kind -or $Before.Value -cne $After.Value))) {
        throw "${Message}: before ($(Describe-UserPathState $Before)), after ($(Describe-UserPathState $After))"
    }
}
$beforeRawPath = Get-UserPathState
$beforePath = [Environment]::GetEnvironmentVariable('Path', 'User')
$workspace = $env:GITHUB_WORKSPACE
$portableZip = Join-Path $workspace '.cache\github-ci\dist\portable\aego-windows-amd64-portable.zip'
$portableDir = Join-Path $env:RUNNER_TEMP 'aego-ci-portable'
if (Test-Path $portableDir) { throw 'Portable smoke directory already exists.' }
Expand-Archive -Path $portableZip -DestinationPath $portableDir
foreach ($file in @('aego.exe', 'aego-editor.exe', 'VERSION', 'README.txt')) {
    if (-not (Test-Path (Join-Path $portableDir $file) -PathType Leaf)) { throw "Missing portable $file" }
}
$previousProcessPath = $env:Path
try {
    $env:Path = "$env:SystemRoot\System32;$env:SystemRoot"
    & (Join-Path $portableDir 'aego.exe') version
    if ($LASTEXITCODE -ne 0) { throw 'Portable CLI did not start with system-only PATH.' }
} finally { $env:Path = $previousProcessPath }
$setup = Join-Path $workspace '.cache\github-ci\dist\installer\aego-windows-amd64-setup.exe'
$proc = Start-Process -FilePath $setup -ArgumentList @('/S', "/D=$install") -Wait -PassThru
if ($proc.ExitCode -ne 0) { throw "Installer returned $($proc.ExitCode)" }
foreach ($file in @('aego.exe', 'aego-editor.exe', 'VERSION', 'README.txt', 'user-path.ps1', 'setup.exe', 'uninstall.exe')) {
    if (-not (Test-Path (Join-Path $install $file) -PathType Leaf)) { throw "Missing installed $file" }
}
foreach ($shortcut in @((Join-Path $startMenu 'Aego.lnk'), (Join-Path $startMenu 'Uninstall Aego.lnk'), $desktop)) {
    if (-not (Test-Path $shortcut -PathType Leaf)) { throw "Missing shortcut: $shortcut" }
}
$shell = New-Object -ComObject WScript.Shell
if ($shell.CreateShortcut((Join-Path $startMenu 'Aego.lnk')).TargetPath -ne (Join-Path $install 'aego-editor.exe') -or
    $shell.CreateShortcut((Join-Path $startMenu 'Uninstall Aego.lnk')).TargetPath -ne (Join-Path $install 'uninstall.exe') -or
    $shell.CreateShortcut($desktop).TargetPath -ne (Join-Path $install 'aego-editor.exe')) {
    throw 'A shortcut targets the wrong executable.'
}
foreach ($association in @(@('.aego', 'Aego.Project'), @('.aescene', 'Aego.Scene'), @('.aeprefab', 'Aego.Prefab'))) {
    $actual = (Get-Item ("HKCU:\Software\Classes\" + $association[0])).GetValue('')
    if ($actual -ne $association[1]) { throw "Incorrect file association for $($association[0])" }
}
if (([Environment]::GetEnvironmentVariable('Path', 'User') -split ';') -notcontains $install) {
    throw 'Installer did not add install directory to user PATH.'
}
$installedRawPath = Get-UserPathState
$expectedPath = Add-AegoPath $beforeRawPath.Value $install
if (-not $installedRawPath.Present -or $installedRawPath.Value -cne $expectedPath.Value -or $installedRawPath.Kind -ne $beforeRawPath.Kind) {
    throw "Installer changed unexpected PATH data: before ($(Describe-UserPathState $beforeRawPath)), after ($(Describe-UserPathState $installedRawPath))"
}
# Updating the same installation must not duplicate its entry or lose ownership.
$proc = Start-Process -FilePath $setup -ArgumentList @('/S', "/D=$install") -Wait -PassThru
if ($proc.ExitCode -ne 0) { throw "Installer update returned $($proc.ExitCode)" }
Assert-SameUserPath $installedRawPath (Get-UserPathState) 'Update changed user PATH'
$previousProcessPath = $env:Path
try {
    $env:Path = "$env:SystemRoot\System32;$env:SystemRoot"
    & (Join-Path $install 'aego.exe') version
    if ($LASTEXITCODE -ne 0) { throw 'Installed CLI did not start with system-only PATH.' }
} finally { $env:Path = $previousProcessPath }
$installedCLI = Join-Path $install 'aego.exe'
# This diagnostic runs even after an unrelated test failure, in which case the
# earlier CLI-smoke step is skipped. Create a fresh project using the installed
# binary instead of depending on that step's workspace or overwriting a project.
$installedProject = Join-Path $env:RUNNER_TEMP 'aego-ci-installed-project'
if (Test-Path $installedProject) { throw 'Installed CLI project directory already exists.' }
& $installedCLI new $installedProject
if ($LASTEXITCODE -ne 0) { throw 'Installed CLI could not create a project.' }
& $installedCLI import $installedProject
if ($LASTEXITCODE -ne 0) { throw 'Installed CLI could not import the project.' }
& $installedCLI check $installedProject
if ($LASTEXITCODE -ne 0) { throw 'Installed CLI could not check the project.' }
& $installedCLI run $installedProject --headless --frames 60
if ($LASTEXITCODE -ne 0) { throw 'Installed headless runtime failed.' }
$userData = Join-Path $install 'user-data'
New-Item -ItemType Directory -Path $userData | Out-Null
Set-Content -Path (Join-Path $userData 'keep.txt') -Value 'keep user data'
$proc = Start-Process -FilePath (Join-Path $install 'uninstall.exe') -ArgumentList '/S' -Wait -PassThru
if ($proc.ExitCode -ne 0) { throw "Uninstaller returned $($proc.ExitCode)" }
for ($i = 0; $i -lt 30 -and (Test-Path (Join-Path $install 'uninstall.exe')); $i++) { Start-Sleep -Seconds 1 }
foreach ($file in @('aego.exe', 'aego-editor.exe', 'VERSION', 'README.txt', 'user-path.ps1', 'setup.exe', 'uninstall.exe')) {
    if (Test-Path (Join-Path $install $file)) { throw "Uninstaller left $file" }
}
if (-not (Test-Path (Join-Path $userData 'keep.txt'))) { throw 'Uninstaller removed user data.' }
foreach ($shortcut in @($startMenu, $desktop)) { if (Test-Path $shortcut) { throw "Uninstaller left $shortcut" } }
foreach ($key in $registry) { if (Test-Path $key) { throw "Uninstaller left $key" } }
Assert-SameUserPath $beforeRawPath (Get-UserPathState) 'User PATH registry state was not restored'
if ([Environment]::GetEnvironmentVariable('Path', 'User') -cne $beforePath) { throw 'User PATH environment value was not restored.' }
Write-Host 'Silent install/uninstall smoke passed. No GUI interaction or signature validation was performed.'

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot/path.ps1"

function Assert-Equal($actual, $expected, [string]$what) {
    if ($actual -cne $expected) { throw "$what`: expected <$expected>, got <$actual>" }
}

$dir = "C:\Program Files\Aego's Editor"
foreach ($before in @('', 'C:\A', 'C:\A;', ';C:\A', 'C:\A;;C:\B;;', '%LOCALAPPDATA%\Tools;')) {
    $install = Plan-AegoPathInstall $before $dir
    if (-not $install.Changed) { throw 'Install did not add a new directory.' }
    $removed = Plan-AegoPathRemove $install.Value $dir $before $true $install.Value
    if (-not $removed.Changed -or -not $removed.Present -or -not $removed.RestoreOriginal) { throw 'Original PATH was not restored.' }
    Assert-Equal $removed.Value $before 'PATH round trip'

    $external = $install.Value + ';C:\Later;;'
    $removed = Plan-AegoPathRemove $external $dir $before $true $install.Value
    $expected = if ($before -eq '') { 'C:\Later;;' } else { $before + ';C:\Later;;' }
    Assert-Equal $removed.Value $expected 'Later PATH edits'
}

$install = Plan-AegoPathInstall $null $dir
$removed = Plan-AegoPathRemove $install.Value $dir '' $false $install.Value
if (-not $removed.Changed -or $removed.Present) { throw 'An originally absent PATH should be absent again.' }
$preexisting = Plan-AegoPathInstall "C:\Tools;$($dir.ToUpperInvariant());" $dir
if ($preexisting.Changed) { throw 'A pre-existing user PATH entry must not be claimed.' }
$unchanged = Plan-AegoPathRemove 'C:\Tools;;' $dir 'C:\Tools' $true "C:\Tools;$dir"
if ($unchanged.Changed) { throw 'A user-removed entry must not change PATH on uninstall.' }
$ambiguous = $false
try { Plan-AegoPathRemove "$dir;$dir;C:\Other" $dir '' $false $dir | Out-Null } catch { $ambiguous = $true }
if (-not $ambiguous) { throw 'Ambiguous duplicate entries must not be removed.' }
Write-Host 'PATH planner tests passed.'

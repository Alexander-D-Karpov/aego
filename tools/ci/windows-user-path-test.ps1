$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../release/windows/user-path.ps1')

function Assert-Equal($Actual, $Expected, [string]$Message) {
    if ($Actual -cne $Expected) {
        throw "${Message}: expected [$Expected], got [$Actual]"
    }
}

$dir = "C:\Users\O'Brien\Aego"
foreach ($before in @('', 'C:\Tools', 'C:\Tools;', 'C:\Tools;;', ';;', '%USERPROFILE%\tools;C:\Bin', ';C:\Bin;;C:\Other;')) {
    $installed = Add-AegoPath $before $dir
    if (-not $installed.Added) { throw "Expected a new entry for [$before]" }
    $removed = Remove-AegoPath $installed.Value $dir
    if (-not $removed.Removed) { throw "Failed to remove owned entry for [$before]" }
    Assert-Equal $removed.Value $before 'Install/uninstall must preserve the original string'
}
$random = [Random]::new(125)
$segments = @('', 'C:\Tools', '%USERPROFILE%\bin', 'C:\Has Space', 'C:\Other')
for ($trial = 0; $trial -lt 500; $trial++) {
    $parts = [System.Collections.Generic.List[string]]::new()
    $count = $random.Next(1, 12)
    for ($i = 0; $i -lt $count; $i++) { $parts.Add($segments[$random.Next($segments.Count)]) }
    $before = [string]::Join(';', $parts)
    $added = Add-AegoPath $before $dir
    $removed = Remove-AegoPath $added.Value $dir
    if (-not $added.Added -or -not $removed.Removed -or $removed.Value -cne $before) {
        throw "Randomized PATH round trip failed on trial $trial"
    }
}

$before = 'C:\Tools;C:\USERS\O''BRIEN\AEGO;;C:\Other;'
$unchanged = Add-AegoPath $before $dir
if ($unchanged.Added) { throw 'Existing entries must not become installer-owned.' }
Assert-Equal $unchanged.Value $before 'Existing entry changed'

$installed = Add-AegoPath 'C:\Tools;' $dir
$edited = $installed.Value + ';C:\New;;'
$removed = Remove-AegoPath $edited $dir
Assert-Equal $removed.Value 'C:\Tools;;C:\New;;' 'Uninstall discarded a later user edit'

$duplicate = Remove-AegoPath ($installed.Value + ';' + $dir) $dir
Assert-Equal $duplicate.Value ('C:\Tools;;' + $dir) 'Uninstall removed a later duplicate'

$absent = Remove-AegoPath 'C:\Tools;;' $dir
if ($absent.Removed) { throw 'Absent entry was reported as removed.' }
Assert-Equal $absent.Value 'C:\Tools;;' 'Absent entry changed PATH'

$refused = $false
try { Add-AegoPath 'C:\Tools' 'C:\bad;segment' | Out-Null } catch { $refused = $true }
if (-not $refused) { throw 'A directory containing a semicolon must be rejected.' }

# Exercise the actual registry algorithm with isolated in-memory keys. The native
# installer smoke additionally checks real HKCU on a GitHub-hosted Windows VM.
class FakeKey {
    [hashtable]$Values = @{}
    [hashtable]$Kinds = @{}
    [string]$FailSetName = ''
    [object]$FailSetValue = $null
    [string[]] GetValueNames() { return [string[]]@($this.Values.Keys) }
    [object] GetValue([string]$name, [object]$fallback) {
        if ($this.Values.ContainsKey($name)) { return $this.Values[$name] }
        return $fallback
    }
    [object] GetValue([string]$name, [object]$fallback, [object]$options) { return $this.GetValue($name, $fallback) }
    [Microsoft.Win32.RegistryValueKind] GetValueKind([string]$name) { return $this.Kinds[$name] }
    [void] SetValue([string]$name, [object]$value, [Microsoft.Win32.RegistryValueKind]$kind) {
        if ($name -eq $this.FailSetName -and $value -eq $this.FailSetValue) { throw 'Simulated registry write failure.' }
        $this.Values[$name] = $value
        $this.Kinds[$name] = $kind
    }
    [void] DeleteValue([string]$name, [bool]$throwOnMissingValue) {
        $this.Values.Remove($name)
        $this.Kinds.Remove($name)
    }
}

foreach ($scenario in @('absent', 'empty', 'trailing')) {
    $envKey = [FakeKey]::new()
    $state = [FakeKey]::new()
    $original = ''
    if ($scenario -eq 'empty') { $envKey.SetValue('Path', '', [Microsoft.Win32.RegistryValueKind]::String) }
    if ($scenario -eq 'trailing') {
        $original = '%USERPROFILE%\tools;;C:\Other;'
        $envKey.SetValue('Path', $original, [Microsoft.Win32.RegistryValueKind]::ExpandString)
    }
    Invoke-AegoPathUpdate Install $dir $envKey $state
    Invoke-AegoPathUpdate Install $dir $envKey $state # update must keep ownership
    Invoke-AegoPathUpdate Uninstall $dir $envKey $state
    $after = Read-AegoUserPath $envKey
    Assert-Equal $after.Value $original "$scenario registry round trip"
    if ($after.Present -ne ($scenario -ne 'absent')) { throw "$scenario changed Path value presence" }
    if ($scenario -eq 'empty' -and $after.Kind -ne [Microsoft.Win32.RegistryValueKind]::String) { throw 'Empty PATH type changed.' }
}

$envKey = [FakeKey]::new()
$state = [FakeKey]::new()
$original = 'C:\Tools;' + $dir + ';;'
$envKey.SetValue('Path', $original, [Microsoft.Win32.RegistryValueKind]::ExpandString)
Invoke-AegoPathUpdate Install $dir $envKey $state
Invoke-AegoPathUpdate Uninstall $dir $envKey $state
Assert-Equal (Read-AegoUserPath $envKey).Value $original 'Pre-existing user entry was removed'

$envKey = [FakeKey]::new()
$state = [FakeKey]::new()
$envKey.SetValue('Path', 'C:\Tools;', [Microsoft.Win32.RegistryValueKind]::String)
Invoke-AegoPathUpdate Install $dir $envKey $state
$edited = (Read-AegoUserPath $envKey).Value + ';C:\UserAdded;'
$envKey.SetValue('Path', $edited, [Microsoft.Win32.RegistryValueKind]::String)
Invoke-AegoPathUpdate Uninstall $dir $envKey $state
Assert-Equal (Read-AegoUserPath $envKey).Value 'C:\Tools;;C:\UserAdded;' 'Later user PATH changes were lost'

$envKey = [FakeKey]::new()
$state = [FakeKey]::new()
Invoke-AegoPathUpdate Install $dir $envKey $state
$newDir = 'C:\Another\Aego'
Invoke-AegoPathUpdate Install $newDir $envKey $state
Assert-Equal (Read-AegoUserPath $envKey).Value $newDir 'Moving the install left its old PATH entry'
Invoke-AegoPathUpdate Uninstall $newDir $envKey $state
if ((Read-AegoUserPath $envKey).Present) { throw 'Moved install did not restore absent PATH.' }

$envKey = [FakeKey]::new()
$state = [FakeKey]::new()
Invoke-AegoPathUpdate Install $dir $envKey $state
$envKey.SetValue('Path', ($dir + ';'), [Microsoft.Win32.RegistryValueKind]::String)
Invoke-AegoPathUpdate Uninstall $dir $envKey $state
$after = Read-AegoUserPath $envKey
if (-not $after.Present -or $after.Value -cne '' -or $after.Kind -ne [Microsoft.Win32.RegistryValueKind]::String) {
    throw 'A later empty PATH entry or registry type was lost.'
}

$envKey = [FakeKey]::new()
$state = [FakeKey]::new()
$envKey.SetValue('Path', '', [Microsoft.Win32.RegistryValueKind]::String)
Invoke-AegoPathUpdate Install $dir $envKey $state
$envKey.DeleteValue('Path', $false) # user removes the owned entry and the value
Invoke-AegoPathUpdate Install $dir $envKey $state
Invoke-AegoPathUpdate Uninstall $dir $envKey $state
if ((Read-AegoUserPath $envKey).Present) { throw 'Update ignored the user removing PATH.' }

$envKey = [FakeKey]::new()
$state = [FakeKey]::new()
$envKey.SetValue('Path', '', [Microsoft.Win32.RegistryValueKind]::String)
Invoke-AegoPathUpdate Install $dir $envKey $state
$envKey.SetValue('Path', $dir, [Microsoft.Win32.RegistryValueKind]::ExpandString)
Invoke-AegoPathUpdate Uninstall $dir $envKey $state
$after = Read-AegoUserPath $envKey
if (-not $after.Present -or $after.Kind -ne [Microsoft.Win32.RegistryValueKind]::ExpandString) {
    throw 'A later registry type change was lost.'
}

$envKey = [FakeKey]::new()
$state = [FakeKey]::new()
$envKey.SetValue('Path', 'unsupported', [Microsoft.Win32.RegistryValueKind]::Binary)
$refused = $false
try { Invoke-AegoPathUpdate Install $dir $envKey $state } catch { $refused = $true }
if (-not $refused -or $envKey.GetValue('Path', '') -cne 'unsupported' -or $state.Values.Count -ne 0) {
    throw 'An unsupported PATH type was not refused safely.'
}
Invoke-AegoPathUpdate Uninstall $dir $envKey $state
if ($envKey.GetValue('Path', '') -cne 'unsupported') { throw 'Unowned uninstall changed unsupported PATH data.' }

$envKey = [FakeKey]::new()
$state = [FakeKey]::new()
$envKey.SetValue('Path', 'C:\Tools;', [Microsoft.Win32.RegistryValueKind]::String)
$state.FailSetName = 'PathOwned'
$state.FailSetValue = 1
$refused = $false
try { Invoke-AegoPathUpdate Install $dir $envKey $state } catch { $refused = $true }
$after = Read-AegoUserPath $envKey
if (-not $refused -or $after.Value -cne 'C:\Tools;' -or $after.Kind -ne [Microsoft.Win32.RegistryValueKind]::String -or $state.GetValue('PathOwned', 0) -ne 0) {
    throw 'A failed ownership write did not roll back the PATH edit.'
}

$envKey = [FakeKey]::new()
$state = [FakeKey]::new()
$envKey.SetValue('Path', 'C:\Tools;', [Microsoft.Win32.RegistryValueKind]::String)
$envKey.FailSetName = 'Path'
$envKey.FailSetValue = 'C:\Tools;;' + $dir
$refused = $false
try { Invoke-AegoPathUpdate Install $dir $envKey $state } catch { $refused = $true }
if (-not $refused -or (Read-AegoUserPath $envKey).Value -cne 'C:\Tools;' -or $state.GetValue('PathOwned', 0) -ne 0) {
    throw 'A failed PATH write changed the existing value or claimed ownership.'
}

$envKey = [FakeKey]::new()
$state = [FakeKey]::new()
$envKey.SetValue('Path', 'C:\Tools;;', [Microsoft.Win32.RegistryValueKind]::String)
$state.SetValue('PathOwned', 1, [Microsoft.Win32.RegistryValueKind]::DWord)
$state.SetValue('PathDir', '', [Microsoft.Win32.RegistryValueKind]::String)
$refused = $false
try { Invoke-AegoPathUpdate Uninstall $dir $envKey $state } catch { $refused = $true }
if (-not $refused -or (Read-AegoUserPath $envKey).Value -cne 'C:\Tools;;') {
    throw 'Corrupt ownership metadata was allowed to alter PATH.'
}

$envKey = [FakeKey]::new()
$state = [FakeKey]::new()
Invoke-AegoPathUpdate Install $dir $envKey $state
$refused = $false
try { Invoke-AegoPathUpdate Uninstall 'C:\Another\Aego' $envKey $state } catch { $refused = $true }
if (-not $refused -or (Read-AegoUserPath $envKey).Value -cne $dir -or $state.GetValue('PathOwned', 0) -ne 1) {
    throw 'Uninstalling a different directory changed an owned entry.'
}

Write-Host 'Windows user PATH string and registry-state tests passed.'

# On Windows, exercise the same algorithm with real registry keys under a fresh
# random HKCU subkey. These keys are not the actual user's environment or Aego
# installation; only the exact test key created here is removed afterwards.
if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    $testPath = 'Software\AegoCiPathTest-' + [guid]::NewGuid().ToString('N')
    $existing = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($testPath)
    if ($null -ne $existing) {
        $existing.Dispose()
        throw 'Refusing to reuse an existing registry test key.'
    }
    $testRoot = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($testPath)
    try {
        foreach ($scenario in @('absent', 'empty', 'trailing', 'preexisting', 'edited')) {
            $scenarioKey = $testRoot.CreateSubKey($scenario)
            $envKey = $scenarioKey.CreateSubKey('Environment')
            $state = $scenarioKey.CreateSubKey('State')
            try {
                $original = ''
                $originalKind = [Microsoft.Win32.RegistryValueKind]::ExpandString
                if ($scenario -eq 'empty') {
                    $originalKind = [Microsoft.Win32.RegistryValueKind]::String
                    $envKey.SetValue('Path', '', $originalKind)
                } elseif ($scenario -eq 'trailing' -or $scenario -eq 'edited') {
                    $original = '%USERPROFILE%\tools;;C:\Other;'
                    $envKey.SetValue('Path', $original, $originalKind)
                } elseif ($scenario -eq 'preexisting') {
                    $original = 'C:\Tools;' + $dir + ';;'
                    $envKey.SetValue('Path', $original, $originalKind)
                }
                Invoke-AegoPathUpdate Install $dir $envKey $state
                Invoke-AegoPathUpdate Install $dir $envKey $state
                if ($scenario -eq 'edited') {
                    $envKey.SetValue('Path', ((Read-AegoUserPath $envKey).Value + ';C:\Added;'), $originalKind)
                    $original += ';C:\Added;'
                }
                Invoke-AegoPathUpdate Uninstall $dir $envKey $state
                $after = Read-AegoUserPath $envKey
                if ($after.Present -ne ($scenario -ne 'absent') -or $after.Value -cne $original -or ($after.Present -and $after.Kind -ne $originalKind)) {
                    throw "Real registry PATH round trip failed: $scenario"
                }
            } finally {
                $state.Dispose()
                $envKey.Dispose()
                $scenarioKey.Dispose()
            }
        }
    } finally {
        $testRoot.Dispose()
        [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree($testPath)
    }
    Write-Host 'Isolated real HKCU PATH tests passed.'
}

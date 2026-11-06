param(
    [ValidateSet('Install', 'Uninstall')]
    [string]$Action,
    [string]$InstallDir
)

$ErrorActionPreference = 'Stop'

# Preserve empty entries and delimiters; do not normalize the user's PATH.
function Add-AegoPath([AllowEmptyString()][string]$Current, [string]$Directory) {
    if ([string]::IsNullOrEmpty($Directory) -or $Directory.Contains(';')) {
        throw 'The installation directory cannot be empty or contain a semicolon.'
    }
    foreach ($part in $Current.Split([char]';')) {
        if ([string]::Equals($part, $Directory, [StringComparison]::OrdinalIgnoreCase)) {
            return [pscustomobject]@{ Value = $Current; Added = $false }
        }
    }
    $updated = if ($Current.Length -eq 0) { $Directory } else { $Current + ';' + $Directory }
    return [pscustomobject]@{ Value = $updated; Added = $true }
}

# Remove only one owned entry. In particular, a later duplicate is user data.
function Remove-AegoPath([AllowEmptyString()][string]$Current, [string]$Directory) {
    $parts = @($Current.Split([char]';'))
    $index = -1
    for ($i = 0; $i -lt $parts.Count; $i++) {
        if ([string]::Equals($parts[$i], $Directory, [StringComparison]::OrdinalIgnoreCase)) {
            $index = $i
            break
        }
    }
    if ($index -lt 0) { return [pscustomobject]@{ Value = $Current; Removed = $false; RemainingEntries = $parts.Count } }
    $rest = [System.Collections.Generic.List[string]]::new()
    for ($i = 0; $i -lt $parts.Count; $i++) {
        if ($i -ne $index) { $rest.Add($parts[$i]) }
    }
    return [pscustomobject]@{ Value = [string]::Join(';', $rest); Removed = $true; RemainingEntries = $rest.Count }
}

function Read-AegoUserPath($Key) {
    $present = @($Key.GetValueNames()) -icontains 'Path'
    if (-not $present) {
        return [pscustomobject]@{ Present = $false; Value = ''; Kind = [Microsoft.Win32.RegistryValueKind]::ExpandString }
    }
    $kind = $Key.GetValueKind('Path')
    if ($kind -ne [Microsoft.Win32.RegistryValueKind]::String -and $kind -ne [Microsoft.Win32.RegistryValueKind]::ExpandString) {
        throw 'The user PATH has an unsupported registry type; it was not changed.'
    }
    $value = $Key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
    return [pscustomobject]@{ Present = $true; Value = [string]$value; Kind = $kind }
}

function Set-AegoUserPath($Key, [string]$Value, $Kind, [bool]$Present) {
    if ($Present) { $Key.SetValue('Path', $Value, $Kind) }
    else { $Key.DeleteValue('Path', $false) }
}

function Invoke-AegoPathUpdate([ValidateSet('Install', 'Uninstall')][string]$Mode, [string]$Directory, $environment, $state) {
    if ([string]::IsNullOrEmpty($Directory) -or $Directory.Contains(';')) {
        throw 'The installation directory cannot be empty or contain a semicolon.'
    }
    $owned = $state.GetValue('PathOwned', 0) -eq 1
    # Old installations and pre-existing PATH entries are unowned. Uninstall
    # has nothing to change, even if PATH has an unfamiliar registry type.
    if ($Mode -eq 'Uninstall' -and -not $owned) { return }
    $current = Read-AegoUserPath $environment
    $ownedDir = [string]$state.GetValue('PathDir', '')
    $wasPresent = $state.GetValue('PathWasPresent', 0) -eq 1
    if ($owned -and ([string]::IsNullOrEmpty($ownedDir) -or $ownedDir.Contains(';'))) {
        throw 'The owned PATH directory is invalid; the user PATH was not changed.'
    }

    if ($owned -and $Mode -eq 'Uninstall' -and -not [string]::Equals($ownedDir, $Directory, [StringComparison]::OrdinalIgnoreCase)) {
        throw 'The owned PATH entry belongs to a different installation; it was not changed.'
    }

    # A moved installation first releases the old entry it actually owned.
    if ($owned -and ($Mode -eq 'Uninstall' -or -not [string]::Equals($ownedDir, $Directory, [StringComparison]::OrdinalIgnoreCase))) {
        $removed = Remove-AegoPath $current.Value $ownedDir
        if ($removed.Removed) {
            # An empty entry added by the user still makes PATH present, even
            # when its serialized value is empty after removing our entry.
            $present = $removed.RemainingEntries -gt 0 -or $wasPresent
            Set-AegoUserPath $environment $removed.Value $current.Kind $present
        }
        $state.SetValue('PathOwned', 0, [Microsoft.Win32.RegistryValueKind]::DWord)
        $owned = $false
        $current = Read-AegoUserPath $environment
    }
    if ($Mode -eq 'Install') {
        $added = Add-AegoPath $current.Value $Directory
        if ($added.Added) {
            # If an update must add the entry again, the user removed it in the
            # meantime. Capture the current state, not the original install's.
            $state.SetValue('PathOwned', 0, [Microsoft.Win32.RegistryValueKind]::DWord)
            $state.SetValue('PathWasPresent', [int]$current.Present, [Microsoft.Win32.RegistryValueKind]::DWord)
            $state.SetValue('PathDir', $Directory, [Microsoft.Win32.RegistryValueKind]::String)
            try {
                Set-AegoUserPath $environment $added.Value $current.Kind $true
                $state.SetValue('PathOwned', 1, [Microsoft.Win32.RegistryValueKind]::DWord)
            } catch {
                $failure = $_
                # Do not overwrite a concurrent user edit if recording ownership
                # fails; otherwise roll back our tentative PATH write.
                try {
                    $state.SetValue('PathOwned', 0, [Microsoft.Win32.RegistryValueKind]::DWord)
                    $latest = Read-AegoUserPath $environment
                    if ($latest.Present -and $latest.Kind -eq $current.Kind -and $latest.Value -ceq $added.Value) {
                        Set-AegoUserPath $environment $current.Value $current.Kind $current.Present
                    } elseif ($latest.Present -ne $current.Present -or $latest.Kind -ne $current.Kind -or $latest.Value -cne $current.Value) {
                        throw 'PATH changed concurrently; the tentative entry was left untouched.'
                    }
                } catch {
                    throw "Could not record PATH ownership or safely roll back: $($_.Exception.Message)"
                }
                throw $failure
            }
        } elseif (-not $owned) {
            # This path predated Aego; uninstall must leave it alone.
            $state.SetValue('PathDir', $Directory, [Microsoft.Win32.RegistryValueKind]::String)
            $state.SetValue('PathOwned', 0, [Microsoft.Win32.RegistryValueKind]::DWord)
        }
    }
}

function Update-AegoUserPath([string]$Mode, [string]$Directory) {
    $environment = $null
    $state = $null
    try {
        $environment = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
        $state = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Software\Aego')
        Invoke-AegoPathUpdate $Mode $Directory $environment $state
    } finally {
        if ($null -ne $environment) { $environment.Dispose() }
        if ($null -ne $state) { $state.Dispose() }
    }
}

# Dot-sourcing exposes pure helpers to tests without touching the registry.
if ($Action) {
    try {
        Update-AegoUserPath $Action $InstallDir
    } catch {
        [Console]::Error.WriteLine($_.Exception.Message)
        exit 1
    }
}

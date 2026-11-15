param(
    [string]$Action = '',
    [string]$Directory = ''
)

$ErrorActionPreference = 'Stop'

# Keep empty PATH segments and their order: they are part of the user's value.
# These planners are independent of the registry so edge cases can be tested on
# every CI platform, while the real registry path is tested on Windows.
function Plan-AegoPathInstall([AllowNull()][string]$Current, [string]$InstallDir) {
    if ($null -ne $Current) {
        foreach ($part in $Current.Split([char]';')) {
            if ($part -ieq $InstallDir) {
                return [pscustomobject]@{ Changed = $false; Value = $Current }
            }
        }
    }
    $value = if ([string]::IsNullOrEmpty($Current)) { $InstallDir } else { $Current + ';' + $InstallDir }
    return [pscustomobject]@{ Changed = $true; Value = $value }
}

function Plan-AegoPathRemove(
    [AllowNull()][string]$Current, [string]$InstallDir, [string]$Before,
    [bool]$BeforePresent, [string]$Installed
) {
    if ($null -eq $Current) {
        return [pscustomobject]@{ Changed = $false; Present = $false; RestoreOriginal = $false; Value = '' }
    }
    if ($Current -ceq $Installed) {
        return [pscustomobject]@{ Changed = $true; Present = $BeforePresent; RestoreOriginal = $true; Value = $Before }
    }
    $parts = $Current.Split([char]';')
    $matches = [System.Collections.Generic.List[int]]::new()
    for ($i = 0; $i -lt $parts.Length; $i++) {
        if ($parts[$i] -ieq $InstallDir) { $matches.Add($i) }
    }
    if ($matches.Count -eq 0) {
        return [pscustomobject]@{ Changed = $false; Present = $true; RestoreOriginal = $false; Value = $Current }
    }
    if ($matches.Count -ne 1) {
        throw 'PATH contains multiple matching Aego entries; refusing to remove an ambiguous user entry.'
    }
    $remaining = [System.Collections.Generic.List[string]]::new()
    for ($i = 0; $i -lt $parts.Length; $i++) {
        if ($i -ne $matches[0]) { $remaining.Add($parts[$i]) }
    }
    $value = [string]::Join(';', $remaining)
    $present = $remaining.Count -gt 0 -or $BeforePresent
    return [pscustomobject]@{ Changed = $true; Present = $present; RestoreOriginal = $false; Value = $value }
}

function Invoke-AegoPath([string]$Operation, [string]$InstallDir) {
    if ($Operation -notin @('Install', 'Uninstall') -or [string]::IsNullOrEmpty($InstallDir)) {
        throw 'Expected Install or Uninstall and a nonempty installation directory.'
    }
    $envKey = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment', $true)
    if ($null -eq $envKey) { throw 'Cannot open the user environment registry key.' }
    $appKey = $null
    try {
        $present = @($envKey.GetValueNames()) -icontains 'Path'
        $current = if ($present) { [string]$envKey.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames) } else { $null }
        $kind = if ($present) { $envKey.GetValueKind('Path') } else { [Microsoft.Win32.RegistryValueKind]::ExpandString }
        if ($Operation -eq 'Install') {
            $appKey = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Software\Aego', $true)
            if ($null -eq $appKey) { throw 'Cannot open the Aego registry key.' }
            $owned = [string]$appKey.GetValue('ManagedPath', '')
            if ($owned -and $owned -ine $InstallDir) {
                throw 'Another Aego installation owns a different PATH entry; uninstall it first.'
            }
            $plan = Plan-AegoPathInstall $current $InstallDir
            if ($plan.Changed) {
                # Record ownership before writing PATH. If writing fails, removal
                # finds no matching entry and leaves the user's PATH alone.
                $appKey.SetValue('PathBefore', [string]$current, [Microsoft.Win32.RegistryValueKind]::String)
                $appKey.SetValue('PathBeforePresent', [int]$present, [Microsoft.Win32.RegistryValueKind]::DWord)
                $appKey.SetValue('PathBeforeKind', [int]$kind, [Microsoft.Win32.RegistryValueKind]::DWord)
                $appKey.SetValue('PathInstalled', $plan.Value, [Microsoft.Win32.RegistryValueKind]::String)
                $appKey.SetValue('ManagedPath', $InstallDir, [Microsoft.Win32.RegistryValueKind]::String)
                $envKey.SetValue('Path', $plan.Value, $kind)
            }
        } else {
            $appKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Software\Aego', $true)
            if ($null -eq $appKey) { return }
            $owned = [string]$appKey.GetValue('ManagedPath', '')
            if (-not $owned -or $owned -ine $InstallDir) { return }
            $before = [string]$appKey.GetValue('PathBefore', '')
            $beforePresent = [bool]$appKey.GetValue('PathBeforePresent', 0)
            $beforeKind = [Microsoft.Win32.RegistryValueKind][int]$appKey.GetValue('PathBeforeKind', [int][Microsoft.Win32.RegistryValueKind]::ExpandString)
            $installed = [string]$appKey.GetValue('PathInstalled', '')
            $plan = Plan-AegoPathRemove $current $InstallDir $before $beforePresent $installed
            if ($plan.Changed) {
                if (-not $plan.Present) {
                    $envKey.DeleteValue('Path', $false)
                } else {
                    $outKind = if ($plan.RestoreOriginal) { $beforeKind } else { $kind }
                    $envKey.SetValue('Path', $plan.Value, $outKind)
                }
            }
            foreach ($name in @('ManagedPath', 'PathBefore', 'PathBeforePresent', 'PathBeforeKind', 'PathInstalled')) {
                $appKey.DeleteValue($name, $false)
            }
        }
    } finally {
        if ($null -ne $appKey) { $appKey.Dispose() }
        $envKey.Dispose()
    }
    # Notify other applications just as a user environment change does.
    if (-not ('AegoEnvironmentChange' -as [type])) {
        Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class AegoEnvironmentChange {
    [DllImport("user32.dll", CharSet = CharSet.Auto, SetLastError = true)]
    public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint msg, UIntPtr wParam, string lParam, uint flags, uint timeout, out UIntPtr result);
}
'@
    }
    $result = [UIntPtr]::Zero
    [AegoEnvironmentChange]::SendMessageTimeout([IntPtr]0xffff, 0x001A, [UIntPtr]::Zero, 'Environment', 2, 5000, [ref]$result) | Out-Null
}

if ($MyInvocation.InvocationName -ne '.') {
    Invoke-AegoPath $Action $Directory
}

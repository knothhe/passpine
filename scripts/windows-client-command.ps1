function Invoke-PasspineClientCommand {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][ValidateSet('--build-channel', '--quit-for-uninstall')][string]$Argument
    )
    # Windows PowerShell does not reliably wait for GUI-subsystem executables
    # invoked with &, or populate $LASTEXITCODE. Own the process explicitly.
    $start = New-Object System.Diagnostics.ProcessStartInfo
    $start.FileName = $FilePath
    $start.Arguments = $Argument
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $process = New-Object System.Diagnostics.Process
    $process.StartInfo = $start
    try {
        if (-not $process.Start()) { throw 'Could not start the Dev client command.' }
        $output = $process.StandardOutput.ReadToEndAsync()
        $errors = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit(30000)) { throw 'The Dev client command did not finish. No files were removed.' }
        return [PSCustomObject]@{
            ExitCode = $process.ExitCode
            Output = $output.GetAwaiter().GetResult().Trim()
            ErrorOutput = $errors.GetAwaiter().GetResult().Trim()
        }
    } finally { $process.Dispose() }
}

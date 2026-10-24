param([string]$InstallDirectory = '')
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$registration = 'HKCU:\Software\Passpine Dev'
if (-not (Test-Path -LiteralPath $registration)) {
    if ($InstallDirectory) {
        foreach ($name in @('passpine.exe', 'passpine-host.exe', 'Uninstall.exe', 'build-channel.ini')) {
            if (Test-Path -LiteralPath (Join-Path $InstallDirectory $name)) { throw 'Installation ownership is unknown. No files were removed.' }
        }
    }
    Write-Output 'Passpine Dev is not installed.'
    exit 0
}
$directory = Get-ItemPropertyValue -LiteralPath $registration -Name 'InstallDir'
if ($InstallDirectory -and [IO.Path]::GetFullPath($InstallDirectory) -ne [IO.Path]::GetFullPath($directory)) { throw 'The requested directory does not match the Dev installation record.' }
$current = Get-Item -LiteralPath $directory
while ($current) {
    if ($current.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Refusing an installation path containing a reparse point.' }
    $current = $current.Parent
}
$marker = Join-Path $directory 'build-channel.ini'
if (-not (Test-Path -LiteralPath $marker) -or (Get-Content -LiteralPath $marker -Raw) -notmatch '(?m)^channel=dev\s*$') { throw 'The installed channel is not Dev.' }
$client = Join-Path $directory 'passpine.exe'
if (Test-Path -LiteralPath $client) {
    $channel = & $client --build-channel
    if ($LASTEXITCODE -ne 0 -or $channel -ne 'dev') { throw 'Refusing a non-Dev executable.' }
    & $client --quit-for-uninstall
    if ($LASTEXITCODE -ne 0) { throw 'Finish pending edits and quit the Dev application before retrying.' }
}
$running = Get-Process | Where-Object { $_.Path -and ($_.Path -eq $client -or $_.Path -eq (Join-Path $directory 'passpine-host.exe')) }
if ($running) { throw 'The installed Dev process or helper is still running. Close it and retry.' }
$uninstaller = Join-Path $directory 'Uninstall.exe'
if (-not (Test-Path -LiteralPath $uninstaller)) { throw 'The Dev uninstaller is missing. Repair this installation before removing it.' }
$process = Start-Process -FilePath $uninstaller -ArgumentList @('/S', "_?=$directory") -Wait -PassThru
if ($process.ExitCode -ne 0) { throw "Partial uninstall (exit $($process.ExitCode)); retry the same command. User data was preserved." }
# _?= keeps NSIS in this process so the exit code is reliable; remove its own
# executable after it has exited, then remove only an empty installation folder.
if (Test-Path -LiteralPath $uninstaller) { Remove-Item -LiteralPath $uninstaller }
if ((Test-Path -LiteralPath $directory) -and @(Get-ChildItem -LiteralPath $directory -Force).Count -eq 0) { Remove-Item -LiteralPath $directory }
Write-Output 'Removed Passpine Dev. User data and credentials were preserved.'

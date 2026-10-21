param([string]$InstallDirectory = '')
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
try {
    $version = & node scripts/version.mjs show
    if ($LASTEXITCODE -ne 0) { throw 'Cannot determine application version.' }
    $arch = & go env GOARCH
    if ($LASTEXITCODE -ne 0) { throw 'Go is required.' }
    $installer = Join-Path $root "dist\Passpine-$version-windows-$arch-setup.exe"
    if (-not (Test-Path -LiteralPath $installer)) { throw 'Missing Windows installer. Run make client:build first.' }
    $arguments = @('/S')
    if ($InstallDirectory) {
        $directory = [IO.Path]::GetFullPath($InstallDirectory)
        $repository = [IO.Path]::GetFullPath($root).TrimEnd('\')
        if ($directory -eq $repository -or $directory.StartsWith($repository + '\', [StringComparison]::OrdinalIgnoreCase)) {
            throw 'Client installation directory must be outside the repository.'
        }
        # NSIS requires /D to be last and unquoted, including paths with spaces.
        $arguments += "/D=$directory"
    }
    $process = Start-Process -FilePath $installer -ArgumentList $arguments -WindowStyle Hidden -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw "Installation failed (exit $($process.ExitCode))." }
    $installedDirectory = Get-ItemPropertyValue -LiteralPath 'HKCU:\Software\Passpine' -Name 'InstallDir'
    Write-Output "Installed client directory: $installedDirectory"
} finally { Pop-Location }

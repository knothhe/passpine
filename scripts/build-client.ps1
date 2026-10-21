param()
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$root = Split-Path $PSScriptRoot -Parent

function Invoke-Checked {
    param([string]$Command, [string[]]$Arguments)
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Command failed (exit $LASTEXITCODE)." }
}

Push-Location $root
try {
    $version = & node scripts/version.mjs show
    if ($LASTEXITCODE -ne 0) { throw 'Cannot determine application version.' }
    Invoke-Checked node @('scripts/dependencies.mjs')
    Invoke-Checked node @('scripts/version.mjs', 'check')
    $arch = & go env GOARCH
    if ($LASTEXITCODE -ne 0) { throw 'Go is required.' }
    $platform = & go env GOOS
    if ($LASTEXITCODE -ne 0 -or $platform -ne 'windows') { throw 'A native Windows Go toolchain is required.' }
    $compiler = Get-Command makensis.exe -ErrorAction SilentlyContinue
    if ($compiler) { $nsis = $compiler.Source }
    else { $nsis = Join-Path ${env:ProgramFiles(x86)} 'NSIS\makensis.exe' }
    if (-not (Test-Path -LiteralPath $nsis)) { throw 'NSIS is required to build the Windows installer (choco install nsis).' }
    $output = Join-Path $root 'dist'
    New-Item -ItemType Directory -Force $output | Out-Null
    $resourceDir = Join-Path $output 'windows-resources'
    Push-Location (Join-Path $PSScriptRoot 'windows-resources')
    try {
        Invoke-Checked go @('run', '.', '-icon', "$root\client\assets\app-icon-monochrome.png", '-out', $resourceDir, '-arch', $arch)
    } finally { Pop-Location }
    $resource = Join-Path $root "client\cmd\passpine\rsrc_windows_$arch.syso"
    if (Test-Path -LiteralPath $resource) { throw "Refusing to overwrite an existing resource: $resource" }
    Copy-Item -LiteralPath (Join-Path $resourceDir "rsrc_windows_$arch.syso") -Destination $resource
    Push-Location (Join-Path $root 'client')
    try {
        Invoke-Checked go @('build', '-trimpath', "-ldflags=-s -w -H windowsgui -X main.version=$version", '-o', "$output\passpine.exe", './cmd/passpine')
        Invoke-Checked go @('build', '-trimpath', "-ldflags=-s -w -X main.version=$version", '-o', "$output\passpine-host.exe", './cmd/passpine-host')
    } finally {
        Pop-Location
        Remove-Item -LiteralPath $resource
    }
    Invoke-Checked $nsis @('/V2', "/DAPP_VERSION=$version", "/DAPP_ARCH=$arch", "/DDIST_DIR=$output", (Join-Path $PSScriptRoot 'windows-installer.nsi'))
    Write-Output "Built: $output\Passpine-$version-windows-$arch-setup.exe"
} finally { Pop-Location }

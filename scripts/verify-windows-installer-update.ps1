param()
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# This is an opt-in packaging acceptance test on an ephemeral GitHub runner.
# It never uses a developer's installation, password store, or application data.
if (-not $IsWindows -or $env:GITHUB_ACTIONS -ne 'true' -or -not $env:RUNNER_TEMP) {
    throw 'Installer extraction acceptance runs only on an ephemeral Windows GitHub Actions runner.'
}
$project = Split-Path $PSScriptRoot -Parent
$configurationJSON = & node (Join-Path $PSScriptRoot 'build-channel.mjs') json
if ($LASTEXITCODE -ne 0) { throw 'Invalid build channel configuration.' }
$configuration = $configurationJSON | ConvertFrom-Json
$installer = Join-Path $configuration.outputDirectory "$($configuration.packageName)-$($configuration.version)-windows-amd64-setup.exe"
if (-not (Test-Path -LiteralPath $installer -PathType Leaf)) { throw "Installer is missing: $installer" }

$protectedPaths = @(
    (Join-Path $env:LOCALAPPDATA 'Programs\Passpine'),
    (Join-Path $env:LOCALAPPDATA 'Programs\Passpine Dev'),
    (Join-Path $env:LOCALAPPDATA 'Passpine'),
    (Join-Path $env:LOCALAPPDATA 'Passpine Dev'),
    (Join-Path $env:APPDATA 'Passpine'),
    (Join-Path $env:APPDATA 'Passpine Dev'),
    (Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Passpine'),
    (Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Passpine Dev')
)
$protectedRegistry = @(
    'HKCU:\Software\Passpine',
    'HKCU:\Software\Passpine Dev',
    'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Passpine',
    'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Passpine Dev'
)
foreach ($item in @($protectedPaths) + @($protectedRegistry)) {
    if (Test-Path -LiteralPath $item) { throw "Refusing to test with an existing installation or application data: $item" }
}

$root = Join-Path $env:RUNNER_TEMP ('passpine-installer-extraction-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory $root | Out-Null
$caseCount = 0
function Invoke-ExtractionCase {
    param([string]$Name, [string[]]$Arguments, [bool]$Success)
    $process = Start-Process -FilePath $installer -ArgumentList $Arguments -PassThru
    if (-not $process.WaitForExit(30000)) {
        $process.Kill($true)
        throw "$Name did not exit promptly."
    }
    $process.Refresh()
    if (($process.ExitCode -eq 0) -ne $Success) {
        throw "$Name returned unexpected exit code $($process.ExitCode)."
    }
    foreach ($item in @($protectedPaths) + @($protectedRegistry)) {
        if (Test-Path -LiteralPath $item) { throw "$Name changed protected installation state: $item" }
    }
    $script:caseCount++
    Write-Host "$Name passed (exit $($process.ExitCode))."
}
function Quote-ExtractionDirectory {
    param([string]$Directory)
    # Every generated fixture path has no double quotes or terminal backslash.
    return '"/PASSPINE_EXTRACT_DIR=' + $Directory + '"'
}
try {
    $stage = Join-Path $root 'stage-提取 full'
    New-Item -ItemType Directory $stage | Out-Null
    Invoke-ExtractionCase 'full-installer-extraction' @('/S', (Quote-ExtractionDirectory $stage)) $true
    $actual = @(Get-ChildItem -LiteralPath $stage -Force)
    if ($actual.Count -ne 2 -or @($actual | Where-Object { $_.PSIsContainer }).Count -ne 0) {
        throw 'Extraction produced files outside the two application executables.'
    }
    foreach ($name in @('passpine.exe', 'passpine-host.exe')) {
        $extracted = Join-Path $stage $name
        $original = Join-Path $configuration.outputDirectory $name
        if ((Get-FileHash -LiteralPath $extracted -Algorithm SHA256).Hash -ne (Get-FileHash -LiteralPath $original -Algorithm SHA256).Hash) {
            throw "Extracted payload mismatch: $name"
        }
    }
    $empty = Join-Path $root 'stage-empty'
    New-Item -ItemType Directory $empty | Out-Null
    Invoke-ExtractionCase 'empty-argument' @('/S', '/PASSPINE_EXTRACT_DIR=') $false
    Invoke-ExtractionCase 'missing-value' @('/S', '/PASSPINE_EXTRACT_DIR') $false
    Invoke-ExtractionCase 'relative-target' @('/S', '/PASSPINE_EXTRACT_DIR=stage-relative') $false
    Invoke-ExtractionCase 'duplicate-target' @('/S', (Quote-ExtractionDirectory $empty), (Quote-ExtractionDirectory $empty)) $false
    Invoke-ExtractionCase 'missing-silent-flag' @((Quote-ExtractionDirectory $empty)) $false
    Invoke-ExtractionCase 'wrong-case-mode' @('/S', '/passpine_extract_dir=stage-relative') $false
    Invoke-ExtractionCase 'occupied-target' @('/S', (Quote-ExtractionDirectory $stage)) $false
    Invoke-ExtractionCase 'extra-normal-install-argument' @('/S', (Quote-ExtractionDirectory $empty), ('/D=' + (Join-Path $root 'must-not-install'))) $false
    Invoke-ExtractionCase 'old-install-target' @('/S', (Quote-ExtractionDirectory $configuration.outputDirectory)) $false
    Invoke-ExtractionCase 'unc-target' @('/S', '/PASSPINE_EXTRACT_DIR=\\localhost\C$\passpine-stage') $false
    Invoke-ExtractionCase 'noncanonical-target' @('/S', (Quote-ExtractionDirectory ($empty + '\..\stage-empty'))) $false
    $junction = Join-Path $root 'junction'
    New-Item -ItemType Junction -Path $junction -Target $empty | Out-Null
    Invoke-ExtractionCase 'junction-target' @('/S', (Quote-ExtractionDirectory $junction)) $false
    Remove-Item -LiteralPath $junction -Force
    if (@(Get-ChildItem -LiteralPath $empty -Force).Count -ne 0) {
        throw 'A rejected argument wrote to the empty stage.'
    }
    if (Test-Path -LiteralPath (Join-Path $root 'must-not-install')) {
        throw 'A rejected extraction request entered the normal installation flow.'
    }

    # Compile the same argument parser in an explicitly named, no-install test
    # fixture. This checks NSIS's special unquoted /D= paths containing spaces
    # without ever allowing the ordinary installation sections to run.
    $parserDist = Join-Path $root 'parser-fixture'
    $parserResources = Join-Path $parserDist 'windows-resources'
    New-Item -ItemType Directory $parserResources -Force | Out-Null
    foreach ($name in @('passpine.exe', 'passpine-host.exe')) {
        Copy-Item -LiteralPath (Join-Path $configuration.outputDirectory $name) -Destination (Join-Path $parserDist $name)
    }
    Copy-Item -LiteralPath (Join-Path $configuration.outputDirectory 'windows-resources\Passpine.ico') -Destination (Join-Path $parserResources 'Passpine.ico')
    $compiler = Get-Command makensis.exe -ErrorAction SilentlyContinue
    if ($compiler) { $nsis = $compiler.Source }
    else { $nsis = Join-Path ${env:ProgramFiles(x86)} 'NSIS\makensis.exe' }
    $compileArguments = @('/V2', '/DPASSPINE_TEST_PARSE_ONLY', '/DAPP_VERSION=0.0.0', '/DAPP_BASE_VERSION=0.0.0', '/DAPP_ARCH=amd64', '/DAPP_CHANNEL=dev', '/DAPP_NAME=Passpine NSIS Parser Test', '/DPACKAGE_NAME=Passpine-NSIS-Parser-Test', "/DDIST_DIR=$parserDist", (Join-Path $PSScriptRoot 'windows-installer.nsi'))
    & $nsis @compileArguments
    if ($LASTEXITCODE -ne 0) { throw 'NSIS no-install parser fixture compilation failed.' }
    $ordinaryInstaller = $installer
    $installer = Join-Path $parserDist 'Passpine-NSIS-Parser-Test-0.0.0-windows-amd64-setup.exe'
    $manualDirectory = Join-Path $root 'manual destination with spaces'
    try {
        Invoke-ExtractionCase 'manual-unquoted-directory-with-spaces' @('/S', ('/D=' + $manualDirectory)) $true
        Invoke-ExtractionCase 'manual-standard-ncrc-directory' @('/NCRC', '/S', ('/D=' + $manualDirectory)) $true
        Invoke-ExtractionCase 'manual-directory-without-silent-flag' @(('/D=' + $manualDirectory)) $true
        Invoke-ExtractionCase 'invalid-extraction-after-unquoted-directory' @('/S', ('/D=' + $manualDirectory), '/PASSPINE_EXTRACT_DIR') $false
        Invoke-ExtractionCase 'mixed-extraction-after-unquoted-directory' @('/S', ('/D=' + $manualDirectory), (Quote-ExtractionDirectory $empty)) $false
        Invoke-ExtractionCase 'malformed-extraction-after-unquoted-directory' @('/S', ('/D=' + $manualDirectory), '--PASSPINE_EXTRACT_DIR=relative') $false
        if (Test-Path -LiteralPath $manualDirectory) { throw 'The parser fixture entered a normal installation section.' }
    } finally { $installer = $ordinaryInstaller }

    # Exercise the actual Go caller against this same installer, including its
    # resource marker, argument quoting, reparse checks and payload comparison.
    $env:PASSPINE_TEST_NSIS_INSTALLER = $installer
    $env:PASSPINE_TEST_NSIS_PAYLOAD = $configuration.outputDirectory
    Push-Location (Join-Path $project 'client')
    try {
        & go test -count=1 ./update -run '^TestNSIS'
        if ($LASTEXITCODE -ne 0) { throw 'Go installer extraction tests failed.' }
    } finally {
        Pop-Location
        Remove-Item Env:PASSPINE_TEST_NSIS_INSTALLER -ErrorAction SilentlyContinue
        Remove-Item Env:PASSPINE_TEST_NSIS_PAYLOAD -ErrorAction SilentlyContinue
    }
    foreach ($item in @($protectedPaths) + @($protectedRegistry)) {
        if (Test-Path -LiteralPath $item) { throw "Go extraction tests changed protected installation state: $item" }
    }
    Write-Host "Verified $caseCount isolated installer extraction cases and Go integration; no installation or application data was created."
} finally {
    # All fixture files belong to this newly created, random runner directory.
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Force -Recurse }
}

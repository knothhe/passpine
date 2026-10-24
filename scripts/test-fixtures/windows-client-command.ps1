param([Parameter(Mandatory = $true)][string]$Directory)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path (Split-Path $PSScriptRoot -Parent) 'windows-client-command.ps1')

$executable = Join-Path $Directory 'GUI client with spaces.exe'
Add-Type -OutputAssembly $executable -OutputType WindowsApplication -TypeDefinition @'
using System;
using System.Threading;
public static class GUIClient {
    [STAThread]
    public static int Main(string[] args) {
        Thread.Sleep(200);
        if (args[0] == "--build-channel") {
            Console.WriteLine("dev");
            return 0;
        }
        Console.Error.WriteLine("pending edits");
        return 7;
    }
}
'@

# A fresh Windows PowerShell process has no native-command exit-code variable.
Remove-Variable -Name LASTEXITCODE -Scope Global -ErrorAction SilentlyContinue
$channel = Invoke-PasspineClientCommand -FilePath $executable -Argument '--build-channel'
if ($channel.ExitCode -ne 0 -or $channel.Output -ne 'dev') { throw 'GUI channel output or exit code was lost.' }
$quit = Invoke-PasspineClientCommand -FilePath $executable -Argument '--quit-for-uninstall'
if ($quit.ExitCode -ne 7 -or $quit.ErrorOutput -ne 'pending edits') { throw 'A failed GUI command was accepted or its diagnostics were lost.' }

# An earlier native command must not supply the exit code of this GUI process.
$global:LASTEXITCODE = 42
$channel = Invoke-PasspineClientCommand -FilePath $executable -Argument '--build-channel'
if ($channel.ExitCode -ne 0 -or $channel.Output -ne 'dev') { throw 'A stale LASTEXITCODE affected the GUI command.' }
Write-Output 'Windows GUI client command checks passed.'

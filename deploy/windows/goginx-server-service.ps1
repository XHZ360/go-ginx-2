[CmdletBinding()]
param(
    [ValidateSet("install", "uninstall", "start", "stop", "restart", "status")]
    [string]$Action = "status",

    [string]$InstallRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$Name = "goginx-server",
    [string]$DisplayName = "go-ginx server",
    [string]$Description = "go-ginx server daemon",

    [ValidateSet("auto", "manual", "disabled")]
    [string]$Startup = "auto",

    [string]$Config = ""
)

$ErrorActionPreference = "Stop"

$exe = Join-Path $InstallRoot "bin\goginx-server.exe"
if (-not (Test-Path -LiteralPath $exe)) {
    throw "Cannot find goginx-server.exe at $exe"
}

$serviceArgs = @("service", $Action, "-name", $Name)
if ($Action -eq "install") {
    $serviceArgs += @(
        "-display-name", $DisplayName,
        "-description", $Description,
        "-startup", $Startup
    )
    if ($Config -ne "") {
        $serviceArgs += @("-config", $Config)
    }
}

& $exe @serviceArgs
exit $LASTEXITCODE

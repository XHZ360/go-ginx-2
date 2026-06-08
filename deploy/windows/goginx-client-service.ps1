[CmdletBinding()]
param(
    [ValidateSet("install", "uninstall", "start", "stop", "restart", "status")]
    [string]$Action = "status",

    [string]$InstallRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$Name = "goginx-client",
    [string]$DisplayName = "go-ginx client",
    [string]$Description = "go-ginx client daemon",

    [ValidateSet("auto", "manual", "disabled")]
    [string]$Startup = "auto",

    [string]$Config = ""
)

$ErrorActionPreference = "Stop"

$exe = Join-Path $InstallRoot "bin\goginx-client.exe"
if (-not (Test-Path -LiteralPath $exe)) {
    throw "Cannot find goginx-client.exe at $exe"
}

if ($Action -eq "install" -and $Config -eq "") {
    $statePath = Join-Path $InstallRoot "data\client-state.json"
    if (-not (Test-Path -LiteralPath $statePath)) {
        throw "Managed client state is missing at $statePath. Run bin\goginx-client.exe join <token> before installing the client service."
    }
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

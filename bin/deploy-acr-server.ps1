<#
.SYNOPSIS
Build, publish, and update New API, then synchronize the managed AIPDD catalog and prices.

.DESCRIPTION
The application image is built locally and pushed to Alibaba Cloud ACR. The
remote update preserves existing state, recreates only the New API service,
then refreshes the managed AIPDD channel, models, and prices.

Secrets can be supplied through environment variables or entered when prompted:

  $env:ACR_PASSWORD = "..."
  $env:DEPLOY_SERVER_PASSWORD = "..."
  $env:DEPLOY_ADMIN_PASSWORD = "..."
  .\bin\deploy-acr-server.ps1

Use -ForceAipddChannelOverwrite only for an explicitly approved destructive
AIPDD channel rebuild; an ordinary update synchronizes without deleting it.
#>

[CmdletBinding()]
param(
    [string]$Registry = $env:ACR_REGISTRY,
    [string]$Image = $env:ACR_IMAGE,
    [string]$AcrUsername = $env:ACR_USERNAME,
    [string]$AcrPassword = $env:ACR_PASSWORD,
    [string]$Tag = $env:DEPLOY_TAG,
    [string]$Platform = "linux/amd64",

    [string]$ServerHost = $env:DEPLOY_SERVER_HOST,
    [int]$ServerPort = 22,
    [string]$ServerUser = $env:DEPLOY_SERVER_USER,
    [string]$ServerPassword = $env:DEPLOY_SERVER_PASSWORD,

    [string]$RemoteProjectDir = "/www/wwwroot/new-api-aipdd",
    [string]$ComposeFile = "docker-compose.yml",
    [string]$EnvFile = ".env.compose",
    [int]$PublicPort = 6070,

    [string]$AdminUser = $env:DEPLOY_ADMIN_USER,
    [string]$AdminPassword = $env:DEPLOY_ADMIN_PASSWORD,
    [switch]$ForceAipddChannelOverwrite,
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"

function First-NonEmpty {
    param([string[]]$Values, [string]$Default = "")

    foreach ($value in $Values) {
        if (-not [string]::IsNullOrWhiteSpace($value)) {
            return $value
        }
    }
    return $Default
}

function Get-SecretValue {
    param(
        [string]$CurrentValue,
        [string[]]$EnvNames,
        [string]$Prompt
    )

    if (-not [string]::IsNullOrWhiteSpace($CurrentValue)) {
        return $CurrentValue
    }
    foreach ($name in $EnvNames) {
        $value = [Environment]::GetEnvironmentVariable($name, "Process")
        if (-not [string]::IsNullOrWhiteSpace($value)) {
            return $value
        }
    }

    $secureValue = Read-Host $Prompt -AsSecureString
    $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureValue)
    try {
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
    }
    finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
    }
}

function Invoke-Checked {
    param([string]$Name, [scriptblock]$Command)

    Write-Host ""
    Write-Host "==> $Name"
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$Name failed with exit code $LASTEXITCODE"
    }
}

function Set-TempEnv {
    param([hashtable]$Values, [scriptblock]$Command)

    $oldValues = @{}
    foreach ($key in $Values.Keys) {
        $oldValues[$key] = [Environment]::GetEnvironmentVariable($key, "Process")
        [Environment]::SetEnvironmentVariable($key, [string]$Values[$key], "Process")
    }
    try {
        & $Command
    }
    finally {
        foreach ($key in $Values.Keys) {
            [Environment]::SetEnvironmentVariable($key, $oldValues[$key], "Process")
        }
    }
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$RemoteUpdateScript = Join-Path $RepoRoot ".agents\skills\new-api-docker-deploy\scripts\run_remote_update.py"
$Registry = First-NonEmpty @($Registry) "crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com"
$Image = First-NonEmpty @($Image) "$Registry/aipdd/new-api-aipdd"
$AcrUsername = First-NonEmpty @($AcrUsername) "issay"
$Tag = First-NonEmpty @($Tag) (Get-Date -Format "yyyyMMdd-HHmmss")
$ServerHost = First-NonEmpty @($ServerHost) "118.178.32.102"
$ServerUser = First-NonEmpty @($ServerUser) "root"
$AdminUser = First-NonEmpty @($AdminUser) "root"

if (-not [string]::IsNullOrWhiteSpace($env:SERVER_PASSWORD) -and [string]::IsNullOrWhiteSpace($ServerPassword)) {
    $ServerPassword = $env:SERVER_PASSWORD
}

Write-Host "Deploy plan:"
Write-Host "  Repo:                $RepoRoot"
Write-Host "  Image:               ${Image}:$Tag"
Write-Host "  Aliases:             ${Image}:latest, ${Image}:aipdd"
Write-Host "  Platform:            $Platform"
Write-Host "  Server:              ${ServerUser}@${ServerHost}:$ServerPort"
Write-Host "  Compose:             $RemoteProjectDir/$ComposeFile"
Write-Host "  Environment file:    $RemoteProjectDir/$EnvFile"
Write-Host "  AIPDD sync:          channel, models, prices"
Write-Host "  AIPDD overwrite:     $ForceAipddChannelOverwrite"

$git = Get-Command git -ErrorAction SilentlyContinue
if ($git) {
    Push-Location $RepoRoot
    try {
        Write-Host "  Git HEAD:            $((& git log -1 --oneline) -join '')"
        if (& git status --short) {
            Write-Warning "Git working tree is not clean. The image will include current local changes."
        }
    }
    finally {
        Pop-Location
    }
}

if ($DryRun) {
    Write-Host ""
    Write-Host "Dry run only. No image or remote state was changed."
    exit 0
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker is required but was not found in PATH."
}
$python = Get-Command python -ErrorAction SilentlyContinue
if (-not $python) {
    throw "python is required but was not found in PATH."
}
if (-not (Test-Path -LiteralPath $RemoteUpdateScript -PathType Leaf)) {
    throw "Remote update helper not found: $RemoteUpdateScript"
}

$AcrPassword = Get-SecretValue `
    -CurrentValue $AcrPassword `
    -EnvNames @("ACR_PASSWORD", "DEPLOY_ACR_PASSWORD") `
    -Prompt "ACR password"
$ServerPassword = Get-SecretValue `
    -CurrentValue $ServerPassword `
    -EnvNames @("DEPLOY_SERVER_PASSWORD", "SERVER_PASSWORD") `
    -Prompt "Server SSH password"
$AdminPassword = Get-SecretValue `
    -CurrentValue $AdminPassword `
    -EnvNames @("DEPLOY_ADMIN_PASSWORD") `
    -Prompt "Existing New API administrator password"

Push-Location $RepoRoot
try {
    Invoke-Checked "Docker login to ACR" {
        $AcrPassword | docker login $Registry --username $AcrUsername --password-stdin
    }
    Invoke-Checked "Docker build and test" {
        docker build --platform $Platform `
            -t "${Image}:$Tag" `
            -t "${Image}:latest" `
            -t "${Image}:aipdd" `
            .
    }
    foreach ($pushTag in @($Tag, "latest", "aipdd")) {
        Invoke-Checked "Docker push ${Image}:$pushTag" {
            docker push "${Image}:$pushTag"
        }
    }
}
finally {
    Pop-Location
}

Write-Host ""
Write-Host "==> Update server and synchronize AIPDD"

Set-TempEnv @{
    "DEPLOY_SSH_HOST" = $ServerHost
    "DEPLOY_SSH_PORT" = $ServerPort
    "DEPLOY_SSH_USER" = $ServerUser
    "DEPLOY_SSH_PASSWORD" = $ServerPassword
    "DEPLOY_ADMIN_USER" = $AdminUser
    "DEPLOY_ADMIN_PASSWORD" = $AdminPassword
    "DEPLOY_DIR" = $RemoteProjectDir
    "DEPLOY_COMPOSE_FILE" = $ComposeFile
    "DEPLOY_ENV_FILE" = $EnvFile
    "DEPLOY_PUBLIC_PORT" = $PublicPort
    "DEPLOY_EXPECTED_IMAGE" = "${Image}:latest"
    "DEPLOY_CHANNEL_OVERWRITE" = $ForceAipddChannelOverwrite.IsPresent.ToString().ToLowerInvariant()
    "DEPLOY_ACR_REGISTRY" = $Registry
    "DEPLOY_ACR_USERNAME" = $AcrUsername
    "DEPLOY_ACR_PASSWORD" = $AcrPassword
} {
    & $python.Source $RemoteUpdateScript
    if ($LASTEXITCODE -ne 0) {
        throw "Server update failed with exit code $LASTEXITCODE"
    }
}

Write-Host ""
Write-Host "Deploy completed."
Write-Host "  Image:       ${Image}:$Tag"
Write-Host "  Active tag:  ${Image}:latest"
Write-Host "  AIPDD sync:  completed"

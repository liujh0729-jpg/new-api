<#
.SYNOPSIS
Build, push, and deploy the AIPDD Docker image.

.DESCRIPTION
This script builds the current repository into a Docker image, pushes it to
Aliyun ACR, then connects to the server over SSH to pull the new image and
recreate the docker compose service.

Secrets are never stored in this file. Provide them through environment
variables or enter them when prompted:

  $env:ACR_PASSWORD = "..."
  $env:DEPLOY_SERVER_PASSWORD = "..."
  .\bin\deploy-acr-server.ps1

Optional environment variables:
  ACR_REGISTRY, ACR_IMAGE, ACR_USERNAME, ACR_PASSWORD
  DEPLOY_SERVER_HOST, DEPLOY_SERVER_USER, DEPLOY_SERVER_PASSWORD
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
    [string]$ComposeService = "new-api",
    [string]$ContainerName = "new-api-aipdd",
    [string]$StatusUrl = "http://127.0.0.1:6070/api/status",

    [int]$HealthRetries = 24,
    [int]$HealthIntervalSeconds = 5,

    [switch]$DryRun
)

$ErrorActionPreference = "Stop"

function First-NonEmpty {
    param(
        [string[]]$Values,
        [string]$Default = ""
    )

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
    param(
        [string]$Name,
        [scriptblock]$Command
    )

    Write-Host ""
    Write-Host "==> $Name"
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$Name failed with exit code $LASTEXITCODE"
    }
}

function Set-TempEnv {
    param(
        [hashtable]$Values,
        [scriptblock]$Command
    )

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

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$Registry = First-NonEmpty @($Registry) "crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com"
$Image = First-NonEmpty @($Image) "$Registry/aipdd/new-api-aipdd"
$AcrUsername = First-NonEmpty @($AcrUsername) "issay"
$Tag = First-NonEmpty @($Tag) (Get-Date -Format "yyyyMMdd-HHmmss")
$ServerHost = First-NonEmpty @($ServerHost) "118.178.32.102"
$ServerUser = First-NonEmpty @($ServerUser) "root"

if (-not [string]::IsNullOrWhiteSpace($env:SERVER_PASSWORD) -and [string]::IsNullOrWhiteSpace($ServerPassword)) {
    $ServerPassword = $env:SERVER_PASSWORD
}

Write-Host "Deploy plan:"
Write-Host "  Repo:       $RepoRoot"
Write-Host "  Image:      ${Image}:$Tag"
Write-Host "  Aliases:    ${Image}:latest, ${Image}:aipdd"
Write-Host "  Platform:   $Platform"
Write-Host "  Server:     ${ServerUser}@${ServerHost}:$ServerPort"
Write-Host "  Compose:    $RemoteProjectDir/$ComposeFile service=$ComposeService"
Write-Host "  Container:  $ContainerName"

$git = Get-Command git -ErrorAction SilentlyContinue
if ($git) {
    Push-Location $RepoRoot
    try {
        $head = (& git log -1 --oneline) -join ""
        Write-Host "  Git HEAD:   $head"

        $dirty = (& git status --short)
        if ($dirty) {
            Write-Warning "Git working tree is not clean. The Docker image will include current local changes."
        }
    }
    finally {
        Pop-Location
    }
}

if ($DryRun) {
    Write-Host ""
    Write-Host "Dry run only. No Docker build, push, or server update was executed."
    exit 0
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker is required but was not found in PATH."
}

$python = Get-Command python -ErrorAction SilentlyContinue
if (-not $python) {
    throw "python is required for the SSH deployment step but was not found in PATH."
}

$AcrPassword = Get-SecretValue `
    -CurrentValue $AcrPassword `
    -EnvNames @("ACR_PASSWORD", "DEPLOY_ACR_PASSWORD") `
    -Prompt "ACR password"

$ServerPassword = Get-SecretValue `
    -CurrentValue $ServerPassword `
    -EnvNames @("DEPLOY_SERVER_PASSWORD", "SERVER_PASSWORD") `
    -Prompt "Server SSH password"

Push-Location $RepoRoot
try {
    Invoke-Checked "Docker login to ACR" {
        $AcrPassword | docker login $Registry --username $AcrUsername --password-stdin
    }

    Invoke-Checked "Docker build" {
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

$remoteDeployScript = @'
import os
import shlex
import sys
import time

try:
    import paramiko
except ImportError:
    sys.stderr.write("Python package 'paramiko' is required. Install it with: python -m pip install paramiko\n")
    sys.exit(2)


def required_env(name):
    value = os.environ.get(name)
    if not value:
        sys.stderr.write(f"Missing environment variable: {name}\n")
        sys.exit(2)
    return value


host = required_env("DEPLOY_SERVER_HOST")
port = int(os.environ.get("DEPLOY_SERVER_PORT", "22"))
user = required_env("DEPLOY_SERVER_USER")
password = required_env("DEPLOY_SERVER_PASSWORD")

registry = required_env("DEPLOY_ACR_REGISTRY")
acr_user = required_env("DEPLOY_ACR_USERNAME")
acr_password = required_env("DEPLOY_ACR_PASSWORD")
image = required_env("DEPLOY_IMAGE")
tag = required_env("DEPLOY_TAG")

remote_project_dir = required_env("DEPLOY_REMOTE_PROJECT_DIR")
compose_file = required_env("DEPLOY_COMPOSE_FILE")
env_file = required_env("DEPLOY_ENV_FILE")
compose_service = required_env("DEPLOY_COMPOSE_SERVICE")
container_name = required_env("DEPLOY_CONTAINER_NAME")
status_url = required_env("DEPLOY_STATUS_URL")
health_retries = int(required_env("DEPLOY_HEALTH_RETRIES"))
health_interval = int(required_env("DEPLOY_HEALTH_INTERVAL_SECONDS"))

inspect_format = "{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{else}}no-health{{end}} image={{.Image}}"
inspect_cmd = "docker inspect {container} --format {fmt}".format(
    container=shlex.quote(container_name),
    fmt=shlex.quote(inspect_format),
)

remote_lines = [
    "set -e",
    "read -r ACR_PASSWORD",
    "cd {path}".format(path=shlex.quote(remote_project_dir)),
    "printf 'Before: '",
    "{cmd} || true".format(cmd=inspect_cmd),
    "printf '%s\\n' \"$ACR_PASSWORD\" | docker login {registry} --username {user} --password-stdin >/dev/null".format(
        registry=shlex.quote(registry),
        user=shlex.quote(acr_user),
    ),
    "docker pull {image}".format(image=shlex.quote(f"{image}:{tag}")),
    "docker pull {image}".format(image=shlex.quote(f"{image}:latest")),
    "docker compose --env-file {env_file} -f {compose_file} config --quiet".format(
        env_file=shlex.quote(env_file),
        compose_file=shlex.quote(compose_file),
    ),
    "docker compose --env-file {env_file} -f {compose_file} up -d --no-deps --force-recreate {service}".format(
        env_file=shlex.quote(env_file),
        compose_file=shlex.quote(compose_file),
        service=shlex.quote(compose_service),
    ),
    "printf 'After recreate: '",
    inspect_cmd,
    "for i in $(seq 1 {retries}); do".format(retries=health_retries),
    "  state=$({cmd})".format(cmd=inspect_cmd),
    "  echo \"poll_${i} $state\"",
    "  echo \"$state\" | grep -q 'running healthy' && break",
    "  sleep {interval}".format(interval=health_interval),
    "done",
    "printf '\\nps:\\n'",
    "docker ps --filter {name_filter} --format {fmt}".format(
        name_filter=shlex.quote("name=" + container_name),
        fmt=shlex.quote("table {{.Names}}\\t{{.Image}}\\t{{.Status}}\\t{{.Ports}}"),
    ),
    "printf '\\nstatus endpoint:\\n'",
    "wget -q -O - {url} || true".format(url=shlex.quote(status_url)),
    "printf '\\n'",
]

remote_command = "\n".join(remote_lines)

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    client.connect(
        host,
        port=port,
        username=user,
        password=password,
        timeout=15,
        banner_timeout=15,
        auth_timeout=15,
    )
    stdin, stdout, stderr = client.exec_command(remote_command, get_pty=False, timeout=900)
    stdin.write(acr_password + "\n")
    stdin.flush()
    stdin.channel.shutdown_write()

    channel = stdout.channel
    while True:
        while channel.recv_ready():
            sys.stdout.write(channel.recv(4096).decode("utf-8", "replace"))
            sys.stdout.flush()
        while channel.recv_stderr_ready():
            sys.stderr.write(channel.recv_stderr(4096).decode("utf-8", "replace"))
            sys.stderr.flush()
        if channel.exit_status_ready():
            break
        time.sleep(0.2)

    while channel.recv_ready():
        sys.stdout.write(channel.recv(4096).decode("utf-8", "replace"))
    while channel.recv_stderr_ready():
        sys.stderr.write(channel.recv_stderr(4096).decode("utf-8", "replace"))

    sys.exit(channel.recv_exit_status())
finally:
    client.close()
'@

Write-Host ""
Write-Host "==> Server deploy"

Set-TempEnv @{
    "DEPLOY_SERVER_HOST" = $ServerHost
    "DEPLOY_SERVER_PORT" = $ServerPort
    "DEPLOY_SERVER_USER" = $ServerUser
    "DEPLOY_SERVER_PASSWORD" = $ServerPassword
    "DEPLOY_ACR_REGISTRY" = $Registry
    "DEPLOY_ACR_USERNAME" = $AcrUsername
    "DEPLOY_ACR_PASSWORD" = $AcrPassword
    "DEPLOY_IMAGE" = $Image
    "DEPLOY_TAG" = $Tag
    "DEPLOY_REMOTE_PROJECT_DIR" = $RemoteProjectDir
    "DEPLOY_COMPOSE_FILE" = $ComposeFile
    "DEPLOY_ENV_FILE" = $EnvFile
    "DEPLOY_COMPOSE_SERVICE" = $ComposeService
    "DEPLOY_CONTAINER_NAME" = $ContainerName
    "DEPLOY_STATUS_URL" = $StatusUrl
    "DEPLOY_HEALTH_RETRIES" = $HealthRetries
    "DEPLOY_HEALTH_INTERVAL_SECONDS" = $HealthIntervalSeconds
} {
    $remoteDeployScript | & $python.Source -
    if ($LASTEXITCODE -ne 0) {
        throw "Server deploy failed with exit code $LASTEXITCODE"
    }
}

Write-Host ""
Write-Host "Deploy completed."
Write-Host "  Image:  ${Image}:$Tag"
Write-Host "  Latest: ${Image}:latest"
Write-Host "  Alias:  ${Image}:aipdd"

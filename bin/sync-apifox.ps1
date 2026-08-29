<#
.SYNOPSIS
Sync docs/openapi/public.json into the Apifox project.

.DESCRIPTION
Reads projectId from .apifox/settings.json (fallback: docs/apifox/project.json).
Uses a locally logged-in Apifox CLI, or APIFOX_ACCESS_TOKEN for CI.

Never store the access token in this repository.
#>

[CmdletBinding()]
param(
    [string]$ProjectId = $env:APIFOX_PROJECT_ID,
    [string]$OpenApiFile,
    [string]$AccessToken = $env:APIFOX_ACCESS_TOKEN
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot

function Read-JsonFile([string]$Path) {
    if (-not (Test-Path $Path)) {
        return $null
    }
    return Get-Content -Raw -Encoding UTF8 $Path | ConvertFrom-Json
}

$settings = Read-JsonFile (Join-Path $repoRoot ".apifox\settings.json")
$project = Read-JsonFile (Join-Path $repoRoot "docs\apifox\project.json")

if (-not $ProjectId -or $ProjectId -eq "0") {
    if ($settings -and $settings.projectId -and [string]$settings.projectId -ne "0") {
        $ProjectId = [string]$settings.projectId
    }
    elseif ($project -and $project.projectId -and [string]$project.projectId -ne "0") {
        $ProjectId = [string]$project.projectId
    }
}

if (-not $OpenApiFile) {
    if ($settings -and $settings.openapiFile) {
        $OpenApiFile = [string]$settings.openapiFile
    }
    elseif ($project -and $project.openapiFile) {
        $OpenApiFile = [string]$project.openapiFile
    }
    else {
        $OpenApiFile = "docs/openapi/public.json"
    }
}

$openApiPath = if ([System.IO.Path]::IsPathRooted($OpenApiFile)) {
    $OpenApiFile
} else {
    Join-Path $repoRoot $OpenApiFile
}

if (-not $ProjectId -or $ProjectId -eq "0") {
    throw "Set projectId in .apifox/settings.json or docs/apifox/project.json, or pass -ProjectId / APIFOX_PROJECT_ID."
}
if (-not (Test-Path $openApiPath)) {
    throw "OpenAPI file not found: $openApiPath"
}

$apifox = Get-Command apifox -ErrorAction SilentlyContinue
if (-not $apifox) {
    throw "Apifox CLI not found. Install with: npm i -g apifox-cli@latest"
}

$importArgs = @(
    "import",
    "--project", $ProjectId,
    "--format", "openapi",
    "--file", $openApiPath
)
if ($AccessToken) {
    $importArgs += @("--access-token", $AccessToken)
}

Write-Host "Importing $openApiPath into Apifox project $ProjectId"
& apifox @importArgs
if ($LASTEXITCODE -ne 0) {
    throw "apifox import failed with exit code $LASTEXITCODE"
}

function Invoke-ApifoxJson {
    param([string[]]$Arguments)
    $previous = [Console]::OutputEncoding
    try {
        [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
        $raw = & apifox @Arguments | Out-String
    } finally {
        [Console]::OutputEncoding = $previous
    }
    if ($LASTEXITCODE -ne 0) {
        throw "apifox $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
    $start = $raw.IndexOf('{')
    $end = $raw.LastIndexOf('}')
    if ($start -lt 0 -or $end -le $start) {
        throw "apifox $($Arguments -join ' ') did not return JSON"
    }
    return $raw.Substring($start, $end - $start + 1) | ConvertFrom-Json
}

function Write-Utf8Json {
    param($Object, [string]$Path)
    $json = $Object | ConvertTo-Json -Depth 30
    [System.IO.File]::WriteAllText($Path, $json, [System.Text.UTF8Encoding]::new($false))
}

$openApi = Read-JsonFile $openApiPath

function Get-JsonProperty {
    param($Object, [string]$Name)
    if ($null -eq $Object) { return $null }
    $property = $Object.PSObject.Properties | Where-Object Name -EQ $Name | Select-Object -First 1
    if ($property) { return $property.Value }
    return $null
}

function Get-OpenApiReference {
    param([string]$Reference)
    if (-not $Reference.StartsWith("#/")) {
        throw "Only local OpenAPI references are supported: $Reference"
    }
    $value = $openApi
    foreach ($segment in $Reference.Substring(2).Split('/')) {
        $name = $segment.Replace('~1', '/').Replace('~0', '~')
        $value = Get-JsonProperty $value $name
        if ($null -eq $value) {
            throw "OpenAPI reference not found: $Reference"
        }
    }
    return $value
}

function Resolve-OpenApiValue {
    param($Value)
    if ($null -eq $Value -or $Value -is [string] -or $Value -is [ValueType]) {
        return $Value
    }
    if ($Value -is [System.Collections.IEnumerable] -and $Value -isnot [System.Management.Automation.PSCustomObject]) {
        return @($Value | ForEach-Object { Resolve-OpenApiValue $_ })
    }
    $reference = Get-JsonProperty $Value '$ref'
    if ($reference) {
        return Resolve-OpenApiValue (Get-OpenApiReference ([string]$reference))
    }
    $copy = [ordered]@{}
    foreach ($property in $Value.PSObject.Properties) {
        $copy[$property.Name] = Resolve-OpenApiValue $property.Value
    }
    return [pscustomobject]$copy
}

function Convert-OpenApiParameters {
    param($Operation)
    $groups = [ordered]@{ path = @(); query = @(); cookie = @(); header = @() }
    $index = 0
    foreach ($parameter in @($Operation.parameters)) {
        $location = [string]$parameter.in
        if (-not $groups.Contains($location)) { continue }
        $schema = Resolve-OpenApiValue $parameter.schema
        $type = [string](Get-JsonProperty $schema 'type')
        if (-not $type) { $type = "string" }
        $groups[$location] += [ordered]@{
            id = "$($parameter.name)#$index"
            name = [string]$parameter.name
            required = [bool]$parameter.required
            enable = $true
            description = [string]$parameter.description
            type = $type
            schema = $schema
        }
        $index++
    }
    return $groups
}

function Convert-OpenApiRequestBody {
    param($Operation)
    if (-not $Operation.requestBody) {
        return [ordered]@{ type = "none"; parameters = @(); required = $false; additionalContentTypes = @() }
    }
    $contentProperties = @($Operation.requestBody.content.PSObject.Properties)
    if ($contentProperties.Count -eq 0) {
        return [ordered]@{ type = "none"; parameters = @(); required = [bool]$Operation.requestBody.required; additionalContentTypes = @() }
    }
    $media = $contentProperties[0]
    $schema = Resolve-OpenApiValue $media.Value.schema
    if ($media.Name -eq "multipart/form-data") {
        $requiredNames = @((Get-JsonProperty $schema 'required'))
        $fields = @()
        $properties = Get-JsonProperty $schema 'properties'
        foreach ($property in @($properties.PSObject.Properties)) {
            $fieldSchema = Resolve-OpenApiValue $property.Value
            $fieldType = [string](Get-JsonProperty $fieldSchema 'type')
            if ((Get-JsonProperty $fieldSchema 'format') -eq 'binary') { $fieldType = 'file' }
            if (-not $fieldType) { $fieldType = 'string' }
            $fields += [ordered]@{
                id = "$($property.Name)#body"
                name = $property.Name
                required = $requiredNames -contains $property.Name
                enable = $true
                description = [string](Get-JsonProperty $fieldSchema 'description')
                type = $fieldType
                schema = $fieldSchema
            }
        }
        return [ordered]@{
            type = "multipart/form-data"
            parameters = $fields
            required = [bool]$Operation.requestBody.required
            additionalContentTypes = @()
        }
    }
    return [ordered]@{
        type = $media.Name
        required = [bool]$Operation.requestBody.required
        jsonSchema = $schema
        additionalContentTypes = @()
    }
}

function Convert-OpenApiResponses {
    param($Operation)
    $responses = @()
    foreach ($property in @($Operation.responses.PSObject.Properties)) {
        $response = Resolve-OpenApiValue $property.Value
        $content = Get-JsonProperty $response 'content'
        $mediaProperties = if ($content) { @($content.PSObject.Properties) } else { @() }
        $schema = [ordered]@{ type = "object"; additionalProperties = $true }
        $mediaType = "application/json"
        if ($mediaProperties.Count -gt 0) {
            $mediaType = $mediaProperties[0].Name
            $resolvedSchema = Resolve-OpenApiValue $mediaProperties[0].Value.schema
            if ($resolvedSchema) { $schema = $resolvedSchema }
        }
        $code = 0
        if (-not [int]::TryParse($property.Name, [ref]$code)) { continue }
        $responses += [ordered]@{
            name = if ($code -ge 200 -and $code -lt 300) { "成功" } else { "错误 $code" }
            code = $code
            contentType = if ($mediaType -eq "application/json") { "json" } else { $mediaType }
            description = [string](Get-JsonProperty $response 'description')
            jsonSchema = $schema
        }
    }
    return $responses
}

function Sync-SeedanceOpenApiEndpoints {
    param($EndpointList, [int]$FolderId, [string]$TempPath)
    foreach ($pathProperty in @($openApi.paths.PSObject.Properties)) {
        foreach ($method in @("get", "post", "put", "patch", "delete")) {
            $operation = Get-JsonProperty $pathProperty.Value $method
            if (-not $operation -or @($operation.tags) -notcontains "Seedance") { continue }
            $endpoint = @($EndpointList) | Where-Object {
                [int]$_.folderId -eq $FolderId -and [string]$_.method -eq $method -and [string]$_.path -eq $pathProperty.Name
            } | Select-Object -First 1
            if (-not $endpoint) {
                Write-Warning "Seedance endpoint missing after import: $method $($pathProperty.Name)"
                continue
            }
            $payload = [ordered]@{
                name = [string]$operation.summary
                type = "http"
                method = $method
                path = $pathProperty.Name
                status = "released"
                operationId = [string]$operation.operationId
                description = [string]$operation.description
                tags = @($operation.tags)
                auth = [ordered]@{ type = "bearer"; bearer = [ordered]@{ token = "" } }
                requestBody = Convert-OpenApiRequestBody $operation
                parameters = Convert-OpenApiParameters $operation
                responses = Convert-OpenApiResponses $operation
            }
            $operationId = if ($operation.operationId) { [string]$operation.operationId } else { "endpoint-$($endpoint.id)" }
            $endpointFile = Join-Path $TempPath "$operationId.endpoint.json"
            Write-Utf8Json $payload $endpointFile
            & apifox cli-schema validate endpoint-update --file $endpointFile | Out-Null
            if ($LASTEXITCODE -ne 0) { throw "Seedance endpoint failed validation: $method $($pathProperty.Name)" }
            Write-Host "Updating Seedance endpoint $method $($pathProperty.Name) (#$($endpoint.id))"
            & apifox endpoint update $endpoint.id --project $ProjectId --file $endpointFile
            if ($LASTEXITCODE -ne 0) { throw "failed to update Seedance endpoint $($endpoint.id)" }
        }
    }
}

$minimaxDir = Join-Path $repoRoot "docs\apifox\minimax"
$seedanceDir = Join-Path $repoRoot "docs\apifox\seedance"
$tempDir = Join-Path $repoRoot ".apifox\tmp"
New-Item -ItemType Directory -Force -Path $tempDir | Out-Null

$folders = Invoke-ApifoxJson @("folder", "list", "--project", $ProjectId, "--type", "endpoint")
$minimaxFolder = @($folders.data) | Where-Object { $_.name -eq "MiniMax H3" } | Select-Object -First 1
if (-not $minimaxFolder) {
    $folderFile = Join-Path $minimaxDir "folder.json"
    Write-Host "Creating MiniMax H3 folder"
    $created = Invoke-ApifoxJson @("folder", "create", "--project", $ProjectId, "--type", "endpoint", "--file", $folderFile)
    $minimaxFolderId = [int]$created.data.id
} else {
    $minimaxFolderId = [int]$minimaxFolder.id
}
$seedanceFolder = @($folders.data) | Where-Object { $_.name -eq "Seedance" } | Select-Object -First 1
if (-not $seedanceFolder) {
    throw "Seedance folder missing after OpenAPI import"
}
$seedanceFolderId = [int]$seedanceFolder.id

$endpoints = Invoke-ApifoxJson @("endpoint", "list", "--project", $ProjectId, "--page", "1", "--page-size", "500")
$legacyEndpoints = @($endpoints.data) | Where-Object {
    $path = [string]$_.path
    $name = [string]$_.name
    $path -eq "/v1/video/generations" -or $path.StartsWith("/v1/video/generations/") -or $name -match "旧版兼容"
}
foreach ($legacy in $legacyEndpoints) {
    Write-Host "Deleting leftover $($legacy.method) $($legacy.path) ($($legacy.name) #$($legacy.id))"
    & apifox endpoint delete $legacy.id --project $ProjectId
    if ($LASTEXITCODE -ne 0) { throw "failed to delete leftover endpoint $($legacy.id)" }
}
$minimaxCreate = @($endpoints.data) | Where-Object {
    [int]$_.folderId -eq $minimaxFolderId -and [string]$_.method -eq "post" -and [string]$_.path -eq "/v1/videos"
} | Select-Object -First 1
if (-not $minimaxCreate) {
    Write-Host "Creating MiniMax H3 create endpoint"
    $endpointFile = Join-Path $minimaxDir "create-video.json"
    & apifox cli-schema validate endpoint-create --file $endpointFile | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "MiniMax H3 create endpoint failed validation" }
    & apifox endpoint create --project $ProjectId --folder-id $minimaxFolderId --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "failed to create MiniMax H3 create endpoint" }
} else {
    Write-Host "Updating MiniMax H3 create endpoint #$($minimaxCreate.id)"
    $endpointFile = Join-Path $minimaxDir "create-video.json"
    & apifox cli-schema validate endpoint-update --file $endpointFile | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "MiniMax H3 create endpoint failed validation" }
    & apifox endpoint update $minimaxCreate.id --project $ProjectId --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "failed to update MiniMax H3 create endpoint" }
}
$minimaxGet = @($endpoints.data) | Where-Object {
    [int]$_.folderId -eq $minimaxFolderId -and [string]$_.method -eq "get" -and [string]$_.path -eq "/v1/videos/{task_id}"
} | Select-Object -First 1
if (-not $minimaxGet) {
    Write-Host "Creating MiniMax H3 get endpoint"
    $endpointFile = Join-Path $minimaxDir "get-video.json"
    & apifox cli-schema validate endpoint-create --file $endpointFile | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "MiniMax H3 get endpoint failed validation" }
    & apifox endpoint create --project $ProjectId --folder-id $minimaxFolderId --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "failed to create MiniMax H3 get endpoint" }
} else {
    Write-Host "Updating MiniMax H3 get endpoint #$($minimaxGet.id)"
    $endpointFile = Join-Path $minimaxDir "get-video.json"
    & apifox cli-schema validate endpoint-update --file $endpointFile | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "MiniMax H3 get endpoint failed validation" }
    & apifox endpoint update $minimaxGet.id --project $ProjectId --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "failed to update MiniMax H3 get endpoint" }
}
$seedanceOpenAICreate = @($endpoints.data) | Where-Object {
    [int]$_.folderId -eq $seedanceFolderId -and [string]$_.method -eq "post" -and [string]$_.path -eq "/v1/videos"
} | Select-Object -First 1
if (-not $seedanceOpenAICreate) {
    Write-Host "Creating Seedance OpenAI Video create endpoint"
    & apifox endpoint create --project $ProjectId --folder-id $seedanceFolderId --file (Join-Path $seedanceDir "create-video.json")
    if ($LASTEXITCODE -ne 0) { throw "failed to create Seedance OpenAI Video create endpoint" }
} else {
    Write-Host "Updating Seedance OpenAI Video create endpoint #$($seedanceOpenAICreate.id)"
    $endpointFile = Join-Path $seedanceDir "create-video.json"
    & apifox cli-schema validate endpoint-update --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "Seedance OpenAI Video create endpoint failed validation" }
    & apifox endpoint update $seedanceOpenAICreate.id --project $ProjectId --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "failed to update Seedance OpenAI Video create endpoint" }
}
$seedanceOpenAIGet = @($endpoints.data) | Where-Object {
    [int]$_.folderId -eq $seedanceFolderId -and [string]$_.method -eq "get" -and [string]$_.path -eq "/v1/videos/{task_id}"
} | Select-Object -First 1
if (-not $seedanceOpenAIGet) {
    Write-Host "Creating Seedance OpenAI Video get endpoint"
    & apifox endpoint create --project $ProjectId --folder-id $seedanceFolderId --file (Join-Path $seedanceDir "get-video.json")
    if ($LASTEXITCODE -ne 0) { throw "failed to create Seedance OpenAI Video get endpoint" }
} else {
    Write-Host "Updating Seedance OpenAI Video get endpoint #$($seedanceOpenAIGet.id)"
    $endpointFile = Join-Path $seedanceDir "get-video.json"
    & apifox cli-schema validate endpoint-update --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "Seedance OpenAI Video get endpoint failed validation" }
    & apifox endpoint update $seedanceOpenAIGet.id --project $ProjectId --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "failed to update Seedance OpenAI Video get endpoint" }
}

# OpenAPI imports use the project's conflict policy and may leave existing endpoints
# untouched. Explicitly update every Seedance-tagged OpenAPI operation so parameter,
# response, status-code, and description changes are always synchronized.
Sync-SeedanceOpenApiEndpoints @($endpoints.data) $seedanceFolderId $tempDir

$seedanceOfficialCreate = @($endpoints.data) | Where-Object {
    [int]$_.folderId -eq $seedanceFolderId -and [string]$_.method -eq "post" -and [string]$_.path -eq "/api/v3/contents/generations/tasks"
} | Select-Object -First 1
if ($seedanceOfficialCreate -and [string]$seedanceOfficialCreate.name -ne "创建视频任务（Seedance官方协议）") {
    Write-Host "Renaming Seedance official create endpoint #$($seedanceOfficialCreate.id)"
    $endpointFile = Join-Path $tempDir "seedance-official-create.endpoint.json"
    Write-Utf8Json @{ name = "创建视频任务（Seedance官方协议）" } $endpointFile
    & apifox cli-schema validate endpoint-update --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "Seedance official create endpoint failed validation" }
    & apifox endpoint update $seedanceOfficialCreate.id --project $ProjectId --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "failed to rename Seedance official create endpoint" }
}

$seedanceOfficialGet = @($endpoints.data) | Where-Object {
    [int]$_.folderId -eq $seedanceFolderId -and [string]$_.method -eq "get" -and [string]$_.path -eq "/api/v3/contents/generations/tasks/{task_id}"
} | Select-Object -First 1
if ($seedanceOfficialGet -and [string]$seedanceOfficialGet.name -ne "查询视频任务（Seedance官方协议）") {
    Write-Host "Renaming Seedance official get endpoint #$($seedanceOfficialGet.id)"
    $endpointFile = Join-Path $tempDir "seedance-official-get.endpoint.json"
    Write-Utf8Json @{ name = "查询视频任务（Seedance官方协议）" } $endpointFile
    & apifox cli-schema validate endpoint-update --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "Seedance official get endpoint failed validation" }
    & apifox endpoint update $seedanceOfficialGet.id --project $ProjectId --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "failed to rename Seedance official get endpoint" }
}

$personalMaterialCreate = @($endpoints.data) | Where-Object {
    [int]$_.folderId -eq $seedanceFolderId -and [string]$_.method -eq "post" -and [string]$_.path -eq "/v1/virtual-characters"
} | Select-Object -First 1
if ($personalMaterialCreate -and [string]$personalMaterialCreate.name -ne "创建个人素材") {
    Write-Host "Renaming personal material create endpoint #$($personalMaterialCreate.id)"
    $endpointFile = Join-Path $tempDir "personal-material-create.endpoint.json"
    Write-Utf8Json @{ name = "创建个人素材" } $endpointFile
    & apifox cli-schema validate endpoint-update --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "Personal material create endpoint failed validation" }
    & apifox endpoint update $personalMaterialCreate.id --project $ProjectId --file $endpointFile
    if ($LASTEXITCODE -ne 0) { throw "failed to rename personal material create endpoint" }
}

$docs = Invoke-ApifoxJson @("doc", "list", "--project", $ProjectId)
$overviewMd = [System.IO.File]::ReadAllText((Join-Path $minimaxDir "overview.md"))
$minimaxOverview = @($docs.data) | Where-Object { $_.name -eq "MiniMax H3 模型与约束" } | Select-Object -First 1
if (-not $minimaxOverview) {
    Write-Host "Creating MiniMax H3 overview doc"
    $docFile = Join-Path $tempDir "minimax-overview.doc.json"
    Write-Utf8Json @{
        name = "MiniMax H3 模型与约束"
        folderId = $minimaxFolderId
        content = $overviewMd
    } $docFile
    & apifox cli-schema validate doc-create --file $docFile
    if ($LASTEXITCODE -ne 0) { throw "MiniMax overview doc failed validation" }
    & apifox doc create --project $ProjectId --file $docFile
    if ($LASTEXITCODE -ne 0) { throw "failed to create MiniMax overview doc" }
} else {
    Write-Host "Updating MiniMax H3 overview doc #$($minimaxOverview.id)"
    $existing = Invoke-ApifoxJson @("doc", "get", [string]$minimaxOverview.id, "--project", $ProjectId)
    $updatePayload = $existing.data
    if (-not $updatePayload) {
        $updatePayload = $existing
    }
    $updatePayload.content = $overviewMd
    $docFile = Join-Path $tempDir "minimax-overview.doc.json"
    Write-Utf8Json $updatePayload $docFile
    & apifox cli-schema validate doc-update --file $docFile | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "MiniMax overview doc failed validation" }
    & apifox doc update $minimaxOverview.id --project $ProjectId --file $docFile
    if ($LASTEXITCODE -ne 0) { throw "failed to update MiniMax overview doc" }
}
$overviewMd = [System.IO.File]::ReadAllText((Join-Path $seedanceDir "overview.md"))
$seedanceOverview = @($docs.data) | Where-Object { $_.name -eq "Seedance 调用说明" } | Select-Object -First 1
if (-not $seedanceOverview) {
    Write-Host "Creating Seedance overview doc"
    $docFile = Join-Path $tempDir "seedance-overview.doc.json"
    Write-Utf8Json @{
        name = "Seedance 调用说明"
        folderId = $seedanceFolderId
        content = $overviewMd
    } $docFile
    & apifox cli-schema validate doc-create --file $docFile
    if ($LASTEXITCODE -ne 0) { throw "Seedance overview doc failed validation" }
    & apifox doc create --project $ProjectId --file $docFile
    if ($LASTEXITCODE -ne 0) { throw "failed to create Seedance overview doc" }
} else {
    Write-Host "Updating Seedance overview doc #$($seedanceOverview.id)"
    $existing = Invoke-ApifoxJson @("doc", "get", [string]$seedanceOverview.id, "--project", $ProjectId)
    $updatePayload = $existing.data
    if (-not $updatePayload) {
        $updatePayload = $existing
    }
    $updatePayload.content = $overviewMd
    $docFile = Join-Path $tempDir "seedance-overview.doc.json"
    Write-Utf8Json $updatePayload $docFile
    & apifox cli-schema validate doc-update --file $docFile
    if ($LASTEXITCODE -ne 0) { throw "Seedance overview doc failed validation" }
    & apifox doc update $seedanceOverview.id --project $ProjectId --file $docFile
    if ($LASTEXITCODE -ne 0) { throw "failed to update Seedance overview doc" }
}

$publicDocsUrl = ""
if ($settings -and $settings.publicDocsUrl) {
    $publicDocsUrl = [string]$settings.publicDocsUrl
}
if (-not $publicDocsUrl -and $project -and $project.publicDocsUrl) {
    $publicDocsUrl = [string]$project.publicDocsUrl
}

if ($publicDocsUrl) {
    Write-Host "Public docs: $publicDocsUrl"
} else {
    Write-Host "Import finished. Publish a docs site or shared doc, then put the URL in docs/apifox/project.json as publicDocsUrl."
}

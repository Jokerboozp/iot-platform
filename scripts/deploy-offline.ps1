[CmdletBinding()]
param(
    [string]$BundleDir = "",
    [switch]$SkipHashCheck,
    [switch]$SkipHealthCheck
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
if ([string]::IsNullOrWhiteSpace($BundleDir)) {
    $BundleDir = Split-Path -Parent $scriptDir
}
$BundleDir = [System.IO.Path]::GetFullPath($BundleDir)

function Invoke-Checked {
    param([Parameter(Mandatory)][string[]]$Arguments)

    Write-Host ("> docker " + ($Arguments -join " ")) -ForegroundColor DarkGray
    & docker @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Docker 命令失败，退出码 $LASTEXITCODE：docker $($Arguments -join ' ')"
    }
}

function Get-EnvValue {
    param([Parameter(Mandatory)][string]$Path, [Parameter(Mandatory)][string]$Key)

    foreach ($line in @(Get-Content -LiteralPath $Path -Encoding UTF8)) {
        if ($line -match ('^\s*' + [Regex]::Escape($Key) + '\s*=\s*(.*)$')) {
            return $matches[1].Trim().Trim('"').Trim("'")
        }
    }
    return $null
}

if (-not (Test-Path -LiteralPath $BundleDir -PathType Container)) {
    throw "离线包目录不存在：$BundleDir"
}
$envPath = Join-Path $BundleDir ".env.offline"
$composePath = Join-Path $BundleDir "compose.yaml"
$offlineComposePath = Join-Path $BundleDir "compose.offline.yaml"
$archivePath = Join-Path $BundleDir "images.tar"
$hashPath = Join-Path $BundleDir "images.tar.sha256"
$profilesPath = Join-Path $BundleDir "profiles.txt"
$ollamaVolumePath = Join-Path $BundleDir "ollama-volume.txt"
foreach ($path in @($envPath, $composePath, $offlineComposePath, $archivePath, $hashPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "离线包缺少文件：$path"
    }
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "找不到 docker 命令，请先安装 Docker Engine/Desktop。"
}
& docker info *> $null
if ($LASTEXITCODE -ne 0) {
    throw "Docker Engine 不可用，请先启动 Docker。"
}

if (-not $SkipHashCheck) {
    $expectedHash = ((Get-Content -LiteralPath $hashPath -Encoding UTF8 | Select-Object -First 1) -split '\s+')[0].ToLowerInvariant()
    $actualHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($expectedHash -ne $actualHash) {
        throw "镜像包 SHA256 校验失败：期望 $expectedHash，实际 $actualHash"
    }
    Write-Host "镜像包 SHA256 校验通过。" -ForegroundColor Green
}

Invoke-Checked -Arguments @("load", "-i", $archivePath)

$ollamaArchive = Join-Path $BundleDir "ollama-data.tgz"
$ollamaVolume = "iot-platform_ollama-data"
if (Test-Path -LiteralPath $ollamaVolumePath -PathType Leaf) {
    $ollamaVolume = ((Get-Content -LiteralPath $ollamaVolumePath -Encoding UTF8 | Select-Object -First 1).Trim())
}
if (Test-Path -LiteralPath $ollamaArchive -PathType Leaf) {
    & docker volume inspect $ollamaVolume *> $null
    if ($LASTEXITCODE -eq 0) {
        Write-Warning "检测到已有 $ollamaVolume，跳过 Ollama 模型恢复以避免覆盖现有数据。"
    } else {
        Invoke-Checked -Arguments @("volume", "create", $ollamaVolume)
        Invoke-Checked -Arguments @(
            "run", "--rm",
            "--mount", "type=volume,source=$ollamaVolume,target=/dst",
            "--mount", "type=bind,source=$BundleDir,target=/backup",
            "alpine:3.22", "sh", "-ec", "tar -xzf /backup/ollama-data.tgz -C /dst"
        )
        Write-Host "Ollama 模型卷已恢复。" -ForegroundColor Green
    }
}

$composeArguments = @(
    "compose",
    "--env-file", $envPath,
    "-f", $composePath,
    "-f", $offlineComposePath
)
$env:COMPOSE_PROJECT_NAME = "iot-platform"
if (Test-Path -LiteralPath $profilesPath -PathType Leaf) {
    foreach ($profile in @(Get-Content -LiteralPath $profilesPath -Encoding UTF8 | Where-Object { $_.Trim() })) {
        $composeArguments += @("--profile", $profile.Trim())
    }
}

Invoke-Checked -Arguments ($composeArguments + @("config", "--quiet"))
Invoke-Checked -Arguments ($composeArguments + @("up", "-d", "--no-build", "--pull", "never"))
Invoke-Checked -Arguments ($composeArguments + @("ps"))

if (-not $SkipHealthCheck) {
    $apiPort = Get-EnvValue -Path $envPath -Key "IOT_API_PORT"
    if ([string]::IsNullOrWhiteSpace($apiPort)) { $apiPort = "8081" }
    $healthUrl = "http://127.0.0.1:$apiPort/health/live"
    $healthy = $false
    for ($i = 0; $i -lt 60; $i++) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri $healthUrl -TimeoutSec 3
            if ($response.StatusCode -eq 200) {
                $healthy = $true
                break
            }
        } catch {
            # Compose health/dependency checks may still be in progress.
        }
        Start-Sleep -Seconds 2
    }
    if (-not $healthy) {
        & docker @($composeArguments + @("logs", "--tail=100", "platform-api", "postgres", "redpanda", "emqx"))
        throw "平台健康检查失败：$healthUrl"
    }
    Write-Host "平台健康检查通过：$healthUrl" -ForegroundColor Green
}

Write-Host "离线部署完成。Web 默认地址：http://127.0.0.1:8080" -ForegroundColor Green

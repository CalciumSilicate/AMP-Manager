# AMP Manager 开发启动脚本
# 使用方法: .\dev.ps1

$ErrorActionPreference = "Stop"
$Host.UI.RawUI.WindowTitle = "AMP Manager Dev"

$repoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$webRoot = Join-Path $repoRoot "web"
$composeFile = Join-Path $repoRoot "docker-compose.dev.yml"

function Ensure-Command {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [string]$InstallHint = ""
    )

    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        if ($InstallHint) {
            throw "未找到 $Name。$InstallHint"
        }
        throw "未找到 $Name。"
    }
}

function Ensure-Pnpm {
    if (-not (Get-Command pnpm -ErrorAction SilentlyContinue)) {
        Write-Host "[信息] 未找到 pnpm，正在安装..." -ForegroundColor Yellow
        npm install -g pnpm
    }
}

function Get-AirExecutable {
    $air = Get-Command air -ErrorAction SilentlyContinue
    if ($air) {
        return $air.Source
    }

    Write-Host "[信息] 未找到 air，正在安装..." -ForegroundColor Yellow
    & go install github.com/air-verse/air@latest

    $goBin = Join-Path (& go env GOPATH) "bin"
    $airPath = Join-Path $goBin "air.exe"
    if (Test-Path $airPath) {
        return $airPath
    }

    throw "air 安装完成但未在 PATH 中找到，请确认 GOPATH/bin 已加入 PATH。"
}

function Wait-ForPort {
    param(
        [string]$HostName,
        [int]$Port,
        [int]$TimeoutSeconds = 60
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        if (Test-NetConnection -ComputerName $HostName -Port $Port -InformationLevel Quiet -WarningAction SilentlyContinue) {
            return
        }
        Start-Sleep -Seconds 1
    }

    throw "等待 ${HostName}:$Port 就绪超时。"
}

Write-Host ""
Write-Host "==============================" -ForegroundColor Cyan
Write-Host "   AMP Manager 开发启动脚本" -ForegroundColor Cyan
Write-Host "==============================" -ForegroundColor Cyan
Write-Host ""

Ensure-Command -Name node -InstallHint "请先安装 Node.js"
Ensure-Pnpm
Ensure-Command -Name go -InstallHint "请先安装 Go"
Ensure-Command -Name docker -InstallHint "请先安装 Docker Desktop 并确保 docker compose 可用"

$airExecutable = Get-AirExecutable

Write-Host "[1/4] 启动 PostgreSQL 与 Redis 容器..." -ForegroundColor Green
& docker compose -f $composeFile up -d postgres redis | Out-Host
Wait-ForPort -HostName "127.0.0.1" -Port 5432
Wait-ForPort -HostName "127.0.0.1" -Port 6379

Write-Host ""
Write-Host "[2/4] 安装前端依赖..." -ForegroundColor Green
Push-Location $webRoot
& pnpm install | Out-Host
Pop-Location

$frontendCommand = @"
Set-Location '$webRoot'
pnpm run dev -- --host 0.0.0.0
"@

$backendCommand = @"
`$env:ALLOW_INSECURE_DEFAULTS='true'
`$env:AMP_DEV_RUNTIME_DB_CONFIG='true'
`$env:CORS_ALLOWED_ORIGINS='http://localhost:5274'
`$env:SERVER_PORT='16823'
`$env:REDIS_URL='redis://localhost:6379/0'
`$env:REDIS_PREFIX='ampmanager-dev'
Set-Location '$repoRoot'
& '$airExecutable'
"@

Write-Host ""
Write-Host "[3/4] 启动后端热更 (Air)..." -ForegroundColor Green
Start-Process powershell -WorkingDirectory $repoRoot -ArgumentList @('-NoExit', '-Command', $backendCommand) | Out-Null

Write-Host ""
Write-Host "[4/4] 启动前端热更 (Vite)..." -ForegroundColor Green
Start-Process powershell -WorkingDirectory $webRoot -ArgumentList @('-NoExit', '-Command', $frontendCommand) | Out-Null

Write-Host ""
Write-Host "==============================" -ForegroundColor Cyan
Write-Host "   前端: http://localhost:5274" -ForegroundColor Yellow
Write-Host "   后端: http://localhost:16823" -ForegroundColor Yellow
Write-Host "   PostgreSQL 容器: localhost:5432" -ForegroundColor Yellow
Write-Host "   Redis 容器: localhost:6379" -ForegroundColor Yellow
Write-Host "   默认数据库模式: 读取 ./data/config.json；首次缺省为 PostgreSQL" -ForegroundColor Yellow
Write-Host "==============================" -ForegroundColor Cyan
Write-Host ""

Start-Process "http://localhost:5274" | Out-Null

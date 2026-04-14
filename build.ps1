# AMP Manager 编译脚本
# 使用方法: 右键 -> 使用 PowerShell 运行

$ErrorActionPreference = "Stop"
$Host.UI.RawUI.WindowTitle = "AMP Manager Build"

Write-Host ""
Write-Host "==============================" -ForegroundColor Cyan
Write-Host "   AMP Manager 编译脚本" -ForegroundColor Cyan
Write-Host "==============================" -ForegroundColor Cyan
Write-Host ""

# 检查 Node.js
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Host "[错误] 未找到 Node.js，请先安装 Node.js" -ForegroundColor Red
    Read-Host "按回车键退出"
    exit 1
}

# 检查 pnpm
if (-not (Get-Command pnpm -ErrorAction SilentlyContinue)) {
    Write-Host "[信息] 未找到 pnpm，正在安装..." -ForegroundColor Yellow
    npm install -g pnpm
}

# 检查 Go
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "[错误] 未找到 Go，请先安装 Go" -ForegroundColor Red
    Read-Host "按回车键退出"
    exit 1
}

Write-Host "[1/4] 安装前端依赖..." -ForegroundColor Green
Set-Location web
& pnpm install
if (-not $?) {
    Write-Host "[错误] 前端依赖安装失败" -ForegroundColor Red
    Set-Location ..
    Read-Host "按回车键退出"
    exit 1
}

Write-Host ""
Write-Host "[2/4] 编译前端..." -ForegroundColor Green
& pnpm run build
if (-not $?) {
    Write-Host "[错误] 前端编译失败" -ForegroundColor Red
    Set-Location ..
    Read-Host "按回车键退出"
    exit 1
}
Set-Location ..

Write-Host ""
Write-Host "[3/4] 复制前端文件到嵌入目录..." -ForegroundColor Green
if (-not (Test-Path "internal\web\dist")) {
    New-Item -ItemType Directory -Path "internal\web\dist" -Force | Out-Null
}
Copy-Item -Path "web\dist\*" -Destination "internal\web\dist\" -Recurse -Force

Write-Host ""
Write-Host "[4/4] 编译后端二进制文件..." -ForegroundColor Green
& go build -tags embed_frontend -ldflags="-s -w" -o ampmanager.exe ./cmd/server
if (-not $?) {
    Write-Host "[错误] 后端编译失败" -ForegroundColor Red
    Read-Host "按回车键退出"
    exit 1
}

Write-Host ""
Write-Host "==============================" -ForegroundColor Cyan
Write-Host "   编译完成！" -ForegroundColor Green
Write-Host "   输出文件: ampmanager.exe" -ForegroundColor Yellow
Write-Host "==============================" -ForegroundColor Cyan
Write-Host ""

Read-Host "按回车键退出"

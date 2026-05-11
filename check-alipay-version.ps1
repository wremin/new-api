# 查询支付宝 SDK 可用版本
$env:GOPROXY = "https://goproxy.cn,direct"

Write-Host "正在查询支付宝 SDK 可用版本..." -ForegroundColor Cyan
Write-Host ""

# 方法1：通过 GOPROXY 查询
try {
    $response = Invoke-RestMethod -Uri "https://goproxy.cn/github.com/smartwalle/alipay/v3/@v/list" -Method Get
    Write-Host "从 goproxy.cn 查询到的版本：" -ForegroundColor Green
    $response -split "`n" | Where-Object { $_.Trim() -ne "" } | ForEach-Object { Write-Host "  $_" }
} catch {
    Write-Host "从 goproxy.cn 查询失败: $_" -ForegroundColor Yellow
}

Write-Host ""

# 方法2：通过 GitHub 查询
try {
    $tags = Invoke-RestMethod -Uri "https://api.github.com/repos/smartwalle/alipay/tags?per_page=30" -Method Get
    Write-Host "从 GitHub 查询到的版本（前20个）：" -ForegroundColor Green
    $tags | Select-Object -First 20 | ForEach-Object { Write-Host "  $($_.name)" }
} catch {
    Write-Host "从 GitHub 查询失败: $_" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "推荐的稳定版本：" -ForegroundColor Magenta
Write-Host "  v3.3.0" -ForegroundColor White
Write-Host "  v3.2.29" -ForegroundColor White
Write-Host "  v3.2.0" -ForegroundColor White
Write-Host "  v3.0.0" -ForegroundColor White

# New-API 部署脚本 - PowerShell版

$KeyPath = "D:\work\ai\测试服务器\test.pem"
$Server = "ubuntu@3.26.235.238"
$SSHArgs = "-i", "`"$KeyPath`"", "-o", "StrictHostKeyChecking=no"

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host "  启动 New-API 服务" -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan

# 启动服务
Write-Host "`n🚀 启动 Docker Compose 服务..." -ForegroundColor Yellow
$StartCmd = "cd /opt/new-api && sudo docker compose up -d"
ssh @SSHArgs $Server $StartCmd

# 等待启动
Write-Host "⏳ 等待服务启动..." -ForegroundColor Yellow
Start-Sleep -Seconds 10

# 检查状态
Write-Host "`n=========================================" -ForegroundColor Cyan
Write-Host "  服务状态" -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan
$StatusCmd = "sudo docker compose -f /opt/new-api/docker-compose.yml ps"
ssh @SSHArgs $Server $StatusCmd

# 查看日志
Write-Host "`n=========================================" -ForegroundColor Cyan
Write-Host "  New-API 日志（最新20行）" -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan
$LogCmd = "sudo docker logs new-api --tail 20"
ssh @SSHArgs $Server $LogCmd

Write-Host "`n=========================================" -ForegroundColor Green
Write-Host "  ✅ 部署完成！" -ForegroundColor Green
Write-Host "=========================================" -ForegroundColor Green
Write-Host "📍 访问地址: http://3.26.235.238:3000" -ForegroundColor White
Write-Host "📝 查看日志: ssh -i test.pem ubuntu@3.26.235.238 'sudo docker logs -f new-api'" -ForegroundColor White
Write-Host "🔄 重启服务: ssh -i test.pem ubuntu@3.26.235.238 'cd /opt/new-api && sudo docker compose restart'" -ForegroundColor White
Write-Host "🛑 停止服务: ssh -i test.pem ubuntu@3.26.235.238 'cd /opt/new-api && sudo docker compose down'" -ForegroundColor White
Write-Host "=========================================" -ForegroundColor Green

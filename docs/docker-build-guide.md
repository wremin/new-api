# Docker 构建指南 - 支付宝 SDK 集成

## 问题说明

在中国大陆环境下，Go 依赖下载存在以下问题：
- 大部分国外依赖需要翻墙才能下载
- 翻墙后支付宝 SDK (`github.com/smartwalle/alipay`) 可能无法访问
- 网络不稳定导致构建失败

## 解决方案

### ✅ 已配置的解决方案

Dockerfile 已配置国内 Go 模块代理：

```dockerfile
ENV GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
ENV GONOSUMDB=github.com/smartwalle/*
```

**工作原理**：
1. 优先使用 `goproxy.cn`（国内镜像）下载依赖
2. 如果国内镜像没有，尝试 `proxy.golang.org`（官方代理）
3. 最后直接访问源站
4. 对 `smartwalle` 包跳过校验和检查（避免网络问题导致的校验失败）

## 构建步骤

### 方法一：使用 docker compose（推荐）

```bash
# 1. 清理旧镜像（可选）
docker compose down

# 2. 构建镜像（会自动使用国内代理）
docker compose build

# 3. 启动服务
docker compose up -d

# 4. 查看日志确认构建成功
docker compose logs -f new-api
```

### 方法二：使用 docker build

```bash
# 构建镜像
docker build -t metamind_new-api:latest .

# 运行容器
docker run -d \
  --name new-api \
  -p 3000:3000 \
  -v $(pwd)/data:/data \
  -v $(pwd)/logs:/app/logs \
  metamind_new-api:latest
```

### 方法三：无缓存构建（如果遇到问题）

```bash
# 清理缓存重新构建
docker compose build --no-cache

# 或者
docker build --no-cache -t metamind_new-api:latest .
```

## 验证支付宝 SDK 安装

### 1. 检查构建日志

```bash
# 查看依赖下载日志
docker compose build 2>&1 | grep -i alipay

# 应该看到类似输出：
# downloading github.com/smartwalle/alipay/v3 v3.6.0
```

### 2. 检查运行日志

```bash
# 启动后查看日志
docker compose logs new-api | grep -i alipay
```

### 3. 测试支付宝接口

```bash
# 获取支付信息
curl http://localhost:3000/api/user/topup/info

# 响应中应该包含：
# "enable_alipay_topup": true
```

## 常见问题和解决方案

### 问题 1: 依赖下载超时

**症状**：
```
timeout: github.com/xxx/xxx: Get "https://proxy.golang.org/...": dial tcp ...: i/o timeout
```

**解决方案**：

方法 A - 修改 Dockerfile 使用纯国内代理：
```dockerfile
ENV GOPROXY=https://goproxy.cn,direct
```

方法 B - 构建时传递代理参数：
```bash
docker compose build \
  --build-arg GOPROXY=https://goproxy.cn,direct
```

方法 C - 使用多个国内代理：
```dockerfile
ENV GOPROXY=https://goproxy.cn,https://goproxy.io,direct
```

### 问题 2: 支付宝 SDK 校验和失败

**症状**：
```
verifying go.sum: github.com/smartwalle/alipay/v3@v3.6.0: checksum mismatch
```

**解决方案**：

已在 Dockerfile 中配置：
```dockerfile
ENV GONOSUMDB=github.com/smartwalle/*
```

如果仍然失败，可以临时禁用校验：
```dockerfile
ENV GONOSUMCHECK=github.com/smartwalle/*
```

### 问题 3: 网络问题导致构建中断

**解决方案**：

方法 A - 使用国内镜像源加速：
```bash
# 在 docker compose.yml 中配置
services:
  new-api:
    build:
      context: .
      args:
        GOPROXY: "https://goproxy.cn,direct"
```

方法 B - 预先下载依赖：
```bash
# 在 Windows  PowerShell 中先下载依赖
$env:GOPROXY="https://goproxy.cn,direct"
go mod download

# 然后构建 Docker
docker compose build
```

### 问题 4: DNS 解析失败

**症状**：
```
dial tcp: lookup proxy.golang.org: no such host
```

**解决方案**：

在 Docker 中配置 DNS：
```bash
# 修改 /etc/docker/daemon.json (Linux)
# 或在 Docker Desktop 设置中配置 DNS
{
  "dns": ["8.8.8.8", "114.114.114.114"]
}
```

## 代理配置说明

### 可用的 Go 模块代理

| 代理地址 | 地区 | 速度 | 说明 |
|---------|------|------|------|
| `https://goproxy.cn` | 中国 | ⚡⚡⚡ | 推荐，七牛云提供 |
| `https://goproxy.io` | 中国 | ⚡⚡⚡ | 备用国内代理 |
| `https://proxy.golang.org` | 全球 | ⚡⚡ | Google 官方代理 |
| `https://goproxy.baidu.com` | 中国 | ⚡⚡ | 百度代理 |
| `direct` | - | - | 直接从源站下载 |

### 推荐配置

**中国大陆用户**：
```dockerfile
ENV GOPROXY=https://goproxy.cn,direct
```

**海外用户**：
```dockerfile
ENV GOPROXY=https://proxy.golang.org,direct
```

**混合配置（当前使用）**：
```dockerfile
ENV GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
```

## 构建优化建议

### 1. 使用构建缓存

```bash
# 首次构建（慢）
docker compose build

# 后续构建（快，使用缓存）
docker compose build
```

### 2. 多阶段构建优化

当前 Dockerfile 已使用多阶段构建：
- Stage 1: 前端构建（bun）
- Stage 2: 后端构建（golang）
- Stage 3: 运行环境（debian）

### 3. 清理无用镜像

```bash
# 清理悬空镜像
docker image prune

# 清理所有未使用的镜像
docker image prune -a
```

## 环境变量说明

### Dockerfile 中的环境变量

| 变量 | 说明 | 值 |
|------|------|-----|
| `GO111MODULE` | 启用 Go Modules | `on` |
| `CGO_ENABLED` | 禁用 CGO（静态编译） | `0` |
| `GOPROXY` | Go 模块代理 | `https://goproxy.cn,...` |
| `GONOSUMDB` | 跳过校验和检查 | `github.com/smartwalle/*` |
| `GOOS` | 目标操作系统 | `linux` |
| `GOARCH` | 目标架构 | `amd64` |

### 运行时环境变量

在 `docker compose.yml` 中配置：

```yaml
environment:
  - ALIPAY_ENABLED=true
  - ALIPAY_APP_ID=your_app_id
  - ALIPAY_PRIVATE_KEY=your_private_key
  - ALIPAY_PUBLIC_KEY=alipay_public_key
```

## 完整构建示例

```bash
#!/bin/bash
# build.sh - 完整构建脚本

echo "🚀 开始构建 New-API..."

# 1. 清理旧容器
echo "📦 清理旧容器..."
docker compose down

# 2. 构建镜像
echo "🔨 构建镜像..."
docker compose build --no-cache

# 3. 启动服务
echo "🚀 启动服务..."
docker compose up -d

# 4. 等待服务启动
echo "⏳ 等待服务启动..."
sleep 10

# 5. 检查服务状态
echo "✅ 检查服务状态..."
docker compose ps

# 6. 查看日志
echo "📋 查看日志..."
docker compose logs --tail=50 new-api

echo "✨ 构建完成！访问 http://localhost:3000"
```

使用方法：

```bash
chmod +x build.sh
./build.sh
```

## 故障排除

### 查看详细的构建日志

```bash
# 显示完整的构建输出
docker compose build --progress=plain 2>&1 | tee build.log

# 搜索错误
grep -i "error" build.log
grep -i "timeout" build.log
```

### 测试网络连接

```bash
# 在 Docker 构建环境中测试网络
docker run --rm golang:1.26.1-alpine \
  sh -c "apk add curl && curl -I https://goproxy.cn"
```

### 手动下载依赖

如果自动下载失败，可以手动下载：

```bash
# 1. 在 Windows 上下载
$env:GOPROXY="https://goproxy.cn,direct"
go get github.com/smartwalle/alipay/v3@v3.6.0

# 2. 检查 go.mod 和 go.sum 是否更新
cat go.mod | grep alipay

# 3. 重新构建 Docker
docker compose build
```

## 性能对比

| 方案 | 下载速度 | 稳定性 | 推荐指数 |
|------|---------|--------|---------|
| 直连（翻墙） | ⭐⭐ | ⭐⭐ | ⭐⭐ |
| goproxy.cn | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| proxy.golang.org | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| 多个代理组合 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

## 总结

✅ **当前配置已优化**：
- 使用 `goproxy.cn` 作为主要代理
- 备用 `proxy.golang.org` 代理
- 智能回退到直连
- 支付宝 SDK 跳过校验和检查

✅ **构建命令**：
```bash
docker compose build
docker compose up -d
```

✅ **验证方法**：
```bash
curl http://localhost:3000/api/user/topup/info
```

如有问题，请查看详细日志：
```bash
docker compose logs -f new-api
```

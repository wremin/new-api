#!/bin/bash

echo "========================================="
echo "  New-API 部署脚本 v8.2"
echo "========================================="

# 1. 检查 Docker 是否安装
if ! command -v docker &> /dev/null; then
    echo "❌ Docker 未安装，正在安装..."
    sudo apt update
    sudo apt install -y docker.io docker-compose
    sudo systemctl enable docker
    sudo systemctl start docker
    sudo usermod -aG docker $USER
    echo "✅ Docker 安装完成"
else
    echo "✅ Docker 已安装: $(docker --version)"
fi

# 2. 创建部署目录
DEPLOY_DIR="/opt/new-api"
sudo mkdir -p $DEPLOY_DIR
cd $DEPLOY_DIR

# 3. 创建 docker-compose.yml
echo "📝 创建 docker-compose.yml..."
sudo tee docker-compose.yml > /dev/null << 'EOF'
version: '3.8'

services:
  new-api:
    image: crpi-fel7tio2diby8fhl.cn-shenzhen.personal.cr.aliyuncs.com/wangxm42/new-api:v8.2
    container_name: new-api
    restart: always
    ports:
      - "3000:3000"
    environment:
      - SQL_DSN=root:newapi_password@tcp(mysql:3306)/new-api?charset=utf8mb4&parseTime=true
      - REDIS_CONN_STRING=redis:6379
      - SESSION_SECRET=newapi-session-secret-change-me
      - TZ=Asia/Shanghai
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_started
    networks:
      - new-api-network

  mysql:
    image: mysql:8.0
    container_name: new-api-mysql
    restart: always
    environment:
      - MYSQL_ROOT_PASSWORD=newapi_password
      - MYSQL_DATABASE=new-api
      - MYSQL_CHARSET=utf8mb4
    volumes:
      - mysql-data:/var/lib/mysql
    ports:
      - "3306:3306"
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      timeout: 20s
      retries: 10
    networks:
      - new-api-network

  redis:
    image: redis:7-alpine
    container_name: new-api-redis
    restart: always
    command: redis-server --appendonly yes
    volumes:
      - redis-data:/data
    ports:
      - "6379:6379"
    networks:
      - new-api-network

volumes:
  mysql-data:
    driver: local
  redis-data:
    driver: local

networks:
  new-api-network:
    driver: bridge
EOF

echo "✅ docker-compose.yml 创建完成"

# 4. 拉取镜像
echo "📦 拉取 Docker 镜像..."
sudo docker pull crpi-fel7tio2diby8fhl.cn-shenzhen.personal.cr.aliyuncs.com/wangxm42/new-api:v8.2

# 5. 启动服务
echo "🚀 启动 New-API 服务..."
sudo docker-compose up -d

# 6. 等待服务启动
echo "⏳ 等待服务启动..."
sleep 10

# 7. 检查服务状态
echo ""
echo "========================================="
echo "  服务状态检查"
echo "========================================="
sudo docker-compose ps

# 8. 查看日志
echo ""
echo "========================================="
echo "  New-API 最新日志"
echo "========================================="
sudo docker logs new-api --tail 20

echo ""
echo "========================================="
echo "  ✅ 部署完成！"
echo "========================================="
echo "📍 访问地址: http://3.26.235.238:3000"
echo "📝 查看日志: sudo docker logs -f new-api"
echo "🔄 重启服务: sudo docker-compose restart"
echo "🛑 停止服务: sudo docker-compose down"
echo "========================================="

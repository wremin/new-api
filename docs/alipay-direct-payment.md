# 支付宝直连支付模块

## 概述

支付宝直连支付模块实现了与支付宝官方 API 的直接对接，无需通过第三方支付聚合平台（如易支付）。该模块支持电脑网站支付、手机网站支付和扫码支付等多种支付方式。

## 功能特性

- ✅ 电脑网站支付（FAST_INSTANT_TRADE_PAY）
- ✅ 手机网站支付（QUICK_WAP_WAY）
- ✅ 扫码支付（当面付）
- ✅ 异步通知签名验证
- ✅ 订单状态查询
- ✅ 订单关闭
- ✅ 完整的错误处理和日志记录

## 文件结构

```
setting/
  └── payment_alipay.go      # 支付宝配置文件
  
service/
  └── alipay.go              # 支付宝服务层实现
  
controller/
  └── topup_alipay.go        # 支付宝控制器
  
router/
  └── api-router.go          # 路由配置（已更新）
```

## 前置准备

### 1. 注册支付宝开放平台账号

1. 访问 [支付宝开放平台](https://open.alipay.com/)
2. 注册企业账号
3. 完成实名认证

### 2. 创建应用

1. 登录支付宝开放平台控制台
2. 进入"开发者中心" -> "网页/移动应用"
3. 点击"创建应用"
4. 填写应用信息并提交审核

### 3. 签约支付产品

根据需要签约相应的支付产品：
- **电脑网站支付** - PC 端网页支付
- **手机网站支付** - 移动端网页支付
- **当面付** - 扫码支付

### 4. 配置密钥

#### 生成应用密钥对

使用支付宝提供的密钥生成工具：

```bash
# 使用 OpenSSL 生成 RSA2 密钥（推荐）
openssl genrsa -out app_private_key.pem 2048
openssl rsa -in app_private_key.pem -pubout -out app_public_key.pem
```

#### 配置应用公钥

1. 在应用详情中点击"设置应用公钥"
2. 上传生成的应用公钥（`app_public_key.pem`）
3. 保存后获取**支付宝公钥**

### 5. 获取配置信息

需要准备以下信息：
- **APPID** - 应用 APPID
- **应用私钥** - RSA2 私钥
- **支付宝公钥** - 从支付宝开放平台获取

## 配置方法

### 方式一：环境变量配置

在 `.env` 文件中添加：

```env
# 支付宝直连支付配置
ALIPAY_APP_ID=your_app_id
ALIPAY_PRIVATE_KEY=your_private_key
ALIPAY_PUBLIC_KEY=alipay_public_key
ALIPAY_ENABLED=true
ALIPAY_MIN_TOPUP=1
```

### 方式二：数据库配置

通过管理后台的"选项管理"页面配置：

| 选项键 | 说明 | 示例值 |
|--------|------|--------|
| `AlipayAppID` | 支付宝应用 APPID | `2021001234567890` |
| `AlipayPrivateKey` | 应用私钥（RSA2） | `MIIEvQIBADANBgkqhkiG9w0BAQE...` |
| `AlipayPublicKey` | 支付宝公钥 | `MIIBIjANBgkqhkiG9w0BAQE...` |
| `AlipayEnabled` | 是否启用 | `true` |
| `AlipayMinTopUp` | 最小充值金额（元） | `1` |
| `AlipayNotifyURL` | 异步通知地址（可选） | `https://yourdomain.com/api/user/alipay/notify` |
| `AlipayReturnURL` | 同步返回地址（可选） | `https://yourdomain.com/console/log` |

### 方式三：代码配置

直接修改 `setting/payment_alipay.go` 文件：

```go
var (
    AlipayAppID = "2021001234567890"
    AlipayPrivateKey = "MIIEvQIBADANBgkqhkiG9w0BAQE..."
    AlipayPublicKey = "MIIBIjANBgkqhkiG9w0BAQE..."
    AlipayEnabled = true
    AlipayMinTopUp = 1
)
```

## API 接口

### 1. 获取充值信息

**接口**: `GET /api/user/topup/info`

**响应**:
```json
{
  "code": 0,
  "data": {
    "enable_alipay_topup": true,
    "alipay_min_topup": 1,
    "pay_methods": [
      {
        "name": "支付宝",
        "type": "alipay_direct",
        "color": "rgba(var(--semi-blue-5), 1)",
        "min_topup": "1"
      }
    ]
  }
}
```

### 2. 发起支付宝支付

**接口**: `POST /api/user/alipay/pay`

**请求头**:
```
Authorization: Bearer YOUR_TOKEN
Content-Type: application/json
```

**请求体**:
```json
{
  "amount": 100
}
```

**响应**:
```json
{
  "message": "success",
  "data": {
    "pay_url": "https://openapi.alipay.com/gateway.do?...",
    "trade_no": "ALIPAYabc1231234567890"
  }
}
```

### 3. 支付宝异步通知

**接口**: `POST /api/alipay/notify`

此接口由支付宝服务器自动调用，无需手动触发。

### 4. 支付宝同步返回

**接口**: `GET /api/alipay/return`

用户支付完成后从支付宝跳转至此接口。

## 使用示例

### JavaScript 示例

```javascript
// 发起支付宝支付
async function requestAlipay(amount) {
  try {
    const response = await fetch('/api/user/alipay/pay', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ amount })
    });
    
    const result = await response.json();
    
    if (result.message === 'success') {
      // 跳转到支付宝支付页面
      window.location.href = result.data.pay_url;
    } else {
      console.error('创建订单失败:', result.data);
    }
  } catch (error) {
    console.error('请求失败:', error);
  }
}

// 使用示例
requestAlipay(100); // 充值 100 单位
```

### cURL 示例

```bash
# 发起支付
curl -X POST "https://yourdomain.com/api/user/alipay/pay" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"amount": 100}'

# 响应示例
{
  "message": "success",
  "data": {
    "pay_url": "https://openapi.alipay.com/gateway.do?...",
    "trade_no": "ALIPAYabc1231234567890"
  }
}
```

## 支付流程

```
1. 用户选择充值金额
   ↓
2. 前端调用 POST /api/user/alipay/pay
   ↓
3. 后端创建订单并返回支付 URL
   ↓
4. 前端跳转到支付宝支付页面
   ↓
5. 用户完成支付
   ↓
6. 支付宝异步通知 POST /api/alipay/notify
   ↓
7. 后端验证签名并处理充值
   ↓
8. 更新订单状态和用户余额
   ↓
9. 返回 success 给支付宝
```

## 订单状态说明

| 状态 | 说明 |
|------|------|
| `pending` | 订单待支付 |
| `success` | 支付成功 |
| `failed` | 支付失败或关闭 |

## 异步通知状态

| trade_status | 说明 |
|--------------|------|
| `TRADE_SUCCESS` | 交易成功 |
| `TRADE_FINISHED` | 交易完成 |
| `TRADE_CLOSED` | 交易关闭 |
| `WAIT_BUYER_PAY` | 等待买家付款 |

## 安全说明

### 1. 签名验证

所有异步通知都会进行签名验证，确保请求来自支付宝：

```go
// 验证签名
if !alipayService.VerifyNotify(params) {
    log.Printf("Alipay Webhook 签名验证失败")
    c.String(200, "fail")
    return
}
```

### 2. 订单锁

使用订单锁防止重复处理：

```go
LockOrder(tradeNo)
defer UnlockOrder(tradeNo)
```

### 3. 金额精度

使用 `decimal` 包确保金额计算精度：

```go
import "github.com/shopspring/decimal"

payMoney := decimal.NewFromFloat(amount).Mul(price).Round(2)
```

## 常见问题

### Q1: 签名验证失败怎么办？

**原因**:
- 应用私钥和公钥不匹配
- 支付宝公钥配置错误
- 参数被篡改

**解决方案**:
1. 检查密钥配置是否正确
2. 重新生成密钥对并配置
3. 查看日志确认具体错误信息

### Q2: 异步通知收不到怎么办？

**原因**:
- 回调地址配置错误
- 服务器无法外网访问
- 防火墙拦截

**解决方案**:
1. 确认 `AlipayNotifyURL` 配置正确
2. 确保服务器可以被外网访问
3. 检查防火墙和安全组设置
4. 在支付宝开放平台测试回调地址

### Q3: 支付成功但订单未充值？

**原因**:
- 异步通知处理失败
- 订单状态异常
- 数据库操作失败

**解决方案**:
1. 查看服务器日志
2. 检查订单状态
3. 手动执行充值（如有必要）
4. 联系技术支持

## 测试模式

支付宝提供沙箱环境用于测试：

1. 访问 [支付宝沙箱](https://openhome.alipay.com/develop/sandbox/app)
2. 使用沙箱 APPID 和密钥
3. 修改 `service/alipay.go` 中的初始化参数：

```go
// 第三个参数改为 false 使用沙箱环境
client, err := alipay.New(appID, privateKey, false)
```

## 依赖说明

需要安装支付宝 Go SDK：

```bash
go get github.com/smartwalle/alipay/v3
```

## 监控和日志

### 关键日志

```go
// 订单创建成功
log.Printf("Alipay 订单创建成功 - 用户: %d, 订单: %s, 金额: %.2f, 额度: %d", 
    userId, tradeNo, payMoney, amount)

// 充值成功
log.Printf("Alipay 充值成功 - 订单: %s, 用户: %d, 金额: %.2f", 
    tradeNo, topUp.UserId, topUp.Money)

// 签名验证失败
log.Printf("Alipay Webhook 签名验证失败")
```

### 监控指标

建议监控以下指标：
- 订单创建成功率
- 支付成功率
- 异步通知处理成功率
- 平均处理时间

## 更新日志

### v1.0.0 (2026-04-17)
- ✨ 初始版本发布
- ✅ 支持电脑网站支付
- ✅ 支持异步通知验证
- ✅ 完整的订单管理

## 技术支持

- 支付宝开放平台文档: https://opendocs.alipay.com/
- 项目 GitHub: https://github.com/QuantumNous/new-api

## 许可证

本项目采用 GNU Affero General Public License v3.0 许可证。

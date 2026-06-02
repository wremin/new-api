package controller

import (
	"fmt"
	"log"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// AlipayPayRequest 支付宝支付请求
type AlipayPayRequest struct {
	Amount int64 `json:"amount"`
}

// RequestAlipayPay 发起支付宝支付
func RequestAlipayPay(c *gin.Context) {
	if !setting.AlipayEnabled {
		c.JSON(200, gin.H{"message": "error", "data": "支付宝支付未启用"})
		return
	}

	if setting.AlipayAppID == "" || setting.AlipayPrivateKey == "" || setting.AlipayPublicKey == "" {
		c.JSON(200, gin.H{"message": "error", "data": "支付宝支付配置不完整"})
		return
	}

	var req AlipayPayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	// 检查最小充值金额
	minTopUp := int64(setting.AlipayMinTopUp)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopUp := decimal.NewFromInt(int64(setting.AlipayMinTopUp))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopUp = dMinTopUp.Mul(dQuotaPerUnit).IntPart()
	}

	if req.Amount < minTopUp {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值金额不能小于 %d", minTopUp)})
		return
	}

	userId := c.GetInt("id")
	group, err := model.GetUserGroup(userId, true)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to get user group: %s", err.Error()))
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	// 计算支付金额
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	// 生成订单号
	tradeNo := fmt.Sprintf("ALIPAY%s%d", common.GetRandomString(6), time.Now().Unix())

	// 创建支付宝服务
	alipayService, err := service.NewAlipayService(
		setting.AlipayAppID,
		setting.AlipayPrivateKey,
		setting.AlipayPublicKey,
	)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create alipay service: %s", err.Error()))
		c.JSON(200, gin.H{"message": "error", "data": "支付配置错误"})
		return
	}

	// 构建回调地址
	callBackAddress := service.GetCallbackAddress()
	notifyURL := callBackAddress + "/api/user/alipay/notify"
	returnURL := system_setting.ServerAddress + "/console/log"

	// 使用配置的回调地址（如果有）
	if setting.AlipayNotifyURL != "" {
		notifyURL = setting.AlipayNotifyURL
	}
	if setting.AlipayReturnURL != "" {
		returnURL = setting.AlipayReturnURL
	}

	// 创建支付订单
	payURL, err := alipayService.CreateTradePage(
		tradeNo,
		fmt.Sprintf("账户充值 - %d 额度", req.Amount),
		fmt.Sprintf("%.2f", payMoney),
		notifyURL,
		returnURL,
	)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create alipay order: %s", err.Error()))
		c.JSON(200, gin.H{"message": "error", "data": "创建支付订单失败"})
		return
	}

	// 计算实际充值额度
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(int64(amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}

	// 保存订单记录
	topUp := &model.TopUp{
		UserId:        userId,
		Amount:        amount,
		Money:         payMoney,
		TradeNo:       tradeNo,
		PaymentMethod: "alipay",
		CreateTime:    time.Now().Unix(),
		Status:        "pending",
	}
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create topup order: %s", err.Error()))
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	log.Printf("Alipay 订单创建成功 - 用户: %d, 订单: %s, 金额: %.2f, 额度: %d", userId, tradeNo, payMoney, amount)

	c.JSON(200, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_url":  payURL,
			"trade_no": tradeNo,
		},
	})
}

// AlipayNotify 支付宝异步回调通知
func AlipayNotify(c *gin.Context) {
	if !setting.AlipayEnabled {
		c.String(200, "fail")
		return
	}

	// 创建支付宝服务
	alipayService, err := service.NewAlipayService(
		setting.AlipayAppID,
		setting.AlipayPrivateKey,
		setting.AlipayPublicKey,
	)
	if err != nil {
		log.Printf("Alipay Webhook 服务初始化失败: %v", err)
		c.String(200, "fail")
		return
	}

	// 解析回调参数
	c.Request.ParseForm()
	params := make(map[string]string)
	for key, values := range c.Request.Form {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}

	// 验证签名
	if !alipayService.VerifyNotify(params) {
		log.Printf("Alipay Webhook 签名验证失败")
		c.String(200, "fail")
		return
	}

	// 获取订单信息
	tradeNo := params["out_trade_no"]
	tradeStatus := params["trade_status"]
	
	if tradeNo == "" {
		log.Printf("Alipay Webhook 缺少订单号")
		c.String(200, "fail")
		return
	}

	log.Printf("Alipay Webhook - 订单: %s, 状态: %s", tradeNo, tradeStatus)

	// 锁定订单，防止重复处理
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	// 处理支付成功
	if tradeStatus == "TRADE_SUCCESS" || tradeStatus == "TRADE_FINISHED" {
		if topUp := model.GetTopUpByTradeNo(tradeNo); topUp != nil {
			// 校验实付金额与订单金额一致（单位均为元），防止金额被篡改或不匹配
			paidAmount, perr := decimal.NewFromString(params["total_amount"])
			expected := decimal.NewFromFloat(topUp.Money)
			if perr != nil || paidAmount.Sub(expected).Abs().GreaterThan(decimal.NewFromFloat(0.005)) {
				log.Printf("Alipay Webhook 金额不匹配 - 订单: %s, 期望: %.2f, 实付: %q", tradeNo, topUp.Money, params["total_amount"])
				c.String(200, "fail")
				return
			}
			if topUp.Status == "pending" {
				// 执行充值
				if err := model.RechargeByTradeNo(tradeNo); err != nil {
					log.Printf("Alipay 充值处理失败: %v, 订单: %s", err, tradeNo)
					c.String(200, "fail")
					return
				}
				log.Printf("Alipay 充值成功 - 订单: %s, 用户: %d, 金额: %.2f", tradeNo, topUp.UserId, topUp.Money)
			}
		} else {
			log.Printf("Alipay 订单不存在: %s", tradeNo)
			c.String(200, "fail")
			return
		}
	} else if tradeStatus == "TRADE_CLOSED" {
		// 处理订单关闭
		if topUp := model.GetTopUpByTradeNo(tradeNo); topUp != nil && topUp.Status == "pending" {
			topUp.Status = "failed"
			_ = topUp.Update()
			log.Printf("Alipay 订单关闭 - 订单: %s", tradeNo)
		}
	}

	// 返回成功响应
	c.String(200, "success")
}

// AlipayReturn 支付宝同步返回
func AlipayReturn(c *gin.Context) {
	// 同步返回不做业务处理，仅跳转到结果页面
	// 实际支付结果以异步通知为准
	c.Redirect(302, system_setting.ServerAddress+"/console/log")
}

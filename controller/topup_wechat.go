package controller

import (
	"fmt"
	"log"
	"strings"
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

// WechatPayRequest 微信支付请求
type WechatPayRequest struct {
	Amount int64 `json:"amount"`
}

// RequestWechatPay 发起微信支付
func RequestWechatPay(c *gin.Context) {
	if !setting.WechatPayEnabled {
		c.JSON(200, gin.H{"message": "error", "data": "微信支付未启用"})
		return
	}

	if setting.WechatPayAppID == "" || setting.WechatPayMchID == "" || setting.WechatPayKey == "" {
		c.JSON(200, gin.H{"message": "error", "data": "微信支付配置不完整"})
		return
	}

	var req WechatPayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	// 检查最小充值金额
	minTopUp := int64(setting.WechatPayMinTopUp)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopUp := decimal.NewFromInt(int64(setting.WechatPayMinTopUp))
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
	tradeNo := fmt.Sprintf("WXPAY%s%d", common.GetRandomString(6), time.Now().Unix())

	// 构建回调地址
	callBackAddress := service.GetCallbackAddress()
	notifyURL := callBackAddress + "/api/wechatpay/notify"

	if setting.WechatPayNotifyURL != "" {
		notifyURL = setting.WechatPayNotifyURL
	}

	// 创建微信支付服务
	wechatPayService, err := service.NewWechatPayService(
		setting.WechatPayAppID,
		setting.WechatPayMchID,
		setting.WechatPayKey,
		notifyURL,
	)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create wechat pay service: %s", err.Error()))
		c.JSON(200, gin.H{"message": "error", "data": "支付配置错误"})
		return
	}

	// 获取客户端IP
	clientIP := c.ClientIP()
	if clientIP == "" {
		clientIP = "127.0.0.1"
	}

	// 金额单位为分（使用 decimal 精确换算并四舍五入，避免浮点截断少收费用）
	totalFee := wechatTotalFee(payMoney)

	// 计算实际充值额度
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(int64(amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}

	// 根据User-Agent判断是手机端还是PC端
	userAgent := c.GetHeader("User-Agent")
	isMobile := isMobileUserAgent(userAgent)

	var payURL string
	if isMobile {
		// H5支付
		sceneInfo := fmt.Sprintf(`{"h5_info":{"type":"Wap","app_name":"%s","package_name":"%s"}}`, common.SystemName, common.SystemName)
		payURL, err = wechatPayService.CreateH5Order(
			tradeNo,
			fmt.Sprintf("账户充值 - %d 额度", req.Amount),
			fmt.Sprintf("%d", totalFee),
			clientIP,
			notifyURL,
			sceneInfo,
		)
	} else {
		// Native扫码支付
		payURL, err = wechatPayService.CreateNativeOrder(
			tradeNo,
			fmt.Sprintf("账户充值 - %d 额度", req.Amount),
			fmt.Sprintf("%d", totalFee),
			clientIP,
			notifyURL,
		)
	}

	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create wechat pay order: %s", err.Error()))
		c.JSON(200, gin.H{"message": "error", "data": "创建支付订单失败"})
		return
	}

	// 保存订单记录
	topUp := &model.TopUp{
		UserId:        userId,
		Amount:        amount,
		Money:         payMoney,
		TradeNo:       tradeNo,
		PaymentMethod: "wxpay",
		CreateTime:    time.Now().Unix(),
		Status:        "pending",
	}
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create topup order: %s", err.Error()))
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	log.Printf("WechatPay 订单创建成功 - 用户: %d, 订单: %s, 金额: %.2f, 额度: %d", userId, tradeNo, payMoney, amount)

	c.JSON(200, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_url":   payURL,
			"trade_no":  tradeNo,
			"is_native": !isMobile,
		},
	})
}

// WechatPayNotify 微信支付异步回调通知
func WechatPayNotify(c *gin.Context) {
	if !setting.WechatPayEnabled {
		c.String(200, "fail")
		return
	}

	// 创建微信支付服务
	wechatPayService, err := service.NewWechatPayService(
		setting.WechatPayAppID,
		setting.WechatPayMchID,
		setting.WechatPayKey,
		"",
	)
	if err != nil {
		log.Printf("WechatPay Webhook 服务初始化失败: %v", err)
		c.String(200, "fail")
		return
	}

	// 解析回调通知
	result, err := wechatPayService.ParseNotify(c.Request)
	if err != nil {
		log.Printf("WechatPay Webhook 解析失败: %v", err)
		c.String(200, "fail")
		return
	}

	// 验证签名
	if !wechatPayService.VerifyPaidSign(result) {
		log.Printf("WechatPay Webhook 签名验证失败")
		c.String(200, "fail")
		return
	}

	// 检查返回码
	if result.ReturnCode == nil || *result.ReturnCode != "SUCCESS" {
		log.Printf("WechatPay Webhook 返回码错误")
		c.String(200, "fail")
		return
	}

	// 获取订单信息
	tradeNo := ""
	if result.OutTradeNo != nil {
		tradeNo = *result.OutTradeNo
	}
	resultCode := ""
	if result.ResultCode != nil {
		resultCode = *result.ResultCode
	}

	if tradeNo == "" {
		log.Printf("WechatPay Webhook 缺少订单号")
		c.String(200, "fail")
		return
	}

	log.Printf("WechatPay Webhook - 订单: %s, 结果: %s", tradeNo, resultCode)

	// 锁定订单，防止重复处理
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	// 处理支付成功
	if resultCode == "SUCCESS" {
		if topUp := model.GetTopUpByTradeNo(tradeNo); topUp != nil {
			// 校验实付金额与订单应付金额一致，防止金额被篡改或不匹配
			expectedFee := wechatTotalFee(topUp.Money)
			if result.TotalFee == nil || *result.TotalFee != expectedFee {
				paid := -1
				if result.TotalFee != nil {
					paid = *result.TotalFee
				}
				log.Printf("WechatPay Webhook 金额不匹配 - 订单: %s, 期望: %d, 实付: %d", tradeNo, expectedFee, paid)
				c.String(200, "fail")
				return
			}
			if topUp.Status == "pending" {
				// 执行充值
				if err := model.RechargeWechat(tradeNo); err != nil {
					log.Printf("WechatPay 充值处理失败: %v, 订单: %s", err, tradeNo)
					c.String(200, "fail")
					return
				}
				log.Printf("WechatPay 充值成功 - 订单: %s, 用户: %d, 金额: %.2f", tradeNo, topUp.UserId, topUp.Money)
			}
		} else {
			log.Printf("WechatPay 订单不存在: %s", tradeNo)
			c.String(200, "fail")
			return
		}
	}

	// 返回成功响应
	c.Header("Content-Type", "application/xml")
	c.String(200, "<xml><return_code><![CDATA[SUCCESS]]></return_code><return_msg><![CDATA[OK]]></return_msg></xml>")
}

// WechatPayReturn 微信支付同步返回
func WechatPayReturn(c *gin.Context) {
	// 同步返回不做业务处理，仅跳转到结果页面
	c.Redirect(302, system_setting.ServerAddress+"/console/log")
}

// wechatTotalFee 将充值金额（元）精确换算为微信支付所需的金额（分）。
// 使用 decimal 四舍五入，避免 int(money*100) 的浮点截断导致少收 1 分。
func wechatTotalFee(money float64) int {
	fee := int(decimal.NewFromFloat(money).Mul(decimal.NewFromInt(100)).Round(0).IntPart())
	if fee < 1 {
		fee = 1
	}
	return fee
}

// reconcileWechatOrder 主动向微信查单并结算订单，作为异步回调丢失时的兜底。
// 仅在订单仍处于 pending 且查得微信侧已支付（TradeState=SUCCESS）时才入账，
// 并校验实付金额，确保与异步回调路径一致的安全性与幂等性。
func reconcileWechatOrder(tradeNo string) {
	if !setting.WechatPayEnabled {
		return
	}
	if setting.WechatPayAppID == "" || setting.WechatPayMchID == "" || setting.WechatPayKey == "" {
		return
	}

	wechatPayService, err := service.NewWechatPayService(
		setting.WechatPayAppID,
		setting.WechatPayMchID,
		setting.WechatPayKey,
		"",
	)
	if err != nil {
		log.Printf("WechatPay 查单服务初始化失败: %v", err)
		return
	}

	result, err := wechatPayService.QueryOrder(tradeNo)
	if err != nil {
		log.Printf("WechatPay 主动查单失败 - 订单: %s, 错误: %v", tradeNo, err)
		return
	}

	// 通信标识与业务结果均需成功，且交易状态必须为已支付
	if result.ReturnCode == nil || *result.ReturnCode != "SUCCESS" {
		return
	}
	if result.ResultCode == nil || *result.ResultCode != "SUCCESS" {
		return
	}
	if result.TradeState == nil || *result.TradeState != "SUCCESS" {
		// NOTPAY / CLOSED / USERPAYING 等，未支付成功，直接返回
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.Status != "pending" {
		return
	}

	// 校验实付金额，防止金额不一致
	expectedFee := wechatTotalFee(topUp.Money)
	if result.TotalFee == nil || *result.TotalFee != expectedFee {
		paid := -1
		if result.TotalFee != nil {
			paid = *result.TotalFee
		}
		log.Printf("WechatPay 查单金额不匹配 - 订单: %s, 期望: %d, 实付: %d", tradeNo, expectedFee, paid)
		return
	}

	if err := model.RechargeWechat(tradeNo); err != nil {
		log.Printf("WechatPay 查单结算失败 - 订单: %s, 错误: %v", tradeNo, err)
		return
	}
	log.Printf("WechatPay 查单结算成功 - 订单: %s, 用户: %d, 金额: %.2f", tradeNo, topUp.UserId, topUp.Money)
}

// isMobileUserAgent 根据User-Agent判断是否为移动设备
func isMobileUserAgent(userAgent string) bool {
	if userAgent == "" {
		return false
	}
	mobileKeywords := []string{
		"Android", "iPhone", "iPad", "iPod", "Windows Phone",
		"Mobile", "BlackBerry", "Opera Mini", "Symbian",
	}
	lowerUA := userAgent
	for _, keyword := range mobileKeywords {
		if strings.Contains(lowerUA, keyword) {
			return true
		}
	}
	return false
}

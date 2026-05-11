package setting

// 支付宝直连支付配置
// Alipay Direct Payment Configuration

var (
	// AlipayAppID 支付宝应用 APPID
	AlipayAppID = ""
	
	// AlipayPrivateKey 应用私钥（RSA2）
	AlipayPrivateKey = ""
	
	// AlipayPublicKey 支付宝公钥
	AlipayPublicKey = ""
	
	// AlipayNotifyURL 异步通知地址（可选，默认自动生成）
	AlipayNotifyURL = ""
	
	// AlipayReturnURL 同步返回地址（可选，默认自动生成）
	AlipayReturnURL = ""
	
	// AlipayMinTopUp 最小充值金额（元）
	AlipayMinTopUp = 1
	
	// AlipayEnabled 是否启用支付宝支付
	AlipayEnabled = false
)

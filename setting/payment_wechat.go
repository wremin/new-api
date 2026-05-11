package setting

// 微信支付直连配置
// WeChat Pay Direct Payment Configuration

var (
	// WechatPayEnabled 是否启用微信支付
	WechatPayEnabled = false

	// WechatPayAppID 微信应用 APPID
	WechatPayAppID = ""

	// WechatPayMchID 微信支付商户号
	WechatPayMchID = ""

	// WechatPayKey 微信支付 API 密钥
	WechatPayKey = ""

	// WechatPayNotifyURL 异步通知地址（可选，默认自动生成）
	WechatPayNotifyURL = ""

	// WechatPayReturnURL 同步返回地址（可选，默认自动生成）
	WechatPayReturnURL = ""

	// WechatPayMinTopUp 最小充值金额（元）
	WechatPayMinTopUp = 1
)

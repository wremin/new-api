package service

import (
	"context"
	"fmt"
	"net/url"

	"github.com/smartwalle/alipay/v3"
)

// AlipayService 支付宝支付服务
type AlipayService struct {
	client *alipay.Client
	appID  string
}

// NewAlipayService 创建支付宝服务实例
func NewAlipayService(appID, privateKey, publicKey string) (*AlipayService, error) {
	if appID == "" || privateKey == "" || publicKey == "" {
		return nil, fmt.Errorf("alipay configuration incomplete")
	}

	// 初始化支付宝客户端（true 表示生产环境，false 表示沙箱环境）
	client, err := alipay.New(appID, privateKey, true)
	if err != nil {
		return nil, fmt.Errorf("init alipay client failed: %w", err)
	}

	// 加载支付宝公钥
	err = client.LoadAliPayPublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("load alipay public key failed: %w", err)
	}

	return &AlipayService{
		client: client,
		appID:  appID,
	}, nil
}

// CreateTradePage 创建网页支付订单（电脑网站支付）
func (s *AlipayService) CreateTradePage(orderNo, subject, amount, notifyURL, returnURL string) (string, error) {
	var p = alipay.TradePagePay{}
	p.NotifyURL = notifyURL
	p.ReturnURL = returnURL
	p.Subject = subject
	p.OutTradeNo = orderNo
	p.TotalAmount = amount
	p.ProductCode = "FAST_INSTANT_TRADE_PAY"

	// 生成支付URL
	result, err := s.client.TradePagePay(p)
	if err != nil {
		return "", fmt.Errorf("create trade page failed: %w", err)
	}

	return result.String(), nil
}

// CreateTradeWAP 创建手机网站支付订单
func (s *AlipayService) CreateTradeWAP(orderNo, subject, amount, notifyURL, returnURL string) (string, error) {
	var p = alipay.TradeWapPay{}
	p.NotifyURL = notifyURL
	p.ReturnURL = returnURL
	p.Subject = subject
	p.OutTradeNo = orderNo
	p.TotalAmount = amount
	p.ProductCode = "QUICK_WAP_WAY"

	result, err := s.client.TradeWapPay(p)
	if err != nil {
		return "", fmt.Errorf("create trade wap failed: %w", err)
	}

	return result.String(), nil
}

// CreateTradePreCreate 创建扫码支付订单（当面付）
func (s *AlipayService) CreateTradePreCreate(orderNo, subject, amount, notifyURL string) (string, error) {
	var p = alipay.TradePreCreate{}
	p.NotifyURL = notifyURL
	p.Subject = subject
	p.OutTradeNo = orderNo
	p.TotalAmount = amount

	result, err := s.client.TradePreCreate(context.Background(), p)
	if err != nil {
		return "", fmt.Errorf("create trade pre create failed: %w", err)
	}

	// 返回二维码链接
	if result.QRCode != "" {
		return result.QRCode, nil
	}
	return "", nil
}

// VerifyNotify 验证异步通知签名
func (s *AlipayService) VerifyNotify(params map[string]string) bool {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	err := s.client.VerifySign(context.Background(), values)
	return err == nil
}

// QueryTrade 查询订单状态
func (s *AlipayService) QueryTrade(orderNo string) (interface{}, error) {
	var p = alipay.TradeQuery{}
	p.OutTradeNo = orderNo

	result, err := s.client.TradeQuery(context.Background(), p)
	if err != nil {
		return nil, fmt.Errorf("query trade failed: %w", err)
	}

	return result, nil
}

// CloseTrade 关闭订单
func (s *AlipayService) CloseTrade(orderNo string) error {
	var p = alipay.TradeClose{}
	p.OutTradeNo = orderNo

	_, err := s.client.TradeClose(context.Background(), p)
	if err != nil {
		return fmt.Errorf("close trade failed: %w", err)
	}

	return nil
}

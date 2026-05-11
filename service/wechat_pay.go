package service

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"

	"github.com/silenceper/wechat/v2/pay/config"
	"github.com/silenceper/wechat/v2/pay/notify"
	"github.com/silenceper/wechat/v2/pay/order"
)

// WechatPayService 微信支付服务
type WechatPayService struct {
	cfg    *config.Config
	order  *order.Order
	notify *notify.Notify
}

// NewWechatPayService 创建微信支付服务实例
func NewWechatPayService(appID, mchID, key, notifyURL string) (*WechatPayService, error) {
	if appID == "" || mchID == "" || key == "" {
		return nil, fmt.Errorf("wechat pay configuration incomplete")
	}

	cfg := &config.Config{
		AppID:     appID,
		MchID:     mchID,
		Key:       key,
		NotifyURL: notifyURL,
	}

	return &WechatPayService{
		cfg:    cfg,
		order:  order.NewOrder(cfg),
		notify: notify.NewNotify(cfg),
	}, nil
}

// CreateNativeOrder 创建微信Native扫码支付订单
func (s *WechatPayService) CreateNativeOrder(orderNo, body, totalFee, createIP, notifyURL string) (string, error) {
	params := &order.Params{
		TotalFee:   totalFee,
		CreateIP:   createIP,
		Body:       body,
		OutTradeNo: orderNo,
		TradeType:  "NATIVE",
		SignType:   "MD5",
		NotifyURL:  notifyURL,
	}

	preOrder, err := s.order.PrePayOrder(params)
	if err != nil {
		return "", fmt.Errorf("create native order failed: %w", err)
	}

	if preOrder.ReturnCode != order.SUCCESS {
		return "", fmt.Errorf("wechat pay return error: %s, %s", preOrder.ReturnCode, preOrder.ReturnMsg)
	}
	if preOrder.ResultCode != order.SUCCESS {
		return "", fmt.Errorf("wechat pay result error: %s, %s", preOrder.ErrCode, preOrder.ErrCodeDes)
	}

	return preOrder.CodeURL, nil
}

// CreateH5Order 创建微信H5支付订单
func (s *WechatPayService) CreateH5Order(orderNo, body, totalFee, createIP, notifyURL, sceneInfo string) (string, error) {
	params := &order.Params{
		TotalFee:   totalFee,
		CreateIP:   createIP,
		Body:       body,
		OutTradeNo: orderNo,
		TradeType:  "MWEB",
		SignType:   "MD5",
		NotifyURL:  notifyURL,
	}

	// scene_info 是H5支付必需的，格式为JSON字符串
	if sceneInfo != "" {
		params.Attach = sceneInfo
	}

	preOrder, err := s.order.PrePayOrder(params)
	if err != nil {
		return "", fmt.Errorf("create h5 order failed: %w", err)
	}

	if preOrder.ReturnCode != order.SUCCESS {
		return "", fmt.Errorf("wechat pay return error: %s, %s", preOrder.ReturnCode, preOrder.ReturnMsg)
	}
	if preOrder.ResultCode != order.SUCCESS {
		return "", fmt.Errorf("wechat pay result error: %s, %s", preOrder.ErrCode, preOrder.ErrCodeDes)
	}

	return preOrder.MWebURL, nil
}

// ParseNotify 解析微信支付异步通知
func (s *WechatPayService) ParseNotify(r *http.Request) (*notify.PaidResult, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read notify body failed: %w", err)
	}
	defer r.Body.Close()

	var result notify.PaidResult
	err = xml.Unmarshal(body, &result)
	if err != nil {
		return nil, fmt.Errorf("unmarshal notify xml failed: %w", err)
	}

	return &result, nil
}

// VerifyPaidSign 验证微信支付通知签名
func (s *WechatPayService) VerifyPaidSign(result *notify.PaidResult) bool {
	return s.notify.PaidVerifySign(*result)
}

// QueryOrder 查询订单状态
func (s *WechatPayService) QueryOrder(orderNo string) (*notify.PaidResult, error) {
	result, err := s.order.QueryOrder(&order.QueryParams{
		OutTradeNo: orderNo,
		SignType:   "MD5",
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

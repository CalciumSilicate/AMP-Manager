package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"ampmanager/internal/model"

	sdk "github.com/smartwalle/alipay/v3"
)

const purchaseOrderTimeout = 15 * time.Minute

type PaymentCreateResult struct {
	TradeNo   string
	QRCode    string
	QRURL     string
	ExpiresAt time.Time
}

type PaymentQueryResult struct {
	TradeNo       string
	PaymentStatus model.PurchasePaymentStatus
	PaidAt        *time.Time
}

type PaymentNotification struct {
	OrderNo       string
	TradeNo       string
	PaymentStatus model.PurchasePaymentStatus
	PaidAt        *time.Time
}

type AlipayService struct {
	settingsSvc *PurchaseSettingsService
}

func NewAlipayService() *AlipayService {
	return &AlipayService{
		settingsSvc: NewPurchaseSettingsService(),
	}
}

func (s *AlipayService) CreateOrder(ctx context.Context, order *model.PurchaseOrder, product *model.PurchaseProductResponse, username string) (*PaymentCreateResult, error) {
	settings, err := s.settingsSvc.Get()
	if err != nil {
		return nil, err
	}
	client, err := s.newClient(settings)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().UTC().Add(purchaseOrderTimeout)
	result, err := client.TradePreCreate(ctx, sdk.TradePreCreate{
		Trade: sdk.Trade{
			NotifyURL:      settings.AlipayNotifyURL,
			Subject:        s.buildSubject(product, username),
			OutTradeNo:     order.OrderNo,
			TotalAmount:    centsToYuanString(order.AmountCNYCent),
			ProductCode:    "FACE_TO_FACE_PAYMENT",
			Body:           strings.TrimSpace(product.Summary),
			TimeoutExpress: "15m",
			TimeExpire:     expiresAt.In(time.Local).Format("2006-01-02 15:04:05"),
		},
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("支付宝预下单失败")
	}
	if result.IsFailure() {
		return nil, fmt.Errorf("支付宝预下单失败: %s", alipayErrorMessage(result.Error))
	}
	if strings.TrimSpace(result.QRCode) == "" {
		return nil, fmt.Errorf("支付宝预下单成功但未返回二维码")
	}

	return &PaymentCreateResult{
		QRCode:    strings.TrimSpace(result.QRCode),
		QRURL:     strings.TrimSpace(result.QRCode),
		ExpiresAt: expiresAt,
	}, nil
}

func (s *AlipayService) QueryOrder(ctx context.Context, orderNo string) (*PaymentQueryResult, error) {
	settings, err := s.settingsSvc.Get()
	if err != nil {
		return nil, err
	}
	client, err := s.newClient(settings)
	if err != nil {
		return nil, err
	}

	result, err := client.TradeQuery(ctx, sdk.TradeQuery{
		OutTradeNo: orderNo,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("支付宝查单失败")
	}
	if result.IsFailure() {
		return nil, fmt.Errorf("支付宝查单失败: %s", alipayErrorMessage(result.Error))
	}

	queryResult := &PaymentQueryResult{
		TradeNo: strings.TrimSpace(result.TradeNo),
	}

	switch result.TradeStatus {
	case sdk.TradeStatusSuccess, sdk.TradeStatusFinished:
		queryResult.PaymentStatus = model.PurchasePaymentStatusPaid
		queryResult.PaidAt = parseAlipayTime(result.SendPayDate)
	case sdk.TradeStatusClosed:
		queryResult.PaymentStatus = model.PurchasePaymentStatusClosed
	default:
		queryResult.PaymentStatus = model.PurchasePaymentStatusPending
	}

	return queryResult, nil
}

func (s *AlipayService) DecodeNotification(ctx context.Context, values url.Values) (*PaymentNotification, error) {
	settings, err := s.settingsSvc.Get()
	if err != nil {
		return nil, err
	}
	client, err := s.newClient(settings)
	if err != nil {
		return nil, err
	}

	notification, err := client.DecodeNotification(ctx, values)
	if err != nil {
		return nil, err
	}
	if notification == nil {
		return nil, fmt.Errorf("支付宝回调解析失败")
	}
	if appID := strings.TrimSpace(notification.AppId); appID != "" && appID != settings.AlipayAppID {
		return nil, fmt.Errorf("支付宝回调 app_id 不匹配")
	}

	result := &PaymentNotification{
		OrderNo: strings.TrimSpace(notification.OutTradeNo),
		TradeNo: strings.TrimSpace(notification.TradeNo),
	}

	switch notification.TradeStatus {
	case sdk.TradeStatusSuccess, sdk.TradeStatusFinished:
		result.PaymentStatus = model.PurchasePaymentStatusPaid
		result.PaidAt = parseAlipayTime(notification.GmtPayment)
	case sdk.TradeStatusClosed:
		result.PaymentStatus = model.PurchasePaymentStatusClosed
	default:
		result.PaymentStatus = model.PurchasePaymentStatusPending
	}

	return result, nil
}

func (s *AlipayService) newClient(settings *model.PurchaseSettings) (*sdk.Client, error) {
	if settings == nil || !s.settingsSvc.IsAlipayConfigured(settings) {
		return nil, fmt.Errorf("支付宝配置不完整")
	}

	client, err := sdk.New(
		settings.AlipayAppID,
		settings.AlipayPrivateKey,
		settings.AlipayEnvironment == model.AlipayEnvironmentProduction,
	)
	if err != nil {
		return nil, err
	}
	if err := client.LoadAliPayPublicKey(settings.AlipayPublicKey); err != nil {
		return nil, err
	}
	return client, nil
}

func (s *AlipayService) buildSubject(product *model.PurchaseProductResponse, username string) string {
	if product == nil {
		return "AMP Manager 订阅"
	}

	name := strings.TrimSpace(product.Name)
	if name == "" {
		name = "订阅"
	}
	user := strings.TrimSpace(username)
	if user == "" {
		return "AMP Manager " + name
	}
	return fmt.Sprintf("AMP %s · %s", name, user)
}

func alipayErrorMessage(err sdk.Error) string {
	if strings.TrimSpace(err.SubMsg) != "" {
		return err.SubMsg
	}
	if strings.TrimSpace(err.Msg) != "" {
		return err.Msg
	}
	return string(err.Code)
}

func parseAlipayTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
	} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			utc := parsed.UTC()
			return &utc
		}
	}
	return nil
}

func centsToYuanString(value int64) string {
	return fmt.Sprintf("%.2f", float64(value)/100)
}

//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestRechargePromotionMasterSwitchDisablesNewOrdersAndConfigurationWrites(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	u, err := client.User.Create().SetEmail("switch@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	p := promotionForTest()
	p.StartsAt = time.Now().Add(-time.Hour).Format(time.RFC3339)
	p.EndsAt = time.Now().Add(time.Hour).Format(time.RFC3339)
	configSvc := NewPaymentConfigService(client, &paymentConfigSettingRepoStub{}, nil)
	require.Equal(t, "RECHARGE_PROMOTION_DISABLED", infraerrors.Reason(configSvc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{RechargePromotion: p})))
	svc := &PaymentService{entClient: client, configService: configSvc}
	o, err := svc.createOrderInTx(ctx, CreateOrderRequest{UserID: u.ID, Amount: 200, OrderType: "balance", PaymentType: "alipay"}, &User{ID: u.ID, Email: u.Email}, nil, &PaymentConfig{RechargePromotion: p, BalanceRechargeMultiplier: 1}, 200, 200, 0, 200, &payment.InstanceSelection{ProviderKey: "alipay"})
	require.NoError(t, err)
	require.Equal(t, 200.0, o.Amount)
	require.Empty(t, o.PromotionSnapshot)
	_, err = svc.GetPromotionSummary(ctx, PromotionOrderFilter{})
	require.Equal(t, "RECHARGE_PROMOTION_DISABLED", infraerrors.Reason(err))
	_, _, err = svc.ListPromotionOrders(ctx, PromotionOrderFilter{})
	require.Equal(t, "RECHARGE_PROMOTION_DISABLED", infraerrors.Reason(err))
}

func seedPromotionReports(t *testing.T) (*PaymentService, []*dbent.PaymentOrder) {
	t.Helper()
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	u, err := client.User.Create().SetEmail("campaign@example.com").SetUsername("Campaign User").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	created := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	type row struct {
		status, currency          string
		base, bonus, paid, refund float64
	}
	rows := []row{{OrderStatusCompleted, "CNY", 100, 10, 100, 0}, {OrderStatusRefunded, "CNY", 200, 20, 200, 220}, {OrderStatusPending, "CNY", 50, 5, 50, 0}, {OrderStatusRefundPending, "USD", 100, 10, 20, 110}, {OrderStatusPaid, "KWD", 100, 10, 12.345, 0}, {OrderStatusRefundFailed, "CNY", 50, 5, 50, 0}, {OrderStatusCompleted, "CNY", 100, 0, 100, 0}}
	orders := make([]*dbent.PaymentOrder, 0, len(rows))
	for i, r := range rows {
		b := client.PaymentOrder.Create().SetUserID(u.ID).SetUserEmail(u.Email).SetUserName(u.Username).SetAmount(r.base + r.bonus).SetPayAmount(r.paid).SetRefundAmount(r.refund).SetFeeRate(0).SetRechargeCode("PROMO-" + strconv.Itoa(i)).SetOutTradeNo("promo-report-" + strconv.Itoa(i)).SetPaymentType("stripe").SetPaymentTradeNo("trade-" + strconv.Itoa(i)).SetOrderType("balance").SetStatus(r.status).SetExpiresAt(created.Add(time.Hour)).SetCreatedAt(created.Add(time.Duration(i) * time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("example.com").SetProviderSnapshot(map[string]any{"currency": r.currency})
		if r.bonus > 0 {
			b.SetPromotionSnapshot(map[string]any{"base_amount": r.base, "bonus_amount": r.bonus, "title": "Holiday"})
		}
		if r.status != OrderStatusPending {
			b.SetPaidAt(created.Add(time.Minute))
		}
		o, err := b.Save(ctx)
		require.NoError(t, err)
		orders = append(orders, o)
	}
	return &PaymentService{entClient: client, configService: NewPaymentConfigService(client, &paymentConfigSettingRepoStub{values: map[string]string{SettingKeyRechargePromotionEnabled: "true"}}, nil)}, orders
}

func TestPaymentPromotionSummarySeparatesIssuedRefundsAndCurrencies(t *testing.T) {
	svc, _ := seedPromotionReports(t)
	summary, err := svc.GetPromotionSummary(context.Background(), PromotionOrderFilter{})
	require.NoError(t, err)
	require.Equal(t, 6, summary.OrderCount)
	require.Equal(t, 1, summary.PendingOrderCount)
	require.Equal(t, 4, summary.IssuedOrderCount)
	require.Equal(t, 1, summary.IssuedUserCount)
	require.Equal(t, 450.0, summary.BaseAmount)
	require.Equal(t, 45.0, summary.BonusAmount)
	require.Equal(t, 200.0, summary.RefundedBaseAmount)
	require.Equal(t, 20.0, summary.RefundedBonusAmount)
	require.Equal(t, 250.0, summary.NetBaseAmount)
	require.Equal(t, 25.0, summary.NetBonusAmount)
	require.Equal(t, []PromotionCashSummary{{Currency: "CNY", PaidAmount: 350, RefundedAmount: 200, NetAmount: 150}, {Currency: "KWD", PaidAmount: 12.345, NetAmount: 12.345}, {Currency: "USD", PaidAmount: 20, NetAmount: 20}}, summary.CashByCurrency)
}

func TestPaymentPromotionOrdersAndSummaryShareFilters(t *testing.T) {
	svc, seeded := seedPromotionReports(t)
	ctx := context.Background()
	start, end := seeded[1].CreatedAt, seeded[3].CreatedAt
	filter := PromotionOrderFilter{StartTime: &start, EndTime: &end, Page: 1, PageSize: 1, Keyword: "campaign@example.com"}
	orders, total, err := svc.ListPromotionOrders(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, orders, 1)
	require.Equal(t, seeded[2].ID, orders[0].ID)
	summary, err := svc.GetPromotionSummary(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, total, summary.OrderCount)
	filter.Page = 2
	orders, _, err = svc.ListPromotionOrders(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, seeded[1].ID, orders[0].ID)
	filter = PromotionOrderFilter{Status: OrderStatusRefundPending, Keyword: "promo-report-3"}
	orders, total, err = svc.ListPromotionOrders(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, seeded[3].ID, orders[0].ID)
	filter.UserID = 999
	_, total, err = svc.ListPromotionOrders(ctx, filter)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Error(t, ValidatePromotionOrderFilter(PromotionOrderFilter{Status: "invalid"}))
	require.Error(t, ValidatePromotionOrderFilter(PromotionOrderFilter{StartTime: &end, EndTime: &start}))
}

func TestPaymentPromotionSummaryCountsPartialRefundProportionally(t *testing.T) {
	svc, orders := seedPromotionReports(t)
	ctx := context.Background()
	_, err := svc.entClient.PaymentOrder.UpdateOneID(orders[0].ID).SetStatus(OrderStatusPartiallyRefunded).SetRefundAmount(55).Save(ctx)
	require.NoError(t, err)
	summary, err := svc.GetPromotionSummary(ctx, PromotionOrderFilter{Status: OrderStatusPartiallyRefunded})
	require.NoError(t, err)
	require.Equal(t, 50.0, summary.RefundedBaseAmount)
	require.Equal(t, 5.0, summary.RefundedBonusAmount)
	require.Equal(t, []PromotionCashSummary{{Currency: "CNY", PaidAmount: 100, RefundedAmount: 50, NetAmount: 50}}, summary.CashByCurrency)
}

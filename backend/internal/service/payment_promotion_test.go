//go:build unit

package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func promotionForTest() *RechargePromotion {
	return &RechargePromotion{Enabled: true, Title: "Holiday", Currency: "CNY", StartsAt: "2026-10-01T00:00:00+08:00", EndsAt: "2026-10-08T00:00:00+08:00", Tiers: []RechargePromotionTier{{MinAmount: 100, BonusPercent: 10}, {MinAmount: 200, BonusPercent: 20}}}
}

func TestRechargePromotionTierTimeCurrencyAndCap(t *testing.T) {
	p, err := normalizeRechargePromotion(promotionForTest())
	require.NoError(t, err)
	start, _ := time.Parse(time.RFC3339, p.StartsAt)
	end, _ := time.Parse(time.RFC3339, p.EndsAt)
	for _, tc := range []struct {
		name                string
		amount, base, bonus float64
		currency, orderType string
		now                 time.Time
	}{
		{"below threshold", 99.99, 15, 0, "CNY", "balance", start},
		{"inclusive threshold and start", 100, 15, 1.5, "CNY", "balance", start},
		{"highest tier only", 200, 30, 6, "CNY", "balance", start},
		{"round bonus to cents", 100, 10.05, 1.01, "CNY", "balance", start},
		{"wrong currency", 200, 30, 0, "USD", "balance", start},
		{"subscription excluded", 200, 30, 0, "CNY", "subscription", start},
		{"before start", 200, 30, 0, "CNY", "balance", start.Add(-time.Nanosecond)},
		{"exclusive end", 200, 30, 0, "CNY", "balance", end},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := buildRechargePromotionSnapshot(p, tc.orderType, tc.amount, tc.base, .15, tc.currency, tc.now)
			if tc.bonus == 0 {
				require.Nil(t, snapshot)
				return
			}
			require.Equal(t, tc.bonus, snapshot["bonus_amount"])
			require.Equal(t, tc.base, snapshot["base_amount"])
		})
	}
	p.MaxBonus = 2.35
	require.Equal(t, 2.35, buildRechargePromotionSnapshot(p, "balance", 200, 30, .15, "CNY", start)["bonus_amount"])
	p.Enabled = false
	require.Nil(t, buildRechargePromotionSnapshot(p, "balance", 200, 30, .15, "CNY", start))
}

func TestRechargePromotionConfigurationValidationAndPersistence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*RechargePromotion)
	}{
		{"duplicate threshold", func(p *RechargePromotion) { p.Tiers[1].MinAmount = 100 }},
		{"zero threshold", func(p *RechargePromotion) { p.Tiers[0].MinAmount = 0 }},
		{"nan percentage", func(p *RechargePromotion) { p.Tiers[0].BonusPercent = math.NaN() }},
		{"too much bonus", func(p *RechargePromotion) { p.Tiers[0].BonusPercent = 101 }},
		{"bad currency", func(p *RechargePromotion) { p.Currency = "YUAN" }},
		{"currency precision", func(p *RechargePromotion) { p.Currency = "JPY"; p.Tiers[0].MinAmount = 100.1 }},
		{"reversed time", func(p *RechargePromotion) { p.EndsAt = p.StartsAt }},
		{"missing timezone", func(p *RechargePromotion) { p.StartsAt = "2026-10-01T00:00:00" }},
		{"bad cap", func(p *RechargePromotion) { p.MaxBonus = -.1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := promotionForTest()
			tc.mutate(p)
			_, err := normalizeRechargePromotion(p)
			require.Error(t, err)
		})
	}
	repo := &paymentConfigSettingRepoStub{values: map[string]string{SettingKeyRechargePromotionEnabled: "true"}}
	svc := NewPaymentConfigService(nil, repo, nil)
	cfg, err := svc.GetPaymentConfig(context.Background())
	require.NoError(t, err)
	require.Nil(t, cfg.RechargePromotion)
	p := promotionForTest()
	p.StartsAt, p.EndsAt = time.Now().Add(-time.Hour).Format(time.RFC3339), time.Now().Add(time.Hour).Format(time.RFC3339)
	p.Active = false // Client-supplied Active must not control server eligibility.
	require.NoError(t, svc.UpdatePaymentConfig(context.Background(), UpdatePaymentConfigRequest{RechargePromotion: p}))
	cfg, err = svc.GetPaymentConfig(context.Background())
	require.NoError(t, err)
	require.True(t, cfg.RechargePromotion.Active)
	p.Enabled, p.Active = false, true
	require.NoError(t, svc.UpdatePaymentConfig(context.Background(), UpdatePaymentConfigRequest{RechargePromotion: p}))
	cfg, err = svc.GetPaymentConfig(context.Background())
	require.NoError(t, err)
	require.False(t, cfg.RechargePromotion.Active)
}

func newPromotionOrder(t *testing.T, status string) (*dbent.Client, *dbent.PaymentOrder) {
	t.Helper()
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
	u, err := client.User.Create().SetEmail("promotion@example.com").SetPasswordHash("hash").SetBalance(110).SetTotalRecharged(100).Save(ctx)
	require.NoError(t, err)
	o, err := client.PaymentOrder.Create().SetUserID(u.ID).SetUserEmail(u.Email).SetUserName("promo").SetAmount(110).SetPayAmount(100).SetFeeRate(0).SetRechargeCode("PAY-PROMOTION").SetOutTradeNo("promotion-order").SetPaymentType("alipay").SetPaymentTradeNo("promo-trade").SetOrderType("balance").SetStatus(status).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("example.com").SetPromotionSnapshot(map[string]any{"title": "Holiday", "base_amount": 100.0, "bonus_amount": 10.0, "bonus_percent": 10.0, "currency": "CNY"}).Save(ctx)
	require.NoError(t, err)
	return client, o
}

func TestPaymentPromotionOrderSnapshotAndBaseRebate(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	u, err := client.User.Create().SetEmail("quote@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	p := promotionForTest()
	p.StartsAt, p.EndsAt = time.Now().Add(-time.Hour).Format(time.RFC3339), time.Now().Add(time.Hour).Format(time.RFC3339)
	svc := &PaymentService{entClient: client}
	o, err := svc.createOrderInTx(ctx, CreateOrderRequest{UserID: u.ID, Amount: 200, OrderType: "balance", PaymentType: "alipay"}, &User{ID: u.ID, Email: u.Email}, nil, &PaymentConfig{RechargePromotionEnabled: true, RechargePromotion: p, BalanceRechargeMultiplier: .15}, calculateCreditedBalance(200, .15), 200, 2, 204, &payment.InstanceSelection{ProviderKey: "alipay", Config: map[string]string{}})
	require.NoError(t, err)
	require.Equal(t, 36.0, o.Amount)
	require.Equal(t, 204.0, o.PayAmount, "bonus must not alter gateway charge or fee")
	require.Equal(t, 30.0, affiliateRebateBaseAmount(o))
	p.Enabled, p.Title, p.Tiers[1].BonusPercent = false, "Changed", 99
	reloaded, err := client.PaymentOrder.Get(ctx, o.ID)
	require.NoError(t, err)
	require.Equal(t, "Holiday", reloaded.PromotionSnapshot["title"])
	require.Equal(t, 6.0, PaymentOrderBonusAmount(reloaded))
	require.Equal(t, 30.0, PaymentOrderBaseAmount(reloaded))
}

func TestPaymentPromotionFulfillmentCreditsOnceAndExcludesBonusFromTotal(t *testing.T) {
	ctx := context.Background()
	client, order := newPromotionOrder(t, OrderStatusPaid)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
	_, err := client.User.UpdateOneID(order.UserID).SetBalance(0).SetTotalRecharged(0).Save(ctx)
	require.NoError(t, err)
	repo := &paymentFulfillmentRedeemRepo{}
	userRepo := &mockUserRepo{getByIDUser: &User{ID: order.UserID}}
	userRepo.updateBalanceFn = func(ctx context.Context, id int64, amount float64) error {
		_, err := dbent.TxFromContext(ctx).User.UpdateOneID(id).AddBalance(amount).AddTotalRecharged(amount).Save(ctx)
		return err
	}
	svc := &PaymentService{entClient: client, configService: NewPaymentConfigService(client, &paymentConfigSettingRepoStub{}, nil), userRepo: userRepo, redeemService: NewRedeemService(repo, userRepo, nil, &paymentFulfillmentRedeemCacheStub{}, nil, client, nil, nil)}
	require.NoError(t, svc.ExecuteBalanceFulfillment(ctx, order.ID))
	require.NoError(t, svc.ExecuteBalanceFulfillment(ctx, order.ID))
	u, err := client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Equal(t, 110.0, u.Balance)
	require.Equal(t, 100.0, u.TotalRecharged)
	require.Len(t, repo.useCalls, 1)
}

func TestPaymentPromotionRefundGuardsAndExactDeduction(t *testing.T) {
	ctx := context.Background()
	client, order := newPromotionOrder(t, OrderStatusCompleted)
	svc := &PaymentService{entClient: client}
	for _, tc := range []struct {
		amount        float64
		force, deduct bool
	}{{100, false, true}, {110, true, true}, {110, false, false}} {
		require.Equal(t, "PROMOTION_REFUND_FULL_ONLY", infraerrors.Reason(svc.validatePromotionRefund(ctx, order, tc.amount, tc.force, tc.deduct)))
	}
	require.NoError(t, svc.validatePromotionRefund(ctx, order, 110, false, true))
	audit, err := client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(order.ID, 10)).SetAction("AFFILIATE_REBATE_APPLIED").SetDetail(`{"rebateAmount":0}`).SetOperator("system").Save(ctx)
	require.NoError(t, err)
	require.NoError(t, svc.validatePromotionRefund(ctx, order, 110, false, true))
	_, err = client.PaymentAuditLog.UpdateOneID(audit.ID).SetDetail(`{"rebateAmount":5}`).Save(ctx)
	require.NoError(t, err)
	require.Equal(t, "PROMOTION_REFUND_AFFILIATE_REVIEW_REQUIRED", infraerrors.Reason(svc.validatePromotionRefund(ctx, order, 110, false, true)))
	plan := &RefundPlan{Order: order, BalanceToDeduct: 110}
	_, err = client.User.UpdateOneID(order.UserID).SetBalance(109).Save(ctx)
	require.NoError(t, err)
	_, err = svc.deductRefundBalance(ctx, plan)
	require.Error(t, err)
	u, err := client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Equal(t, 109.0, u.Balance, "failed deduction must not partially remove funds")
	_, err = client.User.UpdateOneID(order.UserID).SetBalance(110).Save(ctx)
	require.NoError(t, err)
	deducted, err := svc.deductRefundBalance(ctx, plan)
	require.NoError(t, err)
	require.Equal(t, 110.0, deducted)
	require.Equal(t, 100.0, calculateGatewayRefundAmount(order.Amount, order.PayAmount, 110, "CNY"))
}

func TestPaymentPromotionPendingRefundRetainsCreditAndRestoresOnFailureOnce(t *testing.T) {
	ctx := context.Background()
	client, order := newPromotionOrder(t, OrderStatusRefunding)
	invalidator := &authCacheInvalidatorStub{}
	svc := &PaymentService{entClient: client, redeemService: &RedeemService{authCacheInvalidator: invalidator}}
	plan := &RefundPlan{OrderID: order.ID, Order: order, RefundAmount: 110, BalanceToDeduct: 110, DeductionType: payment.DeductionTypeBalance}
	_, err := svc.deductRefundBalance(ctx, plan)
	require.NoError(t, err)
	_, err = svc.markRefundPending(ctx, plan, &payment.RefundResponse{Status: payment.ProviderStatusPending, RefundID: "r1"})
	require.NoError(t, err)
	u, err := client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Zero(t, u.Balance)
	require.True(t, svc.latestRefundPendingDetail(ctx, order.ID).DeductionRetained)
	order, err = client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	_, err = svc.finalizeRefundFailed(ctx, order, errors.New("gateway rejected refund"))
	require.NoError(t, err)
	_, err = svc.finalizeRefundFailed(ctx, order, errors.New("duplicate failure"))
	require.Error(t, err)
	u, err = client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Equal(t, 110.0, u.Balance)
	require.Equal(t, 100.0, u.TotalRecharged)
	require.Len(t, invalidator.userIDs, 2, "committed deduction and restoration invalidate the balance cache")
	// A second genuine attempt must update the unique per-order action log.
	for attempt := 0; attempt < 2; attempt++ {
		order, err = client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRefunding).Save(ctx)
		require.NoError(t, err)
		plan.Order = order
		_, err = svc.deductRefundBalance(ctx, plan)
		require.NoError(t, err)
		_, err = svc.markRefundPending(ctx, plan, &payment.RefundResponse{Status: payment.ProviderStatusPending, RefundID: "retry"})
		require.NoError(t, err)
		order, err = client.PaymentOrder.Get(ctx, order.ID)
		require.NoError(t, err)
		_, err = svc.finalizeRefundFailed(ctx, order, errors.New("retry failed"))
		require.NoError(t, err)
	}
	u, err = client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Equal(t, 110.0, u.Balance)
	require.Equal(t, 100.0, u.TotalRecharged)
	require.Len(t, invalidator.userIDs, 6)
}

func TestPaymentPromotionRefundRollbackFailureRequiresOfflineHandling(t *testing.T) {
	ctx := context.Background()
	client, order := newPromotionOrder(t, OrderStatusRefundFailed)
	_, err := client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(order.ID, 10)).SetAction("REFUND_ROLLBACK_FAILED").SetDetail(`{"balanceDeducted":110}`).SetOperator("admin").Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{entClient: client}
	require.Equal(t, "PROMOTION_REFUND_MANUAL_REQUIRED", infraerrors.Reason(svc.validatePromotionRefund(ctx, order, 110, false, true)))
	plan := &RefundPlan{OrderID: order.ID, Order: order, RefundAmount: 110, BalanceToDeduct: 110, DeductBalance: true, DeductionType: payment.DeductionTypeBalance}
	_, err = svc.ExecuteRefund(ctx, plan)
	require.Equal(t, "PROMOTION_REFUND_MANUAL_REQUIRED", infraerrors.Reason(err))
	u, err := client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Equal(t, 110.0, u.Balance)
	current, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundFailed, current.Status)
}

func TestPaymentPromotionPendingRefundDoesNotDependOnAuditForDeduction(t *testing.T) {
	for _, status := range []string{payment.ProviderStatusSuccess, payment.ProviderStatusFailed} {
		for _, corruptAudit := range []bool{false, true} {
			t.Run(status+strconv.FormatBool(corruptAudit), func(t *testing.T) {
				ctx := context.Background()
				client := newPaymentConfigServiceTestClient(t)
				ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
				order := createPendingRefundOrderForTest(t, ctx, client, "promo-query")
				order, err := client.PaymentOrder.UpdateOneID(order.ID).SetUpdatedAt(time.Now().UTC().Truncate(time.Microsecond)).SetAmount(110).SetRefundAmount(110).SetPromotionSnapshot(map[string]any{"base_amount": 100.0, "bonus_amount": 10.0}).Save(ctx)
				require.NoError(t, err)
				// User has already had 110 reclaimed and later recharged 150.
				_, err = client.User.UpdateOneID(order.UserID).SetBalance(150).Save(ctx)
				require.NoError(t, err)
				if corruptAudit {
					_, err = client.PaymentAuditLog.Update().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).SetDetail("malformed").Save(ctx)
				} else {
					_, err = client.PaymentAuditLog.Delete().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).Exec(ctx)
				}
				require.NoError(t, err)
				svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}
				restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{refundResponse: &payment.RefundResponse{Status: status, RefundID: "rf_test"}})
				defer restore()
				_, err = svc.QueryAndFinalizeRefund(ctx, order.ID)
				require.NoError(t, err)
				u, err := client.User.Get(ctx, order.UserID)
				require.NoError(t, err)
				if status == payment.ProviderStatusSuccess {
					require.Equal(t, 150.0, u.Balance)
				} else {
					require.Equal(t, 260.0, u.Balance)
				}
			})
		}
	}
}

func TestPaymentPromotionRefundRejectsStalePendingVersion(t *testing.T) {
	ctx := context.Background()
	client, order := newPromotionOrder(t, OrderStatusRefundPending)
	svc := &PaymentService{entClient: client}
	version := time.Now().UTC().Truncate(time.Microsecond)
	order, err := client.PaymentOrder.UpdateOneID(order.ID).SetRefundAmount(110).SetUpdatedAt(version).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentOrder.UpdateOneID(order.ID).SetUpdatedAt(version.Add(time.Second)).Save(ctx)
	require.NoError(t, err)
	_, err = svc.finalizePromotionRefundFailure(ctx, order, errors.New("delayed prior attempt failure"))
	require.Equal(t, "CONFLICT", infraerrors.Reason(err))
	_, err = svc.finalizePendingRefundSuccess(ctx, svc.refundFinalizePlan(order))
	require.Equal(t, "CONFLICT", infraerrors.Reason(err))
	u, err := client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Equal(t, 110.0, u.Balance)
	current, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, current.Status)
	require.True(t, current.UpdatedAt.Equal(version.Add(time.Second)))
}

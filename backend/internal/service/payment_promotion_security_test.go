//go:build unit

package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestPaymentPromotionRejectsUnknownOrderTypeBeforePaymentAndFulfillment(t *testing.T) {
	ctx := context.Background()
	for _, orderType := range []string{"other", "BALANCE", " balance "} {
		for _, disabled := range []string{"true", "false"} {
			svc := &PaymentService{configService: NewPaymentConfigService(nil, &paymentConfigSettingRepoStub{values: map[string]string{
				SettingPaymentEnabled: "true", SettingBalancePayDisabled: disabled, SettingBalanceRechargeMult: "0.15",
			}}, nil)}
			_, err := svc.CreateOrder(ctx, CreateOrderRequest{OrderType: orderType, Amount: 100})
			require.Equal(t, "INVALID_ORDER_TYPE", infraerrors.Reason(err))
		}
	}
	client, order := newPromotionOrder(t, OrderStatusPaid)
	_, err := client.PaymentOrder.UpdateOneID(order.ID).SetOrderType("other").Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{entClient: client}
	require.Equal(t, "INVALID_ORDER_TYPE", infraerrors.Reason(svc.executeFulfillment(ctx, order.ID)))
	require.Equal(t, "INVALID_ORDER_TYPE", infraerrors.Reason(svc.ExecuteBalanceFulfillment(ctx, order.ID)))
	current, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPaid, current.Status)
}

func TestPaymentPromotionRejectsZeroOrOverflowCreditBeforePayment(t *testing.T) {
	for _, tc := range []struct {
		name, multiplier string
		amount           float64
	}{{"rounds to zero", "0.15", .01}, {"overflows", "2", math.MaxFloat64}} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &PaymentService{configService: NewPaymentConfigService(nil, &paymentConfigSettingRepoStub{values: map[string]string{
				SettingPaymentEnabled: "true", SettingMinRechargeAmount: "0.01", SettingBalanceRechargeMult: tc.multiplier,
			}}, nil), userRepo: &mockUserRepo{getByIDUser: &User{ID: 1, Status: payment.EntityStatusActive}}}
			_, err := svc.CreateOrder(context.Background(), CreateOrderRequest{UserID: 1, OrderType: payment.OrderTypeBalance, Amount: tc.amount})
			require.Equal(t, "INVALID_AMOUNT", infraerrors.Reason(err))
		})
	}
}

func TestPaymentPromotionPreservesSmallRechargeMultiplier(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{}
	svc := NewPaymentConfigService(nil, repo, nil)
	multiplier := .001
	require.NoError(t, svc.UpdatePaymentConfig(context.Background(), UpdatePaymentConfigRequest{BalanceRechargeMultiplier: &multiplier}))
	cfg, err := svc.GetPaymentConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, multiplier, cfg.BalanceRechargeMultiplier)
	require.Equal(t, .1, calculateCreditedBalance(100, cfg.BalanceRechargeMultiplier))
}

func TestPaymentPromotionInternalCodeCannotBypassSettlement(t *testing.T) {
	ctx := context.Background()
	client, order := newPromotionOrder(t, OrderStatusPaid)
	_, err := client.User.UpdateOneID(order.UserID).SetBalance(0).SetTotalRecharged(0).Save(ctx)
	require.NoError(t, err)
	repo := &paymentFulfillmentRedeemRepo{}
	require.NoError(t, repo.Create(ctx, &RedeemCode{Code: order.RechargeCode, Type: RedeemTypeBalance, Value: order.Amount, Status: StatusUnused}))
	userRepo := &mockUserRepo{getByIDUser: &User{ID: order.UserID}}
	userRepo.updateBalanceFn = func(ctx context.Context, id int64, amount float64) error {
		_, err := dbent.TxFromContext(ctx).User.UpdateOneID(id).AddBalance(amount).AddTotalRecharged(amount).Save(ctx)
		return err
	}
	redeem := NewRedeemService(repo, userRepo, nil, &paymentFulfillmentRedeemCacheStub{}, nil, client, nil, nil)
	for _, id := range []int64{order.UserID, order.UserID + 1} {
		_, err := redeem.Redeem(ctx, id, order.RechargeCode)
		require.ErrorIs(t, err, ErrRedeemCodeNotFound)
	}
	_, err = redeem.RedeemForAdminFulfillment(ctx, order.UserID, order.RechargeCode)
	require.ErrorIs(t, err, ErrRedeemCodeNotFound)
	require.Empty(t, repo.useCalls)
	svc := &PaymentService{entClient: client, redeemService: redeem}
	require.NoError(t, svc.ExecuteBalanceFulfillment(ctx, order.ID))
	u, err := client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Equal(t, 110.0, u.Balance)
	require.Equal(t, 100.0, u.TotalRecharged)
	require.Len(t, repo.useCalls, 1)
}

type promotionRefundOutcomeProvider struct {
	refundProviderTestDouble
	response *payment.RefundResponse
	err      error
	calls    int
}

func (p *promotionRefundOutcomeProvider) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	p.calls++
	return p.response, p.err
}

func TestPaymentPromotionUnknownRefundOutcomeKeepsCreditReclaimed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *payment.RefundResponse
		err      error
	}{{"timeout after submission", nil, context.DeadlineExceeded}, {"empty response", nil, nil}, {"unknown status", &payment.RefundResponse{Status: "unrecognized", RefundID: "rf_unknown"}, nil}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client, order := newPromotionOrder(t, OrderStatusCompleted)
			inst, err := client.PaymentProviderInstance.Create().SetProviderKey(payment.TypeStripe).SetName("refund-security").SetConfig("{}").SetSupportedTypes("stripe").SetEnabled(true).SetRefundEnabled(true).Save(ctx)
			require.NoError(t, err)
			order, err = client.PaymentOrder.UpdateOneID(order.ID).SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).SetProviderKey(payment.TypeStripe).SetPaymentType(payment.TypeStripe).Save(ctx)
			require.NoError(t, err)
			provider := &promotionRefundOutcomeProvider{response: tc.response, err: tc.err}
			defer replacePaymentProviderFactoryForTest(t, provider)()
			svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}, userRepo: &mockUserRepo{getByIDUser: &User{ID: order.UserID, Balance: 110}}}
			plan, early, err := svc.PrepareRefund(ctx, order.ID, 110, "security test", false, true)
			require.NoError(t, err)
			require.Nil(t, early)
			result, err := svc.ExecuteRefund(ctx, plan)
			require.NoError(t, err)
			require.False(t, result.Success)
			current, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusRefundPending, current.Status)
			u, err := client.User.Get(ctx, order.UserID)
			require.NoError(t, err)
			require.Zero(t, u.Balance)
			require.Equal(t, 100.0, u.TotalRecharged)
			_, _, err = svc.PrepareRefund(ctx, order.ID, 110, "retry", false, true)
			require.Equal(t, "PROMOTION_REFUND_PENDING", infraerrors.Reason(err))
			require.Equal(t, 1, provider.calls)
			// Only a definitive later gateway failure may release the funds.
			_, err = svc.finalizeRefundFailed(ctx, current, errors.New("confirmed failure"))
			require.NoError(t, err)
			u, err = client.User.Get(ctx, order.UserID)
			require.NoError(t, err)
			require.Equal(t, 110.0, u.Balance)
		})
	}
}

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Keep reclaimed credit unavailable while an asynchronous refund is pending.
// Otherwise it could be spent before the gateway confirms the cash refund.
func (s *PaymentService) markPromotionRefundPending(ctx context.Context, p *RefundPlan, resp *payment.RefundResponse) (*RefundResult, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	n, err := tx.PaymentOrder.Update().Where(paymentorder.IDEQ(p.OrderID), paymentorder.StatusEQ(OrderStatusRefunding)).
		SetStatus(OrderStatusRefundPending).SetUpdatedAt(time.Now().UTC().Truncate(time.Microsecond)).SetRefundAmount(p.RefundAmount).SetRefundReason(p.Reason).ClearRefundAt().SetForceRefund(false).ClearFailedAt().ClearFailedReason().Save(ctx)
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, infraerrors.Conflict("CONFLICT", "order status changed")
	}
	detail, err := json.Marshal(map[string]any{"refundID": refundResponseID(resp), "refundAmount": p.RefundAmount, "deductionRetained": true, "deductionRollbackOK": true, "balanceDeducted": p.RefundAmount})
	if err != nil {
		return nil, err
	}
	if err := tx.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(p.OrderID, 10)).SetAction("REFUND_PENDING").SetDetail(string(detail)).SetOperator("admin").OnConflictColumns(paymentauditlog.FieldOrderID, paymentauditlog.FieldAction).UpdateNewValues().Exec(ctx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &RefundResult{Success: false, BalanceDeducted: p.RefundAmount, Warning: "gateway refund is pending confirmation; base credit and bonus remain deducted"}, nil
}

func (s *PaymentService) finalizePromotionRefundFailure(ctx context.Context, order *dbent.PaymentOrder, cause error) (*RefundResult, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	n, err := tx.PaymentOrder.Update().Where(paymentorder.IDEQ(order.ID), paymentorder.StatusEQ(OrderStatusRefundPending), paymentorder.UpdatedAtEQ(order.UpdatedAt)).SetStatus(OrderStatusRefundFailed).SetRefundAmount(0).SetFailedAt(time.Now()).SetFailedReason(psErrMsg(cause)).Save(ctx)
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, infraerrors.Conflict("CONFLICT", "order status changed")
	}
	if _, err := tx.User.UpdateOneID(order.UserID).AddBalance(order.RefundAmount).Save(ctx); err != nil {
		return nil, err
	}
	detail, _ := json.Marshal(map[string]any{"detail": psErrMsg(cause), "balanceRestored": order.RefundAmount})
	if err := tx.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(order.ID, 10)).SetAction("REFUND_FAILED").SetDetail(string(detail)).SetOperator("admin").OnConflictColumns(paymentauditlog.FieldOrderID, paymentauditlog.FieldAction).UpdateNewValues().Exec(ctx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit promotion refund failure: %w", err)
	}
	s.invalidatePromotionRefundCaches(ctx, order)
	return &RefundResult{Success: false, Warning: "gateway refund failed; base credit and bonus restored"}, nil
}

func (s *PaymentService) invalidatePromotionRefundCaches(ctx context.Context, order *dbent.PaymentOrder) {
	if s.redeemService != nil {
		s.redeemService.invalidateRedeemCaches(ctx, order.UserID, &RedeemCode{Type: RedeemTypeBalance})
	}
}

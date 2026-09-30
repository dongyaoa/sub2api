package service

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

// Promotion reports select a cohort by order creation time. Refunds reflect the
// current confirmed state of those orders, not a cash-flow period report.
type PromotionOrderFilter struct {
	StartTime *time.Time
	EndTime   *time.Time
	Status    string
	Keyword   string
	UserID    int64
	Page      int
	PageSize  int
}

type PromotionCashSummary struct {
	Currency       string  `json:"currency"`
	PaidAmount     float64 `json:"paid_amount"`
	RefundedAmount float64 `json:"refunded_amount"`
	NetAmount      float64 `json:"net_amount"`
}

type PromotionSummary struct {
	OrderCount          int                    `json:"order_count"`
	PendingOrderCount   int                    `json:"pending_order_count"`
	IssuedOrderCount    int                    `json:"issued_order_count"`
	IssuedUserCount     int                    `json:"issued_user_count"`
	BaseAmount          float64                `json:"base_amount"`
	BonusAmount         float64                `json:"bonus_amount"`
	RefundedBaseAmount  float64                `json:"refunded_base_amount"`
	RefundedBonusAmount float64                `json:"refunded_bonus_amount"`
	NetBaseAmount       float64                `json:"net_base_amount"`
	NetBonusAmount      float64                `json:"net_bonus_amount"`
	CashByCurrency      []PromotionCashSummary `json:"cash_by_currency"`
}

func ValidatePromotionOrderFilter(filter PromotionOrderFilter) error {
	if filter.StartTime != nil && filter.EndTime != nil && !filter.EndTime.After(*filter.StartTime) {
		return infraerrors.BadRequest("INVALID_PROMOTION_FILTER", "end_time must be after start_time")
	}
	if filter.UserID < 0 || len(filter.Keyword) > 256 {
		return infraerrors.BadRequest("INVALID_PROMOTION_FILTER", "invalid user or search filter")
	}
	switch filter.Status {
	case "", OrderStatusPending, OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted, OrderStatusExpired, OrderStatusCancelled, OrderStatusFailed, OrderStatusRefundRequested, OrderStatusRefunding, OrderStatusRefundPending, OrderStatusPartiallyRefunded, OrderStatusRefunded, OrderStatusRefundFailed:
		return nil
	default:
		return infraerrors.BadRequest("INVALID_PROMOTION_FILTER", "unknown payment order status")
	}
}

func (s *PaymentService) promotionOrdersQuery(ctx context.Context, filter PromotionOrderFilter) (*dbent.PaymentOrderQuery, error) {
	if !s.configService.IsRechargePromotionEnabled(ctx) {
		return nil, rechargePromotionDisabledError()
	}
	if err := ValidatePromotionOrderFilter(filter); err != nil {
		return nil, err
	}
	query := s.entClient.PaymentOrder.Query().Where(paymentorder.OrderTypeEQ(payment.OrderTypeBalance), func(selector *sql.Selector) {
		selector.Where(sqljson.ValueGT(paymentorder.FieldPromotionSnapshot, float64(0), sqljson.Path("bonus_amount")))
	})
	if filter.StartTime != nil {
		query = query.Where(paymentorder.CreatedAtGTE(*filter.StartTime))
	}
	if filter.EndTime != nil {
		query = query.Where(paymentorder.CreatedAtLT(*filter.EndTime))
	}
	if filter.Status != "" {
		query = query.Where(paymentorder.StatusEQ(filter.Status))
	}
	if filter.UserID > 0 {
		query = query.Where(paymentorder.UserIDEQ(filter.UserID))
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		matches := []predicate.PaymentOrder{paymentorder.UserEmailContainsFold(keyword), paymentorder.UserNameContainsFold(keyword), paymentorder.OutTradeNoContainsFold(keyword)}
		if id, err := strconv.ParseInt(keyword, 10, 64); err == nil && id > 0 {
			matches = append(matches, paymentorder.IDEQ(id), paymentorder.UserIDEQ(id))
		}
		query = query.Where(paymentorder.Or(matches...))
	}
	return query, nil
}

func (s *PaymentService) ListPromotionOrders(ctx context.Context, filter PromotionOrderFilter) ([]*dbent.PaymentOrder, int, error) {
	query, err := s.promotionOrdersQuery(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	count, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	page, size := filter.Page, filter.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	orders, err := query.Order(paymentorder.ByCreatedAt(sql.OrderDesc()), paymentorder.ByID(sql.OrderDesc())).Offset((page - 1) * size).Limit(size).All(ctx)
	return orders, count, err
}

func (s *PaymentService) GetPromotionSummary(ctx context.Context, filter PromotionOrderFilter) (*PromotionSummary, error) {
	query, err := s.promotionOrdersQuery(ctx, filter)
	if err != nil {
		return nil, err
	}
	summary := &PromotionSummary{CashByCurrency: []PromotionCashSummary{}}
	users := make(map[int64]struct{})
	cash := make(map[string]*PromotionCashSummary)
	// Read order details in bounded batches rather than loading all rows at once.
	var lastID int64
	for {
		orders, err := query.Clone().Where(paymentorder.IDGT(lastID)).Order(paymentorder.ByID(sql.OrderAsc())).Limit(500).
			Select(paymentorder.FieldID, paymentorder.FieldUserID, paymentorder.FieldStatus, paymentorder.FieldAmount, paymentorder.FieldPayAmount, paymentorder.FieldRefundAmount, paymentorder.FieldOrderType, paymentorder.FieldPaidAt, paymentorder.FieldCompletedAt, paymentorder.FieldProviderSnapshot, paymentorder.FieldPromotionSnapshot).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, order := range orders {
			addPromotionOrderSummary(summary, users, cash, order)
			lastID = order.ID
		}
		if len(orders) < 500 {
			break
		}
	}
	summary.IssuedUserCount = len(users)
	summary.NetBaseAmount = promotionMoneySubtract(summary.BaseAmount, summary.RefundedBaseAmount)
	summary.NetBonusAmount = promotionMoneySubtract(summary.BonusAmount, summary.RefundedBonusAmount)
	for _, amount := range cash {
		amount.NetAmount = promotionMoneySubtract(amount.PaidAmount, amount.RefundedAmount)
		summary.CashByCurrency = append(summary.CashByCurrency, *amount)
	}
	sort.Slice(summary.CashByCurrency, func(i, j int) bool { return summary.CashByCurrency[i].Currency < summary.CashByCurrency[j].Currency })
	return summary, nil
}

func promotionOrderIssued(order *dbent.PaymentOrder) bool {
	if order.CompletedAt != nil {
		return true
	}
	switch order.Status {
	case OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefunding, OrderStatusRefundPending, OrderStatusRefundFailed, OrderStatusPartiallyRefunded, OrderStatusRefunded:
		return true
	default:
		return false
	}
}

func addPromotionOrderSummary(summary *PromotionSummary, users map[int64]struct{}, cash map[string]*PromotionCashSummary, order *dbent.PaymentOrder) {
	summary.OrderCount++
	if order.Status == OrderStatusPending {
		summary.PendingOrderCount++
	}
	issued := promotionOrderIssued(order)
	if issued {
		summary.IssuedOrderCount++
		users[order.UserID] = struct{}{}
		summary.BaseAmount = promotionMoneyAdd(summary.BaseAmount, PaymentOrderBaseAmount(order))
		summary.BonusAmount = promotionMoneyAdd(summary.BonusAmount, PaymentOrderBonusAmount(order))
	}
	if !issued && order.PaidAt == nil && order.Status != OrderStatusPaid && order.Status != OrderStatusRecharging {
		return
	}
	currency := PaymentOrderCurrency(order)
	if cash[currency] == nil {
		cash[currency] = &PromotionCashSummary{Currency: currency}
	}
	cash[currency].PaidAmount = promotionMoneyAdd(cash[currency].PaidAmount, order.PayAmount)
	if order.Status != OrderStatusRefunded && order.Status != OrderStatusPartiallyRefunded {
		return
	}
	refund := decimal.Min(decimal.NewFromFloat(order.Amount), decimal.Max(decimal.Zero, decimal.NewFromFloat(order.RefundAmount)))
	if !refund.IsPositive() || order.Amount <= 0 {
		return
	}
	refundedBonus := refund.Mul(decimal.NewFromFloat(PaymentOrderBonusAmount(order))).Div(decimal.NewFromFloat(order.Amount)).Round(2).InexactFloat64()
	summary.RefundedBonusAmount = promotionMoneyAdd(summary.RefundedBonusAmount, refundedBonus)
	summary.RefundedBaseAmount = promotionMoneyAdd(summary.RefundedBaseAmount, promotionMoneySubtract(refund.InexactFloat64(), refundedBonus))
	cash[currency].RefundedAmount = promotionMoneyAdd(cash[currency].RefundedAmount, calculateGatewayRefundAmount(order.Amount, order.PayAmount, refund.InexactFloat64(), currency))
}

func promotionMoneyAdd(a, b float64) float64 {
	return decimal.NewFromFloat(a).Add(decimal.NewFromFloat(b)).InexactFloat64()
}
func promotionMoneySubtract(a, b float64) float64 {
	return decimal.NewFromFloat(a).Sub(decimal.NewFromFloat(b)).InexactFloat64()
}

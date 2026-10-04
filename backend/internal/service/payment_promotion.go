package service

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const SettingRechargePromotion = "RECHARGE_PROMOTION"

func rechargePromotionDisabledError() error {
	return infraerrors.Forbidden("RECHARGE_PROMOTION_DISABLED", "recharge promotions are disabled in system settings")
}

type RechargePromotionTier struct {
	MinAmount    float64 `json:"min_amount"`
	BonusPercent float64 `json:"bonus_percent"`
}

// RechargePromotion uses gateway currency for thresholds and USD balance units
// for MaxBonus. Active is a response-only value; it never authorizes a reward.
type RechargePromotion struct {
	Enabled  bool                    `json:"enabled"`
	Title    string                  `json:"title"`
	Subtitle string                  `json:"subtitle"`
	StartsAt string                  `json:"starts_at"`
	EndsAt   string                  `json:"ends_at"`
	Currency string                  `json:"currency"`
	Tiers    []RechargePromotionTier `json:"tiers"`
	MaxBonus float64                 `json:"max_bonus"`
	Active   bool                    `json:"active"`
}

func normalizeRechargePromotion(input *RechargePromotion) (*RechargePromotion, error) {
	if input == nil {
		return nil, nil
	}
	p := *input
	p.Tiers = append([]RechargePromotionTier(nil), input.Tiers...)
	p.Title, p.Subtitle = strings.TrimSpace(p.Title), strings.TrimSpace(p.Subtitle)
	p.Active = false
	bad := func(message string) (*RechargePromotion, error) {
		return nil, infraerrors.BadRequest("INVALID_RECHARGE_PROMOTION", message)
	}
	if utf8.RuneCountInString(p.Title) > 80 || utf8.RuneCountInString(p.Subtitle) > 200 {
		return bad("promotion title or subtitle is too long")
	}
	if !finitePromotionNumber(p.MaxBonus, 0, 1e9) || !decimal.NewFromFloat(p.MaxBonus).Equal(decimal.NewFromFloat(p.MaxBonus).Round(2)) {
		return bad("maximum bonus must be between 0 and 1000000000 with at most two decimals")
	}
	// An empty disabled configuration is useful before the first campaign.
	if !p.Enabled && p.StartsAt == "" && p.EndsAt == "" && len(p.Tiers) == 0 {
		return &p, nil
	}
	if p.Title == "" {
		return bad("promotion title is required")
	}
	start, err := time.Parse(time.RFC3339, p.StartsAt)
	if err != nil {
		return bad("promotion starts_at must be an RFC3339 timestamp")
	}
	end, err := time.Parse(time.RFC3339, p.EndsAt)
	if err != nil || !end.After(start) {
		return bad("promotion ends_at must be after starts_at")
	}
	p.StartsAt, p.EndsAt = start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano)
	p.Currency, err = payment.NormalizePaymentCurrency(p.Currency)
	if err != nil {
		return bad("promotion currency is unsupported")
	}
	if len(p.Tiers) == 0 || len(p.Tiers) > 10 {
		return bad("promotion requires between 1 and 10 tiers")
	}
	seen := make(map[float64]bool, len(p.Tiers))
	for _, tier := range p.Tiers {
		if !finitePromotionNumber(tier.MinAmount, 0.01, 1e9) || seen[tier.MinAmount] {
			return bad("tier thresholds must be positive, unique and at most 1000000000")
		}
		if err := validateCreateOrderAmountCurrency(tier.MinAmount, p.Currency); err != nil {
			return bad("tier threshold precision does not match its currency")
		}
		if !finitePromotionNumber(tier.BonusPercent, 0.01, 100) || !decimal.NewFromFloat(tier.BonusPercent).Equal(decimal.NewFromFloat(tier.BonusPercent).Round(2)) {
			return bad("bonus percentage must be between 0.01 and 100 with at most two decimals")
		}
		seen[tier.MinAmount] = true
	}
	sort.Slice(p.Tiers, func(i, j int) bool { return p.Tiers[i].MinAmount < p.Tiers[j].MinAmount })
	return &p, nil
}

func finitePromotionNumber(value, min, max float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= min && value <= max
}

func (p *RechargePromotion) activeAt(now time.Time) bool {
	if p == nil || !p.Enabled {
		return false
	}
	start, startErr := time.Parse(time.RFC3339, p.StartsAt)
	end, endErr := time.Parse(time.RFC3339, p.EndsAt)
	return startErr == nil && endErr == nil && !now.Before(start) && now.Before(end)
}

func parseRechargePromotion(raw string, now time.Time) *RechargePromotion {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var p RechargePromotion
	if json.Unmarshal([]byte(raw), &p) != nil {
		return nil
	}
	valid, err := normalizeRechargePromotion(&p)
	if err != nil {
		return nil
	}
	valid.Active = valid.activeAt(now)
	return valid
}

func buildRechargePromotionSnapshot(p *RechargePromotion, orderType string, rechargeAmount, baseAmount, multiplier float64, currency string, now time.Time) map[string]any {
	if orderType != payment.OrderTypeBalance || !p.activeAt(now) || p.Currency != currency || baseAmount <= 0 {
		return nil
	}
	var matched RechargePromotionTier
	for _, tier := range p.Tiers {
		if rechargeAmount >= tier.MinAmount && tier.MinAmount > matched.MinAmount {
			matched = tier
		}
	}
	if matched.MinAmount == 0 {
		return nil
	}
	bonus := decimal.NewFromFloat(baseAmount).Mul(decimal.NewFromFloat(matched.BonusPercent)).Div(decimal.NewFromInt(100)).Round(2)
	if p.MaxBonus > 0 && bonus.GreaterThan(decimal.NewFromFloat(p.MaxBonus)) {
		bonus = decimal.NewFromFloat(p.MaxBonus)
	}
	if !bonus.IsPositive() {
		return nil
	}
	return map[string]any{
		"version": 1, "title": p.Title, "currency": currency,
		"recharge_amount": rechargeAmount, "recharge_multiplier": normalizeBalanceRechargeMultiplier(multiplier),
		"base_amount": baseAmount, "bonus_amount": bonus.InexactFloat64(),
		"bonus_percent": matched.BonusPercent, "min_amount": matched.MinAmount,
		"starts_at": p.StartsAt, "ends_at": p.EndsAt, "applied_at": now.UTC().Format(time.RFC3339),
	}
}

func PaymentOrderBonusAmount(order *dbent.PaymentOrder) float64 {
	if order == nil || order.OrderType != payment.OrderTypeBalance {
		return 0
	}
	if finitePromotionNumber(order.BonusAmount, 0.01, order.Amount) {
		return order.BonusAmount
	}
	bonus, _ := order.PromotionSnapshot["bonus_amount"].(float64)
	if !finitePromotionNumber(bonus, 0, order.Amount) {
		return 0
	}
	return bonus
}

// A qualifying scheduled promotion takes precedence over the upstream tiers.
func quoteBalanceRecharge(cfg *PaymentConfig, amount float64, currency string, now time.Time) (rechargeBonusQuote, map[string]any) {
	if cfg != nil && cfg.RechargePromotionEnabled {
		base := calculateCreditedBalance(amount, cfg.BalanceRechargeMultiplier)
		snapshot := buildRechargePromotionSnapshot(cfg.RechargePromotion, payment.OrderTypeBalance, amount, base, cfg.BalanceRechargeMultiplier, currency, now)
		if snapshot != nil {
			bonus := snapshot["bonus_amount"].(float64)
			return rechargeBonusQuote{PayBase: amount, Credited: addRechargeBonus(base, bonus), Bonus: bonus, Percent: snapshot["bonus_percent"].(float64)}, snapshot
		}
	}
	return quoteRechargeBonus(cfg, amount, currency), nil
}

func PaymentOrderBaseAmount(order *dbent.PaymentOrder) float64 {
	if order == nil {
		return 0
	}
	return decimal.NewFromFloat(order.Amount).Sub(decimal.NewFromFloat(PaymentOrderBonusAmount(order))).InexactFloat64()
}

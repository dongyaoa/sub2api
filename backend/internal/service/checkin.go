package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SettingKeyCheckinConfig  = "daily_checkin_config"
	RedeemTypeCheckinBalance = "checkin_balance"
	legacyCheckinRewardRatio = 0.20
)

var checkinLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type CheckinConfig struct {
	Enabled            bool    `json:"enabled"`
	MinRechargeAmount  float64 `json:"min_recharge_amount"`
	QualificationDays  int     `json:"qualification_days"`
	RewardMin          float64 `json:"reward_min"`
	RewardMax          float64 `json:"reward_max"`
	MaxPeriodReward    float64 `json:"max_period_reward"`
	MinAccountAgeHours int     `json:"min_account_age_hours"`
	Timezone           string  `json:"timezone"`
	Version            int     `json:"version"`
}

func DefaultCheckinConfig() CheckinConfig {
	return CheckinConfig{
		Enabled:            false,
		MinRechargeAmount:  10,
		QualificationDays:  30,
		RewardMin:          0.01,
		RewardMax:          0.05,
		MaxPeriodReward:    2,
		MinAccountAgeHours: 24,
		Timezone:           "Asia/Shanghai",
		Version:            1,
	}
}

type CheckinHistoryItem struct {
	ID           int64     `json:"id"`
	BusinessDate string    `json:"business_date"`
	Reward       float64   `json:"reward"`
	ClaimedAt    time.Time `json:"claimed_at"`
}

type CheckinStatus struct {
	Enabled            bool                 `json:"enabled"`
	Eligible           bool                 `json:"eligible"`
	Reason             string               `json:"reason"`
	BusinessDate       string               `json:"business_date"`
	ClaimedToday       bool                 `json:"claimed_today"`
	TodayReward        float64              `json:"today_reward"`
	CurrentBalance     float64              `json:"current_balance"`
	RechargeAmount     float64              `json:"recharge_amount"`
	MinRechargeAmount  float64              `json:"min_recharge_amount"`
	QualificationDays  int                  `json:"qualification_days"`
	MinAccountAgeHours int                  `json:"min_account_age_hours"`
	RewardMin          float64              `json:"reward_min"`
	RewardMax          float64              `json:"reward_max"`
	NextResetAt        time.Time            `json:"next_reset_at"`
	History            []CheckinHistoryItem `json:"history"`
}

type CheckinClaimResult struct {
	BusinessDate   string    `json:"business_date"`
	Reward         float64   `json:"reward"`
	NewBalance     float64   `json:"new_balance"`
	ClaimedAt      time.Time `json:"claimed_at"`
	AlreadyClaimed bool      `json:"already_claimed"`
}

type CheckinService struct {
	client               *dbent.Client
	settingRepo          SettingRepository
	billingCacheService  *BillingCacheService
	authCacheInvalidator APIKeyAuthCacheInvalidator
}

func NewCheckinService(
	client *dbent.Client,
	settingRepo SettingRepository,
	billingCacheService *BillingCacheService,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
) *CheckinService {
	return &CheckinService{
		client:               client,
		settingRepo:          settingRepo,
		billingCacheService:  billingCacheService,
		authCacheInvalidator: authCacheInvalidator,
	}
}

func (s *CheckinService) GetConfig(ctx context.Context) (CheckinConfig, error) {
	config := DefaultCheckinConfig()
	value, err := s.settingRepo.GetValue(ctx, SettingKeyCheckinConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return config, nil
		}
		return CheckinConfig{}, fmt.Errorf("load checkin config: %w", err)
	}
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return CheckinConfig{}, fmt.Errorf("decode checkin config: %w", err)
	}
	var storedFields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &storedFields); err != nil {
		return CheckinConfig{}, fmt.Errorf("decode checkin config fields: %w", err)
	}
	if _, exists := storedFields["max_period_reward"]; !exists {
		config.MaxPeriodReward = math.Ceil(config.MinRechargeAmount*legacyCheckinRewardRatio*100-0.0000001) / 100
	}
	if err := ValidateCheckinConfig(config); err != nil {
		return CheckinConfig{}, err
	}
	return config, nil
}

func (s *CheckinService) UpdateConfig(ctx context.Context, config CheckinConfig) (CheckinConfig, error) {
	current, err := s.GetConfig(ctx)
	if err != nil {
		return CheckinConfig{}, err
	}
	config.Timezone = "Asia/Shanghai"
	config.Version = current.Version + 1
	if config.Version < 1 {
		config.Version = 1
	}
	if err := ValidateCheckinConfig(config); err != nil {
		return CheckinConfig{}, err
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return CheckinConfig{}, fmt.Errorf("encode checkin config: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyCheckinConfig, string(encoded)); err != nil {
		return CheckinConfig{}, fmt.Errorf("save checkin config: %w", err)
	}
	return config, nil
}

func ValidateCheckinConfig(config CheckinConfig) error {
	if config.MinRechargeAmount <= 0 || !isCentAmount(config.MinRechargeAmount) {
		return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "minimum recharge amount must be a positive amount with at most two decimals")
	}
	if config.QualificationDays < 1 || config.QualificationDays > 365 {
		return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "qualification days must be between 1 and 365")
	}
	if config.RewardMin <= 0 || config.RewardMax < config.RewardMin || !isCentAmount(config.RewardMin) || !isCentAmount(config.RewardMax) {
		return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "reward range must use positive amounts with at most two decimals")
	}
	if config.MinAccountAgeHours < 0 || config.MinAccountAgeHours > 24*365 {
		return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "minimum account age must be between 0 and 8760 hours")
	}
	if config.MaxPeriodReward <= 0 || !isCentAmount(config.MaxPeriodReward) {
		return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "maximum period reward must be a positive amount with at most two decimals")
	}
	if float64(config.QualificationDays)*config.RewardMax > config.MaxPeriodReward+0.0000001 {
		return infraerrors.BadRequest("CHECKIN_CONFIG_COST_TOO_HIGH", "maximum reward over the qualification period cannot exceed the configured safety limit")
	}
	return nil
}

func isCentAmount(value float64) bool {
	return value > 0 && math.Abs(value*100-math.Round(value*100)) < 0.0000001
}

func (s *CheckinService) GetStatus(ctx context.Context, userID int64) (*CheckinStatus, error) {
	config, err := s.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	userEntity, err := s.client.User.Query().Where(dbuser.IDEQ(userID)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("load checkin user: %w", err)
	}
	businessDate := checkinBusinessDate(now)
	claim, err := s.findClaim(ctx, userID, businessDate)
	if err != nil {
		return nil, err
	}
	cutoff := checkinQualificationCutoff(now, config.QualificationDays)
	recharge, err := s.queryEffectiveRecharge(ctx, s.client, userID, cutoff)
	if err != nil {
		return nil, err
	}
	history, err := s.GetHistory(ctx, userID, 14)
	if err != nil {
		return nil, err
	}
	status := &CheckinStatus{
		Enabled:            config.Enabled,
		BusinessDate:       businessDate,
		CurrentBalance:     userEntity.Balance,
		RechargeAmount:     recharge,
		MinRechargeAmount:  config.MinRechargeAmount,
		QualificationDays:  config.QualificationDays,
		MinAccountAgeHours: config.MinAccountAgeHours,
		RewardMin:          config.RewardMin,
		RewardMax:          config.RewardMax,
		NextResetAt:        checkinNextReset(now),
		History:            history,
	}
	if claim != nil {
		status.ClaimedToday = true
		status.TodayReward = claim.Value
	}
	status.Eligible, status.Reason = evaluateCheckinEligibility(config, userEntity.Status, userEntity.CreatedAt, now, recharge, status.ClaimedToday)
	return status, nil
}

func evaluateCheckinEligibility(config CheckinConfig, userStatus string, createdAt, now time.Time, recharge float64, claimed bool) (bool, string) {
	if !config.Enabled {
		return false, "disabled"
	}
	if userStatus != StatusActive {
		return false, "account_inactive"
	}
	if now.Before(createdAt.Add(time.Duration(config.MinAccountAgeHours) * time.Hour)) {
		return false, "account_too_new"
	}
	if recharge+0.0000001 < config.MinRechargeAmount {
		return false, "recharge_required"
	}
	if claimed {
		return false, "claimed_today"
	}
	return true, "eligible"
}

func (s *CheckinService) Claim(ctx context.Context, userID int64) (*CheckinClaimResult, error) {
	config, err := s.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	businessDate := checkinBusinessDate(now)
	if existing, err := s.findClaim(ctx, userID, businessDate); err != nil {
		return nil, err
	} else if existing != nil {
		userEntity, getErr := s.client.User.Get(ctx, userID)
		if getErr != nil {
			return nil, getErr
		}
		return claimResult(existing, userEntity.Balance, true), nil
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin checkin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	userEntity, err := tx.User.Query().Where(dbuser.IDEQ(userID)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("load checkin user: %w", err)
	}
	recharge, err := s.queryEffectiveRecharge(ctx, tx.Client(), userID, checkinQualificationCutoff(now, config.QualificationDays))
	if err != nil {
		return nil, err
	}
	eligible, reason := evaluateCheckinEligibility(config, userEntity.Status, userEntity.CreatedAt, now, recharge, false)
	if !eligible {
		switch reason {
		case "disabled":
			return nil, infraerrors.Forbidden("CHECKIN_DISABLED", "daily check-in is disabled")
		case "account_too_new":
			return nil, infraerrors.Forbidden("CHECKIN_ACCOUNT_TOO_NEW", "account has not reached the minimum age for check-in")
		case "recharge_required":
			return nil, infraerrors.Forbidden("CHECKIN_RECHARGE_REQUIRED", "recharge requirement has not been met")
		default:
			return nil, infraerrors.Forbidden("CHECKIN_NOT_ELIGIBLE", "account is not eligible for daily check-in")
		}
	}
	reward, err := secureCheckinReward(config.RewardMin, config.RewardMax)
	if err != nil {
		return nil, err
	}
	notes, _ := json.Marshal(map[string]any{
		"kind":           "daily_checkin",
		"business_date":  businessDate,
		"config_version": config.Version,
	})
	created, err := tx.RedeemCode.Create().
		SetCode(checkinCode(userID, businessDate)).
		SetType(RedeemTypeCheckinBalance).
		SetValue(reward).
		SetStatus(StatusUsed).
		SetUsedBy(userID).
		SetUsedAt(now).
		SetNotes(string(notes)).
		Save(ctx)
	if err != nil {
		if dbent.IsConstraintError(err) {
			_ = tx.Rollback()
			existing, findErr := s.findClaim(ctx, userID, businessDate)
			if findErr != nil || existing == nil {
				return nil, infraerrors.Conflict("CHECKIN_ALREADY_CLAIMED", "daily reward has already been claimed")
			}
			current, getErr := s.client.User.Get(ctx, userID)
			if getErr != nil {
				return nil, getErr
			}
			return claimResult(existing, current.Balance, true), nil
		}
		return nil, fmt.Errorf("create checkin record: %w", err)
	}
	updated, err := tx.User.Update().
		Where(dbuser.IDEQ(userID), dbuser.StatusEQ(StatusActive)).
		AddBalance(reward).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("credit checkin reward: %w", err)
	}
	if updated != 1 {
		return nil, infraerrors.Conflict("CHECKIN_ACCOUNT_CHANGED", "account status changed while claiming reward")
	}
	userEntity, err = tx.User.Get(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("reload checkin balance: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit checkin transaction: %w", err)
	}
	s.invalidateBalance(ctx, userID)
	return &CheckinClaimResult{
		BusinessDate: businessDate,
		Reward:       created.Value,
		NewBalance:   userEntity.Balance,
		ClaimedAt:    now,
	}, nil
}

func claimResult(claim *dbent.RedeemCode, balance float64, already bool) *CheckinClaimResult {
	claimedAt := claim.CreatedAt
	if claim.UsedAt != nil {
		claimedAt = *claim.UsedAt
	}
	return &CheckinClaimResult{
		BusinessDate:   checkinBusinessDate(claimedAt),
		Reward:         claim.Value,
		NewBalance:     balance,
		ClaimedAt:      claimedAt,
		AlreadyClaimed: already,
	}
}

func (s *CheckinService) GetHistory(ctx context.Context, userID int64, limit int) ([]CheckinHistoryItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.client.RedeemCode.Query().
		Where(
			redeemcode.UsedByEQ(userID),
			redeemcode.TypeEQ(RedeemTypeCheckinBalance),
			redeemcode.StatusEQ(StatusUsed),
		).
		Order(dbent.Desc(redeemcode.FieldUsedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list checkin history: %w", err)
	}
	out := make([]CheckinHistoryItem, 0, len(rows))
	for _, row := range rows {
		claimedAt := row.CreatedAt
		if row.UsedAt != nil {
			claimedAt = *row.UsedAt
		}
		out = append(out, CheckinHistoryItem{
			ID:           row.ID,
			BusinessDate: checkinBusinessDate(claimedAt),
			Reward:       row.Value,
			ClaimedAt:    claimedAt,
		})
	}
	return out, nil
}

func (s *CheckinService) findClaim(ctx context.Context, userID int64, businessDate string) (*dbent.RedeemCode, error) {
	row, err := s.client.RedeemCode.Query().Where(redeemcode.CodeEQ(checkinCode(userID, businessDate))).Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load daily checkin: %w", err)
	}
	if row.Type != RedeemTypeCheckinBalance || row.UsedBy == nil || *row.UsedBy != userID {
		return nil, infraerrors.Conflict("CHECKIN_RECORD_CONFLICT", "daily check-in record conflicts with an existing code")
	}
	return row, nil
}

func (s *CheckinService) queryEffectiveRecharge(ctx context.Context, client *dbent.Client, userID int64, cutoff time.Time) (float64, error) {
	const query = `
		SELECT GREATEST(
			COALESCE((
				SELECT SUM(value)
				FROM redeem_codes
				WHERE used_by = $1
				  AND status = 'used'
				  AND type IN ('balance', 'admin_balance')
				  AND value > 0
				  AND used_at >= $2
			), 0)
			- COALESCE((
				SELECT SUM(refund_amount)
				FROM payment_orders
				WHERE user_id = $1
				  AND order_type = 'balance'
				  AND refund_amount > 0
				  AND COALESCE(completed_at, paid_at, created_at) >= $2
			), 0),
			0
		)::double precision`
	rows, err := client.QueryContext(ctx, query, userID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("query effective recharge: %w", err)
	}
	defer rows.Close()
	var amount float64
	if !rows.Next() {
		return 0, fmt.Errorf("query effective recharge returned no rows")
	}
	if err := rows.Scan(&amount); err != nil {
		return 0, fmt.Errorf("scan effective recharge: %w", err)
	}
	return amount, rows.Err()
}

func secureCheckinReward(minAmount, maxAmount float64) (float64, error) {
	minCents := int64(math.Round(minAmount * 100))
	maxCents := int64(math.Round(maxAmount * 100))
	if minCents <= 0 || maxCents < minCents {
		return 0, errors.New("invalid checkin reward range")
	}
	span := big.NewInt(maxCents - minCents + 1)
	offset, err := rand.Int(rand.Reader, span)
	if err != nil {
		return 0, fmt.Errorf("generate checkin reward: %w", err)
	}
	return float64(minCents+offset.Int64()) / 100, nil
}

func checkinBusinessDate(now time.Time) string {
	return now.In(checkinLocation).Format("2006-01-02")
}

func checkinCode(userID int64, businessDate string) string {
	compactDate := businessDate
	if parsed, err := time.Parse("2006-01-02", businessDate); err == nil {
		compactDate = parsed.Format("20060102")
	}
	return "CI-" + compactDate + "-" + strconv.FormatInt(userID, 36)
}

func checkinQualificationCutoff(now time.Time, days int) time.Time {
	local := now.In(checkinLocation)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, checkinLocation)
	return start.AddDate(0, 0, -(days - 1)).UTC()
}

func checkinNextReset(now time.Time) time.Time {
	local := now.In(checkinLocation)
	return time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, checkinLocation).UTC()
}

func (s *CheckinService) invalidateBalance(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCacheService != nil {
		_ = s.billingCacheService.InvalidateUserBalance(ctx, userID)
	}
}

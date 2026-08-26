package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
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
	Enabled            bool                `json:"enabled"`
	MinRechargeAmount  float64             `json:"min_recharge_amount"`
	QualificationDays  int                 `json:"qualification_days"`
	RewardMin          float64             `json:"reward_min"`
	RewardMax          float64             `json:"reward_max"`
	DefaultTierName    string              `json:"default_tier_name"`
	DefaultTierVisible bool                `json:"default_tier_visible"`
	RewardTiers        []CheckinRewardTier `json:"reward_tiers"`
	MaxPeriodReward    float64             `json:"max_period_reward"`
	MinAccountAgeHours int                 `json:"min_account_age_hours"`
	Timezone           string              `json:"timezone"`
	Version            int                 `json:"version"`
}

type CheckinRewardTier struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	MinRechargeAmount    float64 `json:"min_recharge_amount"`
	RewardMin            float64 `json:"reward_min"`
	RewardMax            float64 `json:"reward_max"`
	Enabled              bool    `json:"enabled"`
	Visible              bool    `json:"visible"`
	QualifiedOnlyVisible bool    `json:"qualified_only_visible"`
	ShowNextProgress     bool    `json:"show_next_progress"`
	CustomButtonEnabled  bool    `json:"custom_button_enabled"`
	ButtonColor          string  `json:"button_color"`
	TierBadgeEnabled     bool    `json:"tier_badge_enabled"`
	IsDefault            bool    `json:"is_default,omitempty"`
}

func DefaultCheckinConfig() CheckinConfig {
	return CheckinConfig{
		Enabled:            false,
		MinRechargeAmount:  10,
		QualificationDays:  30,
		RewardMin:          0.01,
		RewardMax:          0.05,
		DefaultTierName:    "默认档位",
		DefaultTierVisible: true,
		RewardTiers:        []CheckinRewardTier{},
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
	TotalReward        float64              `json:"total_reward"`
	RechargeAmount     float64              `json:"recharge_amount"`
	MinRechargeAmount  float64              `json:"min_recharge_amount"`
	QualificationDays  int                  `json:"qualification_days"`
	MinAccountAgeHours int                  `json:"min_account_age_hours"`
	RewardMin          float64              `json:"reward_min"`
	RewardMax          float64              `json:"reward_max"`
	CurrentTier        *CheckinRewardTier   `json:"current_tier,omitempty"`
	NextTier           *CheckinRewardTier   `json:"next_tier,omitempty"`
	RewardTiers        []CheckinRewardTier  `json:"reward_tiers"`
	NextResetAt        time.Time            `json:"next_reset_at"`
	History            []CheckinHistoryItem `json:"history"`
}

type CheckinClaimResult struct {
	BusinessDate   string    `json:"business_date"`
	Reward         float64   `json:"reward"`
	NewBalance     float64   `json:"new_balance"`
	ClaimedAt      time.Time `json:"claimed_at"`
	AlreadyClaimed bool      `json:"already_claimed"`
	TierID         string    `json:"tier_id"`
	TierName       string    `json:"tier_name"`
	TierIsDefault  bool      `json:"tier_is_default"`
}

type checkinRecordMetadata struct {
	Kind          string `json:"kind"`
	BusinessDate  string `json:"business_date"`
	ConfigVersion int    `json:"config_version"`
	TierID        string `json:"tier_id"`
	TierName      string `json:"tier_name"`
	TierIsDefault bool   `json:"tier_is_default"`
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
	var storedTierFields struct {
		RewardTiers []map[string]json.RawMessage `json:"reward_tiers"`
	}
	if err := json.Unmarshal([]byte(value), &storedTierFields); err != nil {
		return CheckinConfig{}, fmt.Errorf("decode checkin tier fields: %w", err)
	}
	for index := range config.RewardTiers {
		if index >= len(storedTierFields.RewardTiers) {
			config.RewardTiers[index].Enabled = true
			continue
		}
		fields := storedTierFields.RewardTiers[index]
		if _, exists := fields["enabled"]; !exists {
			config.RewardTiers[index].Enabled = true
		}
		if _, exists := fields["custom_button_enabled"]; !exists {
			_ = json.Unmarshal(fields["exclusive_ui_enabled"], &config.RewardTiers[index].CustomButtonEnabled)
		}
	}
	config = normalizeCheckinConfig(config)
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
	config = normalizeCheckinConfig(config)
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
	if strings.TrimSpace(config.DefaultTierName) == "" || len([]rune(config.DefaultTierName)) > 30 {
		return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "default tier name must contain between 1 and 30 characters")
	}
	if len(config.RewardTiers) > 20 {
		return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "no more than 20 custom reward tiers are allowed")
	}
	seenIDs := map[string]struct{}{"default": {}}
	seenThresholds := map[int64]struct{}{int64(math.Round(config.MinRechargeAmount * 100)): {}}
	for _, tier := range config.RewardTiers {
		if tier.ID == "" || len(tier.ID) > 64 {
			return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "reward tier ID must contain between 1 and 64 characters")
		}
		if _, exists := seenIDs[tier.ID]; exists {
			return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "reward tier IDs must be unique")
		}
		seenIDs[tier.ID] = struct{}{}
		if tier.Name == "" || len([]rune(tier.Name)) > 30 {
			return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "reward tier name must contain between 1 and 30 characters")
		}
		if tier.MinRechargeAmount <= config.MinRechargeAmount || !isCentAmount(tier.MinRechargeAmount) {
			return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "custom tier recharge amount must be greater than the default tier and use at most two decimals")
		}
		thresholdCents := int64(math.Round(tier.MinRechargeAmount * 100))
		if _, exists := seenThresholds[thresholdCents]; exists {
			return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "reward tier recharge amounts must be unique")
		}
		seenThresholds[thresholdCents] = struct{}{}
		if tier.RewardMin <= 0 || tier.RewardMax < tier.RewardMin || !isCentAmount(tier.RewardMin) || !isCentAmount(tier.RewardMax) {
			return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "custom tier reward range must use positive amounts with at most two decimals")
		}
		if tier.Enabled && float64(config.QualificationDays)*tier.RewardMax > config.MaxPeriodReward+0.0000001 {
			return infraerrors.BadRequest("CHECKIN_CONFIG_COST_TOO_HIGH", "maximum tier reward over the qualification period cannot exceed the configured safety limit")
		}
		if tier.ButtonColor != "" && !isCheckinButtonColor(tier.ButtonColor) {
			return infraerrors.BadRequest("CHECKIN_CONFIG_INVALID", "custom tier button color is not supported")
		}
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

func normalizeCheckinConfig(config CheckinConfig) CheckinConfig {
	config.DefaultTierName = strings.TrimSpace(config.DefaultTierName)
	if config.DefaultTierName == "" {
		config.DefaultTierName = DefaultCheckinConfig().DefaultTierName
	}
	if config.RewardTiers == nil {
		config.RewardTiers = []CheckinRewardTier{}
	}
	usedIDs := make(map[string]struct{}, len(config.RewardTiers)+1)
	usedIDs["default"] = struct{}{}
	for index := range config.RewardTiers {
		tier := &config.RewardTiers[index]
		tier.ID = strings.TrimSpace(tier.ID)
		tier.Name = strings.TrimSpace(tier.Name)
		tier.ButtonColor = strings.ToLower(strings.TrimSpace(tier.ButtonColor))
		if tier.ButtonColor == "" {
			tier.ButtonColor = "emerald"
		}
		tier.IsDefault = false
		if tier.ID != "" {
			usedIDs[tier.ID] = struct{}{}
		}
	}
	nextID := 1
	for index := range config.RewardTiers {
		if config.RewardTiers[index].ID != "" {
			continue
		}
		for {
			candidate := "tier-" + strconv.Itoa(nextID)
			nextID++
			if _, exists := usedIDs[candidate]; exists {
				continue
			}
			config.RewardTiers[index].ID = candidate
			usedIDs[candidate] = struct{}{}
			break
		}
	}
	sort.SliceStable(config.RewardTiers, func(i, j int) bool {
		return config.RewardTiers[i].MinRechargeAmount < config.RewardTiers[j].MinRechargeAmount
	})
	return config
}

func (config CheckinConfig) allRewardTiers() []CheckinRewardTier {
	tiers := make([]CheckinRewardTier, 0, len(config.RewardTiers)+1)
	tiers = append(tiers, CheckinRewardTier{
		ID:                "default",
		Name:              config.DefaultTierName,
		MinRechargeAmount: config.MinRechargeAmount,
		RewardMin:         config.RewardMin,
		RewardMax:         config.RewardMax,
		Enabled:           true,
		Visible:           config.DefaultTierVisible,
		ButtonColor:       "emerald",
		IsDefault:         true,
	})
	for _, tier := range config.RewardTiers {
		if tier.Enabled {
			tiers = append(tiers, tier)
		}
	}
	return tiers
}

func selectCheckinRewardTier(config CheckinConfig, recharge float64) (CheckinRewardTier, bool) {
	tiers := config.allRewardTiers()
	selected := tiers[0]
	if recharge+0.0000001 < selected.MinRechargeAmount {
		return selected, false
	}
	for _, tier := range tiers[1:] {
		if recharge+0.0000001 < tier.MinRechargeAmount {
			break
		}
		selected = tier
	}
	return selected, true
}

func visibleCheckinRewardTiers(config CheckinConfig, recharge float64) []CheckinRewardTier {
	all := config.allRewardTiers()
	visible := make([]CheckinRewardTier, 0, len(all))
	for _, tier := range all {
		if tier.Visible && (!tier.QualifiedOnlyVisible || recharge+0.0000001 >= tier.MinRechargeAmount) {
			visible = append(visible, tier)
		}
	}
	return visible
}

func nextCheckinRewardTier(config CheckinConfig, recharge float64) (CheckinRewardTier, bool) {
	tiers := config.allRewardTiers()
	if len(tiers) < 2 || recharge+0.0000001 < tiers[0].MinRechargeAmount {
		return CheckinRewardTier{}, false
	}
	currentIndex := 0
	for index := 1; index < len(tiers); index++ {
		if recharge+0.0000001 < tiers[index].MinRechargeAmount {
			break
		}
		currentIndex = index
	}
	nextIndex := currentIndex + 1
	if nextIndex >= len(tiers) || !tiers[nextIndex].ShowNextProgress {
		return CheckinRewardTier{}, false
	}
	return tiers[nextIndex], true
}

func isCheckinButtonColor(value string) bool {
	switch value {
	case "emerald", "blue", "amber", "rose", "violet", "slate":
		return true
	default:
		return false
	}
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
	totalReward, err := s.queryTotalCheckinReward(ctx, userID)
	if err != nil {
		return nil, err
	}
	selectedTier, tierReached := selectCheckinRewardTier(config, recharge)
	status := &CheckinStatus{
		Enabled:            config.Enabled,
		BusinessDate:       businessDate,
		CurrentBalance:     userEntity.Balance,
		TotalReward:        totalReward,
		RechargeAmount:     recharge,
		MinRechargeAmount:  config.MinRechargeAmount,
		QualificationDays:  config.QualificationDays,
		MinAccountAgeHours: config.MinAccountAgeHours,
		RewardMin:          selectedTier.RewardMin,
		RewardMax:          selectedTier.RewardMax,
		RewardTiers:        visibleCheckinRewardTiers(config, recharge),
		NextResetAt:        checkinNextReset(now),
		History:            history,
	}
	if tierReached {
		status.CurrentTier = &selectedTier
		if nextTier, ok := nextCheckinRewardTier(config, recharge); ok {
			status.NextTier = &nextTier
		}
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
	selectedTier, ok := selectCheckinRewardTier(config, recharge)
	if !ok {
		return nil, infraerrors.Forbidden("CHECKIN_RECHARGE_REQUIRED", "recharge requirement has not been met")
	}
	reward, err := secureCheckinReward(selectedTier.RewardMin, selectedTier.RewardMax)
	if err != nil {
		return nil, err
	}
	notes, _ := json.Marshal(checkinRecordMetadata{
		Kind:          "daily_checkin",
		BusinessDate:  businessDate,
		ConfigVersion: config.Version,
		TierID:        selectedTier.ID,
		TierName:      selectedTier.Name,
		TierIsDefault: selectedTier.IsDefault,
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
		BusinessDate:  businessDate,
		Reward:        created.Value,
		NewBalance:    userEntity.Balance,
		ClaimedAt:     now,
		TierID:        selectedTier.ID,
		TierName:      selectedTier.Name,
		TierIsDefault: selectedTier.IsDefault,
	}, nil
}

func claimResult(claim *dbent.RedeemCode, balance float64, already bool) *CheckinClaimResult {
	claimedAt := claim.CreatedAt
	if claim.UsedAt != nil {
		claimedAt = *claim.UsedAt
	}
	metadata := parseCheckinRecordMetadata(claim.Notes)
	return &CheckinClaimResult{
		BusinessDate:   checkinBusinessDate(claimedAt),
		Reward:         claim.Value,
		NewBalance:     balance,
		ClaimedAt:      claimedAt,
		AlreadyClaimed: already,
		TierID:         metadata.TierID,
		TierName:       metadata.TierName,
		TierIsDefault:  metadata.TierIsDefault,
	}
}

func parseCheckinRecordMetadata(notes *string) checkinRecordMetadata {
	metadata := checkinRecordMetadata{TierID: "default", TierIsDefault: true}
	if notes == nil || strings.TrimSpace(*notes) == "" {
		return metadata
	}
	var stored checkinRecordMetadata
	if err := json.Unmarshal([]byte(*notes), &stored); err != nil {
		return metadata
	}
	if stored.TierID == "" {
		return metadata
	}
	return stored
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

func (s *CheckinService) queryTotalCheckinReward(ctx context.Context, userID int64) (float64, error) {
	const query = `
		SELECT COALESCE(SUM(value), 0)::double precision
		FROM redeem_codes
		WHERE used_by = $1 AND type = $2 AND status = 'used'`
	var total float64
	if err := scanCheckinScalar(ctx, s.client, query, []any{userID, RedeemTypeCheckinBalance}, &total); err != nil {
		return 0, fmt.Errorf("query total checkin reward: %w", err)
	}
	return total, nil
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

package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type CheckinDailyStat struct {
	Date          string  `json:"date"`
	UserCount     int64   `json:"user_count"`
	RewardTotal   float64 `json:"reward_total"`
	RewardAverage float64 `json:"reward_average"`
	RewardMin     float64 `json:"reward_min"`
	RewardMax     float64 `json:"reward_max"`
}

type CheckinAdminSummary struct {
	TodayUsers         int64                      `json:"today_users"`
	TodayReward        float64                    `json:"today_reward"`
	TodayAverage       float64                    `json:"today_average"`
	Users7Days         int64                      `json:"users_7_days"`
	Reward30Days       float64                    `json:"reward_30_days"`
	EligibleUsers      int64                      `json:"eligible_users"`
	TierQualifiedStats []CheckinTierQualifiedStat `json:"tier_qualified_stats"`
	Daily              []CheckinDailyStat         `json:"daily"`
}

type CheckinTierQualifiedStat struct {
	TierID            string  `json:"tier_id"`
	TierName          string  `json:"tier_name"`
	MinRechargeAmount float64 `json:"min_recharge_amount"`
	QualifiedUsers    int64   `json:"qualified_users"`
	IsDefault         bool    `json:"is_default"`
}

type CheckinAdminRecord struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	Email          string    `json:"email"`
	Username       string    `json:"username"`
	Reward         float64   `json:"reward"`
	CurrentBalance float64   `json:"current_balance"`
	BusinessDate   string    `json:"business_date"`
	ClaimedAt      time.Time `json:"claimed_at"`
	TierID         string    `json:"tier_id"`
	TierName       string    `json:"tier_name"`
	TierIsDefault  bool      `json:"tier_is_default"`
}

type CheckinUserReport struct {
	UserID            int64      `json:"user_id"`
	Email             string     `json:"email"`
	Username          string     `json:"username"`
	Status            string     `json:"status"`
	CurrentBalance    float64    `json:"current_balance"`
	TotalRecharge     float64    `json:"total_recharge"`
	EffectiveRecharge float64    `json:"effective_recharge"`
	CheckinDays       int64      `json:"checkin_days"`
	RewardTotal       float64    `json:"reward_total"`
	LastCheckinAt     *time.Time `json:"last_checkin_at"`
	Eligible          bool       `json:"eligible"`
	TierID            string     `json:"tier_id"`
	TierName          string     `json:"tier_name"`
	TierIsDefault     bool       `json:"tier_is_default"`
}

func (s *CheckinService) GetAdminSummary(ctx context.Context) (*CheckinAdminSummary, error) {
	now := time.Now().UTC()
	localNow := now.In(checkinLocation)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day()-29, 0, 0, 0, 0, checkinLocation).UTC()
	end := checkinNextReset(now)
	daily, err := s.GetDailyStats(ctx, start, end)
	if err != nil {
		return nil, err
	}
	summary := &CheckinAdminSummary{Daily: daily}
	for i, item := range daily {
		summary.Reward30Days += item.RewardTotal
		if i >= len(daily)-7 {
			summary.Users7Days += item.UserCount
		}
		if item.Date == checkinBusinessDate(now) {
			summary.TodayUsers = item.UserCount
			summary.TodayReward = item.RewardTotal
			summary.TodayAverage = item.RewardAverage
		}
	}
	config, err := s.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	tiers := config.allRewardTiers()
	counts, err := s.countEligibleUsersByThresholds(ctx, checkinQualificationCutoff(now, config.QualificationDays), now, config, tiers)
	if err != nil {
		return nil, err
	}
	summary.TierQualifiedStats = make([]CheckinTierQualifiedStat, 0, len(tiers))
	for index, tier := range tiers {
		stat := CheckinTierQualifiedStat{
			TierID:            tier.ID,
			TierName:          tier.Name,
			MinRechargeAmount: tier.MinRechargeAmount,
			QualifiedUsers:    counts[index],
			IsDefault:         tier.IsDefault,
		}
		summary.TierQualifiedStats = append(summary.TierQualifiedStats, stat)
	}
	if len(counts) > 0 {
		summary.EligibleUsers = counts[0]
	}
	return summary, nil
}

func (s *CheckinService) GetDailyStats(ctx context.Context, start, end time.Time) ([]CheckinDailyStat, error) {
	if !end.After(start) {
		return nil, fmt.Errorf("end date must be after start date")
	}
	const query = `
		SELECT TO_CHAR((used_at AT TIME ZONE 'Asia/Shanghai')::date, 'YYYY-MM-DD'),
		       COUNT(DISTINCT used_by),
		       COALESCE(SUM(value), 0)::double precision,
		       COALESCE(AVG(value), 0)::double precision,
		       COALESCE(MIN(value), 0)::double precision,
		       COALESCE(MAX(value), 0)::double precision
		FROM redeem_codes
		WHERE type = $1 AND status = 'used' AND used_at >= $2 AND used_at < $3
		GROUP BY (used_at AT TIME ZONE 'Asia/Shanghai')::date
		ORDER BY (used_at AT TIME ZONE 'Asia/Shanghai')::date`
	rows, err := s.client.QueryContext(ctx, query, RedeemTypeCheckinBalance, start, end)
	if err != nil {
		return nil, fmt.Errorf("query daily checkin report: %w", err)
	}
	defer rows.Close()
	found := make(map[string]CheckinDailyStat)
	for rows.Next() {
		var item CheckinDailyStat
		if err := rows.Scan(&item.Date, &item.UserCount, &item.RewardTotal, &item.RewardAverage, &item.RewardMin, &item.RewardMax); err != nil {
			return nil, fmt.Errorf("scan daily checkin report: %w", err)
		}
		found[item.Date] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]CheckinDailyStat, 0, 31)
	for day := start.In(checkinLocation); day.Before(end.In(checkinLocation)); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		item, ok := found[key]
		if !ok {
			item.Date = key
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *CheckinService) ListAdminRecords(ctx context.Context, page, pageSize int, search string) ([]CheckinAdminRecord, int64, error) {
	page, pageSize = normalizeCheckinPagination(page, pageSize)
	search = strings.TrimSpace(search)
	pattern := "%" + strings.ToLower(search) + "%"
	const countQuery = `
		SELECT COUNT(*)
		FROM redeem_codes rc
		JOIN users u ON u.id = rc.used_by AND u.deleted_at IS NULL
		WHERE rc.type = $1 AND rc.status = 'used'
		  AND ($2 = '' OR LOWER(u.email) LIKE $3 OR LOWER(u.username) LIKE $3 OR u.id::text = $2)`
	var total int64
	if err := scanCheckinScalar(ctx, s.client, countQuery, []any{RedeemTypeCheckinBalance, search, pattern}, &total); err != nil {
		return nil, 0, fmt.Errorf("count checkin records: %w", err)
	}
	const query = `
		SELECT rc.id, u.id, u.email, u.username, rc.value::double precision,
		       u.balance::double precision,
		       TO_CHAR((rc.used_at AT TIME ZONE 'Asia/Shanghai')::date, 'YYYY-MM-DD'),
		       rc.used_at, rc.notes
		FROM redeem_codes rc
		JOIN users u ON u.id = rc.used_by AND u.deleted_at IS NULL
		WHERE rc.type = $1 AND rc.status = 'used'
		  AND ($2 = '' OR LOWER(u.email) LIKE $3 OR LOWER(u.username) LIKE $3 OR u.id::text = $2)
		ORDER BY rc.used_at DESC, rc.id DESC
		LIMIT $4 OFFSET $5`
	rows, err := s.client.QueryContext(ctx, query, RedeemTypeCheckinBalance, search, pattern, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list checkin records: %w", err)
	}
	defer rows.Close()
	items := make([]CheckinAdminRecord, 0, pageSize)
	for rows.Next() {
		var item CheckinAdminRecord
		var notes sql.NullString
		if err := rows.Scan(&item.ID, &item.UserID, &item.Email, &item.Username, &item.Reward, &item.CurrentBalance, &item.BusinessDate, &item.ClaimedAt, &notes); err != nil {
			return nil, 0, fmt.Errorf("scan checkin record: %w", err)
		}
		var notesPointer *string
		if notes.Valid {
			notesPointer = &notes.String
		}
		metadata := parseCheckinRecordMetadata(notesPointer)
		item.TierID = metadata.TierID
		item.TierName = metadata.TierName
		item.TierIsDefault = metadata.TierIsDefault
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *CheckinService) ListUserReports(ctx context.Context, page, pageSize int, search, sortBy, sortOrder, qualification string) ([]CheckinUserReport, int64, error) {
	page, pageSize = normalizeCheckinPagination(page, pageSize)
	config, err := s.GetConfig(ctx)
	if err != nil {
		return nil, 0, err
	}
	now := time.Now().UTC()
	cutoff := checkinQualificationCutoff(now, config.QualificationDays)
	minimumCreatedAt := now.Add(-time.Duration(config.MinAccountAgeHours) * time.Hour)
	search = strings.TrimSpace(search)
	pattern := "%" + strings.ToLower(search) + "%"
	qualification = normalizeCheckinQualification(qualification)
	const reportCTE = `
		WITH recharge AS (
			SELECT used_by AS user_id,
			       SUM(value) FILTER (WHERE type IN ('balance', 'admin_balance'))::double precision AS total_recharge,
			       SUM(value) FILTER (WHERE type IN ('balance', 'admin_balance') AND used_at >= $1)::double precision AS effective_recharge
			FROM redeem_codes
			WHERE status = 'used' AND used_by IS NOT NULL AND value > 0
			  AND type IN ('balance', 'admin_balance')
			GROUP BY used_by
		), refunds AS (
			SELECT user_id,
			       SUM(refund_amount)::double precision AS total_refund,
			       SUM(refund_amount) FILTER (WHERE COALESCE(completed_at, paid_at, created_at) >= $1)::double precision AS effective_refund
			FROM payment_orders
			WHERE order_type = 'balance' AND refund_amount > 0
			GROUP BY user_id
		), claims AS (
			SELECT used_by AS user_id, COUNT(*) AS checkin_days,
			       SUM(value)::double precision AS reward_total, MAX(used_at) AS last_checkin_at
			FROM redeem_codes
			WHERE type = $2 AND status = 'used' AND used_by IS NOT NULL
			GROUP BY used_by
		), report AS (
			SELECT u.id AS user_id, u.email, u.username, u.status,
			       u.balance::double precision AS current_balance,
			       GREATEST(COALESCE(r.total_recharge, 0) - COALESCE(f.total_refund, 0), 0) AS total_recharge,
			       GREATEST(COALESCE(r.effective_recharge, 0) - COALESCE(f.effective_refund, 0), 0) AS effective_recharge,
			       COALESCE(c.checkin_days, 0) AS checkin_days,
			       COALESCE(c.reward_total, 0) AS reward_total,
			       c.last_checkin_at,
			       ($5 AND u.status = 'active' AND u.created_at <= $6
			         AND GREATEST(COALESCE(r.effective_recharge, 0) - COALESCE(f.effective_refund, 0), 0) >= $7) AS eligible
			FROM users u
			LEFT JOIN recharge r ON r.user_id = u.id
			LEFT JOIN refunds f ON f.user_id = u.id
			LEFT JOIN claims c ON c.user_id = u.id
			WHERE u.deleted_at IS NULL
			  AND ($3 = '' OR LOWER(u.email) LIKE $4 OR LOWER(u.username) LIKE $4 OR u.id::text = $3)
		)`
	args := []any{
		cutoff,
		RedeemTypeCheckinBalance,
		search,
		pattern,
		config.Enabled,
		minimumCreatedAt,
		config.MinRechargeAmount,
		qualification,
	}
	countQuery := reportCTE + `
		SELECT COUNT(*) FROM report
		WHERE ($8 = '' OR ($8 = 'qualified' AND eligible) OR ($8 = 'unqualified' AND NOT eligible))`
	var total int64
	if err := scanCheckinScalar(ctx, s.client, countQuery, args, &total); err != nil {
		return nil, 0, fmt.Errorf("count checkin users: %w", err)
	}
	orderColumn := checkinUserReportOrder(sortBy)
	direction := "DESC"
	if strings.EqualFold(sortOrder, "asc") {
		direction = "ASC"
	}
	query := fmt.Sprintf(reportCTE+`
		SELECT user_id, email, username, status, current_balance, total_recharge,
		       effective_recharge, checkin_days, reward_total, last_checkin_at, eligible
		FROM report
		WHERE ($8 = '' OR ($8 = 'qualified' AND eligible) OR ($8 = 'unqualified' AND NOT eligible))
		ORDER BY %s %s NULLS LAST, user_id DESC
		LIMIT $9 OFFSET $10`, orderColumn, direction)
	rows, err := s.client.QueryContext(ctx, query, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list checkin user report: %w", err)
	}
	defer rows.Close()
	items := make([]CheckinUserReport, 0, pageSize)
	for rows.Next() {
		var item CheckinUserReport
		var last sql.NullTime
		if err := rows.Scan(&item.UserID, &item.Email, &item.Username, &item.Status, &item.CurrentBalance, &item.TotalRecharge, &item.EffectiveRecharge, &item.CheckinDays, &item.RewardTotal, &last, &item.Eligible); err != nil {
			return nil, 0, fmt.Errorf("scan checkin user report: %w", err)
		}
		if last.Valid {
			item.LastCheckinAt = &last.Time
		}
		if item.Eligible {
			tier, _ := selectCheckinRewardTier(config, item.EffectiveRecharge)
			item.TierID = tier.ID
			item.TierName = tier.Name
			item.TierIsDefault = tier.IsDefault
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *CheckinService) countEligibleUsersByThresholds(
	ctx context.Context,
	cutoff time.Time,
	now time.Time,
	config CheckinConfig,
	tiers []CheckinRewardTier,
) ([]int64, error) {
	counts := make([]int64, len(tiers))
	if !config.Enabled || len(tiers) == 0 {
		return counts, nil
	}
	selects := make([]string, len(tiers))
	args := make([]any, 0, len(tiers)+2)
	args = append(args, cutoff, now.Add(-time.Duration(config.MinAccountAgeHours)*time.Hour))
	for index, tier := range tiers {
		placeholder := index + 3
		selects[index] = fmt.Sprintf("COUNT(*) FILTER (WHERE amount >= $%d)", placeholder)
		args = append(args, tier.MinRechargeAmount)
	}
	query := `
		WITH recharge AS (
			SELECT used_by AS user_id, SUM(value)::double precision AS amount
			FROM redeem_codes
			WHERE used_by IS NOT NULL AND status = 'used'
			  AND type IN ('balance', 'admin_balance') AND value > 0 AND used_at >= $1
			GROUP BY used_by
		), refunds AS (
			SELECT user_id, SUM(refund_amount)::double precision AS amount
			FROM payment_orders
			WHERE order_type = 'balance' AND refund_amount > 0
			  AND COALESCE(completed_at, paid_at, created_at) >= $1
			GROUP BY user_id
		), eligible_recharge AS (
			SELECT GREATEST(COALESCE(r.amount, 0) - COALESCE(f.amount, 0), 0) AS amount
		FROM users u
		LEFT JOIN recharge r ON r.user_id = u.id
		LEFT JOIN refunds f ON f.user_id = u.id
		WHERE u.deleted_at IS NULL AND u.status = 'active'
		  AND u.created_at <= $2
		)
		SELECT ` + strings.Join(selects, ", ") + ` FROM eligible_recharge`
	rows, err := s.client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("count eligible checkin users by tier: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	destinations := make([]any, len(counts))
	for index := range counts {
		destinations[index] = &counts[index]
	}
	if err := rows.Scan(destinations...); err != nil {
		return nil, fmt.Errorf("scan eligible checkin users by tier: %w", err)
	}
	return counts, rows.Err()
}

func scanCheckinScalar(ctx context.Context, client checkinQueryClient, query string, args []any, destination any) error {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	if err := rows.Scan(destination); err != nil {
		return err
	}
	return rows.Err()
}

type checkinQueryClient interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func normalizeCheckinPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func checkinUserReportOrder(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "total_recharge":
		return "total_recharge"
	case "effective_recharge":
		return "effective_recharge"
	case "current_balance":
		return "current_balance"
	case "checkin_days":
		return "checkin_days"
	case "reward_total":
		return "reward_total"
	case "last_checkin_at":
		return "last_checkin_at"
	default:
		return "last_checkin_at"
	}
}

func normalizeCheckinQualification(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "qualified":
		return "qualified"
	case "unqualified":
		return "unqualified"
	default:
		return ""
	}
}

package repository

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// Project only non-secret picker fields; filter before LIMIT so unrelated
// accounts cannot crowd this supplier's candidates out of the first page.
const upstreamSupplierAccountsCTE = `WITH candidates AS (
 SELECT a.id,a.name,a.platform,
  COALESCE(NULLIF(BTRIM(a.credentials->>'base_url', E' \t\n\r'),''),
   CASE a.platform WHEN 'openai' THEN 'https://api.openai.com'
    WHEN 'anthropic' THEN 'https://api.anthropic.com'
    WHEN 'gemini' THEN 'https://generativelanguage.googleapis.com' END) AS endpoint
 FROM accounts a
 WHERE a.deleted_at IS NULL AND a.type='apikey'
  AND a.platform IN ('openai','anthropic','gemini') AND a.parent_account_id IS NULL
  AND COALESCE(a.extra->'synthetic_ui_test','false'::jsonb) != 'true'::jsonb
  AND JSONB_TYPEOF(a.credentials->'api_key')='string'
  AND LENGTH(BTRIM(COALESCE(a.credentials->>'api_key',''), E' \t\n\r')) BETWEEN 1 AND 2000
  AND POSITION(CHR(10) IN a.credentials->>'api_key')=0
  AND POSITION(CHR(13) IN a.credentials->>'api_key')=0
), matching AS (
 SELECT id,name,platform,endpoint FROM candidates
 WHERE endpoint ~* ANY($1::text[]) AND endpoint !~ '[[:space:]]'
  AND POSITION(CHR(92) IN endpoint)=0
  AND ($2='' OR STRPOS(LOWER(name),LOWER($2))>0)
)
`

func (r *upstreamCenterRepository) ListSupplierAccounts(ctx context.Context, query service.UpstreamSupplierAccountsQuery) ([]*service.UpstreamSupplierAccount, int64, error) {
	items := []*service.UpstreamSupplierAccount{}
	patterns := make([]string, 0, len(query.Origins))
	for _, origin := range query.Origins {
		if pattern := upstreamSupplierAccountOriginPattern(origin); pattern != "" {
			patterns = append(patterns, pattern)
		}
	}
	if len(patterns) == 0 {
		return items, 0, nil
	}
	args := []any{pq.Array(patterns), query.Search}
	var total int64
	if err := r.db.QueryRowContext(ctx, upstreamSupplierAccountsCTE+`SELECT COUNT(*) FROM matching`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return items, total, nil
	}
	limit := query.PageSize
	if limit < 1 || limit > 100 {
		limit = 100
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	// Check the page against the total before multiplying, also avoiding integer
	// overflow for a maliciously large page parameter.
	if int64(page-1) > (total-1)/int64(limit) {
		return items, total, nil
	}
	args = append(args, limit, int64(page-1)*int64(limit))
	rows, err := r.db.QueryContext(ctx, upstreamSupplierAccountsCTE+`SELECT id,name,platform,endpoint FROM matching ORDER BY name,id LIMIT $3 OFFSET $4`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		item := new(service.UpstreamSupplierAccount)
		if err := rows.Scan(&item.ID, &item.Name, &item.Platform, &item.Endpoint); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// The anchored authority cannot match example.com.evil, userinfo or another
// port. Regex metacharacters in configured hosts are always escaped.
func upstreamSupplierAccountOriginPattern(origin string) string {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	host := u.Hostname()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	pattern := "^" + regexp.QuoteMeta(u.Scheme+"://"+host)
	if port := u.Port(); port != "" {
		pattern += ":0*" + regexp.QuoteMeta(port)
	} else if u.Scheme == "https" {
		pattern += "(:0*443)?"
	} else {
		pattern += "(:0*80)?"
	}
	return pattern + "(/[^?#]*)?$"
}

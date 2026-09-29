package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *channelContributionRepository) Catalog(ctx context.Context, actor int64, admin bool, groupIDs []int64, before int64, statusPage bool) ([]service.ContributionQuotaSource, error) {
	if admin {
		if err := requireContributionAdmin(ctx, r.db, actor); err != nil {
			return nil, err
		}
	}
	rows, err := r.db.QueryContext(ctx, `SELECT c.id,c.site_name,COALESCE(u.username,''),c.platform,c.status,c.settings,
a.id,COALESCE(a.name,''),COALESCE(a.platform,''),COALESCE(a.status,''),COALESCE(a.schedulable,false),COALESCE(a.extra,'{}'),COALESCE(a.concurrency,5),
a.expires_at,COALESCE(a.auto_pause_on_expired,false),a.rate_limit_reset_at,a.overload_until,a.temp_unschedulable_until,
COALESCE(a.credentials->'model_mapping','{}'),
COALESCE((SELECT jsonb_agg(jsonb_build_object('id',g.id,'name',g.name) ORDER BY g.id) FROM groups g JOIN account_groups ag ON ag.group_id=g.id WHERE ag.account_id=a.id AND g.deleted_at IS NULL AND g.status='active' AND ($1 OR g.id=ANY($2))),'[]'),
(SELECT COALESCE(sum(amount),0) FROM channel_contribution_batch_holds WHERE account_id=a.id AND NOT settled),
COALESCE((SELECT used FROM channel_contribution_rpm WHERE account_id=a.id AND window_start>=date_trunc('minute',NOW())),0)
FROM channel_contributions c LEFT JOIN users u ON u.id=c.user_id AND u.deleted_at IS NULL
JOIN accounts a ON a.id=c.account_id AND a.deleted_at IS NULL AND a.type='apikey'
WHERE ($3::bigint=0 OR c.id<$3) AND c.status='approved'
AND (NOT $4 OR c.settings->>'show_in_status'='true')
AND EXISTS(SELECT 1 FROM account_groups ag JOIN groups g ON g.id=ag.group_id WHERE ag.account_id=a.id AND g.deleted_at IS NULL AND g.status='active' AND ($1 OR g.id=ANY($2)))
ORDER BY c.id DESC LIMIT 51`, admin, pq.Array(groupIDs), before, statusPage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.ContributionQuotaSource{}
	for rows.Next() {
		var item service.ContributionQuotaSource
		var held float64
		var rpmUsed int
		a := &service.Account{Type: service.AccountTypeAPIKey}
		var id sql.NullInt64
		var settings, extra, models, groups []byte
		if err := rows.Scan(&item.ID, &item.SiteName, &item.ContributorName, &item.Platform, &item.Status, &settings, &id, &a.Name, &a.Platform, &a.Status, &a.Schedulable, &extra, &a.Concurrency, &a.ExpiresAt, &a.AutoPauseOnExpired, &a.RateLimitResetAt, &a.OverloadUntil, &a.TempUnschedulableUntil, &models, &groups, &held, &rpmUsed); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(settings, &item.Settings); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(groups, &item.Groups); err != nil {
			return nil, err
		}
		if id.Valid {
			a.ID = id.Int64
			if err := json.Unmarshal(extra, &a.Extra); err != nil {
				return nil, err
			}
			var mapping map[string]any
			if err := json.Unmarshal(models, &mapping); err != nil {
				return nil, err
			}
			a.Credentials = map[string]any{"model_mapping": mapping}
			item.Account = a
			item.QuotaAvailable = contributionAvailable(a, held, 0.00000001)
			item.RPMLimited = a.GetBaseRPM() > 0 && rpmUsed >= a.GetBaseRPM()
		}
		item.ContributorName = service.ForumDisplayName(item.ContributorName)
		out = append(out, item)
	}
	return out, rows.Err()
}

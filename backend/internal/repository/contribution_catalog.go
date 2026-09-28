package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *channelContributionRepository) Quotas(ctx context.Context, actor int64, admin bool, groupIDs []int64, before int64) ([]service.ContributionQuotaSource, error) {
	if admin {
		if err := requireContributionAdmin(ctx, r.db, actor); err != nil {
			return nil, err
		}
	}
	rows, err := r.db.QueryContext(ctx, `SELECT c.id,c.site_name,COALESCE(u.username,''),c.platform,c.status,c.settings,c.new_group_name,COALESCE(rg.name,''),
a.id,COALESCE(a.name,''),COALESCE(a.platform,''),COALESCE(a.status,''),COALESCE(a.schedulable,false),COALESCE(a.extra,'{}'),COALESCE(a.concurrency,5),
a.expires_at,COALESCE(a.auto_pause_on_expired,false),a.rate_limit_reset_at,a.overload_until,a.temp_unschedulable_until,
COALESCE(a.credentials->'model_mapping','{}'),
COALESCE((SELECT jsonb_agg(jsonb_build_object('id',g.id,'name',g.name) ORDER BY g.id) FROM groups g JOIN account_groups ag ON ag.group_id=g.id WHERE ag.account_id=a.id AND g.deleted_at IS NULL AND ($2 OR g.id=ANY($3))),'[]')
FROM channel_contributions c LEFT JOIN users u ON u.id=c.user_id AND u.deleted_at IS NULL
LEFT JOIN groups rg ON rg.id=c.requested_group_id AND rg.deleted_at IS NULL
LEFT JOIN accounts a ON a.id=c.account_id AND a.deleted_at IS NULL
WHERE ($4::bigint=0 OR c.id<$4) AND ($2 OR c.user_id=$1 OR (c.status='approved' AND EXISTS(SELECT 1 FROM account_groups ag WHERE ag.account_id=a.id AND ag.group_id=ANY($3)))) ORDER BY c.id DESC LIMIT 51`, actor, admin, pq.Array(groupIDs), before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.ContributionQuotaSource{}
	for rows.Next() {
		var item service.ContributionQuotaSource
		var newGroupName, requestedGroupName string
		a := &service.Account{Type: service.AccountTypeAPIKey}
		var id sql.NullInt64
		var settings, extra, models, groups []byte
		if err := rows.Scan(&item.ID, &item.SiteName, &item.ContributorName, &item.Platform, &item.Status, &settings, &newGroupName, &requestedGroupName, &id, &a.Name, &a.Platform, &a.Status, &a.Schedulable, &extra, &a.Concurrency, &a.ExpiresAt, &a.AutoPauseOnExpired, &a.RateLimitResetAt, &a.OverloadUntil, &a.TempUnschedulableUntil, &models, &groups); err != nil {
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
		}
		item.ContributorName = service.ForumDisplayName(item.ContributorName)
		if item.Status != "approved" && len(item.Groups) == 0 {
			name := requestedGroupName
			if newGroupName != "" {
				name = newGroupName
			}
			if item.Settings.NewGroupConfig != nil {
				name = item.Settings.NewGroupConfig.Name
			}
			if name != "" {
				item.Groups = []service.ContributionGroupView{{Name: name}}
			}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

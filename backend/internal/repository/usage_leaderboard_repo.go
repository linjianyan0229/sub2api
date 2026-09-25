package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *usageLogRepository) GetUsageLeaderboard(ctx context.Context) ([]service.UsageLeaderboardItem, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT COALESCE(u.username, ''), totals.actual_cost, totals.tokens
		FROM user_usage_lifetime_totals totals
		JOIN users u ON u.id = totals.user_id AND u.deleted_at IS NULL
		WHERE totals.actual_cost > 0 OR totals.tokens > 0
		ORDER BY totals.actual_cost DESC, totals.tokens DESC, totals.user_id ASC
		LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.UsageLeaderboardItem, 0, 20)
	for rows.Next() {
		var item service.UsageLeaderboardItem
		if err := rows.Scan(&item.Username, &item.ActualCost, &item.Tokens); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitor"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 调用方持有贡献记录行锁，账号配置和监控配置在同一事务内生效。
func syncContributionMonitor(ctx context.Context, client *dbent.Client, id int64) error {
	var settingsJSON []byte
	var accountID, owner sql.NullInt64
	var status string
	if err := contributionRow(ctx, client, `SELECT settings,account_id,user_id,status FROM channel_contributions WHERE id=$1 FOR UPDATE`, []any{id}, &settingsJSON, &accountID, &owner, &status); err != nil {
		return err
	}
	var settings service.ContributionSettings
	if err := json.Unmarshal(settingsJSON, &settings); err != nil {
		return err
	}
	repo := &channelContributionRepository{client: client}
	monitors, err := repo.ContributionMonitors(ctx, []int64{id})
	if err != nil {
		return err
	}
	var monitor *service.ChannelMonitor
	if len(monitors) > 0 {
		monitor = monitors[0]
	}
	monitorRepo := &channelMonitorRepository{client: client}
	if status != "approved" || !settings.ShowInStatus || !accountID.Valid || !owner.Valid {
		if monitor == nil {
			return nil
		}
		monitor.Enabled = false
		return monitorRepo.Update(ctx, monitor)
	}
	account, err := newAccountRepositoryWithSQL(client, client, nil).GetByID(ctx, accountID.Int64)
	if err != nil {
		return err
	}
	model := settings.MonitorModel
	if model == "" {
		models := make([]string, 0)
		for name := range account.GetModelMapping() {
			if !strings.ContainsAny(name, "*?") {
				models = append(models, name)
			}
		}
		sort.Strings(models)
		if len(models) > 0 {
			model = models[0]
		}
	}
	if model == "" {
		if monitor == nil {
			return nil
		}
		monitor.Enabled = false
		return monitorRepo.Update(ctx, monitor)
	}
	if monitor == nil {
		monitor = &service.ChannelMonitor{CreatedBy: owner.Int64, ContributionID: id, IntervalSeconds: 60}
	}
	monitor.Name, monitor.Provider, monitor.PrimaryModel = account.Name, account.Platform, model
	monitor.AccountID, monitor.Enabled = &accountID.Int64, true
	monitor.APIMode = settings.MonitorAPIMode
	// 凭据只从关联账号实时读取；此非密钥标记满足原表非空约束。
	monitor.APIKey, monitor.Endpoint = "linked-contribution", ""
	if monitor.ID == 0 {
		return monitorRepo.Create(ctx, monitor)
	}
	return monitorRepo.Update(ctx, monitor)
}

func (r *channelContributionRepository) ContributionMonitors(ctx context.Context, ids []int64) ([]*service.ChannelMonitor, error) {
	out := make([]*service.ChannelMonitor, 0, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	values := make([]any, 0, len(ids))
	for _, id := range ids {
		values = append(values, strconv.FormatInt(id, 10))
	}
	rows, err := r.client.ChannelMonitor.Query().Where(func(s *entsql.Selector) {
		s.Where(sqljson.ValueIn(channelmonitor.FieldExtraHeaders, values, sqljson.Path(service.ChannelMonitorContributionMetadataKey)))
	}).Order(dbent.Asc(channelmonitor.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out = append(out, entToServiceMonitor(row))
	}
	return out, nil
}

func (r *channelContributionRepository) ContributionProbeAccount(ctx context.Context, id int64) (*service.Account, error) {
	var accountID int64
	err := contributionRow(ctx, r.db, `SELECT a.id FROM channel_contributions c
JOIN accounts a ON a.id=c.account_id AND a.deleted_at IS NULL AND a.type='apikey' AND a.platform=c.platform
JOIN users u ON u.id=c.user_id AND u.deleted_at IS NULL AND u.status='active'
WHERE c.id=$1 AND c.status='approved' AND c.settings->>'show_in_status'='true'
AND EXISTS(SELECT 1 FROM account_groups ag JOIN groups g ON g.id=ag.group_id AND g.deleted_at IS NULL AND g.status='active' WHERE ag.account_id=a.id)`, []any{id}, &accountID)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrContributionNotFound, nil)
	}
	return newAccountRepositoryWithSQL(r.client, r.db, nil).GetByID(ctx, accountID)
}

func (r *channelContributionRepository) SyncContributionMonitors(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx, `SELECT c.id FROM channel_contributions c JOIN accounts a ON a.id=c.account_id AND a.deleted_at IS NULL WHERE c.status='approved' AND c.settings->>'show_in_status'='true' ORDER BY c.id`)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		tx, err := r.client.Tx(ctx)
		if err != nil {
			return err
		}
		if err := syncContributionMonitor(dbent.NewTxContext(ctx, tx), tx.Client(), id); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

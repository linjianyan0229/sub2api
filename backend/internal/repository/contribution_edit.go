package repository

import (
	"context"
	"encoding/json"
	"sort"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const ownedContributionAccount = ` FROM channel_contributions c
JOIN users u ON u.id=c.user_id AND u.deleted_at IS NULL AND u.status='active'
JOIN accounts a ON a.id=c.account_id AND a.deleted_at IS NULL AND a.type='apikey' AND a.platform=c.platform
WHERE c.id=$1 AND c.user_id=$2 AND c.status='approved'`

func (r *channelContributionRepository) GetOwnedAccount(ctx context.Context, actor, id int64) (*service.ContributionAccountView, error) {
	v := &service.ContributionAccountView{}
	var extra, models []byte
	err := contributionRow(ctx, r.db, `SELECT a.name,COALESCE(a.credentials->>'base_url',''),a.platform,a.concurrency,a.extra,
COALESCE(a.credentials->'model_mapping','{}'),COALESCE((c.settings->>'show_in_status')::boolean,false)`+ownedContributionAccount,
		[]any{id, actor}, &v.SiteName, &v.BaseURL, &v.Platform, &v.Concurrency, &extra, &models, &v.ShowInStatus)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrContributionNotFound, nil)
	}
	a := &service.Account{}
	if err := json.Unmarshal(extra, &a.Extra); err != nil {
		return nil, err
	}
	var mapping map[string]any
	if err := json.Unmarshal(models, &mapping); err != nil {
		return nil, err
	}
	v.RPM, v.DailyLimit, v.TotalLimit = a.GetBaseRPM(), a.GetQuotaDailyLimit(), a.GetQuotaLimit()
	v.Models = make([]string, 0, len(mapping))
	for model := range mapping {
		v.Models = append(v.Models, model)
	}
	sort.Strings(v.Models)
	return v, nil
}

func (r *channelContributionRepository) UpdateOwnedAccount(ctx context.Context, actor, id int64, in service.ContributionAccountInput, timezone string) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	txCtx, client := dbent.NewTxContext(ctx, tx), tx.Client()
	var accountID int64
	// 与扣费互斥，权限与账号存活状态在写入事务内重新确认。
	err = contributionRow(txCtx, client, `SELECT a.id`+ownedContributionAccount+` FOR UPDATE OF c,a`, []any{id, actor}, &accountID)
	if err != nil {
		return translatePersistenceError(err, service.ErrContributionNotFound, nil)
	}
	accounts := newAccountRepositoryWithSQL(client, client, nil)
	account, err := accounts.GetByID(txCtx, accountID)
	if err != nil {
		return err
	}
	extra := account.Extra
	if extra == nil {
		extra = map[string]any{}
	}
	extra["base_rpm"], extra["quota_daily_limit"], extra["quota_limit"] = in.RPM, in.DailyLimit, in.TotalLimit
	// 已有日刷新策略、已用金额和周期原样保留，仅首次启用日额度时补默认策略。
	if _, exists := extra["quota_daily_reset_mode"]; !exists && in.DailyLimit > 0 {
		extra["quota_daily_reset_mode"], extra["quota_daily_reset_hour"], extra["quota_reset_timezone"] = "fixed", 0, timezone
		service.ComputeQuotaResetAt(extra)
	}
	mapping := map[string]any{}
	existingModels, _ := account.Credentials["model_mapping"].(map[string]any)
	for _, model := range in.Models {
		if target, exists := existingModels[model]; exists {
			mapping[model] = target
		} else {
			mapping[model] = model
		}
	}
	credentials := account.Credentials
	if credentials == nil {
		credentials = map[string]any{}
	}
	credentials["base_url"] = in.BaseURL
	delete(credentials, "model_mapping")
	if len(mapping) > 0 {
		credentials["model_mapping"] = mapping
	}
	if in.APIKey != "" {
		credentials["api_key"] = in.APIKey
	}
	settings := service.ContributionSettings{RPM: in.RPM, Concurrency: in.Concurrency, DailyLimit: in.DailyLimit, TotalLimit: in.TotalLimit, Models: in.Models, ShowInStatus: in.ShowInStatus}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	account.Name, account.Concurrency, account.Extra, account.Credentials = in.SiteName, in.Concurrency, extra, credentials
	// 复用账号更新对用量、上游快照及调度 outbox 的保护。
	if err := accounts.UpdateWithAccountBillingSettings(txCtx, account, nil, nil, nil); err != nil {
		return err
	}
	if _, err := client.ExecContext(txCtx, `UPDATE channel_contributions SET site_name=$2,base_url=$3,settings=settings || $4::jsonb,updated_at=NOW() WHERE id=$1`, id, in.SiteName, in.BaseURL, string(settingsJSON)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	newAccountRepositoryWithSQL(r.client, r.db, r.schedulerCache).syncSchedulerAccountSnapshot(ctx, accountID)
	return nil
}

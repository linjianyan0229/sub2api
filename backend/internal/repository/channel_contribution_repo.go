package repository

import (
	"context"
	"database/sql"
	"errors"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type channelContributionRepository struct {
	client *dbent.Client
	db     *sql.DB
}

func NewChannelContributionRepository(client *dbent.Client, db *sql.DB) service.ChannelContributionRepository {
	return &channelContributionRepository{client: client, db: db}
}

func contributionRow(ctx context.Context, exec sqlExecutor, query string, args []any, dest ...any) error {
	rows, err := exec.QueryContext(ctx, query, args...)
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
	return rows.Scan(dest...)
}

func requireContributionAdmin(ctx context.Context, exec sqlExecutor, actor int64) error {
	var role string
	err := contributionRow(ctx, exec, `SELECT role FROM users WHERE id=$1 AND deleted_at IS NULL AND status='active' FOR SHARE`, []any{actor}, &role)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && role != service.RoleAdmin) {
		return service.ErrContributionAdmin
	}
	return err
}

const contributionSelect = `SELECT c.id,c.user_id,COALESCE(u.username,''),c.site_name,c.base_url,c.platform,
c.requested_group_id,c.new_group_name,COALESCE(g.name,''),c.status,c.review_note,c.reviewed_at,c.group_id,c.account_id,c.created_at`
const contributionJoins = ` FROM channel_contributions c LEFT JOIN users u ON u.id=c.user_id AND u.deleted_at IS NULL
LEFT JOIN groups g ON g.id=c.requested_group_id AND g.deleted_at IS NULL `

func scanContribution(rows *sql.Rows, secret bool) (*service.ChannelContribution, error) {
	c := &service.ChannelContribution{}
	dest := []any{&c.ID, &c.UserID, &c.ContributorName, &c.SiteName, &c.BaseURL, &c.Platform, &c.RequestedGroupID, &c.NewGroupName, &c.RequestedGroupName, &c.Status, &c.ReviewNote, &c.ReviewedAt, &c.GroupID, &c.AccountID, &c.CreatedAt}
	if secret {
		dest = append(dest, &c.KeyEncrypted)
	}
	err := rows.Scan(dest...)
	c.ContributorName = service.ForumDisplayName(c.ContributorName)
	return c, err
}

func (r *channelContributionRepository) Submit(ctx context.Context, user int64, c *service.ChannelContribution) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var username string
	if err := tx.QueryRowContext(ctx, `SELECT username FROM users WHERE id=$1 AND deleted_at IS NULL AND status='active' FOR UPDATE`, user).Scan(&username); err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM channel_contributions WHERE user_id=$1 AND status='pending'`, user).Scan(&pending); err != nil {
		return err
	}
	if pending >= 20 {
		return service.ErrContributionLimit
	}
	if c.RequestedGroupID != nil {
		var platform string
		err := tx.QueryRowContext(ctx, `SELECT platform FROM groups WHERE id=$1 AND deleted_at IS NULL AND status='active' FOR SHARE`, *c.RequestedGroupID).Scan(&platform)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && platform != c.Platform) {
			return service.ErrContributionGroup
		}
		if err != nil {
			return err
		}
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO channel_contributions(user_id,site_name,base_url,platform,key_encrypted,requested_group_id,new_group_name)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,created_at,status`, user, c.SiteName, c.BaseURL, c.Platform, c.KeyEncrypted, c.RequestedGroupID, c.NewGroupName).Scan(&c.ID, &c.CreatedAt, &c.Status)
	if err != nil {
		return err
	}
	c.UserID = &user
	c.ContributorName = service.ForumDisplayName(username)
	return tx.Commit()
}

func (r *channelContributionRepository) List(ctx context.Context, actor int64, admin bool, status string, before int64) ([]service.ChannelContribution, error) {
	if admin {
		if err := requireContributionAdmin(ctx, r.db, actor); err != nil {
			return nil, err
		}
	}
	rows, err := r.db.QueryContext(ctx, contributionSelect+contributionJoins+`WHERE ($1 OR c.user_id=$2) AND ($3='' OR c.status=$3) AND ($4::bigint=0 OR c.id<$4) ORDER BY c.id DESC LIMIT 51`, admin, actor, status, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.ChannelContribution{}
	for rows.Next() {
		c, err := scanContribution(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *channelContributionRepository) GetForReview(ctx context.Context, actor, id int64) (*service.ChannelContribution, error) {
	if err := requireContributionAdmin(ctx, r.db, actor); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, contributionSelect+`,c.key_encrypted`+contributionJoins+`WHERE c.id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrContributionNotFound
	}
	return scanContribution(rows, true)
}

func (r *channelContributionRepository) Review(ctx context.Context, actor int64, c *service.ChannelContribution, approve bool, note string, account *service.Account) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	client := tx.Client()
	if err := requireContributionAdmin(ctx, client, actor); err != nil {
		return err
	}
	var status string
	if err := contributionRow(ctx, client, `SELECT status FROM channel_contributions WHERE id=$1 FOR UPDATE`, []any{c.ID}, &status); err != nil {
		return translatePersistenceError(err, service.ErrContributionNotFound, nil)
	}
	if status != "pending" {
		return service.ErrContributionReviewed
	}
	var groupID, accountID *int64
	status = "rejected"
	if approve {
		if c.UserID == nil || account == nil {
			return service.ErrUserNotFound
		}
		var userID int64
		if err := contributionRow(ctx, client, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL AND status='active' FOR SHARE`, []any{*c.UserID}, &userID); err != nil {
			return translatePersistenceError(err, service.ErrUserNotFound, nil)
		}
		if c.RequestedGroupID != nil {
			if err := lockLiveGroups(ctx, client, []int64{*c.RequestedGroupID}); err != nil {
				return service.ErrContributionGroup
			}
			var platform, groupStatus string
			if err := contributionRow(ctx, client, `SELECT platform,status FROM groups WHERE id=$1 AND deleted_at IS NULL`, []any{*c.RequestedGroupID}, &platform, &groupStatus); err != nil {
				return err
			}
			if platform != c.Platform || groupStatus != service.StatusActive {
				return service.ErrContributionGroup
			}
			groupID = c.RequestedGroupID
		} else {
			if c.NewGroupName == "" {
				return service.ErrContributionGroup
			}
			// 沿用 Ent 分组默认值：公开标准分组、倍率 1，不复制其他组权限或价格。
			g, err := client.Group.Create().SetName(c.NewGroupName).SetPlatform(c.Platform).Save(ctx)
			if err != nil {
				return translatePersistenceError(err, nil, service.ErrGroupExists)
			}
			groupID = &g.ID
			if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventGroupChanged, nil, groupID, nil); err != nil {
				return err
			}
		}
		// 账号、绑定和调度 outbox 同事务提交，失败不留下可调度的半成品。
		accounts := newAccountRepositoryWithSQL(client, client, nil)
		if err := accounts.CreateWithAccountGroups(ctx, account, []service.AccountGroup{{GroupID: *groupID, Priority: 50}}); err != nil {
			return err
		}
		accountID = &account.ID
		status = "approved"
	}
	_, err = client.ExecContext(ctx, `UPDATE channel_contributions SET status=$2,review_note=$3,reviewed_by=$4,reviewed_at=NOW(),updated_at=NOW(),group_id=$5,account_id=$6,key_encrypted='' WHERE id=$1`, c.ID, status, note, actor, groupID, accountID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *channelContributionRepository) Contributors(ctx context.Context, ids []int64) (map[int64][]service.GroupContribution, error) {
	out := map[int64][]service.GroupContribution{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT ag.group_id,a.id,a.name,COALESCE(u.username,'') FROM channel_contributions c
JOIN accounts a ON a.id=c.account_id AND a.deleted_at IS NULL JOIN account_groups ag ON ag.account_id=a.id
LEFT JOIN users u ON u.id=c.user_id AND u.deleted_at IS NULL WHERE c.status='approved' AND ag.group_id=ANY($1) ORDER BY c.id`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var item service.GroupContribution
		if err := rows.Scan(&id, &item.AccountID, &item.AccountName, &item.ContributorName); err != nil {
			return nil, err
		}
		item.ContributorName = service.ForumDisplayName(item.ContributorName)
		out[id] = append(out[id], item)
	}
	return out, rows.Err()
}

package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type forumRepository struct{ db *sql.DB }

func NewForumRepository(db *sql.DB) service.ForumRepository { return &forumRepository{db: db} }

const forumMessageSelect = `SELECT m.id,COALESCE(u.id,0),COALESCE(u.username,''),COALESCE(a.url,''),COALESCE(t.actual_cost,0),
    m.content,m.rank_at_send,m.pinned,m.created_at FROM forum_messages m
    LEFT JOIN users u ON u.id=m.user_id AND u.deleted_at IS NULL
    LEFT JOIN user_avatars a ON a.user_id=u.id
    LEFT JOIN user_usage_lifetime_totals t ON t.user_id=u.id `

func scanForumMessage(row interface{ Scan(...any) error }) (service.ForumMessage, error) {
	var m service.ForumMessage
	err := row.Scan(&m.ID, &m.Author.UserID, &m.Author.Username, &m.Author.AvatarURL, &m.Author.ActualCost, &m.Content, &m.Rank, &m.Pinned, &m.CreatedAt)
	m.Author.Username = service.ForumDisplayName(m.Author.Username)
	return m, err
}
func forumMessages(ctx context.Context, tx *sql.Tx, suffix string, args ...any) ([]service.ForumMessage, error) {
	rows, err := tx.QueryContext(ctx, forumMessageSelect+suffix, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.ForumMessage{}
	for rows.Next() {
		m, err := scanForumMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (r *forumRepository) Page(ctx context.Context, user, before int64) (*service.ForumPage, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	p := &service.ForumPage{}
	err = tx.QueryRowContext(ctx, `SELECT muted_until,retention_days FROM forum_settings WHERE id=1`).Scan(&p.Settings.MutedUntil, &p.Settings.RetentionDays)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(ctx, `SELECT muted_until FROM forum_members WHERE user_id=$1`, user).Scan(&p.MutedUntil)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	visible := ` AND ($2=0 OR m.created_at > NOW()-make_interval(days => $2)) `
	p.Messages, err = forumMessages(ctx, tx, `WHERE ($1::bigint=0 OR m.id<$1)`+visible+` ORDER BY m.id DESC LIMIT 51`, before, p.Settings.RetentionDays)
	if err != nil {
		return nil, err
	}
	p.HasMore = len(p.Messages) > 50
	if p.HasMore {
		p.Messages = p.Messages[:50]
	}
	p.Pinned, err = forumMessages(ctx, tx, `WHERE m.pinned AND ($1=0 OR m.created_at > NOW()-make_interval(days => $1)) ORDER BY m.id DESC LIMIT 3`, p.Settings.RetentionDays)
	if err != nil {
		return nil, err
	}
	return p, tx.Commit()
}

// 所有发言和管理操作先锁同一设置行：禁言、清空与事件游标具有明确提交顺序。
func (r *forumRepository) mutation(ctx context.Context, actor int64, admin bool, fn func(*sql.Tx, time.Time, bool, *time.Time) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var until *time.Time
	if err = tx.QueryRowContext(ctx, `SELECT muted_until FROM forum_settings WHERE id=1 FOR UPDATE`).Scan(&until); err != nil {
		return err
	}
	var role, status string
	if err = tx.QueryRowContext(ctx, `SELECT role,status FROM users WHERE id=$1 AND deleted_at IS NULL FOR SHARE`, actor).Scan(&role, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrForumAdmin
		}
		return err
	}
	if status != "active" || admin && role != "admin" {
		return service.ErrForumAdmin
	}
	var now time.Time
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if err = fn(tx, now, role == "admin", until); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *forumRepository) Create(ctx context.Context, user int64, content string) (*service.ForumMessage, error) {
	var m service.ForumMessage
	err := r.mutation(ctx, user, false, func(tx *sql.Tx, now time.Time, admin bool, global *time.Time) error {
		var muted, last *time.Time
		err := tx.QueryRowContext(ctx, `SELECT muted_until,last_post_at FROM forum_members WHERE user_id=$1`, user).Scan(&muted, &last)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if !admin && ((global != nil && global.After(now)) || (muted != nil && muted.After(now))) {
			return service.ErrForumMuted
		}
		if last != nil && now.Sub(*last) < 3*time.Second {
			return service.ErrForumTooFast
		}
		var rank int
		err = tx.QueryRowContext(ctx, `WITH top AS (SELECT t.user_id,ROW_NUMBER() OVER (ORDER BY t.actual_cost DESC,t.tokens DESC,t.user_id ASC) AS rank
            FROM user_usage_lifetime_totals t JOIN users u ON u.id=t.user_id AND u.deleted_at IS NULL
            WHERE t.actual_cost>0 OR t.tokens>0 ORDER BY t.actual_cost DESC,t.tokens DESC,t.user_id ASC LIMIT 20)
            SELECT COALESCE((SELECT rank FROM top WHERE user_id=$1),0)`, user).Scan(&rank)
		if err != nil {
			return err
		}
		var id int64
		if err = tx.QueryRowContext(ctx, `INSERT INTO forum_messages(user_id,content,rank_at_send,created_at) VALUES($1,$2,$3,$4) RETURNING id`, user, content, rank, now).Scan(&id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO forum_members(user_id,last_post_at) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET last_post_at=$2`, user, now); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE forum_settings SET last_message_id=$1 WHERE id=1`, id); err != nil {
			return err
		}
		m, err = scanForumMessage(tx.QueryRowContext(ctx, forumMessageSelect+`WHERE m.id=$1`, id))
		return err
	})
	return &m, err
}
func (r *forumRepository) Events(ctx context.Context, after *int64) (*service.ForumEvents, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result := &service.ForumEvents{Messages: []service.ForumMessage{}}
	if err = tx.QueryRowContext(ctx, `SELECT last_message_id FROM forum_settings WHERE id=1`).Scan(&result.Cursor); err != nil {
		return nil, err
	}
	// 首次连接只建立游标，不把历史发言当成全站通报。
	if after != nil {
		result.Messages, err = forumMessages(ctx, tx, `WHERE m.id>$1 AND m.id<=$2 AND m.rank_at_send>0 AND u.id IS NOT NULL
            AND m.created_at>NOW()-INTERVAL '30 seconds' ORDER BY m.id LIMIT 20`, *after, result.Cursor)
		if err != nil {
			return nil, err
		}
		if len(result.Messages) == 20 {
			result.Cursor = result.Messages[19].ID
		}
	}
	return result, tx.Commit()
}
func (r *forumRepository) UpdateSettings(ctx context.Context, actor int64, minutes, days *int) error {
	return r.mutation(ctx, actor, true, func(tx *sql.Tx, now time.Time, _ bool, _ *time.Time) error {
		if minutes != nil {
			var until *time.Time
			if *minutes > 0 {
				v := now.Add(time.Duration(*minutes) * time.Minute)
				until = &v
			}
			if _, err := tx.ExecContext(ctx, `UPDATE forum_settings SET muted_until=$1 WHERE id=1`, until); err != nil {
				return err
			}
		}
		if days != nil {
			_, err := tx.ExecContext(ctx, `UPDATE forum_settings SET retention_days=$1 WHERE id=1`, *days)
			return err
		}
		return nil
	})
}
func (r *forumRepository) Mute(ctx context.Context, actor, user int64, minutes int) error {
	return r.mutation(ctx, actor, true, func(tx *sql.Tx, now time.Time, _ bool, _ *time.Time) error {
		var role string
		if err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id=$1 AND deleted_at IS NULL`, user).Scan(&role); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return service.ErrForumNotFound
			}
			return err
		}
		if role == "admin" {
			return infraerrors.BadRequest("FORUM_ADMIN_MUTE", "不能禁言管理员")
		}
		var until *time.Time
		if minutes > 0 {
			v := now.Add(time.Duration(minutes) * time.Minute)
			until = &v
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO forum_members(user_id,muted_until) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET muted_until=$2`, user, until)
		return err
	})
}
func (r *forumRepository) Mutes(ctx context.Context, actor int64) ([]service.ForumMute, error) {
	out := []service.ForumMute{}
	err := r.mutation(ctx, actor, true, func(tx *sql.Tx, _ time.Time, _ bool, _ *time.Time) error {
		rows, err := tx.QueryContext(ctx, `SELECT u.id,COALESCE(u.username,''),COALESCE(a.url,''),COALESCE(t.actual_cost,0),f.muted_until
            FROM forum_members f JOIN users u ON u.id=f.user_id AND u.deleted_at IS NULL
            LEFT JOIN user_avatars a ON a.user_id=u.id LEFT JOIN user_usage_lifetime_totals t ON t.user_id=u.id
            WHERE f.muted_until>NOW() ORDER BY f.muted_until DESC LIMIT 500`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item service.ForumMute
			if err = rows.Scan(&item.Author.UserID, &item.Author.Username, &item.Author.AvatarURL, &item.Author.ActualCost, &item.MutedUntil); err != nil {
				return err
			}
			item.Author.Username = service.ForumDisplayName(item.Author.Username)
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}
func (r *forumRepository) Pin(ctx context.Context, actor, id int64, pinned bool) error {
	return r.mutation(ctx, actor, true, func(tx *sql.Tx, _ time.Time, _ bool, _ *time.Time) error {
		if pinned {
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM forum_messages WHERE pinned AND id<>$1`, id).Scan(&count); err != nil {
				return err
			}
			if count >= 3 {
				return infraerrors.BadRequest("FORUM_PIN_LIMIT", "最多置顶 3 条内容")
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE forum_messages SET pinned=$2 WHERE id=$1`, id, pinned)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return service.ErrForumNotFound
		}
		return nil
	})
}
func (r *forumRepository) Clear(ctx context.Context, actor int64) error {
	return r.mutation(ctx, actor, true, func(tx *sql.Tx, _ time.Time, _ bool, _ *time.Time) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM forum_messages`)
		return err
	})
}
func (r *forumRepository) Cleanup(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var days int
	// 小批量清理；多实例使用同一把行锁，不重复处理或长时间阻塞发言。
	err = tx.QueryRowContext(ctx, `SELECT retention_days FROM forum_settings WHERE id=1 FOR UPDATE SKIP LOCKED`).Scan(&days)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if days > 0 {
		_, err = tx.ExecContext(ctx, `DELETE FROM forum_messages WHERE id IN (SELECT id FROM forum_messages WHERE created_at<=NOW()-make_interval(days => $1) ORDER BY created_at LIMIT 5000)`, days)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

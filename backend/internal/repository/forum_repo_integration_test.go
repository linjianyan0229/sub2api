//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestForumPermissionsLifecycleAndEvents(t *testing.T) {
	ctx := context.Background()
	repo := &forumRepository{db: integrationDB}
	users := newUserRepositoryWithSQL(integrationEntClient, integrationDB)
	ids := []int64{}
	for i := 0; i < 23; i++ {
		role := "user"
		if i == 0 {
			role = "admin"
		}
		u := &service.User{Email: uuid.NewString() + "@forum.test", Username: fmt.Sprintf("论坛用户-%d", i), PasswordHash: "hash", Role: role, Status: "active", Concurrency: 5}
		require.NoError(t, users.Create(ctx, u))
		ids = append(ids, u.ID)
	}
	t.Cleanup(func() {
		for _, id := range ids {
			_, err := integrationDB.ExecContext(ctx, `DELETE FROM forum_messages WHERE user_id=$1`, id)
			require.NoError(t, err)
			_, err = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, id)
			require.NoError(t, err)
		}
		_, err := integrationDB.ExecContext(ctx, `UPDATE forum_settings SET muted_until=NULL,retention_days=30 WHERE id=1`)
		require.NoError(t, err)
	})
	admin, user := ids[0], ids[1]
	_, err := integrationDB.ExecContext(ctx, `UPDATE users SET username='private@example.com' WHERE id=$1`, user)
	require.NoError(t, err)
	for i, id := range ids[1:22] {
		_, err = integrationDB.ExecContext(ctx, `INSERT INTO user_usage_lifetime_totals(user_id,actual_cost,tokens) VALUES($1,$2,$3)`, id, 1000000-i, 100+i)
		require.NoError(t, err)
	}
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO user_avatars(user_id,url) VALUES($1,'https://example.com/avatar.png')`, user)
	require.NoError(t, err)
	initial, err := repo.Events(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, initial.Messages)
	m, err := repo.Create(ctx, user, "<script>alert(1)</script>")
	require.NoError(t, err)
	require.Equal(t, 1, m.Rank)
	require.Empty(t, m.Author.Username)
	require.Equal(t, "https://example.com/avatar.png", m.Author.AvatarURL)
	require.Equal(t, 1000000.0, m.Author.ActualCost)
	_, err = repo.Create(ctx, user, "重复发言")
	require.ErrorIs(t, err, service.ErrForumTooFast)
	events, err := repo.Events(ctx, &initial.Cursor)
	require.NoError(t, err)
	require.Len(t, events.Messages, 1)
	require.Equal(t, m.ID, events.Messages[0].ID)
	again, err := repo.Events(ctx, &events.Cursor)
	require.NoError(t, err)
	require.Empty(t, again.Messages)
	fresh, err := repo.Events(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, fresh.Messages)
	_, err = repo.Create(ctx, ids[21], "榜外用户")
	require.NoError(t, err)
	other, err := repo.Events(ctx, &events.Cursor)
	require.NoError(t, err)
	require.Empty(t, other.Messages)

	minutes, days := 10, 7
	require.ErrorIs(t, repo.UpdateSettings(ctx, user, &minutes, nil), service.ErrForumAdmin)
	require.ErrorIs(t, repo.Mute(ctx, user, ids[2], 10), service.ErrForumAdmin)
	require.ErrorIs(t, repo.Pin(ctx, user, m.ID, true), service.ErrForumAdmin)
	require.ErrorIs(t, repo.Clear(ctx, user), service.ErrForumAdmin)
	_, err = repo.Mutes(ctx, user)
	require.ErrorIs(t, err, service.ErrForumAdmin)
	require.NoError(t, repo.UpdateSettings(ctx, admin, &minutes, &days))
	_, err = repo.Create(ctx, ids[2], "全站禁言")
	require.ErrorIs(t, err, service.ErrForumMuted)
	_, err = repo.Create(ctx, admin, "管理员公告")
	require.NoError(t, err)
	zero := 0
	require.NoError(t, repo.UpdateSettings(ctx, admin, &zero, nil))
	require.NoError(t, repo.Mute(ctx, admin, ids[2], 10))
	_, err = repo.Create(ctx, ids[2], "个人禁言")
	require.ErrorIs(t, err, service.ErrForumMuted)
	muted, err := repo.Mutes(ctx, admin)
	require.NoError(t, err)
	require.Len(t, muted, 1)
	require.Error(t, repo.Mute(ctx, admin, admin, 10))
	require.NoError(t, repo.Mute(ctx, admin, ids[2], 0))
	_, err = repo.Create(ctx, ids[2], "解除禁言")
	require.NoError(t, err)
	require.NoError(t, repo.Mute(ctx, admin, ids[3], 10))
	_, err = integrationDB.ExecContext(ctx, `UPDATE forum_members SET muted_until=NOW()-INTERVAL '1 second' WHERE user_id=$1`, ids[3])
	require.NoError(t, err)
	_, err = repo.Create(ctx, ids[3], "禁言到期")
	require.NoError(t, err)
	require.NoError(t, repo.Pin(ctx, admin, m.ID, true))
	page, err := repo.Page(ctx, user, 0)
	require.NoError(t, err)
	require.Len(t, page.Pinned, 1)
	require.Equal(t, 7, page.Settings.RetentionDays)
	older, err := repo.Page(ctx, user, m.ID)
	require.NoError(t, err)
	require.Empty(t, older.Messages)

	// 保留期包含置顶；清空不会重置事件游标、用量或禁言记录。
	_, err = integrationDB.ExecContext(ctx, `UPDATE forum_messages SET created_at=NOW()-INTERVAL '8 days' WHERE id=$1`, m.ID)
	require.NoError(t, err)
	require.NoError(t, repo.Cleanup(ctx))
	page, err = repo.Page(ctx, user, 0)
	require.NoError(t, err)
	require.Empty(t, page.Pinned)
	require.NoError(t, repo.Mute(ctx, admin, ids[4], 10))
	beforeClear, err := repo.Events(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, repo.Clear(ctx, admin))
	page, err = repo.Page(ctx, user, 0)
	require.NoError(t, err)
	require.Empty(t, page.Messages)
	afterClear, err := repo.Events(ctx, &beforeClear.Cursor)
	require.NoError(t, err)
	require.Empty(t, afterClear.Messages)
	require.Equal(t, beforeClear.Cursor, afterClear.Cursor)
	_, err = repo.Create(ctx, ids[4], "清空不解除禁言")
	require.ErrorIs(t, err, service.ErrForumMuted)
	var total float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT actual_cost FROM user_usage_lifetime_totals WHERE user_id=$1`, user).Scan(&total))
	require.Equal(t, 1000000.0, total)
	next, err := repo.Create(ctx, ids[5], "清空后新发言")
	require.NoError(t, err)
	require.Greater(t, next.ID, beforeClear.Cursor)
	after, err := repo.Events(ctx, &beforeClear.Cursor)
	require.NoError(t, err)
	require.Len(t, after.Messages, 1)

	// 同一用户同时提交只有一次成功；置顶总数在并发操作下也不得突破上限。
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := repo.Create(ctx, ids[22], "并发发言"); results <- err }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, service.ErrForumTooFast)
		}
	}
	require.Equal(t, 1, success)
	messageIDs := []int64{next.ID}
	for _, id := range ids[6:10] {
		msg, err := repo.Create(ctx, id, "置顶测试")
		require.NoError(t, err)
		messageIDs = append(messageIDs, msg.ID)
	}
	pinResults := make(chan error, len(messageIDs))
	for _, id := range messageIDs {
		wg.Add(1)
		go func(id int64) { defer wg.Done(); pinResults <- repo.Pin(ctx, admin, id, true) }(id)
	}
	wg.Wait()
	close(pinResults)
	success = 0
	for err := range pinResults {
		if err == nil {
			success++
		} else {
			require.Contains(t, err.Error(), "FORUM_PIN_LIMIT")
		}
	}
	require.Equal(t, 3, success)
	require.NoError(t, repo.UpdateSettings(ctx, admin, nil, &zero))
	_, err = integrationDB.ExecContext(ctx, `UPDATE forum_messages SET created_at=NOW()-INTERVAL '400 days' WHERE id=$1`, next.ID)
	require.NoError(t, err)
	require.NoError(t, repo.Cleanup(ctx))
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM forum_messages WHERE id=$1`, next.ID).Scan(&count))
	require.Equal(t, 1, count)
}

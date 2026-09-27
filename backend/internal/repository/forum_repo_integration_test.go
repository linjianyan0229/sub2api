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
	require.Empty(t, initial.Arrivals)
	m, err := repo.Create(ctx, user, "<script>alert(1)</script>")
	require.NoError(t, err)
	require.Equal(t, 1, m.Rank)
	require.False(t, m.Author.IsAdmin)
	require.Empty(t, m.Author.Username)
	require.Equal(t, "https://example.com/avatar.png", m.Author.AvatarURL)
	require.Equal(t, 1000000.0, m.Author.ActualCost)
	_, err = repo.Create(ctx, user, "重复发言")
	require.ErrorIs(t, err, service.ErrForumTooFast)
	events, err := repo.Events(ctx, &initial.Cursor)
	require.NoError(t, err)
	require.Empty(t, events.Arrivals)
	require.Equal(t, initial.Cursor, events.Cursor)
	again, err := repo.Events(ctx, &events.Cursor)
	require.NoError(t, err)
	require.Empty(t, again.Arrivals)
	fresh, err := repo.Events(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, fresh.Arrivals)
	_, err = repo.Create(ctx, ids[21], "榜外用户")
	require.NoError(t, err)
	other, err := repo.Events(ctx, &events.Cursor)
	require.NoError(t, err)
	require.Empty(t, other.Arrivals)

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
	adminMessage, err := repo.Create(ctx, admin, "管理员公告")
	require.NoError(t, err)
	require.True(t, adminMessage.Author.IsAdmin)
	require.NoError(t, repo.Pin(ctx, admin, adminMessage.ID, true))
	page, err := repo.Page(ctx, user, 0)
	require.NoError(t, err)
	require.Len(t, page.Pinned, 1)
	require.True(t, page.Pinned[0].Author.IsAdmin)
	require.NoError(t, repo.Pin(ctx, admin, adminMessage.ID, false))
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
	page, err = repo.Page(ctx, user, 0)
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
	require.Empty(t, afterClear.Arrivals)
	require.Equal(t, beforeClear.Cursor, afterClear.Cursor)
	_, err = repo.Create(ctx, ids[4], "清空不解除禁言")
	require.ErrorIs(t, err, service.ErrForumMuted)
	var total float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT actual_cost FROM user_usage_lifetime_totals WHERE user_id=$1`, user).Scan(&total))
	require.Equal(t, 1000000.0, total)
	next, err := repo.Create(ctx, ids[5], "清空后新发言")
	require.NoError(t, err)
	require.Greater(t, next.ID, m.ID)
	after, err := repo.Events(ctx, &beforeClear.Cursor)
	require.NoError(t, err)
	require.Empty(t, after.Arrivals)

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

func TestForumArrivalsDeduplicateSessions(t *testing.T) {
	ctx := context.Background()
	repo := &forumRepository{db: integrationDB}
	users := newUserRepositoryWithSQL(integrationEntClient, integrationDB)
	user := &service.User{Email: uuid.NewString() + "@forum.test", Username: "private@example.com", PasswordHash: "hash", Role: "user", Status: "active", Concurrency: 5}
	require.NoError(t, users.Create(ctx, user))
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, user.ID)
		require.NoError(t, err)
	})
	initial, err := repo.Events(ctx, nil)
	require.NoError(t, err)
	// 榜外用户上线不广播，在线期间进入榜单也不追补通报。
	require.NoError(t, repo.Arrive(ctx, user.ID, "unranked"))
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO user_usage_lifetime_totals(user_id,actual_cost,tokens) VALUES($1,10000000,100)`, user.ID)
	require.NoError(t, err)
	require.NoError(t, repo.Arrive(ctx, user.ID, "unranked"))
	events, err := repo.Events(ctx, &initial.Cursor)
	require.NoError(t, err)
	require.Empty(t, events.Arrivals)
	// 多标签、刷新、网络重试以及不同服务实例同时登记同一会话，只生成一条事件。
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other := &forumRepository{db: integrationDB}
			results <- other.Arrive(ctx, user.ID, "session-one")
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	events, err = repo.Events(ctx, &events.Cursor)
	require.NoError(t, err)
	require.Len(t, events.Arrivals, 1)
	event := events.Arrivals[0]
	require.Equal(t, user.ID, event.Author.UserID)
	require.Equal(t, 1, event.Rank)
	require.Empty(t, event.Author.Username)
	require.Equal(t, 10000000.0, event.Author.ActualCost)
	_, err = repo.Create(ctx, user.ID, "发言不再触发")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, `DELETE FROM forum_messages WHERE user_id=$1`, user.ID)
		require.NoError(t, err)
	})
	require.NoError(t, repo.Arrive(ctx, user.ID, "session-one"))
	again, err := repo.Events(ctx, &events.Cursor)
	require.NoError(t, err)
	require.Empty(t, again.Arrivals)
	require.Equal(t, events.Cursor, again.Cursor)
	fresh, err := repo.Events(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, fresh.Arrivals)
	// 通报过期仍保留去重记录，新登录会话才产生新的通报。
	_, err = integrationDB.ExecContext(ctx, `UPDATE forum_arrivals SET created_at=NOW()-INTERVAL '60 seconds' WHERE id=$1`, event.ID)
	require.NoError(t, err)
	require.NoError(t, repo.Arrive(ctx, user.ID, "session-one"))
	expired, err := repo.Events(ctx, &initial.Cursor)
	require.NoError(t, err)
	require.Empty(t, expired.Arrivals)
	require.NoError(t, repo.Arrive(ctx, user.ID, "session-two"))
	next, err := repo.Events(ctx, &events.Cursor)
	require.NoError(t, err)
	require.Len(t, next.Arrivals, 1)
	require.Greater(t, next.Arrivals[0].ID, event.ID)
	// 连续事件分页不漏读；客户端只看到公开资料，永远不返回会话 ID。
	for i := 0; i < 21; i++ {
		require.NoError(t, repo.Arrive(ctx, user.ID, fmt.Sprintf("session-%d", i)))
	}
	page, err := repo.Events(ctx, &next.Cursor)
	require.NoError(t, err)
	require.Len(t, page.Arrivals, 20)
	require.Equal(t, page.Arrivals[19].ID, page.Cursor)
	last, err := repo.Events(ctx, &page.Cursor)
	require.NoError(t, err)
	require.Len(t, last.Arrivals, 1)
	require.Greater(t, last.Arrivals[0].ID, page.Cursor)
}

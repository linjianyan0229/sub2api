//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
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
			_, err := integrationDB.ExecContext(ctx, `DELETE FROM forum_pinned_messages WHERE user_id=$1`, id)
			require.NoError(t, err)
			_, err = integrationDB.ExecContext(ctx, `DELETE FROM forum_messages WHERE user_id=$1`, id)
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
	require.True(t, page.Messages[0].Pinned)
	globalPins, err := repo.Events(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, page.Pinned, globalPins.Pinned)
	require.NoError(t, repo.Pin(ctx, admin, adminMessage.ID, false))
	page, err = repo.Page(ctx, user, 0)
	require.NoError(t, err)
	require.False(t, page.Messages[0].Pinned)
	globalPins, err = repo.Events(ctx, &globalPins.Cursor)
	require.NoError(t, err)
	require.Empty(t, globalPins.Pinned)
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
	require.NoError(t, integrationDB.QueryRowContext(ctx, `UPDATE forum_messages
        SET created_at=NOW()-INTERVAL '8 days' WHERE id=$1 RETURNING created_at`, m.ID).Scan(&m.CreatedAt))
	require.NoError(t, repo.Pin(ctx, admin, m.ID, true))
	page, err = repo.Page(ctx, user, 0)
	require.NoError(t, err)
	require.Len(t, page.Pinned, 1)
	require.Equal(t, 7, page.Settings.RetentionDays)
	older, err := repo.Page(ctx, user, m.ID)
	require.NoError(t, err)
	require.Empty(t, older.Messages)

	// History expires normally, but the pinned copy survives both hiding and deletion.
	globalPins, err = repo.Events(ctx, nil)
	require.NoError(t, err)
	require.Len(t, globalPins.Pinned, 1)
	require.Equal(t, m.ID, globalPins.Pinned[0].ID)
	page, err = repo.Page(ctx, user, 0)
	require.NoError(t, err)
	for _, message := range page.Messages {
		require.NotEqual(t, m.ID, message.ID)
	}
	require.Equal(t, globalPins.Pinned, page.Pinned)
	require.NoError(t, repo.Cleanup(ctx))
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM forum_messages WHERE id=$1`, m.ID).Scan(&count))
	require.Zero(t, count)
	page, err = repo.Page(ctx, user, 0)
	require.NoError(t, err)
	require.Equal(t, globalPins.Pinned, page.Pinned)
	require.Equal(t, m.Content, page.Pinned[0].Content)
	require.Equal(t, m.Rank, page.Pinned[0].Rank)
	require.Equal(t, m.CreatedAt, page.Pinned[0].CreatedAt)
	require.Equal(t, m.Author, page.Pinned[0].Author)
	require.NoError(t, repo.Pin(ctx, admin, m.ID, true))
	// Also clear a pin whose original message has not expired.
	require.NoError(t, repo.Pin(ctx, admin, adminMessage.ID, true))
	require.NoError(t, repo.Mute(ctx, admin, ids[4], 10))
	beforeClear, err := repo.Events(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, repo.Clear(ctx, admin))
	page, err = repo.Page(ctx, user, 0)
	require.NoError(t, err)
	require.Empty(t, page.Messages)
	require.Len(t, page.Pinned, 2)
	afterClear, err := repo.Events(ctx, &beforeClear.Cursor)
	require.NoError(t, err)
	require.Empty(t, afterClear.Arrivals)
	require.Equal(t, page.Pinned, afterClear.Pinned)
	require.Equal(t, beforeClear.Cursor, afterClear.Cursor)
	for _, pin := range afterClear.Pinned {
		require.NoError(t, repo.Pin(ctx, admin, pin.ID, false))
	}
	unpinned, err := repo.Page(ctx, user, 0)
	require.NoError(t, err)
	require.Empty(t, unpinned.Pinned)
	require.Empty(t, unpinned.Messages)
	require.ErrorIs(t, repo.Pin(ctx, admin, m.ID, true), service.ErrForumNotFound)
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
	// Cleared pins still occupy the limit, and unpinning releases a slot.
	pinnedPage, err := repo.Page(ctx, user, 0)
	require.NoError(t, err)
	require.Len(t, pinnedPage.Pinned, 3)
	require.NoError(t, repo.Clear(ctx, admin))
	replacement, err := repo.Create(ctx, ids[10], "Replacement pin")
	require.NoError(t, err)
	require.ErrorContains(t, repo.Pin(ctx, admin, replacement.ID, true), "FORUM_PIN_LIMIT")
	require.NoError(t, repo.Pin(ctx, admin, pinnedPage.Pinned[0].ID, true))
	require.NoError(t, repo.Pin(ctx, admin, pinnedPage.Pinned[0].ID, false))
	require.NoError(t, repo.Pin(ctx, admin, replacement.ID, true))
	require.NoError(t, repo.UpdateSettings(ctx, admin, nil, &zero))
	_, err = integrationDB.ExecContext(ctx, `UPDATE forum_messages SET created_at=NOW()-INTERVAL '400 days' WHERE id=$1`, replacement.ID)
	require.NoError(t, err)
	require.NoError(t, repo.Cleanup(ctx))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM forum_messages WHERE id=$1`, replacement.ID).Scan(&count))
	require.Equal(t, 1, count)
}

func TestForumPinnedMessagesMigration(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	_, err := tx.ExecContext(ctx, `ALTER TABLE forum_messages ADD COLUMN pinned BOOLEAN NOT NULL DEFAULT FALSE`)
	require.NoError(t, err)
	var pinnedID, plainID int64
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO forum_messages(content,rank_at_send,created_at,pinned)
        VALUES('Legacy pin',2,NOW()-INTERVAL '400 days',TRUE) RETURNING id`).Scan(&pinnedID))
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO forum_messages(content) VALUES('Not pinned') RETURNING id`).Scan(&plainID))
	migration, err := dbmigrations.FS.ReadFile("250_forum_pinned_messages.sql")
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err = tx.ExecContext(ctx, string(migration))
		require.NoError(t, err)
		var matches bool
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT p.content=m.content AND p.rank_at_send=m.rank_at_send
            AND p.created_at=m.created_at AND p.user_id IS NOT DISTINCT FROM m.user_id
            FROM forum_pinned_messages p JOIN forum_messages m ON m.id=p.id WHERE p.id=$1`, pinnedID).Scan(&matches))
		require.True(t, matches)
		var count int
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM forum_pinned_messages WHERE id=$1`, plainID).Scan(&count))
		require.Zero(t, count)
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM forum_messages WHERE id=$1`, pinnedID)
	require.NoError(t, err)
	var content string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT content FROM forum_pinned_messages WHERE id=$1`, pinnedID).Scan(&content))
	require.Equal(t, "Legacy pin", content)
	_, err = tx.ExecContext(ctx, `DELETE FROM forum_pinned_messages WHERE id=$1`, pinnedID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	var count int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM forum_pinned_messages WHERE id=$1`, pinnedID).Scan(&count))
	require.Zero(t, count)
}

func TestForumRecallLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := &forumRepository{db: integrationDB}
	users := newUserRepositoryWithSQL(integrationEntClient, integrationDB)
	ids := []int64{}
	for _, role := range []string{"admin", "user", "user"} {
		u := &service.User{Email: uuid.NewString() + "@forum.test", Username: uuid.NewString(), PasswordHash: "hash", Role: role, Status: "active", Concurrency: 5}
		require.NoError(t, users.Create(ctx, u))
		ids = append(ids, u.ID)
	}
	t.Cleanup(func() {
		for _, id := range ids {
			_, err := integrationDB.ExecContext(ctx, `DELETE FROM forum_pinned_messages WHERE user_id=$1`, id)
			require.NoError(t, err)
			_, err = integrationDB.ExecContext(ctx, `DELETE FROM forum_messages WHERE user_id=$1`, id)
			require.NoError(t, err)
			_, err = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, id)
			require.NoError(t, err)
		}
		_, err := integrationDB.ExecContext(ctx, `UPDATE forum_settings SET muted_until=NULL WHERE id=1`)
		require.NoError(t, err)
	})
	admin, user, other := ids[0], ids[1], ids[2]
	message, err := repo.Create(ctx, user, "Recall me")
	require.NoError(t, err)
	require.NoError(t, repo.Pin(ctx, admin, message.ID, true))
	require.ErrorIs(t, repo.Recall(ctx, other, message.ID), service.ErrForumRecallForbidden)
	minutes := 10
	require.NoError(t, repo.Mute(ctx, admin, user, minutes))
	require.NoError(t, repo.UpdateSettings(ctx, admin, &minutes, nil))
	require.NoError(t, repo.Recall(ctx, user, message.ID))
	assertGone := func(id int64) {
		t.Helper()
		var count int
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT
            (SELECT COUNT(*) FROM forum_messages WHERE id=$1) +
            (SELECT COUNT(*) FROM forum_pinned_messages WHERE id=$1)`, id).Scan(&count))
		require.Zero(t, count)
		page, err := repo.Page(ctx, user, 0)
		require.NoError(t, err)
		for _, msg := range append(page.Messages, page.Pinned...) {
			require.NotEqual(t, id, msg.ID)
		}
		events, err := repo.Events(ctx, nil)
		require.NoError(t, err)
		for _, pin := range events.Pinned {
			require.NotEqual(t, id, pin.ID)
		}
	}
	assertGone(message.ID)
	require.ErrorIs(t, repo.Recall(ctx, user, message.ID), service.ErrForumNotFound)
	var lastMessage int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT last_message_id FROM forum_settings WHERE id=1`).Scan(&lastMessage))
	require.Equal(t, message.ID, lastMessage)
	zero := 0
	require.NoError(t, repo.Mute(ctx, admin, user, zero))
	require.NoError(t, repo.UpdateSettings(ctx, admin, &zero, nil))
	var lastPost time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT last_post_at FROM forum_members WHERE user_id=$1`, user).Scan(&lastPost))
	require.True(t, lastPost.Equal(message.CreatedAt), "recall must not reset rate limiting")

	insert := func(author int64, ageSeconds int) int64 {
		t.Helper()
		var id int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO forum_messages(user_id,content,created_at)
            VALUES($1,'Recall test',clock_timestamp()-make_interval(secs => $2)) RETURNING id`, author, ageSeconds).Scan(&id))
		return id
	}
	expired := insert(user, 60)
	require.ErrorIs(t, repo.Recall(ctx, user, expired), service.ErrForumRecallExpired)
	require.NoError(t, repo.Recall(ctx, admin, expired))
	assertGone(expired)
	adminMessage := insert(admin, 400*24*60*60)
	require.NoError(t, repo.Recall(ctx, admin, adminMessage))
	assertGone(adminMessage)
	archived := insert(user, 400*24*60*60)
	require.NoError(t, repo.Pin(ctx, admin, archived, true))
	require.NoError(t, repo.Cleanup(ctx))
	require.ErrorIs(t, repo.Recall(ctx, user, archived), service.ErrForumRecallExpired)
	require.NoError(t, repo.Recall(ctx, admin, archived))
	assertGone(archived)

	// Neither pinning nor a second recall can resurrect a concurrently recalled message.
	concurrent := insert(user, 0)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); results <- repo.Pin(ctx, admin, concurrent, true) }()
	go func() { defer wg.Done(); results <- repo.Recall(ctx, user, concurrent) }()
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			require.ErrorIs(t, err, service.ErrForumNotFound)
		}
	}
	assertGone(concurrent)
	require.ErrorIs(t, repo.Pin(ctx, admin, concurrent, true), service.ErrForumNotFound)
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

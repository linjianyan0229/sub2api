//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsageLeaderboardLifetimeMigration(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	schema := "leaderboard_" + uuid.New().String()[:8]
	_, err := tx.ExecContext(ctx, `CREATE SCHEMA `+schema+`; SET LOCAL search_path TO `+schema+`;
		CREATE TABLE users (id BIGINT PRIMARY KEY, username TEXT, deleted_at TIMESTAMPTZ);
		CREATE TABLE usage_logs (
			id BIGSERIAL PRIMARY KEY, request_id TEXT UNIQUE, user_id BIGINT NOT NULL,
			actual_cost NUMERIC(20,10) NOT NULL, input_tokens INT NOT NULL DEFAULT 0,
			output_tokens INT NOT NULL DEFAULT 0, cache_creation_tokens INT NOT NULL DEFAULT 0,
			cache_read_tokens INT NOT NULL DEFAULT 0
		);
		INSERT INTO users (id,username) SELECT n, 'nickname-' || n FROM generate_series(1,25) n;
		INSERT INTO usage_logs (request_id,user_id,actual_cost,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens)
		SELECT 'history-' || n, n, n, 10, 20, 30, 40 FROM generate_series(1,25) n;`)
	require.NoError(t, err)
	migration, err := dbmigrations.FS.ReadFile("242_user_usage_lifetime_totals.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	repo := newUsageLogRepositoryWithSQL(nil, tx)
	ranking, err := repo.GetUsageLeaderboard(ctx)
	require.NoError(t, err)
	require.Len(t, ranking, 20)
	require.Equal(t, "nickname-25", ranking[0].Username)
	require.Equal(t, 25.0, ranking[0].ActualCost)
	require.Equal(t, int64(100), ranking[0].Tokens)
	require.Equal(t, "nickname-6", ranking[19].Username)

	// 批量写入与重复请求只累计实际插入的记录。
	_, err = tx.ExecContext(ctx, `INSERT INTO usage_logs (request_id,user_id,actual_cost,input_tokens) VALUES
		('new-1',1,100,300),('new-2',1,10,40) ON CONFLICT (request_id) DO NOTHING;
		INSERT INTO usage_logs (request_id,user_id,actual_cost,input_tokens) VALUES
		('new-1',1,100,300) ON CONFLICT (request_id) DO NOTHING;`)
	require.NoError(t, err)
	assertTotals := func(cost float64, tokens int64) {
		t.Helper()
		var gotCost float64
		var gotTokens int64
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT actual_cost,tokens FROM user_usage_lifetime_totals WHERE user_id=1`).Scan(&gotCost, &gotTokens))
		require.Equal(t, cost, gotCost)
		require.Equal(t, tokens, gotTokens)
	}
	assertTotals(111, 440)
	_, err = tx.ExecContext(ctx, `UPDATE usage_logs SET actual_cost=105,input_tokens=350 WHERE request_id='new-1'`)
	require.NoError(t, err)
	assertTotals(116, 490)

	_, err = tx.ExecContext(ctx, `SAVEPOINT before_failed_usage;
		INSERT INTO usage_logs (request_id,user_id,actual_cost,input_tokens) VALUES ('rolled-back',1,999,999);
		ROLLBACK TO SAVEPOINT before_failed_usage;`)
	require.NoError(t, err)
	assertTotals(116, 490)
	_, err = tx.ExecContext(ctx, `DELETE FROM usage_logs WHERE user_id=1`)
	require.NoError(t, err)
	assertTotals(116, 490)

	// 金额相同按 Token 降序，已删除用户不公开。
	_, err = tx.ExecContext(ctx, `INSERT INTO usage_logs (request_id,user_id,actual_cost,input_tokens)
		VALUES ('tie',2,114,500);
		UPDATE users SET deleted_at=NOW() WHERE id=25;`)
	require.NoError(t, err)
	ranking, err = repo.GetUsageLeaderboard(ctx)
	require.NoError(t, err)
	require.Len(t, ranking, 20)
	require.Equal(t, "nickname-2", ranking[0].Username)
	require.Equal(t, "nickname-1", ranking[1].Username)
	for _, item := range ranking {
		require.NotEqual(t, "nickname-25", item.Username)
	}
}

package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestForumEventsPinnedSnapshot(t *testing.T) {
	for _, initial := range []bool{true, false} {
		name := "poll_without_arrivals"
		if initial {
			name = "initial_connection"
		}
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			repo := &forumRepository{db: db}
			cursor := int64(7)
			var after *int64
			if !initial {
				after = &cursor
			}
			now := time.Now()
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta("SELECT last_arrival_id,retention_days FROM forum_settings WHERE id=1")).
				WillReturnRows(sqlmock.NewRows([]string{"last_arrival_id", "retention_days"}).AddRow(cursor, 30))
			mock.ExpectQuery(regexp.QuoteMeta(forumMessageSelect + `WHERE m.pinned AND ($1=0 OR m.created_at > NOW()-make_interval(days => $1)) ORDER BY m.id DESC LIMIT 3`)).
				WithArgs(30).
				WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "username", "is_admin", "url", "actual_cost", "content", "rank", "pinned", "created_at"}).
					AddRow(9, 2, "private@example.com", true, "", 12.5, "<script>plain text</script>", 1, true, now))
			if !initial {
				mock.ExpectQuery("SELECT e.id,u.id,u.username").
					WithArgs(cursor, cursor).
					WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "username", "is_admin", "url", "actual_cost", "rank", "created_at"}))
			}
			mock.ExpectCommit()
			result, err := repo.Events(context.Background(), after)
			require.NoError(t, err)
			require.Equal(t, cursor, result.Cursor)
			require.Empty(t, result.Arrivals)
			require.Len(t, result.Pinned, 1)
			require.Empty(t, result.Pinned[0].Author.Username)
			require.True(t, result.Pinned[0].Author.IsAdmin)
			require.Equal(t, "<script>plain text</script>", result.Pinned[0].Content)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestForumEventsPinnedEmptyAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "empty_snapshot"
		if fail {
			name = "query_failure"
		}
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			repo := &forumRepository{db: db}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT last_arrival_id,retention_days").
				WillReturnRows(sqlmock.NewRows([]string{"last_arrival_id", "retention_days"}).AddRow(0, 0))
			query := mock.ExpectQuery(regexp.QuoteMeta(forumMessageSelect)).WithArgs(0)
			if fail {
				query.WillReturnError(errors.New("database unavailable"))
				mock.ExpectRollback()
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "username", "is_admin", "url", "actual_cost", "content", "rank", "pinned", "created_at"}))
				mock.ExpectCommit()
			}
			result, err := repo.Events(context.Background(), nil)
			if fail {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result.Pinned)
				require.Empty(t, result.Pinned)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
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
			now := time.Now().AddDate(0, 0, -400)
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta("SELECT last_arrival_id FROM forum_settings WHERE id=1")).
				WillReturnRows(sqlmock.NewRows([]string{"last_arrival_id"}).AddRow(cursor))
			mock.ExpectQuery(regexp.QuoteMeta(forumPinnedMessageSelect + `ORDER BY m.id DESC LIMIT 3`)).
				WithArgs().
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
			require.Equal(t, now, result.Pinned[0].CreatedAt)
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
			mock.ExpectQuery("SELECT last_arrival_id FROM forum_settings").
				WillReturnRows(sqlmock.NewRows([]string{"last_arrival_id"}).AddRow(0))
			query := mock.ExpectQuery(regexp.QuoteMeta(forumPinnedMessageSelect)).WithArgs()
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

func expectForumAdminMutation(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT muted_until FROM forum_settings WHERE id=1 FOR UPDATE`)).
		WillReturnRows(sqlmock.NewRows([]string{"muted_until"}).AddRow(nil))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role,status FROM users WHERE id=$1 AND deleted_at IS NULL FOR SHARE`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"role", "status"}).AddRow("admin", "active"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT clock_timestamp()`)).
		WillReturnRows(sqlmock.NewRows([]string{"now"}).AddRow(time.Now()))
}

func TestForumPinIndependentCopy(t *testing.T) {
	for _, tt := range []struct {
		name       string
		exists     bool
		count      int
		inserted   int64
		insertErr  bool
		wantErr    error
		wantErrMsg string
	}{
		{name: "copy_message", inserted: 1},
		{name: "already_pinned_after_history_cleanup", exists: true},
		{name: "archived_pins_count_towards_limit", count: 3, wantErrMsg: "FORUM_PIN_LIMIT"},
		{name: "missing_message", wantErr: service.ErrForumNotFound},
		{name: "copy_failure_rolls_back", insertErr: true, wantErrMsg: "copy failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			repo := &forumRepository{db: db}
			expectForumAdminMutation(mock)
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS(SELECT 1 FROM forum_pinned_messages WHERE id=$1)`)).
				WithArgs(int64(9)).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(tt.exists))
			if !tt.exists {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM forum_pinned_messages`)).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tt.count))
				if tt.count < 3 {
					insert := mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO forum_pinned_messages(id,user_id,content,rank_at_send,created_at)
                        SELECT id,user_id,content,rank_at_send,created_at FROM forum_messages WHERE id=$1`)).
						WithArgs(int64(9))
					if tt.insertErr {
						insert.WillReturnError(errors.New("copy failed"))
					} else {
						insert.WillReturnResult(sqlmock.NewResult(0, tt.inserted))
					}
				}
			}
			if tt.wantErr != nil || tt.wantErrMsg != "" {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			err = repo.Pin(context.Background(), 1, 9, true)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else if tt.wantErrMsg != "" {
				require.ErrorContains(t, err, tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestForumUnpinIndependentCopy(t *testing.T) {
	for _, tt := range []struct {
		name          string
		deleted       int64
		messageExists bool
		wantErr       bool
	}{
		{name: "unpin_after_history_cleanup", deleted: 1},
		{name: "already_unpinned_message", messageExists: true},
		{name: "missing_pin_and_message", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			repo := &forumRepository{db: db}
			expectForumAdminMutation(mock)
			mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM forum_pinned_messages WHERE id=$1`)).
				WithArgs(int64(9)).
				WillReturnResult(sqlmock.NewResult(0, tt.deleted))
			if tt.deleted == 0 {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS(SELECT 1 FROM forum_messages WHERE id=$1)`)).
					WithArgs(int64(9)).
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(tt.messageExists))
			}
			if tt.wantErr {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			err = repo.Pin(context.Background(), 1, 9, false)
			if tt.wantErr {
				require.ErrorIs(t, err, service.ErrForumNotFound)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestForumRecallPermissionsAndAtomicity(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name      string
		admin     bool
		author    any
		age       time.Duration
		missing   bool
		pinError  bool
		chatError bool
		wantErr   error
	}{
		{name: "own_recent_message", author: int64(1)},
		{name: "just_before_deadline", author: int64(1), age: time.Minute - time.Microsecond},
		{name: "exact_deadline", author: int64(1), age: time.Minute, wantErr: service.ErrForumRecallExpired},
		{name: "expired", author: int64(1), age: time.Minute + time.Microsecond, wantErr: service.ErrForumRecallExpired},
		{name: "another_user", author: int64(2), wantErr: service.ErrForumRecallForbidden},
		{name: "deleted_author", wantErr: service.ErrForumRecallForbidden},
		{name: "admin_old_message", admin: true, author: int64(2), age: 400 * 24 * time.Hour},
		{name: "admin_own_old_message", admin: true, author: int64(1), age: 400 * 24 * time.Hour},
		{name: "admin_deleted_author", admin: true},
		{name: "missing_or_already_recalled", author: int64(1), missing: true, wantErr: service.ErrForumNotFound},
		{name: "pin_delete_failure", author: int64(1), pinError: true},
		{name: "chat_delete_failure_rolls_back_pin", author: int64(1), chatError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			repo := &forumRepository{db: db}
			mock.ExpectBegin()
			// Being muted must not prevent recalling a message.
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT muted_until FROM forum_settings WHERE id=1 FOR UPDATE`)).
				WillReturnRows(sqlmock.NewRows([]string{"muted_until"}).AddRow(now.Add(time.Hour)))
			role := "user"
			if tt.admin {
				role = "admin"
			}
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT role,status FROM users WHERE id=$1 AND deleted_at IS NULL FOR SHARE`)).
				WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"role", "status"}).AddRow(role, "active"))
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT clock_timestamp()`)).
				WillReturnRows(sqlmock.NewRows([]string{"now"}).AddRow(now))
			rows := sqlmock.NewRows([]string{"user_id", "created_at"})
			if !tt.missing {
				rows.AddRow(tt.author, now.Add(-tt.age))
			}
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT user_id,created_at FROM forum_messages WHERE id=$1
                UNION ALL SELECT user_id,created_at FROM forum_pinned_messages WHERE id=$1 LIMIT 1`)).
				WithArgs(int64(9)).WillReturnRows(rows)
			if tt.wantErr == nil {
				pin := mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM forum_pinned_messages WHERE id=$1`)).WithArgs(int64(9))
				if tt.pinError {
					pin.WillReturnError(errors.New("delete failed"))
				} else {
					pin.WillReturnResult(sqlmock.NewResult(0, 1))
					chat := mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM forum_messages WHERE id=$1`)).WithArgs(int64(9))
					if tt.chatError {
						chat.WillReturnError(errors.New("delete failed"))
					} else {
						chat.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
			}
			if tt.wantErr != nil || tt.pinError || tt.chatError {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			err = repo.Recall(context.Background(), 1, 9)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else if tt.pinError || tt.chatError {
				require.ErrorContains(t, err, "delete failed")
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestForumRecallRejectsInactiveActors(t *testing.T) {
	for _, status := range []string{"disabled", "deleted"} {
		t.Run(status, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT muted_until FROM forum_settings").
				WillReturnRows(sqlmock.NewRows([]string{"muted_until"}).AddRow(nil))
			rows := sqlmock.NewRows([]string{"role", "status"})
			if status != "deleted" {
				rows.AddRow("admin", status)
			}
			mock.ExpectQuery("SELECT role,status FROM users").WithArgs(int64(1)).WillReturnRows(rows)
			mock.ExpectRollback()
			err = (&forumRepository{db: db}).Recall(context.Background(), 1, 9)
			require.ErrorIs(t, err, service.ErrForumAdmin)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

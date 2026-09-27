//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func (s *UserRepoSuite) TestUsernameUniqueAndEmpty() {
	u := s.mustCreateUser(&service.User{Username: "  Alice\t "})
	s.Equal("Alice", u.Username)
	other := s.mustCreateUser(&service.User{})
	s.mustCreateUser(&service.User{Username: " \n\t "})
	other.Username = " aLiCe "
	s.ErrorIs(s.repo.Update(s.ctx, other, service.UserUpdateFields{Username: true}), service.ErrUsernameExists)
	stored, err := s.repo.GetByID(s.ctx, other.ID)
	s.Require().NoError(err)
	s.Empty(stored.Username)
	duplicate := &service.User{Email: "duplicate-name@example.com", Username: "ALICE", PasswordHash: "hash", Role: service.RoleUser, Status: service.StatusActive}
	s.ErrorIs(s.repo.Create(s.ctx, duplicate), service.ErrUsernameExists)
	s.NoError(s.repo.Update(s.ctx, u, service.UserUpdateFields{Username: true}))
	_, err = s.client.User.UpdateOneID(u.ID).SetDeletedAt(time.Now()).Save(s.ctx)
	s.Require().NoError(err)
	s.NoError(s.repo.Create(s.ctx, duplicate))
	_, err = s.client.User.UpdateOneID(u.ID).ClearDeletedAt().Save(s.ctx)
	s.ErrorIs(translateUserWriteError(err), service.ErrUsernameExists)
}

func (s *UserRepoSuite) TestUsernameReservedForAdmin() {
	admin := s.mustCreateUser(&service.User{Username: "管理员", Role: service.RoleAdmin})
	s.Equal("管理员", admin.Username)
	first := s.mustCreateUser(&service.User{Username: "管理员"})
	s.Equal("小丑", first.Username)
	second := s.mustCreateUser(&service.User{})
	second.Username = " \t管理员\n"
	s.Require().NoError(s.repo.Update(s.ctx, second, service.UserUpdateFields{Username: true}))
	s.Equal("小丑_1", second.Username)
	admin.Role = service.RoleUser
	s.Require().NoError(s.repo.Update(s.ctx, admin, service.UserUpdateFields{Role: true}))
	s.Equal("小丑_2", admin.Username)
	// 直接 Ent 写入覆盖第三方账号采用昵称的入口。
	_, err := s.client.User.UpdateOneID(first.ID).SetUsername("管理员").Save(s.ctx)
	s.Require().NoError(err)
	stored, err := s.repo.GetByID(s.ctx, first.ID)
	s.Require().NoError(err)
	s.Equal("小丑", stored.Username)
	otherAdmin := s.mustCreateUser(&service.User{Username: "管理员", Role: service.RoleAdmin})
	first.Role, first.Username = service.RoleAdmin, "管理员"
	s.ErrorIs(s.repo.Update(s.ctx, first, service.UserUpdateFields{Role: true, Username: true}), service.ErrUsernameExists)
	s.NotZero(otherAdmin.ID)
}

func (s *UserRepoSuite) TestUsernameConcurrentWrites() {
	for _, name := range []string{"同名", "管理员"} {
		const count = 8
		var wg sync.WaitGroup
		errs := make(chan error, count)
		for i := 0; i < count; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				u := &service.User{Email: fmt.Sprintf("%s-%d@example.com", name, i), Username: name, PasswordHash: "hash", Role: service.RoleUser, Status: service.StatusActive}
				errs <- s.repo.Create(s.ctx, u)
			}(i)
		}
		wg.Wait()
		close(errs)
		success, conflicts := 0, 0
		for err := range errs {
			if err == nil {
				success++
			} else if errors.Is(err, service.ErrUsernameExists) {
				conflicts++
			} else {
				s.Require().NoError(err)
			}
		}
		if name == "管理员" {
			s.Equal(count, success)
			s.Zero(conflicts)
		} else {
			s.Equal(1, success)
			s.Equal(count-1, conflicts)
		}
	}
}

func (s *UserRepoSuite) TestUsernameCreateRespectsOuterRollback() {
	tx, err := s.client.Tx(s.ctx)
	s.Require().NoError(err)
	defer tx.Rollback()
	u := &service.User{Email: "rollback-name@example.com", Username: "管理员", PasswordHash: "hash", Role: service.RoleUser, Status: service.StatusActive}
	s.Require().NoError(s.repo.Create(dbent.NewTxContext(s.ctx, tx), u))
	s.Equal("小丑", u.Username)
	s.Require().NoError(tx.Rollback())
	_, err = s.repo.GetByID(s.ctx, u.ID)
	s.ErrorIs(err, service.ErrUserNotFound)
}

func TestUsernameMigration(t *testing.T) {
	raw, err := migrations.FS.ReadFile("247_user_username_rules.sql")
	require.NoError(t, err)
	for _, duplicate := range []bool{false, true} {
		t.Run(fmt.Sprint(duplicate), func(t *testing.T) {
			tx := testTx(t)
			ctx := context.Background()
			_, err := tx.ExecContext(ctx, `CREATE SCHEMA username_migration_test;
SET LOCAL search_path TO username_migration_test, public;
CREATE TABLE users (id bigint PRIMARY KEY, username varchar(100) NOT NULL DEFAULT '', role text NOT NULL, deleted_at timestamptz);
INSERT INTO users VALUES (1,'管理员','admin',NULL),(2,' 管理员 ','user',NULL),(3,'管理员','user',NULL),(4,'小丑','user',NULL),(5,' Alice ','user',NULL),(6,'','user',NULL),(7,'','user',NULL);`)
			require.NoError(t, err)
			if duplicate {
				_, err = tx.ExecContext(ctx, `INSERT INTO users VALUES (8,'ALICE','user',NULL); SAVEPOINT before_migration`)
				require.NoError(t, err)
			}
			_, err = tx.ExecContext(ctx, string(raw))
			if duplicate {
				require.Error(t, err)
				_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT before_migration`)
				require.NoError(t, err)
				var name string
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT username FROM users WHERE id=2`).Scan(&name))
				require.Equal(t, " 管理员 ", name, "历史重名导致迁移失败时必须完整回滚")
				return
			}
			require.NoError(t, err)
			var duplicates, reserved int
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM (SELECT lower(username) FROM users WHERE username<>'' GROUP BY lower(username) HAVING count(*)>1) d`).Scan(&duplicates))
			require.Zero(t, duplicates)
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE username='管理员' AND role<>'admin'`).Scan(&reserved))
			require.Zero(t, reserved)
		})
	}
}

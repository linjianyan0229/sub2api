//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type usernameConflictRepo struct{ *userRepoStub }

func (r *usernameConflictRepo) Update(context.Context, *User, UserUpdateFields) error {
	return ErrUsernameExists
}

func TestAuthOAuthUsernameConflict(t *testing.T) {
	for _, pair := range []bool{false, true} {
		t.Run(map[bool]string{false: "access", true: "pair"}[pair], func(t *testing.T) {
			ctx := context.Background()
			repo := &userRepoStub{createErr: ErrUsernameExists}
			svc := newAuthService(repo, map[string]string{SettingKeyRegistrationEnabled: "true"}, nil, nil)
			svc.refreshTokenCache = &refreshTokenCacheStub{}
			var err error
			if pair {
				_, _, err = svc.LoginOrRegisterOAuthWithTokenPair(ctx, "new@example.com", "同名", "", "", "linuxdo")
			} else {
				_, _, err = svc.LoginOrRegisterOAuth(ctx, "new@example.com", "同名")
			}
			require.ErrorIs(t, err, ErrUsernameExists)
			require.Empty(t, repo.created)
			// 已有账号可继续登录，但不能返回未保存成功的昵称。
			repo.user = &User{ID: 1, Email: "existing@example.com", Role: RoleUser, Status: StatusActive}
			svc.userRepo = &usernameConflictRepo{repo}
			var user *User
			if pair {
				_, user, err = svc.LoginOrRegisterOAuthWithTokenPair(ctx, repo.user.Email, "同名", "", "", "linuxdo")
			} else {
				_, user, err = svc.LoginOrRegisterOAuth(ctx, repo.user.Email, "同名")
			}
			require.NoError(t, err)
			require.Empty(t, user.Username)
		})
	}
}

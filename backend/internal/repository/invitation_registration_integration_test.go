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
	"github.com/Wei-Shaw/sub2api/ent/invitationusage"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type invitationSettings struct{ service.SettingRepository }

func (invitationSettings) GetValue(_ context.Context, key string) (string, error) {
	if key == service.SettingKeyRegistrationEnabled || key == service.SettingKeyInvitationCodeEnabled {
		return "true", nil
	}
	return "", service.ErrSettingNotFound
}
func (s invitationSettings) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	values := map[string]string{}
	for _, key := range keys {
		if value, err := s.GetValue(ctx, key); err == nil {
			values[key] = value
		}
	}
	return values, nil
}

func TestInvitationRegistrationAtomicity(t *testing.T) {
	ctx := context.Background()
	client := integrationEntClient
	userRepo := NewUserRepository(client, integrationDB)
	codeRepo := NewRedeemCodeRepository(client)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "invitation-test-secret", ExpireHour: 1, AccessTokenExpireMinutes: 60, RefreshTokenExpireDays: 1}}
	settings := service.NewSettingService(invitationSettings{}, cfg)
	auth := service.NewAuthService(client, userRepo, codeRepo, NewRefreshTokenCache(integrationRedis), cfg, settings, nil, nil, nil, nil, nil, nil, nil)
	admin := service.NewRedeemService(codeRepo, userRepo, nil, nil, nil, client, nil, nil)
	prefix := uuid.NewString()[:8]
	var createdCodes []int64
	t.Cleanup(func() {
		// 并发用例提交了真实事务，只清理本用例的数据，避免干扰其他仓储套件。
		_, err := client.InvitationUsage.Delete().Where(invitationusage.EmailHasPrefix(prefix)).Exec(ctx)
		require.NoError(t, err)
		for _, id := range createdCodes {
			require.NoError(t, codeRepo.Delete(ctx, id))
		}
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE email LIKE $1`, prefix+"%")
		require.NoError(t, err)
	})
	createCode := func(limit int) *service.RedeemCode {
		code := &service.RedeemCode{Code: uuid.NewString()[:32], Type: service.RedeemTypeInvitation, Status: service.StatusUnused, MaxUses: limit}
		require.NoError(t, codeRepo.Create(ctx, code))
		createdCodes = append(createdCodes, code.ID)
		return code
	}
	register := func(ctx context.Context, code *service.RedeemCode, suffix string) (*service.User, error) {
		_, u, err := auth.RegisterWithVerification(ctx, prefix+suffix+"@example.com", "Password123!", "", "", code.Code, "")
		return u, err
	}
	code := createCode(3)
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := register(service.WithRegistrationIP(ctx, fmt.Sprintf("198.51.100.%d", i+1)), code, fmt.Sprintf("capacity%d", i))
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, service.ErrInvitationCodeInvalid)
		}
	}
	require.Equal(t, 3, successes)
	require.Equal(t, 3, client.User.Query().Where(user.EmailHasPrefix(prefix+"capacity")).CountX(ctx))
	require.Equal(t, 3, client.InvitationUsage.Query().Where(invitationusage.RedeemCodeIDEQ(code.ID)).CountX(ctx))
	full, err := codeRepo.GetByID(ctx, code.ID)
	require.NoError(t, err)
	require.False(t, full.CanUse())
	_, err = admin.UpdateInvitationLimit(ctx, code.ID, 2)
	require.ErrorIs(t, err, service.ErrInvitationLimitInvalid)
	updated, err := admin.UpdateInvitationLimit(ctx, code.ID, 4)
	require.NoError(t, err)
	require.True(t, updated.CanUse())
	_, err = register(service.WithRegistrationIP(ctx, "198.51.100.99"), code, "expanded")
	require.NoError(t, err)
	items, total, err := admin.ListInvitationUsages(ctx, code.ID, pagination.PaginationParams{Page: 2, PageSize: 2})
	require.NoError(t, err)
	require.EqualValues(t, 4, total)
	require.Len(t, items, 2)
	require.NotNil(t, items[0].IP)

	// 同一个 IP 并发使用不同邀请码只能创建一个账号。
	sharedIP := service.WithRegistrationIP(ctx, "2001:db8:1234::1")
	results = make(chan error, 8)
	codes := make([]*service.RedeemCode, 8)
	for i := range codes {
		codes[i] = createCode(2)
	}
	for i, code := range codes {
		wg.Add(1)
		go func(i int, code *service.RedeemCode) {
			defer wg.Done()
			_, err := register(sharedIP, code, fmt.Sprintf("shared%d", i))
			results <- err
		}(i, code)
	}
	wg.Wait()
	close(results)
	successes = 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, service.ErrRegistrationIPUsed)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, client.User.Query().Where(user.EmailHasPrefix(prefix+"shared")).CountX(ctx))
	totalUsed := 0
	for _, code := range codes {
		latest, err := codeRepo.GetByID(ctx, code.ID)
		require.NoError(t, err)
		totalUsed += latest.UsedCount
	}
	require.Equal(t, 1, totalUsed)

	// OAuth 新账号同样不能绕过已占用的 IP；已有账号登录保持可用。
	_, _, err = auth.LoginOrRegisterVerifiedEmailOAuthWithInvitation(sharedIP, service.EmailOAuthIdentityInput{
		ProviderType: "google", ProviderKey: "google", ProviderSubject: prefix + "oauth", Email: prefix + "oauth@example.com", EmailVerified: true,
	}, createCode(2).Code, "")
	require.ErrorIs(t, err, service.ErrRegistrationIPUsed)
	require.Zero(t, client.User.Query().Where(user.EmailEQ(prefix+"oauth@example.com")).CountX(ctx))
	existing := client.User.Query().Where(user.EmailHasPrefix(prefix + "shared")).OnlyX(ctx)
	_, loginUser, err := auth.Login(sharedIP, existing.Email, "Password123!")
	require.NoError(t, err)
	require.Equal(t, existing.ID, loginUser.ID)

	// OAuth 完成阶段与外层事务一起回滚，不消耗名额或 IP。
	rollbackCode := createCode(2)
	rollbackIP := service.WithRegistrationIP(ctx, "2001:db8:1234::2")
	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(rollbackIP, tx)
	u := tx.Client().User.Create().SetEmail(prefix + "rollback@example.com").SetPasswordHash("hash").SaveX(txCtx)
	require.NoError(t, auth.FinalizeOAuthEmailAccount(txCtx, &service.User{ID: u.ID}, rollbackCode.Code, "google", ""))
	require.NoError(t, tx.Rollback())
	require.Zero(t, client.InvitationUsage.Query().Where(invitationusage.RedeemCodeIDEQ(rollbackCode.ID)).CountX(ctx))
	_, err = register(rollbackIP, rollbackCode, "afterrollback")
	require.NoError(t, err)
	// 正常删除账号/邀请码不能重新开放 IP。
	success := client.User.Query().Where(user.EmailEQ(prefix + "afterrollback@example.com")).OnlyX(ctx)
	require.NoError(t, userRepo.Delete(ctx, success.ID))
	require.NoError(t, codeRepo.Delete(ctx, rollbackCode.ID))
	_, err = register(rollbackIP, createCode(2), "afterdelete")
	require.ErrorIs(t, err, service.ErrRegistrationIPUsed)

	// 过期和禁用的邀请码均不能使用。
	for _, status := range []string{service.StatusExpired, "disabled"} {
		blocked := createCode(2)
		blocked.Status = status
		require.NoError(t, codeRepo.Update(ctx, blocked))
		_, err = register(service.WithRegistrationIP(ctx, "192.0.2.199"), blocked, status)
		require.True(t, errors.Is(err, service.ErrInvitationCodeInvalid))
	}
}

func TestInvitationHistoryMigration(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	schema := "invite_" + uuid.NewString()[:8]
	_, err := tx.ExecContext(ctx, `CREATE SCHEMA `+schema+`; SET LOCAL search_path TO `+schema+`;
	CREATE TABLE users (id BIGINT PRIMARY KEY, username TEXT, email TEXT);
	CREATE TABLE redeem_codes (id BIGINT PRIMARY KEY, type TEXT, status TEXT, used_by BIGINT, used_at TIMESTAMPTZ, created_at TIMESTAMPTZ);
	INSERT INTO users VALUES (1,'旧用户','legacy@example.com');
	INSERT INTO redeem_codes VALUES (1,'invitation','used',1,NOW(),NOW()), (2,'balance','used',1,NOW(),NOW());`)
	require.NoError(t, err)
	migration, err := dbmigrations.FS.ReadFile("243_reusable_invitation_codes.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	var ip *string
	var username string
	var created time.Time
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT ip,username,created_at FROM invitation_usages WHERE redeem_code_id=1`).Scan(&ip, &username, &created))
	require.Nil(t, ip)
	require.Equal(t, "旧用户", username)
	require.False(t, created.IsZero())
	var count int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM invitation_usages`).Scan(&count))
	require.Equal(t, 1, count)
}

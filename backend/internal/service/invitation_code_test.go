//go:build unit

package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newInvitationTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:invitation-%s?mode=memory&cache=shared", t.Name()))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestInvitationIPCanonicalization(t *testing.T) {
	for _, test := range []struct{ input, expected string }{
		{"::ffff:192.0.2.1", "192.0.2.1"}, {"2001:0db8::1", "2001:db8::1"},
		{"192.0.2.1", "192.0.2.1"}, {"", ""}, {"not-an-ip", ""}, {"::", ""}, {"fe80::1%eth0", ""},
	} {
		require.Equal(t, test.expected, RegistrationIPFromContext(WithRegistrationIP(context.Background(), test.input)))
	}
}

func TestInvitationClaimCapacityIPAndRollback(t *testing.T) {
	client := newInvitationTestClient(t)
	ctx := context.Background()
	code := client.RedeemCode.Create().SetCode("repeat").SetType(RedeemTypeInvitation).SetMaxUses(2).SaveX(ctx)
	other := client.RedeemCode.Create().SetCode("another").SetType(RedeemTypeInvitation).SetMaxUses(2).SaveX(ctx)
	users := make([]int64, 4)
	for i := range users {
		users[i] = client.User.Create().SetEmail(fmt.Sprintf("member%d@example.com", i)).SetPasswordHash("hash").SaveX(ctx).ID
	}
	oneIP := WithRegistrationIP(ctx, "192.0.2.1")
	require.NoError(t, claimInvitation(oneIP, client, code.ID, users[0]))
	require.ErrorIs(t, claimInvitation(WithRegistrationIP(ctx, "::ffff:192.0.2.1"), client, other.ID, users[1]), ErrRegistrationIPUsed)
	require.Equal(t, 0, client.RedeemCode.GetX(ctx, other.ID).UsedCount)
	twoIP := WithRegistrationIP(ctx, "192.0.2.2")
	require.NoError(t, claimInvitation(twoIP, client, code.ID, users[1]))
	require.ErrorIs(t, claimInvitation(WithRegistrationIP(ctx, "192.0.2.3"), client, code.ID, users[2]), ErrInvitationCodeInvalid)
	require.Equal(t, StatusUsed, client.RedeemCode.GetX(ctx, code.ID).Status)
	// 撤销失败账号只回收它自己的名额/IP，其他成功记录仍然生效。
	require.NoError(t, releaseInvitation(ctx, client, code.Code, users[1]))
	require.NoError(t, releaseInvitation(ctx, client, code.Code, users[1]))
	require.Equal(t, 1, client.RedeemCode.GetX(ctx, code.ID).UsedCount)
	require.NoError(t, claimInvitation(twoIP, client, other.ID, users[2]))
	require.ErrorIs(t, claimInvitation(oneIP, client, other.ID, users[3]), ErrRegistrationIPUsed)
	// 删码后 IP 占用仍然保留。
	require.NoError(t, client.RedeemCode.DeleteOneID(code.ID).Exec(ctx))
	require.ErrorIs(t, claimInvitation(oneIP, client, other.ID, users[3]), ErrRegistrationIPUsed)
	require.ErrorIs(t, claimInvitation(ctx, client, other.ID, users[3]), ErrRegistrationIPMissing)
}

func TestInvitationClaimExpiredAndOuterRollback(t *testing.T) {
	client := newInvitationTestClient(t)
	ctx := WithRegistrationIP(context.Background(), "2001:db8::1")
	code := client.RedeemCode.Create().SetCode("rollback").SetType(RedeemTypeInvitation).SaveX(ctx)
	user := client.User.Create().SetEmail("rollback@example.com").SetPasswordHash("hash").SaveX(ctx)
	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	require.NoError(t, claimInvitation(dbent.NewTxContext(ctx, tx), client, code.ID, user.ID))
	require.NoError(t, tx.Rollback())
	require.Zero(t, client.InvitationUsage.Query().CountX(ctx))
	require.Zero(t, client.RedeemCode.GetX(ctx, code.ID).UsedCount)
	require.NoError(t, client.RedeemCode.UpdateOneID(code.ID).SetExpiresAt(time.Now().Add(-time.Hour)).Exec(ctx))
	require.ErrorIs(t, claimInvitation(ctx, client, code.ID, user.ID), ErrInvitationCodeInvalid)
	require.NoError(t, client.RedeemCode.UpdateOneID(code.ID).ClearExpiresAt().SetStatus(StatusExpired).Exec(ctx))
	require.ErrorIs(t, claimInvitation(ctx, client, code.ID, user.ID), ErrInvitationCodeInvalid)
	require.NoError(t, client.RedeemCode.UpdateOneID(code.ID).SetStatus(StatusUnused).Exec(ctx))
	require.NoError(t, claimInvitation(ctx, client, code.ID, user.ID))
}

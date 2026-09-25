package service

import (
	"context"
	"net/netip"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/invitationusage"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrRegistrationIPUsed     = infraerrors.Conflict("REGISTRATION_IP_USED", "该 IP 已注册过账号")
	ErrRegistrationIPMissing  = infraerrors.BadRequest("REGISTRATION_IP_MISSING", "无法获取注册 IP")
	ErrInvitationLimitInvalid = infraerrors.BadRequest("INVITATION_LIMIT_INVALID", "人数上限必须为正整数，且不能少于已使用人数")
)

type registrationIPKey struct{}

// WithRegistrationIP 只接收经过可信代理链解析的地址，避免原始转发头绕过注册限制。
func WithRegistrationIP(ctx context.Context, value string) context.Context {
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	canonical := ""
	if err == nil && addr.Zone() == "" && !addr.IsUnspecified() {
		canonical = addr.Unmap().String()
	}
	return context.WithValue(ctx, registrationIPKey{}, canonical)
}

func RegistrationIPFromContext(ctx context.Context) string {
	value, _ := ctx.Value(registrationIPKey{}).(string)
	return value
}

// 用户创建、名额占用和 IP 唯一记录必须共用事务。
func withInvitationTx(ctx context.Context, client *dbent.Client, fn func(context.Context, *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}
	if client == nil {
		return ErrServiceUnavailable
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(dbent.NewTxContext(ctx, tx), tx.Client()); err != nil {
		return err
	}
	return tx.Commit()
}

func claimInvitation(ctx context.Context, client *dbent.Client, codeID, userID int64) error {
	registrationIP := RegistrationIPFromContext(ctx)
	if registrationIP == "" {
		return ErrRegistrationIPMissing
	}
	return withInvitationTx(ctx, client, func(ctx context.Context, client *dbent.Client) error {
		now := time.Now().UTC()
		// 条件更新取得邀请码行锁；同码并发不能超过上限。
		n, err := client.RedeemCode.Update().Where(
			redeemcode.IDEQ(codeID), redeemcode.TypeEQ(RedeemTypeInvitation),
			redeemcode.StatusEQ(StatusUnused),
			redeemcode.Or(redeemcode.ExpiresAtIsNil(), redeemcode.ExpiresAtGT(now)),
			func(s *entsql.Selector) {
				s.Where(entsql.ColumnsLT(s.C(redeemcode.FieldUsedCount), s.C(redeemcode.FieldMaxUses)))
			},
		).AddUsedCount(1).SetUsedBy(userID).SetUsedAt(now).Save(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrInvitationCodeInvalid
		}
		user, err := client.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		// IP 的唯一约束跨邀请码生效，ON CONFLICT 不会使外层 PostgreSQL 事务失效。
		result, err := client.ExecContext(ctx, `INSERT INTO invitation_usages
			(redeem_code_id,user_id,username,email,ip,created_at) VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (ip) DO NOTHING`, codeID, userID, user.Username, user.Email, registrationIP, now)
		if err != nil {
			return err
		}
		nRows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if nRows == 0 {
			return ErrRegistrationIPUsed
		}
		return client.RedeemCode.Update().Where(redeemcode.IDEQ(codeID),
			func(s *entsql.Selector) {
				s.Where(entsql.ColumnsGTE(s.C(redeemcode.FieldUsedCount), s.C(redeemcode.FieldMaxUses)))
			},
		).SetStatus(StatusUsed).Exec(ctx)
	})
}

// 仅注册流程失败时撤销该用户的占用，正常删用户或删邀请码不调用此函数。
func releaseInvitation(ctx context.Context, client *dbent.Client, code string, userID int64) error {
	return withInvitationTx(ctx, client, func(ctx context.Context, client *dbent.Client) error {
		invitation, err := client.RedeemCode.Query().Where(redeemcode.CodeEQ(code), redeemcode.TypeEQ(RedeemTypeInvitation)).Only(ctx)
		if dbent.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		// 与领取相同的加锁顺序，避免同码领取和回滚死锁。
		if err := client.RedeemCode.UpdateOneID(invitation.ID).AddUsedCount(0).Exec(ctx); err != nil {
			return err
		}
		n, err := client.InvitationUsage.Delete().Where(invitationusage.RedeemCodeIDEQ(invitation.ID), invitationusage.UserIDEQ(userID)).Exec(ctx)
		if err != nil || n == 0 {
			return err
		}
		_, err = client.ExecContext(ctx, `UPDATE redeem_codes SET used_count = used_count - 1,
			status = CASE WHEN status = 'used' THEN 'unused' ELSE status END,
			used_by = (SELECT user_id FROM invitation_usages WHERE redeem_code_id = $1 ORDER BY id DESC LIMIT 1),
			used_at = (SELECT created_at FROM invitation_usages WHERE redeem_code_id = $1 ORDER BY id DESC LIMIT 1)
			WHERE id = $1 AND used_count > 0`, invitation.ID)
		return err
	})
}

type InvitationUsage struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	IP        *string   `json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *RedeemService) UpdateInvitationLimit(ctx context.Context, id int64, limit int) (*RedeemCode, error) {
	if limit < 1 || limit > 2147483647 {
		return nil, ErrInvitationLimitInvalid
	}
	if s.entClient == nil {
		return nil, ErrServiceUnavailable
	}
	result, err := s.entClient.ExecContext(ctx, `UPDATE redeem_codes SET max_uses = $2,
		status = CASE WHEN status IN ('unused','used') THEN CASE WHEN used_count < $2 THEN 'unused' ELSE 'used' END ELSE status END
		WHERE id = $1 AND type = 'invitation' AND used_count <= $2`, id, limit)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrInvitationLimitInvalid
	}
	return s.redeemRepo.GetByID(ctx, id)
}

func (s *RedeemService) ListInvitationUsages(ctx context.Context, id int64, params pagination.PaginationParams) ([]InvitationUsage, int64, error) {
	if s.entClient == nil {
		return nil, 0, ErrServiceUnavailable
	}
	code, err := s.redeemRepo.GetByID(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	if code.Type != RedeemTypeInvitation {
		return nil, 0, ErrInvitationCodeInvalid
	}
	q := s.entClient.InvitationUsage.Query().Where(invitationusage.RedeemCodeIDEQ(id))
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.Order(dbent.Desc(invitationusage.FieldID)).Offset(params.Offset()).Limit(params.Limit()).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	items := make([]InvitationUsage, 0, len(rows))
	for _, row := range rows {
		items = append(items, InvitationUsage{ID: row.ID, UserID: row.UserID, Username: row.Username, Email: row.Email, IP: row.IP, CreatedAt: row.CreatedAt})
	}
	return items, int64(total), nil
}

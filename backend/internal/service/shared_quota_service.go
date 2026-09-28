package service

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/pkg/pluginapi/sharedquota"
)

var ErrSharedQuotaExhausted = infraerrors.TooManyRequests("SHARED_QUOTA_EXHAUSTED", "全站每日共享额度已用完")
var ErrSharedGroupQuotaExhausted = infraerrors.TooManyRequests("SHARED_GROUP_QUOTA_EXHAUSTED", "该分组每日共享额度上限已用完")

var ErrSharedQuotaPolicyChanged = errors.New("共享额度配置已变更")

type SharedQuotaSnapshot struct {
	ContributionAccountID int64             `json:"contribution_account_id,omitempty"`
	IndependentAccountIDs []int64           `json:"independent_account_ids,omitempty"`
	WindowID              int64             `json:"window_id"`
	Total                 float64           `json:"total"`
	Used                  float64           `json:"used"`
	Held                  float64           `json:"held"`
	Remaining             float64           `json:"remaining"`
	ResetsAt              time.Time         `json:"resets_at"`
	Timezone              string            `json:"timezone"`
	GroupID               int64             `json:"group_id,omitempty"`
	GroupQuota            *SharedGroupQuota `json:"group_quota,omitempty"`
}

type SharedGroupQuota struct {
	Limit     float64 `json:"limit"`
	Used      float64 `json:"used"`
	Held      float64 `json:"held"`
	Remaining float64 `json:"remaining"`
}

func (q *SharedQuotaSnapshot) CheckRemaining() error {
	if len(q.IndependentAccountIDs) > 0 {
		return nil
	}
	return q.CheckPoolRemaining()
}

func (q *SharedQuotaSnapshot) CheckPoolRemaining() error {
	if q.Remaining <= 0 {
		return ErrSharedQuotaExhausted
	}
	if q.GroupQuota != nil && q.GroupQuota.Remaining <= 0 {
		return ErrSharedGroupQuotaExhausted
	}
	return nil
}

type SharedQuotaPolicy struct {
	PluginKey, ConfigEncrypted string
	GroupID                    int64
	GroupLimits                map[int64]float64
}

type SharedQuotaRepository interface {
	ActivePolicy(context.Context) (*SharedQuotaPolicy, error)
	Window(context.Context, *SharedQuotaPolicy, time.Time, time.Time, time.Time, float64) (*SharedQuotaSnapshot, error)
	GetWindow(context.Context, int64) (*SharedQuotaSnapshot, error)
	GetGroupWindow(context.Context, int64, int64) (*SharedQuotaSnapshot, error)
	GetGroupQuotas(context.Context, int64, []int64) (map[int64]SharedGroupQuota, error)
}

type SharedQuotaService struct {
	repo      SharedQuotaRepository
	encryptor SecretEncryptor
	location  *time.Location
}

func NewSharedQuotaService(repo SharedQuotaRepository, encryptor SecretEncryptor, cfg *config.Config) (*SharedQuotaService, error) {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, err
	}
	return &SharedQuotaService{repo: repo, encryptor: encryptor, location: loc}, nil
}

func (s *SharedQuotaService) Snapshot(ctx context.Context, user *User) (*SharedQuotaSnapshot, error) {
	return s.snapshotForGroup(ctx, user, 0)
}

func (s *SharedQuotaService) snapshotForGroup(ctx context.Context, user *User, groupID int64) (*SharedQuotaSnapshot, error) {
	if s == nil || user == nil || user.IsAdmin() {
		return nil, nil
	}
	return s.currentSnapshot(ctx, groupID)
}

func (s *SharedQuotaService) currentSnapshot(ctx context.Context, groupID int64) (*SharedQuotaSnapshot, error) {
	// 配置保存与账期查询可能来自不同实例；旧配置不能覆盖刚保存的额度。
	for attempt := 0; attempt < 3; attempt++ {
		w, err := s.snapshotCurrentPolicy(ctx, groupID)
		if errors.Is(err, ErrSharedQuotaPolicyChanged) {
			continue
		}
		return w, err
	}
	return nil, ErrBillingServiceUnavailable.WithCause(ErrSharedQuotaPolicyChanged)
}

func (s *SharedQuotaService) snapshotCurrentPolicy(ctx context.Context, groupID int64) (*SharedQuotaSnapshot, error) {
	p, err := s.repo.ActivePolicy(ctx)
	if err != nil {
		return nil, ErrBillingServiceUnavailable.WithCause(err)
	}
	if p == nil {
		return nil, nil
	}
	raw, err := s.encryptor.Decrypt(p.ConfigEncrypted)
	if err != nil {
		return nil, ErrBillingServiceUnavailable.WithCause(err)
	}
	c, err := sharedquota.Parse([]byte(raw))
	if err != nil {
		return nil, ErrBillingServiceUnavailable.WithCause(err)
	}
	now := time.Now()
	resolved := *p
	resolved.GroupID, resolved.GroupLimits = groupID, c.GroupLimits
	start, end := c.Window(now, s.location)
	w, err := s.repo.Window(ctx, &resolved, now, start, end, QuantizeUsageBillingAmount(c.DailyQuota))
	if errors.Is(err, ErrSharedQuotaPolicyChanged) {
		return nil, err
	}
	if err != nil {
		return nil, ErrBillingServiceUnavailable.WithCause(err)
	}
	w.Timezone = s.location.String()
	return w, nil
}

func (s *SharedQuotaService) Check(ctx context.Context, snapshot *SharedQuotaSnapshot) error {
	if snapshot == nil {
		return nil
	}
	if s == nil {
		return ErrBillingServiceUnavailable
	}
	var w *SharedQuotaSnapshot
	var err error
	if snapshot.GroupID > 0 {
		w, err = s.repo.GetGroupWindow(ctx, snapshot.WindowID, snapshot.GroupID)
	} else {
		w, err = s.repo.GetWindow(ctx, snapshot.WindowID)
	}
	if err != nil {
		return ErrBillingServiceUnavailable.WithCause(err)
	}
	w.IndependentAccountIDs = snapshot.IndependentAccountIDs
	return w.CheckRemaining()
}

type sharedQuotaContextKey struct{}

func WithSharedQuota(ctx context.Context, snapshot *SharedQuotaSnapshot) context.Context {
	return context.WithValue(ctx, sharedQuotaContextKey{}, snapshot)
}

func SharedQuotaFromContext(ctx context.Context) *SharedQuotaSnapshot {
	q, _ := ctx.Value(sharedQuotaContextKey{}).(*SharedQuotaSnapshot)
	return q
}

// BindSharedQuota 只修改当前请求副本，避免污染鉴权缓存和其他并发请求。
func (s *APIKeyService) BindSharedQuota(ctx context.Context, key *APIKey) (*APIKey, error) {
	if s == nil || s.sharedQuota == nil || key == nil || key.User == nil {
		return key, nil
	}
	w, err := s.sharedQuota.snapshotForGroup(ctx, key.User, valueOrZero(key.GroupID))
	if err != nil {
		return nil, err
	}
	if w != nil && w.GroupID > 0 {
		if source, ok := s.sharedQuota.repo.(interface {
			IndependentContributionAccounts(context.Context, int64) ([]int64, error)
		}); ok {
			w.IndependentAccountIDs, err = source.IndependentContributionAccounts(ctx, w.GroupID)
			if err != nil {
				return nil, ErrBillingServiceUnavailable.WithCause(err)
			}
		}
	}
	k := *key
	u := *key.User
	u.SharedQuota = w
	k.User = &u
	return &k, nil
}

func (s *UserService) ApplySharedQuotaView(ctx context.Context, user *User) error {
	if s == nil || user == nil {
		return nil
	}
	return s.sharedQuota.applyView(ctx, user)
}

func (s *AuthService) ApplySharedQuotaView(ctx context.Context, user *User) error {
	if s == nil {
		return nil
	}
	return s.sharedQuota.applyView(ctx, user)
}

func (s *SharedQuotaService) applyView(ctx context.Context, user *User) error {
	if s == nil || user == nil {
		return nil
	}
	if user.IsAdmin() {
		// 展示异常不能阻断管理员登录及修复插件配置。
		w, err := s.currentSnapshot(ctx, 0)
		user.SiteSharedQuota = w
		if err != nil {
			logger.LegacyPrintf("service.shared_quota", "读取管理员全站额度展示失败: %v", err)
		}
		return nil
	}
	w, err := s.Snapshot(ctx, user)
	if err != nil {
		return err
	}
	if w != nil {
		user.SharedQuota = w
		user.Balance = w.Remaining
		user.FrozenBalance = w.Held
		user.BalanceNotifyEnabled = false
	}
	return nil
}

// 查询服务端身份，避免由客户端决定管理员例外或共享计费开关。
func (s *SharedQuotaService) forUser(ctx context.Context, repo UserRepository, id int64) (*SharedQuotaSnapshot, error) {
	if s == nil {
		return nil, nil
	}
	user, err := repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.Snapshot(ctx, user)
}

func (s *UserService) SharedQuotaForUser(ctx context.Context, id int64) (*SharedQuotaSnapshot, error) {
	if s == nil {
		return nil, nil
	}
	return s.sharedQuota.forUser(ctx, s.userRepo, id)
}

func (s *APIKeyService) SharedQuotaForUser(ctx context.Context, id int64) (*SharedQuotaSnapshot, error) {
	if s == nil {
		return nil, nil
	}
	return s.sharedQuota.forUser(ctx, s.userRepo, id)
}

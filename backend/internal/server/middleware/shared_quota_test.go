//go:build unit

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type sharedAuthRepo struct {
	service.SharedQuotaRepository
	remaining      float64
	err            error
	groupExhausted bool
}

func (r *sharedAuthRepo) ActivePolicy(context.Context) (*service.SharedQuotaPolicy, error) {
	return &service.SharedQuotaPolicy{PluginKey: "test.shared", ConfigEncrypted: `{"daily_quota":100,"reset_time":"00:00"}`}, r.err
}
func (r *sharedAuthRepo) Window(context.Context, *service.SharedQuotaPolicy, time.Time, time.Time, time.Time, float64) (*service.SharedQuotaSnapshot, error) {
	q := &service.SharedQuotaSnapshot{WindowID: 1, Total: 100, Remaining: r.remaining}
	if r.groupExhausted {
		q.GroupQuota = &service.SharedGroupQuota{Limit: 10, Used: 10}
	}
	return q, r.err
}

type sharedAuthEncryptor struct{}

func (sharedAuthEncryptor) Encrypt(s string) (string, error) { return s, nil }
func (sharedAuthEncryptor) Decrypt(s string) (string, error) { return s, nil }

func TestSharedQuotaAuthBypassesOnlyPersonalBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, role, status, path, ip string
		remaining                    float64
		expired, offline             bool
		groupExhausted               bool
		want                         int
	}{
		{name: "普通用户忽略零余额和已耗尽Key", role: service.RoleUser, status: service.StatusAPIKeyQuotaExhausted, path: "/v1/messages", remaining: 100, want: 200},
		{name: "共享池耗尽", role: service.RoleUser, status: service.StatusActive, path: "/v1/messages", want: 429},
		{name: "分组上限耗尽不回落个人计费", role: service.RoleUser, status: service.StatusActive, path: "/v1/messages", remaining: 100, groupExhausted: true, want: 429},
		{name: "分组上限耗尽仍显示共享额度", role: service.RoleUser, status: service.StatusActive, path: "/v1/usage", remaining: 100, groupExhausted: true, want: 200},
		{name: "额度耗尽仍可查询", role: service.RoleUser, status: service.StatusActive, path: "/v1/usage", want: 200},
		{name: "额度耗尽仍可查询视频", role: service.RoleUser, status: service.StatusActive, path: "/v1/videos/task-1", want: 200},
		{name: "管理员仍受个人余额策略约束", role: service.RoleAdmin, status: service.StatusActive, path: "/v1/messages", remaining: 100, want: 403},
		{name: "禁用Key不能恢复", role: service.RoleUser, status: "inactive", path: "/v1/messages", remaining: 100, want: 401},
		{name: "到期Key不能恢复", role: service.RoleUser, status: service.StatusActive, path: "/v1/messages", remaining: 100, expired: true, want: 403},
		{name: "保留IP限制", role: service.RoleUser, status: service.StatusActive, path: "/v1/messages", remaining: 100, ip: "203.0.113.1", want: 403},
		{name: "数据库异常不得回落个人计费", role: service.RoleUser, status: service.StatusActive, path: "/v1/messages", remaining: 100, offline: true, want: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{RunMode: config.RunModeStandard, Timezone: "UTC"}
			key := &service.APIKey{ID: 10, UserID: 1, Key: "test-shared-key", Status: tc.status, Quota: 1, QuotaUsed: 1, RateLimit5h: 1, Usage5h: 99, User: &service.User{ID: 1, Role: tc.role, Status: service.StatusActive, Balance: 0}}
			if tc.role == service.RoleAdmin {
				key.Quota = 0
				key.RateLimit5h = 0
			}
			if tc.expired {
				x := time.Now().Add(-time.Hour)
				key.ExpiresAt = &x
			}
			if tc.ip != "" {
				key.IPWhitelist = []string{tc.ip}
			}
			repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { return key, nil }}
			quotaRepo := &sharedAuthRepo{remaining: tc.remaining, groupExhausted: tc.groupExhausted}
			if tc.offline {
				quotaRepo.err = errors.New("offline")
			}
			quota, err := service.NewSharedQuotaService(quotaRepo, sharedAuthEncryptor{}, cfg)
			require.NoError(t, err)
			svc := service.ProvideAPIKeyService(repo, nil, nil, nil, nil, nil, cfg, nil, nil, quota)
			router := gin.New()
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)))
			router.GET(tc.path, func(c *gin.Context) {
				bound, ok := GetAPIKeyFromContext(c)
				require.True(t, ok)
				if tc.role == service.RoleUser {
					require.NotNil(t, bound.User.SharedQuota)
				}
				c.Status(200)
			})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("x-api-key", key.Key)
			router.ServeHTTP(rec, req)
			require.Equal(t, tc.want, rec.Code, rec.Body.String())
			require.Nil(t, key.User.SharedQuota)
		})
	}
}

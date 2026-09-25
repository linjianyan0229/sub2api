package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// 共享策略在基础鉴权之后执行，过期凭证、IP 和分组授权仍然生效。
func sharedQuotaAuth(c *gin.Context, svc *service.APIKeyService, key *service.APIKey, readOnly, google bool) (*service.APIKey, bool) {
	bound, err := svc.BindSharedQuota(c.Request.Context(), key)
	status := http.StatusServiceUnavailable
	code := "BILLING_SERVICE_UNAVAILABLE"
	if err == nil && bound.User.SharedQuota == nil {
		return bound, false
	}
	if err == nil && bound.Group != nil && !bound.User.CanBindGroup(bound.Group.ID, bound.Group.IsExclusive) {
		status, code, err = http.StatusForbidden, "GROUP_NOT_ALLOWED", service.ErrGroupNotAllowed
	}
	if err == nil && !readOnly {
		if bound.Status == service.StatusAPIKeyExpired || bound.IsExpired() {
			status, code, err = http.StatusForbidden, "API_KEY_EXPIRED", service.ErrAPIKeyExpired
		} else if bound.User.SharedQuota.Remaining <= 0 {
			status, code, err = http.StatusTooManyRequests, "SHARED_QUOTA_EXHAUSTED", service.ErrSharedQuotaExhausted
		}
	}
	if err != nil {
		message := "共享额度暂时不可用"
		if errors.Is(err, service.ErrSharedQuotaExhausted) || errors.Is(err, service.ErrAPIKeyExpired) || errors.Is(err, service.ErrGroupNotAllowed) {
			message = err.Error()
		}
		if google {
			abortWithGoogleError(c, status, message)
		} else {
			AbortWithError(c, status, code, message)
		}
		return key, true
	}
	c.Request = c.Request.WithContext(service.WithSharedQuota(c.Request.Context(), bound.User.SharedQuota))
	c.Set(string(ContextKeyAPIKey), bound)
	c.Set(string(ContextKeyUser), AuthSubject{UserID: bound.User.ID, Concurrency: bound.User.Concurrency})
	c.Set(string(ContextKeyUserRole), bound.User.Role)
	setGroupContext(c, bound.Group)
	_ = svc.TouchLastUsed(c.Request.Context(), bound.ID)
	c.Next()
	return bound, true
}

func isAcceptedMediaTaskRequest(method, path string) bool {
	path = strings.TrimPrefix(path, "/v1")
	if method == http.MethodGet && (strings.HasPrefix(path, "/videos/") || strings.HasPrefix(path, "/images/batches")) {
		return true
	}
	if (method == http.MethodGet || method == http.MethodDelete) && (strings.HasPrefix(path, "/contents/generations/tasks/") || strings.HasPrefix(path, "/api/v3/contents/generations/tasks/")) {
		return true
	}
	return strings.HasPrefix(path, "/images/batches/") && (method == http.MethodDelete || (method == http.MethodPost && strings.HasSuffix(path, "/cancel")))
}

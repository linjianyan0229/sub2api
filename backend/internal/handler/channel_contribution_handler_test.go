package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type contributionHandlerRepo struct {
	service.ChannelContributionRepository
	actor int64
	calls int
}

func (r *contributionHandlerRepo) Submit(_ context.Context, actor int64, c *service.ChannelContribution) error {
	r.actor, r.calls = actor, r.calls+1
	c.Status = "pending"
	return nil
}

type contributionHandlerEncryptor struct{}

func (contributionHandlerEncryptor) Encrypt(s string) (string, error) { return "encrypted:" + s, nil }
func (contributionHandlerEncryptor) Decrypt(s string) (string, error) { return s, nil }

func TestChannelContributionHandlerIdentityAndSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &contributionHandlerRepo{}
	h := NewChannelContributionHandler(service.NewChannelContributionService(repo, contributionHandlerEncryptor{}, nil))
	body := `{"site_name":"示例站","base_url":"https://api.example.com","api_key":"sk-sensitive","new_group_name":"新组","user_id":999,"status":"approved","role":"admin"}`
	call := func(payload string, authenticated bool) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/channel-contributions", strings.NewReader(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		if authenticated {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		}
		h.Submit(c)
		return w
	}
	require.Equal(t, 401, call(body, false).Code)
	require.Zero(t, repo.calls)
	w := call(body, true)
	require.Equal(t, 200, w.Code)
	require.Equal(t, int64(42), repo.actor)
	require.Contains(t, w.Body.String(), `"status":"pending"`)
	require.NotContains(t, w.Body.String(), "sk-sensitive")
	require.NotContains(t, service.RedactAuditBody([]byte(body), "application/json"), "sk-sensitive")
	require.Equal(t, 400, call(`{"site_name":"`+strings.Repeat("x", 17000)+`"}`, true).Code)
	require.Equal(t, 1, repo.calls)
}

//go:build unit

package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type leaderboardRepoStub struct {
	service.UsageLogRepository
	items  []service.UsageLeaderboardItem
	err    error
	called bool
}

func (r *leaderboardRepoStub) GetUsageLeaderboard(context.Context) ([]service.UsageLeaderboardItem, error) {
	r.called = true
	return r.items, r.err
}

func TestUsageLeaderboardAuthenticationAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		auth   bool
		err    error
		status int
	}{
		{"anonymous", false, nil, http.StatusUnauthorized},
		{"authenticated", true, nil, http.StatusOK},
		{"repository_error", true, errors.New("private database detail"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &leaderboardRepoStub{err: tc.err, items: []service.UsageLeaderboardItem{
				{Username: "测试昵称", ActualCost: 12.5, Tokens: 456},
				{Username: "private@example.com", ActualCost: 3, Tokens: 123},
			}}
			h := NewUsageHandler(service.NewUsageService(repo, nil, nil, nil), nil, nil, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/usage/leaderboard", nil)
			if tc.auth {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
			}
			h.Leaderboard(c)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.auth, repo.called)
			require.NotContains(t, w.Body.String(), "private")
			require.NotContains(t, w.Body.String(), "email")
			require.NotContains(t, w.Body.String(), "user_id")
			if tc.status == http.StatusOK {
				require.Contains(t, w.Body.String(), `"username":"测试昵称"`)
				require.Contains(t, w.Body.String(), `"username":""`)
				require.Contains(t, w.Body.String(), `"actual_cost":12.5`)
				require.Contains(t, w.Body.String(), `"tokens":456`)
			}
		})
	}
}

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

type forumHandlerRepo struct {
	service.ForumRepository
	user    int64
	calls   int
	session string
}

func (r *forumHandlerRepo) Arrive(_ context.Context, user int64, session string) error {
	r.user = user
	r.session = session
	r.calls++
	return nil
}

func (r *forumHandlerRepo) Create(_ context.Context, user int64, content string) (*service.ForumMessage, error) {
	r.user = user
	r.calls++
	return &service.ForumMessage{Content: content}, nil
}

func TestForumArrivalUsesAuthenticatedSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &forumHandlerRepo{}
	h := NewForumHandler(service.NewForumService(repo))
	call := func(authenticated bool, session string) int {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/forum/arrival", strings.NewReader(`{"user_id":1,"session_id":"forged"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		if authenticated {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
			c.Set(middleware.ContextKeySessionID, session)
		}
		h.Arrive(c)
		return w.Code
	}
	require.Equal(t, 401, call(false, ""))
	require.Zero(t, repo.calls)
	require.Equal(t, 200, call(true, "verified-session"))
	require.Equal(t, int64(42), repo.user)
	require.Equal(t, "verified-session", repo.session)
	require.Equal(t, 1, repo.calls)
	require.Equal(t, 200, call(true, ""))
	require.Equal(t, 1, repo.calls)
}
func TestForumHandlerUsesAuthenticatedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &forumHandlerRepo{}
	h := NewForumHandler(service.NewForumService(repo))
	call := func(body string, authenticated bool, handler gin.HandlerFunc) int {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/forum/messages", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		if authenticated {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		}
		handler(c)
		return w.Code
	}
	require.Equal(t, 401, call(`{"content":"匿名"}`, false, h.Create))
	require.Zero(t, repo.calls)
	require.Equal(t, 200, call(`{"content":"你好","user_id":1,"rank":1,"actual_cost":999}`, true, h.Create))
	require.Equal(t, int64(42), repo.user)
	require.Equal(t, 1, repo.calls)
	require.Equal(t, 400, call(`{"content":"`+strings.Repeat("x", 20000)+`"}`, true, h.Create))
	require.Equal(t, 1, repo.calls)
	require.Equal(t, 400, call(`{"confirm":false}`, true, h.Clear))
}

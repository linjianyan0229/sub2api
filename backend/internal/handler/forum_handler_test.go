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
	message int64
	err     error
}

func (r *forumHandlerRepo) Recall(_ context.Context, user, message int64) error {
	r.user = user
	r.message = message
	r.calls++
	return r.err
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

func TestForumRecallHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name   string
		id     string
		auth   bool
		err    error
		status int
		calls  int
	}{
		{name: "unauthenticated", id: "9", status: http.StatusUnauthorized},
		{name: "invalid_id", id: "abc", auth: true, status: http.StatusBadRequest},
		{name: "zero_id", id: "0", auth: true, status: http.StatusBadRequest},
		{name: "negative_id", id: "-1", auth: true, status: http.StatusBadRequest},
		{name: "overflow_id", id: "9223372036854775808", auth: true, status: http.StatusBadRequest},
		{name: "authenticated_identity", id: "9", auth: true, status: http.StatusOK, calls: 1},
		{name: "not_owner", id: "9", auth: true, err: service.ErrForumRecallForbidden, status: http.StatusForbidden, calls: 1},
		{name: "expired", id: "9", auth: true, err: service.ErrForumRecallExpired, status: http.StatusForbidden, calls: 1},
		{name: "missing", id: "9", auth: true, err: service.ErrForumNotFound, status: http.StatusNotFound, calls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &forumHandlerRepo{err: tt.err}
			h := NewForumHandler(service.NewForumService(repo))
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodDelete, "/forum/messages/"+tt.id,
				strings.NewReader(`{"user_id":1,"is_admin":true,"created_at":"2099-01-01T00:00:00Z"}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "id", Value: tt.id}}
			if tt.auth {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
			}
			h.Recall(c)
			require.Equal(t, tt.status, w.Code)
			require.Equal(t, tt.calls, repo.calls)
			if tt.calls > 0 {
				require.Equal(t, int64(42), repo.user)
				require.Equal(t, int64(9), repo.message)
			}
		})
	}
}

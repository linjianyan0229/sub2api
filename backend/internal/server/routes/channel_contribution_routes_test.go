package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelContributionRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := &handler.Handlers{Admin: &handler.AdminHandlers{}}
	adminAuth := middleware.AdminAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.AbortWithStatus(http.StatusForbidden)
	})
	userAuth := middleware.JWTAuthMiddleware(func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	audit := middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	stepUp := middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(router.Group("/api/v1"), h, adminAuth, audit, stepUp, nil, nil)
	RegisterUserRoutes(router.Group("/api/v1"), h, userAuth, audit, nil, nil)
	for _, tc := range []struct {
		method, path, auth string
		want               int
	}{
		{"GET", "/api/v1/channel-contributions", "", 401},
		{"POST", "/api/v1/channel-contributions", "", 401},
		{"GET", "/api/v1/channel-contributions/groups", "", 401},
		{"GET", "/api/v1/admin/channel-contributions", "", 401},
		{"POST", "/api/v1/admin/channel-contributions/1/review", "", 401},
		{"GET", "/api/v1/admin/channel-contributions", "Bearer user", 403},
		{"POST", "/api/v1/admin/channel-contributions/1/review", "Bearer user", 403},
	} {
		t.Run(tc.method+tc.path+tc.auth, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			require.Equal(t, tc.want, w.Code)
		})
	}
}

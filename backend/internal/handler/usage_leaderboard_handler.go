package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func (h *UsageHandler) Leaderboard(c *gin.Context) {
	if _, ok := middleware.GetAuthSubjectFromContext(c); !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	items, err := h.usageService.GetUsageLeaderboard(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to get usage leaderboard")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	response.Success(c, gin.H{"ranking": items})
}

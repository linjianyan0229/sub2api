package handler

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ForumHandler struct{ service *service.ForumService }

func NewForumHandler(s *service.ForumService) *ForumHandler { return &ForumHandler{service: s} }
func forumActor(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "请先登录")
		return 0, false
	}
	return subject.UserID, true
}
func forumID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的记录 ID")
		return 0, false
	}
	return id, true
}
func (h *ForumHandler) Page(c *gin.Context) {
	actor, ok := forumActor(c)
	if !ok {
		return
	}
	before, err := strconv.ParseInt(c.DefaultQuery("before", "0"), 10, 64)
	if err != nil || before < 0 {
		response.BadRequest(c, "无效的分页游标")
		return
	}
	p, err := h.service.Page(c.Request.Context(), actor, before)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, p)
}
func (h *ForumHandler) Create(c *gin.Context) {
	actor, ok := forumActor(c)
	if !ok {
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "发言格式无效")
		return
	}
	m, err := h.service.Create(c.Request.Context(), actor, body.Content)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, m)
}
func (h *ForumHandler) Events(c *gin.Context) {
	if _, ok := forumActor(c); !ok {
		return
	}
	var after *int64
	if raw, ok := c.GetQuery("after"); ok {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			response.BadRequest(c, "无效的事件游标")
			return
		}
		after = &v
	}
	events, err := h.service.Events(c.Request.Context(), after)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, events)
}
func (h *ForumHandler) Settings(c *gin.Context) {
	actor, ok := forumActor(c)
	if !ok {
		return
	}
	var body struct {
		Minutes *int `json:"mute_minutes"`
		Days    *int `json:"retention_days"`
	}
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "设置格式无效")
		return
	}
	if err := h.service.UpdateSettings(c.Request.Context(), actor, body.Minutes, body.Days); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
func (h *ForumHandler) Mute(c *gin.Context) {
	actor, ok := forumActor(c)
	if !ok {
		return
	}
	id, ok := forumID(c)
	if !ok {
		return
	}
	var body struct {
		Minutes *int `json:"minutes"`
	}
	if c.ShouldBindJSON(&body) != nil || body.Minutes == nil {
		response.BadRequest(c, "禁言时长无效")
		return
	}
	if err := h.service.Mute(c.Request.Context(), actor, id, *body.Minutes); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
func (h *ForumHandler) Mutes(c *gin.Context) {
	actor, ok := forumActor(c)
	if !ok {
		return
	}
	items, err := h.service.Mutes(c.Request.Context(), actor)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}
func (h *ForumHandler) Pin(c *gin.Context) {
	actor, ok := forumActor(c)
	if !ok {
		return
	}
	id, ok := forumID(c)
	if !ok {
		return
	}
	var body struct {
		Pinned *bool `json:"pinned"`
	}
	if c.ShouldBindJSON(&body) != nil || body.Pinned == nil {
		response.BadRequest(c, "置顶设置无效")
		return
	}
	if err := h.service.Pin(c.Request.Context(), actor, id, *body.Pinned); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
func (h *ForumHandler) Clear(c *gin.Context) {
	actor, ok := forumActor(c)
	if !ok {
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if c.ShouldBindJSON(&body) != nil || !body.Confirm {
		response.BadRequest(c, "请确认清空全部聊天记录")
		return
	}
	if err := h.service.Clear(c.Request.Context(), actor); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

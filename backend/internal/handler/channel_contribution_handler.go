package handler

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ChannelContributionHandler struct {
	service *service.ChannelContributionService
}

func NewChannelContributionHandler(s *service.ChannelContributionService) *ChannelContributionHandler {
	return &ChannelContributionHandler{service: s}
}

func contributionActor(c *gin.Context) (int64, bool) {
	u, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "请先登录")
		return 0, false
	}
	return u.UserID, true
}

func (h *ChannelContributionHandler) Submit(c *gin.Context) {
	actor, ok := contributionActor(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
	var body service.ChannelContributionInput
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "申请格式无效")
		return
	}
	item, err := h.service.Submit(c.Request.Context(), actor, body)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *ChannelContributionHandler) Mine(c *gin.Context)      { h.list(c, false) }
func (h *ChannelContributionHandler) AdminList(c *gin.Context) { h.list(c, true) }
func (h *ChannelContributionHandler) list(c *gin.Context, admin bool) {
	actor, ok := contributionActor(c)
	if !ok {
		return
	}
	before, err := strconv.ParseInt(c.DefaultQuery("before", "0"), 10, 64)
	if err != nil || before < 0 {
		response.BadRequest(c, "无效的分页游标")
		return
	}
	items, err := h.service.List(c.Request.Context(), actor, admin, c.Query("status"), before)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	more := len(items) > 50
	if more {
		items = items[:50]
	}
	response.Success(c, gin.H{"items": items, "has_more": more})
}

func (h *ChannelContributionHandler) Groups(c *gin.Context) {
	actor, ok := contributionActor(c)
	if !ok {
		return
	}
	groups, err := h.service.Groups(c.Request.Context(), actor)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]gin.H, 0, len(groups))
	accounts, err := h.service.GroupAccounts(c.Request.Context(), groups)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	for _, g := range groups {
		out = append(out, gin.H{"id": g.ID, "name": g.Name, "platform": g.Platform, "accounts": accounts[g.ID]})
	}
	response.Success(c, out)
}

func (h *ChannelContributionHandler) Review(c *gin.Context) {
	actor, ok := contributionActor(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的申请 ID")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
	var body struct {
		Decision string    `json:"decision"`
		Note     string    `json:"note"`
		Models   *[]string `json:"models"`
	}
	if c.ShouldBindJSON(&body) != nil || (body.Decision != "approved" && body.Decision != "rejected") {
		response.BadRequest(c, "审核内容无效")
		return
	}
	if err := h.service.ReviewWithModels(c.Request.Context(), actor, id, body.Decision == "approved", body.Note, body.Models); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

func (h *ChannelContributionHandler) Models(c *gin.Context) {
	actor, ok := contributionActor(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的申请 ID")
		return
	}
	models, err := h.service.Models(c.Request.Context(), actor, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"models": models})
}

func (h *ChannelContributionHandler) Quotas(c *gin.Context) {
	h.catalog(c, false)
}

func (h *ChannelContributionHandler) Statuses(c *gin.Context) {
	h.catalog(c, true)
}

func (h *ChannelContributionHandler) catalog(c *gin.Context, statusPage bool) {
	actor, ok := contributionActor(c)
	if !ok {
		return
	}
	before, err := strconv.ParseInt(c.DefaultQuery("before", "0"), 10, 64)
	if err != nil || before < 0 {
		response.BadRequest(c, "无效的分页游标")
		return
	}
	var items []service.ContributionQuotaView
	var shared *service.SharedQuotaSnapshot
	if statusPage {
		items, err = h.service.Statuses(c.Request.Context(), actor, before)
	} else {
		items, shared, err = h.service.Quotas(c.Request.Context(), actor, before)
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	more := len(items) > 50
	if more {
		items = items[:50]
	}
	response.Success(c, gin.H{"items": items, "shared_quota": shared, "has_more": more})
}

func (h *ChannelContributionHandler) OwnedAccount(c *gin.Context) {
	h.ownedAccount(c, false)
}

func (h *ChannelContributionHandler) UpdateOwnedAccount(c *gin.Context) {
	h.ownedAccount(c, true)
}

func (h *ChannelContributionHandler) ownedAccount(c *gin.Context, update bool) {
	actor, ok := contributionActor(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的申请 ID")
		return
	}
	if update {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
		var body service.ContributionAccountInput
		if c.ShouldBindJSON(&body) != nil {
			response.BadRequest(c, "渠道配置无效")
			return
		}
		if err := h.service.UpdateOwnedAccount(c.Request.Context(), actor, id, body); err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Success(c, gin.H{"ok": true})
		return
	}
	item, err := h.service.OwnedAccount(c.Request.Context(), actor, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

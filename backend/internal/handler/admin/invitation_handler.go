package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func (h *RedeemHandler) UpdateInvitationLimit(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid redeem code ID")
		return
	}
	var req struct {
		MaxUses int `json:"max_uses" binding:"required,min=1,max=2147483647"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	code, err := h.redeemService.UpdateInvitationLimit(c.Request.Context(), id, req.MaxUses)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.RedeemCodeFromServiceAdmin(code))
}

func (h *RedeemHandler) ListInvitationUsages(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid redeem code ID")
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.redeemService.ListInvitationUsages(c.Request.Context(), id, pagination.PaginationParams{Page: page, PageSize: pageSize})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

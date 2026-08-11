package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func parseOptionalPositiveIntQuery(c *gin.Context, key string) *int {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &value
}

// GetAIPDDTransitOrders lists NewAPI-side AIPDD transit orders for admins.
func GetAIPDDTransitOrders(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	query := model.AIPDDTransitOrderQuery{
		StartTimestamp:  startTimestamp,
		EndTimestamp:    endTimestamp,
		PlatformOrderID: strings.TrimSpace(c.Query("platform_order_id")),
		UserID:          parseOptionalPositiveIntQuery(c, "user_id"),
		TokenID:         parseOptionalPositiveIntQuery(c, "token_id"),
		ChannelID:       parseOptionalPositiveIntQuery(c, "channel_id"),
		Model:           strings.TrimSpace(c.Query("model")),
		Status:          strings.TrimSpace(c.Query("status")),
		StartIdx:        pageInfo.GetStartIdx(),
		PageSize:        pageInfo.GetPageSize(),
	}

	items, total, err := model.GetAIPDDTransitOrders(query)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

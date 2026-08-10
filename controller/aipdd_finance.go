package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func AdminListAIPDDFinanceOrders(c *gin.Context) {
	page := positiveQueryInt(c, "page", 1)
	pageSize := min(500, positiveQueryInt(c, "page_size", 50))
	filter := aipddFinanceFilterFromQuery(c)
	orders, total, err := model.ListAIPDDFinanceOrders((page-1)*pageSize, pageSize, filter)
	if err != nil {
		aipddFinanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": orders, "total": total, "page": page, "page_size": pageSize})
}

func AdminGetAIPDDFinanceSummary(c *gin.Context) {
	summary, err := model.GetAIPDDFinanceSummary(aipddFinanceFilterFromQuery(c))
	if err != nil {
		aipddFinanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

func AdminGetAIPDDFinanceOrder(c *gin.Context) {
	detail, err := model.GetAIPDDFinanceOrderDetail(strings.TrimSpace(c.Param("id")))
	if err != nil {
		aipddFinanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": detail})
}

func AdminGetAIPDDFinanceSyncStatus(c *gin.Context) {
	statuses, err := model.ListAIPDDFinanceSyncStatus(strings.TrimSpace(c.Query("instance_id")))
	if err != nil {
		aipddFinanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": statuses})
}

func AdminRetryAIPDDFinanceSync(c *gin.Context) {
	var filter model.AIPDDFinanceOrderFilter
	if err := c.ShouldBindJSON(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid retry filter: " + err.Error()})
		return
	}
	if filter.PlatformOrderID == "" && filter.ChannelID <= 0 && filter.InstanceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "platform_order_id, channel_id or instance_id is required"})
		return
	}
	queued, err := model.QueueAIPDDFinanceManualRetry(filter)
	if err != nil {
		aipddFinanceError(c, err)
		return
	}
	service.WakeAIPDDFinanceReconciliation()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"queued": queued}})
}

func AdminCloseAIPDDFinanceOutbox(c *gin.Context) {
	var body struct {
		State  string `json:"state"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid close request: " + err.Error()})
		return
	}
	state := strings.ToUpper(strings.TrimSpace(body.State))
	if state == "" {
		state = model.AIPDDFinanceOutboxIgnored
	}
	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		reason = "manually closed by admin"
	}
	if err := model.CloseAIPDDFinanceOutbox(strings.TrimSpace(c.Param("id")), state, reason); err != nil {
		aipddFinanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func AdminCloseOrphanAIPDDFinanceOutbox(c *gin.Context) {
	closed, err := model.SweepOrphanAIPDDFinanceOutbox()
	if err != nil {
		aipddFinanceError(c, err)
		return
	}
	service.WakeAIPDDFinanceReconciliation()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"closed": closed}})
}

func AdminSkipAIPDDFinancePoisonEvent(c *gin.Context) {
	var body struct {
		ChannelID  int    `json:"channel_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid skip request: " + err.Error()})
		return
	}
	if body.ChannelID <= 0 || strings.TrimSpace(body.InstanceID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "channel_id and instance_id are required"})
		return
	}
	if err := model.SkipAIPDDFinancePoisonEvent(body.ChannelID, strings.TrimSpace(body.InstanceID)); err != nil {
		aipddFinanceError(c, err)
		return
	}
	service.WakeAIPDDFinanceReconciliation()
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func AdminCreateAIPDDFinanceExport(c *gin.Context) {
	var filter model.AIPDDFinanceOrderFilter
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&filter); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid export filter: " + err.Error()})
			return
		}
	}
	job, err := model.CreateAIPDDFinanceExportJob(filter, c.GetInt("id"))
	if err != nil {
		aipddFinanceError(c, err)
		return
	}
	service.StartAIPDDFinanceExport(job.ID)
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": job})
}

func AdminListAIPDDFinanceExports(c *gin.Context) {
	jobs, err := model.ListAIPDDFinanceExportJobs(positiveQueryInt(c, "limit", 20))
	if err != nil {
		aipddFinanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": jobs})
}

func AdminGetAIPDDFinanceExport(c *gin.Context) {
	job, err := model.GetAIPDDFinanceExportJob(strings.TrimSpace(c.Param("id")))
	if err != nil {
		aipddFinanceError(c, err)
		return
	}
	job.FileData = nil
	c.JSON(http.StatusOK, gin.H{"success": true, "data": job})
}

func AdminDownloadAIPDDFinanceExport(c *gin.Context) {
	job, err := model.GetAIPDDFinanceExportJob(strings.TrimSpace(c.Param("id")))
	if err != nil {
		aipddFinanceError(c, err)
		return
	}
	if job.Status != model.AIPDDFinanceExportReady || len(job.FileData) == 0 {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "export is not ready"})
		return
	}
	if job.ExpiresAt > 0 && job.ExpiresAt < time.Now().Unix() {
		c.JSON(http.StatusGone, gin.H{"success": false, "message": "export has expired"})
		return
	}
	fileName := strings.ReplaceAll(job.FileName, `"`, "")
	c.Header("Content-Disposition", `attachment; filename="`+fileName+`"`)
	c.Header("X-Content-SHA256", job.SHA256)
	c.Data(http.StatusOK, job.ContentType, job.FileData)
}

func aipddFinanceFilterFromQuery(c *gin.Context) model.AIPDDFinanceOrderFilter {
	return model.AIPDDFinanceOrderFilter{
		UserID: positiveQueryInt(c, "user_id", 0), TokenID: positiveQueryInt(c, "token_id", 0),
		ChannelID: positiveQueryInt(c, "channel_id", 0), UserQuery: strings.TrimSpace(c.Query("user")),
		TokenQuery: strings.TrimSpace(c.Query("token")), InstanceID: strings.TrimSpace(c.Query("instance_id")),
		Model: strings.TrimSpace(c.Query("model")), PlatformOrderID: strings.TrimSpace(c.Query("platform_order_id")),
		OrderStatus: strings.TrimSpace(c.Query("order_status")), LocalBillingStatus: strings.TrimSpace(c.Query("local_billing_status")),
		CostStatus: strings.TrimSpace(c.Query("cost_status")), IssueView: strings.TrimSpace(c.Query("issue_view")),
		StartTime: queryInt64(c, "start_time"), EndTime: queryInt64(c, "end_time"),
	}
}

func aipddFinanceError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"success": false, "message": err.Error()})
}

func positiveQueryInt(c *gin.Context, key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(c.Query(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func queryInt64(c *gin.Context, key string) int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(c.Query(key)), 10, 64)
	if value < 0 {
		return 0
	}
	return value
}

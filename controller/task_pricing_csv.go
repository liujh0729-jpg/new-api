package controller

import (
	"fmt"
	"mime"
	"net/http"
	"reflect"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func DownloadTaskPricingCSVTemplate(c *gin.Context) {
	writeTaskPricingCSVDownload(c, "task-pricing-template.csv", billing_setting.EmptyTaskPricingCSVTemplate())
}

func ExportTaskPricingCSV(c *gin.Context) {
	options := readTaskPricingCSVOptions()
	rate, err := billing_setting.ParseUSDExchangeRate(options["USDExchangeRate"])
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	configs, err := parseCurrentTaskPricing(options["billing_setting.task_pricing"])
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	data, err := billing_setting.ExportTaskPricingCSV(configs, rate)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	writeTaskPricingCSVDownload(c, "task-pricing-export.csv", data)
}

func PreviewTaskPricingCSV(c *gin.Context) {
	plan, err := buildTaskPricingCSVPlanFromUpload(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    plan,
	})
}

func ImportTaskPricingCSV(c *gin.Context) {
	plan, err := buildTaskPricingCSVPlanFromUpload(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := validateTaskPricingCSVPlan(plan); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	applied := make([]billing_setting.TaskPricingCSVOptionUpdate, 0, len(plan.Updates))
	for _, item := range plan.Updates {
		if err := model.UpdateOption(item.Key, item.Value); err != nil {
			rollbackErrors := rollbackTaskPricingCSV(plan.Rollback)
			detail := "；已回滚"
			if len(rollbackErrors) > 0 {
				detail = "；回滚失败：" + strings.Join(rollbackErrors, " | ")
			}
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("导入失败：写入 %s：%v%s", item.Key, err, detail),
			})
			return
		}
		applied = append(applied, item)
	}

	if err := verifyTaskPricingCSVPlan(plan); err != nil {
		rollbackErrors := rollbackTaskPricingCSV(plan.Rollback)
		detail := "；已回滚"
		if len(rollbackErrors) > 0 {
			detail = "；回滚失败：" + strings.Join(rollbackErrors, " | ")
		}
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("导入失败：%v%s", err, detail),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"summary":        plan.Summary,
			"updated_keys":   optionUpdateKeys(applied),
			"models_updated": plan.Summary.Models,
		},
	})
}

func buildTaskPricingCSVPlanFromUpload(c *gin.Context) (*billing_setting.TaskPricingCSVPlan, error) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("请上传 CSV 文件（字段名 file）")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("无法打开上传文件：%v", err)
	}
	defer file.Close()

	data, err := billing_setting.ReadTaskPricingCSVLimited(file)
	if err != nil {
		return nil, err
	}
	rows, err := billing_setting.ParseRetailPricingCSV(data)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("CSV 没有可导入的数据行")
	}

	options := readTaskPricingCSVOptions()
	rate, err := resolveTaskPricingCSVExchangeRate(c, options)
	if err != nil {
		return nil, err
	}
	imported, summary, err := billing_setting.BuildTaskPricingFromRows(rows, rate)
	if err != nil {
		return nil, err
	}
	summary.RMBPerUSD = rate.String()
	return billing_setting.BuildTaskPricingImportPlan(options, imported, summary)
}

func resolveTaskPricingCSVExchangeRate(c *gin.Context, options map[string]string) (decimal.Decimal, error) {
	if override := strings.TrimSpace(c.PostForm("rmb_per_usd")); override != "" {
		return billing_setting.ParseUSDExchangeRate(override)
	}
	return billing_setting.ParseUSDExchangeRate(options["USDExchangeRate"])
}

func readTaskPricingCSVOptions() map[string]string {
	keys := []string{
		"billing_setting.task_pricing",
		"billing_setting.billing_mode",
		"GroupRatio",
		"UserUsableGroups",
		"USDExchangeRate",
	}
	result := make(map[string]string, len(keys))
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	for _, key := range keys {
		if common.OptionMap == nil {
			result[key] = ""
			continue
		}
		result[key] = common.Interface2String(common.OptionMap[key])
	}
	return result
}

func parseCurrentTaskPricing(raw string) (map[string]billing_setting.TaskPricingConfig, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]billing_setting.TaskPricingConfig{}, nil
	}
	configs, err := billing_setting.ParseTaskPricingMapJSON(trimmed)
	if err != nil {
		return nil, fmt.Errorf("当前 billing_setting.task_pricing 无效：%v", err)
	}
	return configs, nil
}

func validateTaskPricingCSVPlan(plan *billing_setting.TaskPricingCSVPlan) error {
	for _, item := range plan.Updates {
		switch item.Key {
		case "billing_setting.task_pricing":
			if _, err := billing_setting.ParseTaskPricingMapJSON(item.Value); err != nil {
				return fmt.Errorf("按视频时长售价设置失败: %v", err)
			}
		case "GroupRatio":
			if err := ratio_setting.CheckGroupRatio(item.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyTaskPricingCSVPlan(plan *billing_setting.TaskPricingCSVPlan) error {
	actual := readTaskPricingCSVOptions()
	for _, item := range plan.Updates {
		observed := strings.TrimSpace(actual[item.Key])
		expected := strings.TrimSpace(item.Value)
		if observed == expected {
			continue
		}
		// Compare as JSON objects when possible to ignore key ordering.
		if jsonMapsEqual(observed, expected) {
			continue
		}
		return fmt.Errorf("写入后校验失败：%s 与计划不一致", item.Key)
	}
	return nil
}

func jsonMapsEqual(left, right string) bool {
	var leftValue any
	var rightValue any
	if err := common.UnmarshalJsonStr(left, &leftValue); err != nil {
		return false
	}
	if err := common.UnmarshalJsonStr(right, &rightValue); err != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func rollbackTaskPricingCSV(rollback []billing_setting.TaskPricingCSVOptionUpdate) []string {
	errors := make([]string, 0)
	for _, item := range rollback {
		if err := model.UpdateOption(item.Key, item.Value); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", item.Key, err))
		}
	}
	return errors
}

func optionUpdateKeys(updates []billing_setting.TaskPricingCSVOptionUpdate) []string {
	keys := make([]string, 0, len(updates))
	for _, item := range updates {
		keys = append(keys, item.Key)
	}
	return keys
}

func writeTaskPricingCSVDownload(c *gin.Context, filename string, data []byte) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": filename,
	}))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", data)
}

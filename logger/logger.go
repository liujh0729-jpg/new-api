package logger

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const (
	loggerINFO  = "INFO"
	loggerWarn  = "WARN"
	loggerError = "ERR"
	loggerDebug = "DEBUG"
)

const maxLogCount = 1000000

// defaultLogFileCount is how many oneapi-*.log files the log directory keeps
// when LOG_MAX_FILES is unset.
const defaultLogFileCount = 30

// logFileRetention reads the operator's retention choice. Zero or a negative
// value turns pruning off, because deleting historical logs is the one part of
// rotation that cannot be undone.
func logFileRetention() int {
	return common.GetEnvOrDefault("LOG_MAX_FILES", defaultLogFileCount)
}

var logCount int
var setupLogLock sync.Mutex
var setupLogWorking bool
var currentLogPath string
var currentLogPathMu sync.RWMutex
var currentLogFile *os.File

func GetCurrentLogPath() string {
	currentLogPathMu.RLock()
	defer currentLogPathMu.RUnlock()
	return currentLogPath
}

func SetupLogger() {
	defer func() {
		setupLogWorking = false
	}()
	if *common.LogDir != "" {
		ok := setupLogLock.TryLock()
		if !ok {
			log.Println("setup log is already working")
			return
		}
		defer func() {
			setupLogLock.Unlock()
		}()
		logPath := filepath.Join(*common.LogDir, fmt.Sprintf("oneapi-%s.log", time.Now().Format("20060102150405")))
		fd, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal("failed to open log file")
		}
		currentLogPathMu.Lock()
		oldFile := currentLogFile
		currentLogPath = logPath
		currentLogFile = fd
		currentLogPathMu.Unlock()

		common.LogWriterMu.Lock()
		gin.DefaultWriter = io.MultiWriter(os.Stdout, fd)
		gin.DefaultErrorWriter = io.MultiWriter(os.Stderr, fd)
		if oldFile != nil {
			_ = oldFile.Close()
		}
		common.LogWriterMu.Unlock()
		removeOldLogFiles(*common.LogDir, logPath)
	}
}

// removeOldLogFiles deletes the oldest oneapi-*.log files in dir until at most
// maxLogFiles remain. The file currently being written is always kept.
func removeOldLogFiles(dir string, keepPath string) {
	maxLogFiles := logFileRetention()
	if maxLogFiles <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("failed to read log dir %s: %s", dir, err.Error())
		}
		return
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "oneapi-") && strings.HasSuffix(name, ".log") {
			names = append(names, name)
		}
	}
	// file names embed a timestamp, so lexical order is chronological order
	sort.Strings(names)
	toRemove := len(names) - maxLogFiles
	if toRemove <= 0 {
		return
	}
	removed := 0
	for _, name := range names {
		if removed >= toRemove {
			break
		}
		if filepath.Join(dir, name) == keepPath {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			log.Printf("failed to remove old log file %s: %s", name, err.Error())
			continue
		}
		removed++
	}
	if removed > 0 {
		log.Printf("removed %d old log file(s), keeping the newest %d", removed, maxLogFiles)
	}
}

func LogInfo(ctx context.Context, msg string) {
	logHelper(ctx, loggerINFO, msg)
}

func LogWarn(ctx context.Context, msg string) {
	logHelper(ctx, loggerWarn, msg)
}

func LogError(ctx context.Context, msg string) {
	logHelper(ctx, loggerError, msg)
}

func LogDebug(ctx context.Context, msg string, args ...any) {
	if common.DebugEnabled {
		if len(args) > 0 {
			msg = fmt.Sprintf(msg, args...)
		}
		logHelper(ctx, loggerDebug, msg)
	}
}

func logHelper(ctx context.Context, level string, msg string) {
	id := ctx.Value(common.RequestIdKey)
	if id == nil {
		id = "SYSTEM"
	}
	now := time.Now()
	common.LogWriterMu.RLock()
	writer := gin.DefaultErrorWriter
	if level == loggerINFO {
		writer = gin.DefaultWriter
	}
	_, _ = fmt.Fprintf(writer, "[%s] %v | %s | %s \n", level, now.Format("2006/01/02 - 15:04:05"), id, msg)
	common.LogWriterMu.RUnlock()
	logCount++ // we don't need accurate count, so no lock here
	if logCount > maxLogCount && !setupLogWorking {
		logCount = 0
		setupLogWorking = true
		gopool.Go(func() {
			SetupLogger()
		})
	}
}

func LogQuota(quota int) string {
	// 新逻辑：根据额度展示类型输出
	q := float64(quota)
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		usd := q / common.QuotaPerUnit
		cny := usd * operation_setting.USDExchangeRate
		return fmt.Sprintf("¥%.6f 额度", cny)
	case operation_setting.QuotaDisplayTypeCustom:
		usd := q / common.QuotaPerUnit
		rate := operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
		symbol := operation_setting.GetGeneralSetting().CustomCurrencySymbol
		if symbol == "" {
			symbol = "¤"
		}
		if rate <= 0 {
			rate = 1
		}
		v := usd * rate
		return fmt.Sprintf("%s%.6f 额度", symbol, v)
	case operation_setting.QuotaDisplayTypeTokens:
		return fmt.Sprintf("%d 点额度", quota)
	default: // USD
		return fmt.Sprintf("＄%.6f 额度", q/common.QuotaPerUnit)
	}
}

func FormatQuota(quota int) string {
	q := float64(quota)
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		usd := q / common.QuotaPerUnit
		cny := usd * operation_setting.USDExchangeRate
		return fmt.Sprintf("¥%.6f", cny)
	case operation_setting.QuotaDisplayTypeCustom:
		usd := q / common.QuotaPerUnit
		rate := operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
		symbol := operation_setting.GetGeneralSetting().CustomCurrencySymbol
		if symbol == "" {
			symbol = "¤"
		}
		if rate <= 0 {
			rate = 1
		}
		v := usd * rate
		return fmt.Sprintf("%s%.6f", symbol, v)
	case operation_setting.QuotaDisplayTypeTokens:
		return fmt.Sprintf("%d", quota)
	default:
		return fmt.Sprintf("＄%.6f", q/common.QuotaPerUnit)
	}
}

// LogJson 仅供测试使用 only for test
func LogJson(ctx context.Context, msg string, obj any) {
	jsonStr, err := common.Marshal(obj)
	if err != nil {
		LogError(ctx, fmt.Sprintf("json marshal failed: %s", err.Error()))
		return
	}
	LogDebug(ctx, fmt.Sprintf("%s | %s", msg, string(jsonStr)))
}

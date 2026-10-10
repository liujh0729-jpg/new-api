package main

import (
	"fmt"
	"os"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")
	common.InitEnv()
	logger.SetupLogger()
	ratio_setting.InitRatioSettings()

	if err := model.InitDB(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "init database failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := model.CloseDB(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "close database failed: %v\n", err)
		}
	}()

	if err := model.EnsureAIPDDDefaults(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "sync AIPDD defaults failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("AIPDD channel and model defaults synced")
}

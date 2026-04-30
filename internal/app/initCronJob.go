package app

import (
	"context"
	"encoding/json"
	"fmt"
	"nozomi/internal/logger"
	"nozomi/tools"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) InitCronJob() {
	ctx := context.Background()

	var filePath []string
	cronPath := filepath.Join(a.Config.WorkDir, "cron")
	files, err := os.ReadDir(cronPath)
	if err != nil {
		a.Logger.Log("error", fmt.Sprintf("Unable to load %s.", cronPath), logger.Options{})
		return
	}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".json") {
			path := filepath.Join(cronPath, file.Name())
			filePath = append(filePath, path)
		}
	}

	for _, file := range filePath {
		bytes, err := os.ReadFile(file)
		if err != nil {
			a.Logger.Log("info", fmt.Sprintf("Unable to load %s.", file), logger.Options{})
			continue
		}

		cronTask := tools.CronTask{}
		err = json.Unmarshal(bytes, &cronTask)
		if err != nil {
			a.Logger.Log("info", fmt.Sprintf("Unable to unmarshal %s.", file), logger.Options{})
			continue
		}
		cronID, err := a.Router.HandleCronJobRegister(cronTask)
		if err != nil {
			str := fmt.Sprintf("Init cron job(%s) failed.", cronTask.UUID)
			a.Logger.Log("info", str, logger.Options{})
			a.Matrix.SendText(ctx, cronTask.RoomID, str)
			_ = a.Matrix.SendToLogRoom(ctx, str)
			continue
		}
		tools.CronID.Store(cronID, cronTask)
	}
}

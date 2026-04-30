package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
	"maunium.net/go/mautrix/id"
)

type SearchQuota struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

type TimeLog struct {
	Time time.Time `json:"Time"`
}

func isExist(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// 创建所有必要的基础设施
func setupEnv(workdir string) {
	logsPath := filepath.Join(workdir, "logs")
	if ok, _ := isExist(logsPath); !ok {
		err := os.Mkdir(logsPath, 0777)
		if err != nil {
			log.Fatalf("Failed to creating logs path: %v", err)
		}
		log.Println("Create log path sucessfully.")
	}

	configPath := filepath.Join(workdir, "config.yaml")
	if ok, _ := isExist(configPath); !ok {
		log.Println("config.yaml does not exist, creating...")
		data := BotConfig{
			Client: ClientConfig{
				LogRoom:               []string{},
				MaxMemoryLength:       14,
				WhenRetroRemainMemLen: 6,
				DisplayName:           "Nozomi",
				DatabasePassword:      "123456",
			},
			Model: ModelConfig{
				Model:            "gemini-3.1-flash-lite-preview",
				PrefixToCall:     "!c",
				MaxOutputToken:   3000,
				AlargmTokenCount: 4000,
				UseInternet:      true,
				SecureCheck:      false,
				MaxMonthlySearch: 4000,
				TimeOutWhen:      30 * time.Second,
				IncludeThoughts:  true,
				ThinkingBudget:   0,
				ThinkingLevel:    "high",
				Rate:             0.20,
				RateBurst:        1,
			},
			Auth: AuthConfig{
				AdminID: []id.UserID{},
			},
		}
		yamlData, err := yaml.Marshal(&data)
		if err != nil {
			log.Fatalf("Marshal default config.yaml failed: %v", err)
		}
		err = os.WriteFile(configPath, yamlData, 0644)
		if err != nil {
			log.Fatalf("Failed to write default data into config.yaml: %v", err)
		}
		log.Println("Default config.yaml has been sucessfully created. Pls complete it and run bot again.")
		os.Exit(1)
	}

	soulPath := filepath.Join(workdir, "soul.md")
	if ok, _ := isExist(soulPath); !ok {
		log.Println("soul.md does not exist, creating...")
		_, err := os.Create(soulPath)
		if err != nil {
			log.Fatalf("Auto creating soul.md failed: %v", err)
		}
		log.Println("Create soul.md sucessfully.")
	}

	databasePath := filepath.Join(workdir, "database")
	if ok, _ := isExist(databasePath); !ok {
		log.Println("databasePath does not exist, auto creating at " + databasePath)
		err := os.Mkdir(databasePath, 0777)
		if err != nil {
			log.Fatalf("Auto creating databasePath failed: %v", err)
		}
		log.Println("Create database path sucessfully.")
	}

	dataPath := filepath.Join(workdir, "data")
	if ok, _ := isExist(dataPath); !ok {
		log.Println("dataPath does not exist, auto creating at " + dataPath)
		err := os.Mkdir(dataPath, 0777)
		if err != nil {
			log.Fatalf("Auto creating dataPath failed: %v", err)
		}
		log.Println("Create data path sucessfully.")
	}

	cronPath := filepath.Join(workdir, "cron")
	if ok, _ := isExist(cronPath); !ok {
		log.Println("cronPath does not exist, auto creating at " + cronPath)
		err := os.Mkdir(cronPath, 0777)
		if err != nil {
			log.Fatalf("Auto creating cronPath failed: %v", err)
		}
		log.Println("Create cron path sucessfully.")
	}

	quotaPath := filepath.Join(workdir, "data", "search_quota.json")
	if ok, _ := isExist(quotaPath); !ok {
		log.Println("search_quota.json does not exist, creating...")
		defaultQuota := SearchQuota{
			Month: "1970-01",
			Count: 0,
		}
		quotaBytes, _ := json.MarshalIndent(defaultQuota, "", "\t")
		err := os.WriteFile(quotaPath, quotaBytes, 0644)
		if err != nil {
			log.Fatalf("Auto creating search_quota.json failed: %v", err)
		}
		log.Println("Create search_quota.json sucessfully.")
	}

	tokenUsagePath := filepath.Join(workdir, "data", "token_usage.json")
	if ok, _ := isExist(tokenUsagePath); !ok {
		log.Println("token_usage.json does not exist, creating...")
		bytes := []byte("{\n\t\"Day\": {\n\t\t\"Input\": 0,\n\t\t\"Output\": 0,\n\t\t\"Think\": 0\n\t},\n\t\"Month\": {\n\t\t\"Input\": 0,\n\t\t\"Output\": 0,\n\t\t\"Think\": 0\n\t},\n\t\"Year\": {\n\t\t\"Input\": 0,\n\t\t\"Output\": 0,\n\t\t\"Think\": 0\n\t}\n}")
		err := os.WriteFile(tokenUsagePath, bytes, 0644)
		if err != nil {
			log.Fatalf("Auto creating token_usage.json failed: %v", err)
		}
		log.Println("Create token_usage.json sucessfully.")
	}

	timePath := filepath.Join(workdir, "data", "time.json")
	if ok, _ := isExist(timePath); !ok {
		log.Println("time.json does not exist, creating...")
		defaultTime := TimeLog{Time: time.Now()}
		bytes, _ := json.MarshalIndent(defaultTime, "", "\t")
		err := os.WriteFile(timePath, bytes, 0644)
		if err != nil {
			log.Fatalf("Auto creating time.json failed: %v", err)
		}
		log.Println("Create time.json sucessfully.")
		log.Println("All config file has created. Pls check no file is empty.")
		os.Exit(0)
	}
}

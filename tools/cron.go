package tools

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"google.golang.org/genai"
	"maunium.net/go/mautrix/id"
)

var CronEngine *cron.Cron

// CronID 存目前正在运行中的定时任务 cron.EntryID -> CronTask
var CronID sync.Map

// CronTask 定时任务结构体
type CronTask struct {
	UUID       string    `json:"uuid"`
	Sender     id.UserID `json:"sender"`
	RoomID     id.RoomID `json:"roomID"`
	CronExpr   string    `json:"cronExpr"`
	TaskPrompt string    `json:"taskPrompt"`
}

func init() {
	CronEngine = cron.New(cron.WithSeconds()) // 秒级解析
	CronEngine.Start()
}

var CronJobTool = &genai.Tool{
	FunctionDeclarations: []*genai.FunctionDeclaration{
		{
			Name:        "add_cron_job",
			Description: "Add a cron job. You must provide a strict 6-field cron expression including seconds.",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"cron_expression": {
						Type:        genai.TypeString,
						Description: "Standard Cron expression (including seconds, 6 digits in total).",
					},
					"task_prompt": {
						Type:        genai.TypeString,
						Description: "The prompt message given when the event is scheduled to trigger. For example: 'Query today's news and send it to the group', or 'Execute df -h to check the server disk status'.",
					},
				},
				Required: []string{"cron_expression", "task_prompt"},
			},
		},
	},
}

var CronJobList = &genai.Tool{
	FunctionDeclarations: []*genai.FunctionDeclaration{
		{
			Name:        "list_cron_job",
			Description: "Find registered cron jobs. Provide a UUID to get a specific job, or leave empty to list all jobs.",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"uuid": {
						Type:        genai.TypeString,
						Description: "Optional. The UUID of a specific cron job to query.",
					},
				},
			},
		},
	},
}

var RemoveCronJob = &genai.Tool{
	FunctionDeclarations: []*genai.FunctionDeclaration{
		{
			Name:        "remove_cron_job",
			Description: "Remove a cron job with specified uuid.",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"uuid": {
						Type:        genai.TypeString,
						Description: "The UUID of a specific cron job to remove.",
					},
				},
				Required: []string{"uuid"},
			},
		},
	},
}

// 新建一个待确认的定时任务
func AddCronJob(roomID id.RoomID, sender id.UserID, cronExpr string, taskPrompt string) CronTask {
	uuid := uuid.New().String()
	cronTask := CronTask{
		UUID:       uuid,
		Sender:     sender,
		RoomID:     roomID,
		CronExpr:   cronExpr,
		TaskPrompt: taskPrompt,
	}
	return cronTask
}

// 删除不存在房间的定时任务，返回该房间所有定时任务的 uuid
func RemoveCronJobFromInactiveRoom(activeRooms []string) []string {
	var uuid []string
	CronID.Range(func(key, value any) bool {
		entryID := key.(cron.EntryID)
		task := value.(CronTask)
		if !slices.Contains(activeRooms, string(task.RoomID)) {
			RemoveCronJobWithEntryID(entryID)
			uuid = append(uuid, task.UUID)
		}
		return true
	})
	return uuid
}

// 清除特定房间的定时任务，返回该房间所有定时任务的 uuid
func RemoveCronJobWithRoomID(roomID id.RoomID) []string {
	var uuid []string
	CronID.Range(func(key, value any) bool {
		entryID := key.(cron.EntryID)
		task := value.(CronTask)
		if task.RoomID == roomID {
			RemoveCronJobWithEntryID(entryID)
			uuid = append(uuid, task.UUID)
		}
		return true
	})
	return uuid
}

func RemoveCronJobWithUUID(uuid string) {
	CronID.Range(func(key, value any) bool {
		entryID := key.(cron.EntryID)
		task := value.(CronTask)
		if task.UUID == uuid {
			RemoveCronJobWithEntryID(entryID)
			return false
		}
		return true
	})
}

func RemoveCronJobWithEntryID(cronID cron.EntryID) {
	CronEngine.Remove(cronID)
	CronID.Delete(cronID)
}

func ListCronJob(uuid string) string {
	var result strings.Builder

	if uuid != "" {
		// 查找特定的单个任务
		var found bool
		CronID.Range(func(key, value any) bool {
			task := value.(CronTask)
			if task.UUID == uuid {
				result.WriteString(fmt.Sprintf("UUID: %s\nCron: %s\nPrompt: %s\n", task.UUID, task.CronExpr, task.TaskPrompt))
				found = true
				return false
			}
			return true
		})

		if !found {
			return fmt.Sprintf("[System Info: Cron job with UUID '%s' not found.]", uuid)
		}
	} else {
		// 遍历并列出所有任务
		var count int
		CronID.Range(func(key, value any) bool {
			task := value.(CronTask)
			result.WriteString(fmt.Sprintf("- [%s] %s : %s\n", task.UUID, task.CronExpr, task.TaskPrompt))
			count++
			return true
		})

		// 检查列表是否为空
		if count == 0 {
			return "[System Info: No cron jobs registered currently.]"
		}

		// 在列表顶部追加总数统计，方便大模型理解上下文
		return fmt.Sprintf("Total registered jobs: %d\n%s", count, result.String())
	}

	return result.String()
}

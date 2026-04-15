package handler

import (
	"fmt"
	"sync"
	"time"

	"ampmanager/internal/config"
	"ampmanager/internal/database"

	"github.com/google/uuid"
)

type databaseTask struct {
	Error      string     `json:"error,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	ID         string     `json:"id"`
	Logs       []string   `json:"logs"`
	Message    string     `json:"message"`
	Operation  string     `json:"operation"`
	Progress   int        `json:"progress"`
	StartedAt  time.Time  `json:"startedAt"`
	Status     string     `json:"status"`
	SourceType string     `json:"sourceType"`
	TargetType string     `json:"targetType"`
}

type databaseTaskManager struct {
	activeTaskID string
	mu           sync.RWMutex
	tasks        map[string]*databaseTask
}

var globalDatabaseTaskManager = &databaseTaskManager{tasks: make(map[string]*databaseTask)}

func (m *databaseTaskManager) createTask(operation string, sourceType, targetType database.DBType) (*databaseTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeTaskID != "" {
		if activeTask, exists := m.tasks[m.activeTaskID]; exists && (activeTask.Status == "pending" || activeTask.Status == "running") {
			return nil, fmt.Errorf("已有数据库任务正在执行中")
		}
	}

	task := &databaseTask{
		ID:         uuid.New().String(),
		Logs:       []string{},
		Message:    "任务已创建",
		Operation:  operation,
		Progress:   0,
		StartedAt:  time.Now().UTC(),
		Status:     "pending",
		SourceType: string(sourceType),
		TargetType: string(targetType),
	}
	m.tasks[task.ID] = task
	m.activeTaskID = task.ID
	return cloneDatabaseTask(task), nil
}

func (m *databaseTaskManager) updateTask(taskID string, updater func(task *databaseTask)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if task, exists := m.tasks[taskID]; exists {
		updater(task)
	}
}

func (m *databaseTaskManager) finishTask(taskID, status, errMessage string) {
	finishedAt := time.Now().UTC()
	m.updateTask(taskID, func(task *databaseTask) {
		task.Status = status
		task.Error = errMessage
		task.FinishedAt = &finishedAt
		if status == "succeeded" {
			task.Progress = 100
			task.Message = "任务完成"
		}
	})

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeTaskID == taskID {
		m.activeTaskID = ""
	}
}

func (m *databaseTaskManager) getTask(taskID string) (*databaseTask, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, exists := m.tasks[taskID]
	if !exists {
		return nil, false
	}
	return cloneDatabaseTask(task), true
}

func cloneDatabaseTask(task *databaseTask) *databaseTask {
	if task == nil {
		return nil
	}
	copyTask := *task
	copyTask.Logs = append([]string{}, task.Logs...)
	return &copyTask
}

func appendTaskLog(task *databaseTask, line string) {
	if line == "" {
		return
	}
	task.Logs = append(task.Logs, line)
	if len(task.Logs) > 120 {
		task.Logs = append([]string{}, task.Logs[len(task.Logs)-120:]...)
	}
}

func switchRuntimeDatabase(options database.Options) error {
	if err := database.InitWithOptions(options); err != nil {
		return err
	}

	if err := reinitDatabaseBackedRuntimeServices(); err != nil {
		return err
	}

	if cfg := config.Get(); cfg != nil {
		cfg.DBType = string(options.Type)
		cfg.DatabaseURL = options.DatabaseURL
		cfg.SQLitePath = options.SQLitePath
	}

	if err := config.SaveRuntimeDatabaseOptions(options); err != nil {
		return err
	}

	return nil
}

func runDatabaseMigrationTask(taskID string, sourceOptions, targetOptions database.Options, clearTarget, withArchive bool) {
	globalDatabaseTaskManager.updateTask(taskID, func(task *databaseTask) {
		task.Status = "running"
		task.Message = "开始迁移数据库"
		appendTaskLog(task, "开始迁移数据库")
	})

	err := database.MigrateBetweenDatabases(database.MigrationParams{
		ClearTarget: clearTarget,
		OnProgress: func(progress database.MigrationProgress) {
			globalDatabaseTaskManager.updateTask(taskID, func(task *databaseTask) {
				task.Progress = progress.Progress
				task.Message = progress.Message
				appendTaskLog(task, fmt.Sprintf("[%d%%] %s", progress.Progress, progress.Message))
			})
		},
		Source:      sourceOptions,
		Target:      targetOptions,
		WithArchive: withArchive,
	})
	if err != nil {
		globalDatabaseTaskManager.finishTask(taskID, "failed", err.Error())
		return
	}

	globalDatabaseTaskManager.updateTask(taskID, func(task *databaseTask) {
		task.Progress = 97
		task.Message = "迁移完成，正在切换运行数据库"
		appendTaskLog(task, "正在切换当前运行实例到目标数据库")
	})

	if err := switchRuntimeDatabase(targetOptions); err != nil {
		globalDatabaseTaskManager.finishTask(taskID, "failed", err.Error())
		return
	}

	globalDatabaseTaskManager.finishTask(taskID, "succeeded", "")
	globalDatabaseTaskManager.updateTask(taskID, func(task *databaseTask) {
		appendTaskLog(task, "数据库切换完成；如果重启服务，请同步环境变量或启动脚本配置")
	})
}

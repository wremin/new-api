package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/QuantumNous/new-api/dto"
)

var (
	asyncImageTasks sync.Map
	asyncImageTTL   = 30 * time.Minute
)

// AsyncImageTask 异步图像生成任务
type AsyncImageTask struct {
	ID        string             `json:"id"`
	Status    string             `json:"status"` // pending, processing, success, failed
	Model     string             `json:"model"`
	Prompt    string             `json:"prompt"`
	Result    *dto.ImageResponse `json:"result,omitempty"`
	Error     string             `json:"error,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

func getInternalAPIURL() string {
	if url := os.Getenv("INTERNAL_API_URL"); url != "" {
		return url
	}
	return "http://localhost:3000"
}

// SubmitAsyncImageTask 提交异步图像生成任务
func SubmitAsyncImageTask(c *gin.Context) {
	var req dto.ImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task := &AsyncImageTask{
		ID:        "async-img-" + uuid.New().String(),
		Status:    "pending",
		Model:     req.Model,
		Prompt:    req.Prompt,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	asyncImageTasks.Store(task.ID, task)

	authHeader := c.GetHeader("Authorization")
	go processAsyncImageTask(task, req, authHeader)

	c.JSON(http.StatusAccepted, gin.H{
		"id":     task.ID,
		"status": task.Status,
	})
}

// processAsyncImageTask 后台执行图像生成
func processAsyncImageTask(task *AsyncImageTask, req dto.ImageRequest, authHeader string) {
	task.Status = "processing"
	task.UpdatedAt = time.Now()
	asyncImageTasks.Store(task.ID, task)

	body, err := json.Marshal(req)
	if err != nil {
		markTaskFailed(task, err.Error())
		return
	}

	httpReq, err := http.NewRequest("POST", getInternalAPIURL()+"/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		markTaskFailed(task, err.Error())
		return
	}
	httpReq.Header.Set("Authorization", authHeader)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		markTaskFailed(task, err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		markTaskFailed(task, err.Error())
		return
	}

	if resp.StatusCode != http.StatusOK {
		markTaskFailed(task, string(respBody))
		return
	}

	var imageResp dto.ImageResponse
	if err := json.Unmarshal(respBody, &imageResp); err != nil {
		markTaskFailed(task, err.Error())
		return
	}

	task.Status = "success"
	task.Result = &imageResp
	task.UpdatedAt = time.Now()
	asyncImageTasks.Store(task.ID, task)

	// TTL 清理
	go func() {
		time.Sleep(asyncImageTTL)
		asyncImageTasks.Delete(task.ID)
	}()
}

func markTaskFailed(task *AsyncImageTask, errMsg string) {
	task.Status = "failed"
	task.Error = errMsg
	task.UpdatedAt = time.Now()
	asyncImageTasks.Store(task.ID, task)

	go func() {
		time.Sleep(asyncImageTTL)
		asyncImageTasks.Delete(task.ID)
	}()
}

// FetchAsyncImageTask 查询异步图像生成任务状态
func FetchAsyncImageTask(c *gin.Context) {
	taskID := c.Param("task_id")
	val, ok := asyncImageTasks.Load(taskID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	task := val.(*AsyncImageTask)
	c.JSON(http.StatusOK, task)
}

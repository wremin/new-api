package relay

import (
	"net/http"
	"testing"
)

// TestIsSuccessStatusCode 锁定任务提交的成功判定范围。
//
// 这是一个线上真实故障：seegen.ai 提交视频生成任务返回 201 Created，
// 而提交流程原先只认 200，于是把一个已经在上游成功创建的任务判成失败——
// 下游收到 fail_to_fetch_task、本地不落任务记录，上游那边照常出片、无人认领。
func TestIsSuccessStatusCode(t *testing.T) {
	success := []int{
		http.StatusOK,        // 200，多数上游
		http.StatusCreated,   // 201，seegen.ai 提交任务
		http.StatusAccepted,  // 202，异步接口常见
		http.StatusNoContent, // 204
		299,                  // 2xx 上边界
	}
	for _, code := range success {
		if !isSuccessStatusCode(code) {
			t.Errorf("isSuccessStatusCode(%d) = false, want true", code)
		}
	}

	failure := []int{
		100,
		199,
		http.StatusMultipleChoices, // 300
		http.StatusMovedPermanently,
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusNotFound,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
	}
	for _, code := range failure {
		if isSuccessStatusCode(code) {
			t.Errorf("isSuccessStatusCode(%d) = true, want false", code)
		}
	}
}

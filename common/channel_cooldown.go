package common

import (
	"regexp"
	"strconv"
	"sync"
	"time"
)

// Channel 429 cooldown registry / 渠道 429 冷却登记表
//
// 当某个渠道返回 429（速率限制）时，将其临时移出调度池一段时间（优先使用上游
// 报错中的 "retry after N seconds"，否则使用默认冷却时长）。渠道选择时会跳过
// 冷却中的渠道；若同一优先级下所有候选渠道都在冷却，则回退为不过滤（宁可尝试
// 也不直接失败）。
//
// 注意：登记表保存在进程内存中。多实例部署时各实例独立冷却，仍然有效，
// 只是每个实例需要各自"踩一次"才会避让。

var (
	channelCooldownMu    sync.RWMutex
	channelCooldownUntil = make(map[int]time.Time)
)

// ChannelCooldownEnabled 与 ChannelCooldownSeconds 在 init.go 中由环境变量赋值：
// CHANNEL_COOLDOWN_ENABLED（默认 true）、CHANNEL_COOLDOWN_SECONDS（默认 60）
var ChannelCooldownEnabled = true
var ChannelCooldownSeconds = 60

var retryAfterRegex = regexp.MustCompile(`(?i)(?:retry after|try again in)\s+(\d+)\s*second`)

// ParseRetryAfterSeconds 从上游错误信息中提取 "retry after N seconds" 的 N。
// 解析失败时返回 0。
func ParseRetryAfterSeconds(message string) int {
	matches := retryAfterRegex.FindStringSubmatch(message)
	if len(matches) < 2 {
		return 0
	}
	seconds, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return seconds
}

// MarkChannelCooldown 将渠道标记为冷却，直到 now+seconds。
// seconds <= 0 时使用 ChannelCooldownSeconds；上限 3600 秒。
func MarkChannelCooldown(channelId int, seconds int) {
	if !ChannelCooldownEnabled || channelId <= 0 {
		return
	}
	if seconds <= 0 {
		seconds = ChannelCooldownSeconds
	}
	if seconds <= 0 {
		return
	}
	if seconds > 3600 {
		seconds = 3600
	}
	until := time.Now().Add(time.Duration(seconds) * time.Second)
	channelCooldownMu.Lock()
	defer channelCooldownMu.Unlock()
	if existing, ok := channelCooldownUntil[channelId]; !ok || until.After(existing) {
		channelCooldownUntil[channelId] = until
	}
	// 顺手清理已过期条目，避免长期运行下 map 无限增长
	if len(channelCooldownUntil) > 256 {
		now := time.Now()
		for id, t := range channelCooldownUntil {
			if now.After(t) {
				delete(channelCooldownUntil, id)
			}
		}
	}
}

// IsChannelCooling 返回渠道当前是否处于冷却期。
func IsChannelCooling(channelId int) bool {
	if !ChannelCooldownEnabled {
		return false
	}
	channelCooldownMu.RLock()
	until, ok := channelCooldownUntil[channelId]
	channelCooldownMu.RUnlock()
	if !ok {
		return false
	}
	return time.Now().Before(until)
}

// ChannelCooldownRemaining 返回剩余冷却秒数（未冷却返回 0），便于日志输出。
func ChannelCooldownRemaining(channelId int) int {
	channelCooldownMu.RLock()
	until, ok := channelCooldownUntil[channelId]
	channelCooldownMu.RUnlock()
	if !ok {
		return 0
	}
	remain := int(time.Until(until).Seconds())
	if remain < 0 {
		return 0
	}
	return remain
}

package client

import (
	"fmt"
	"strings"
	"time"

	l10n "github.com/Yeah114/Fatalder/utils/localization"
)

// ProgressBar 简单的进度条实现
type ProgressBar struct {
	total      int64
	current    int64
	width      int
	startTime  time.Time
	lastUpdate time.Time
}

// NewProgressBar 创建新的进度条
func NewProgressBar(total int64) *ProgressBar {
	return &ProgressBar{
		total:     total,
		width:     50,
		startTime: time.Now(),
	}
}

// Update 更新进度条
func (pb *ProgressBar) Update(current int64) {
	pb.current = current
	now := time.Now()

	// 限制更新频率，避免过于频繁的输出
	if now.Sub(pb.lastUpdate) < 100*time.Millisecond && current < pb.total {
		return
	}
	pb.lastUpdate = now

	pb.render()
}

// render 渲染进度条
func (pb *ProgressBar) render() {
	percent := float64(pb.current) / float64(pb.total)
	if percent > 1 {
		percent = 1
	}

	filled := int(percent * float64(pb.width))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", pb.width-filled)

	// 计算速度和剩余时间
	elapsed := time.Since(pb.startTime)
	var speed, eta string

	if elapsed.Seconds() > 0 {
		bytesPerSec := float64(pb.current) / elapsed.Seconds()
		speed = formatBytes(int64(bytesPerSec)) + "/s"

		if bytesPerSec > 0 && pb.current > 0 {
			remaining := float64(pb.total-pb.current) / bytesPerSec
			eta = formatDuration(time.Duration(remaining) * time.Second)
		} else {
			eta = l10n.T(l10n.KeyAutoUpdateProgressUnknown)
		}
	}

	// 输出进度条
	fmt.Printf("\r%s: [%s] %.1f%% (%s/%s) %s ETA: %s",
		l10n.T(l10n.KeyAutoUpdateProgressTitle),
		bar,
		percent*100,
		formatBytes(pb.current),
		formatBytes(pb.total),
		speed,
		eta,
	)

	if pb.current >= pb.total {
		fmt.Println() // 完成时换行
	}
}

// formatBytes 格式化字节数
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// formatDuration 格式化时间间隔
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm%.0fs", d.Minutes(), d.Seconds())
	}
	return fmt.Sprintf("%.0fh%.0fm", d.Hours(), d.Minutes())
}

// CreateProgressCallback 创建进度回调函数
func CreateProgressCallback() ProgressCallback {
	var pb *ProgressBar

	return func(downloaded, total int64, percent float64) {
		if pb == nil {
			pb = NewProgressBar(total)
		}
		pb.Update(downloaded)
	}
}

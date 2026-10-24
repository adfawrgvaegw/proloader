//go:build ignore
// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	// 设置中国时区
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err == nil {
		time.Local = loc
	}

	// 1. 获取当前 commit 哈希（短格式）
	hashCmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	hashOutput, err := hashCmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取 commit 哈希失败: %v\n", err)
		os.Exit(1)
	}
	cleanHash := strings.TrimSpace(string(hashOutput))
	if err := os.WriteFile("commit_hash", []byte(cleanHash), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写入 commit_hash 文件失败: %v\n", err)
		os.Exit(1)
	}

	// 2. 获取提交次数
	numCmd := exec.Command("git", "rev-list", "--count", "HEAD")
	numOutput, err := numCmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取提交次数失败: %v\n", err)
		os.Exit(1)
	}
	cleanNum := strings.TrimSpace(string(numOutput))
	if err := os.WriteFile("commit_num", []byte(cleanNum), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写入 commit_num 文件失败: %v\n", err)
		os.Exit(1)
	}

	// 3. 生成构建日期（格式：YYYY-MM-DD HH:MM:SS Monday）
	buildTime := time.Now().Format("2006-01-02 15:04:05 Monday") // Go 固定时间格式模板
	if err := os.WriteFile("build_date", []byte(buildTime), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写入 build_date 文件失败: %v\n", err)
		os.Exit(1)
	}

	// 4. 生成git log信息
	generateGitLogInfo()
}

// CommitInfo 提交信息结构体
type CommitInfo struct {
	Hash        string `json:"hash"`
	Author      string `json:"author"`
	Date        string `json:"date"`
	Description string `json:"description"`
}

// generateGitLogInfo 生成git log信息
func generateGitLogInfo() {
	// 获取所有commit信息，格式：hash|author|date|subject
	logCmd := exec.Command("git", "log", "--pretty=format:%H|%an|%ad|%s", "--date=format:%Y-%m-%d %H:%M:%S")
	logOutput, err := logCmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取git log失败: %v\n", err)
		os.Exit(1)
	}

	lines := strings.Split(strings.TrimSpace(string(logOutput)), "\n")
	var commits []CommitInfo

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			continue
		}

		commit := CommitInfo{
			Hash:        parts[0],
			Author:      parts[1],
			Date:        parts[2],
			Description: parts[3],
		}
		commits = append(commits, commit)
	}

	// 将提交信息序列化为JSON并写入文件
	jsonData, err := json.Marshal(commits)
	if err != nil {
		fmt.Fprintf(os.Stderr, "序列化git log失败: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile("commit_log.json", jsonData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写入commit_log.json文件失败: %v\n", err)
		os.Exit(1)
	}
}

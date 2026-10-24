package client

import (
	"time"
)

// UpdateClient 自动更新客户端
type UpdateClient struct {
	ServerURL   string        // 更新服务器地址（此字段将在GitHub模式下作为基础URL，例如 https://github.com 或镜像）
	CurrentFile string        // 当前可执行文件路径
	Timeout     time.Duration // 请求超时时间
	UserAgent   string        // 用户代理
	Prefix      string        // 文件名前缀
}

// UpdateInfo 更新信息
type UpdateInfo struct {
	HasUpdate    bool   // 是否有更新
	RemoteSHA512 string // 远程文件SHA512
	LocalSHA512  string // 本地文件SHA512
	DownloadURL  string // 下载地址（GitHub Releases 下载直链）
	Filename     string // 文件名
}

// ProgressCallback 进度回调函数类型
type ProgressCallback func(downloaded, total int64, percent float64)

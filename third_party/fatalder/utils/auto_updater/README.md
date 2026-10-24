# Fatalder 自动更新系统

这是 Fatalder 的自动更新系统，包含服务器端和客户端组件。

## 架构概述

- **服务器端** (`server/`): Python FastAPI 服务，监控 Git 仓库变更，下载构建产物并提供更新接口
- **客户端** (`client/`): Go 包，集成到主程序中，自动检查和应用更新

## 服务器端 (Python)

### 功能
- 使用 `pygit2` 监控本地 Git 仓库的提交变更
- 使用 `PyGithub` 查找对应的 GitHub Actions workflow runs
- 自动下载成功构建的 artifacts
- 解压并计算文件 SHA-512 哈希
- 提供 HTTP API 接口供客户端查询和下载

### API 接口
- `GET /sha-512/{filename}` - 获取文件的 SHA-512 哈希值
- `GET /downloads/{filename}` - 下载指定文件

### 运行服务器
```bash
cd utils/auto_updater/server
pip install fastapi uvicorn pygit2 PyGithub requests rich
python main.py
```

服务器将在 `http://127.0.0.1:8888` 上运行。

## 客户端 (Go)

### 功能
- 自动检查更新（对比本地和远程文件的 SHA-512）
- 带进度条的文件下载
- 自动替换可执行文件
- 跨平台程序重启（Windows/Linux/macOS，Android 不支持重启）

### 集成方式

客户端已自动集成到 `cmd/args` 包中，在程序启动时会自动执行更新检查。

### 控制自动更新

#### 全局控制变量
```go
// 禁用自动更新
client.DisableAutoUpdate()

// 启用自动更新
client.EnableAutoUpdateFunc()

// 检查当前状态
if client.EnableAutoUpdate {
    // 自动更新已启用
}
```

#### 配置选项
```go
config := &client.AutoUpdateConfig{
    ServerURL: "http://127.0.0.1:8888", // 更新服务器地址
    Silent:    false,                    // 是否静默模式
    Prefix:    "Fatalder-cli",           // 文件名前缀
}
```

### 手动调用更新
```go
import "github.com/Yeah114/Fatalder/utils/auto_updater/client"

// 执行完整的自动更新流程
err := client.PerformAutoUpdate(config)

// 仅检查更新，不下载
updateInfo, err := client.CheckUpdateOnly(config)
```

## 工作流程

1. **监控阶段**: 服务器每秒检查 Git 仓库是否有新提交
2. **构建等待**: 发现新提交后，等待对应的 GitHub Actions workflow 完成
3. **下载处理**: 构建成功后下载 artifacts，解压并计算 SHA-512
4. **客户端检查**: 客户端启动时自动对比本地和远程文件哈希
5. **自动更新**: 发现差异时自动下载、替换文件并重启程序

## 安全特性

- **SHA-512 校验**: 确保下载文件的完整性
- **备份机制**: 替换文件前自动备份原文件
- **错误恢复**: 替换失败时自动恢复备份
- **静默处理**: 更新失败不会影响程序正常启动

## 平台支持

| 平台 | 更新下载 | 文件替换 | 自动重启 |
|------|----------|----------|----------|
| Windows | ✅ | ✅ | ✅ (批处理脚本) |
| Linux | ✅ | ✅ | ✅ (Shell 脚本) |
| macOS | ✅ | ✅ | ✅ (Shell 脚本) |
| Android | ✅ | ✅ | ❌ (手动重启) |

## 配置说明

### 默认配置
- 更新服务器: `http://127.0.0.1:8888`
- 检查间隔: 程序启动时检查
- 模式: 静默模式（后台执行）

### 文件映射
程序会根据运行平台自动选择对应的文件：
- Windows: `Fatalder-cli_windows_amd64.exe`
- Linux: `Fatalder-cli_linux_amd64`
- Android: `Fatalder-cli_android_aarch64`

## 注意事项

1. 服务器需要在 Git 仓库目录内运行
2. 需要配置 GitHub Personal Access Token (在 `constants.py` 中)
3. 客户端更新是强制的，用户无法在界面中关闭
4. 只能通过修改 `client.EnableAutoUpdate` 全局变量来控制
5. 更新过程中程序会自动退出并重启

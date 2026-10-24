# Fatalder

[![License](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.24.5-blue.svg)](https://golang.org/)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-green.svg)]()

**Fatalder** 是一个强大的 Minecraft 自动化建筑工具，支持建筑结构的导入导出、自动构建等功能。

## ✨ 主要特性-

- 🏗️ **自动建筑构建** - 支持从 `.schematic` 文件自动构建建筑
- 📥 **导入导出功能** - 支持多种格式的建筑文件导入导出
- 🌐 **多语言支持** - 支持中文、英文、日文界面
- ⚙️ **配置管理** - 灵活的服务器配置和任务配置管理
- 🚀 **高性能** - 支持高速建筑构建和批量处理
- 🎯 **精确控制** - 支持坐标精确定位和建筑验证
- 🔧 **模块化设计** - 可扩展的插件式架构

## 🚀 快速开始

### 安装要求

- Go 1.24.5 或更高版本
- 支持的操作系统：Windows、Linux、macOS

### 构建项目

```bash
# 克隆仓库
git clone https://github.com/Yeah114/Fatalder.git
cd Fatalder

# 安装依赖
go mod download

# 构建项目
go build -o fatalder main.go

# 构建命令行版本（含自动更新）
go build -o fatalder-cli ./cmd/cli

# 构建图形界面版本（基于 go-astilectron）
go build -o fatalder-gui ./cmd/gui
```

### 基本使用

#### 1. 启动交互式界面

```bash
./fatalder-cli   # 终端交互式 CLI（启动时会执行自动更新检查）

# 或

./fatalder-gui   # 图形界面（Electron，首次启动会自动下载运行时）
```

#### 2. 命令行参数

```bash
# 显示帮助信息
./fatalder-cli --help

# 设置自定义存储目录
./fatalder-cli --storage-path /path/to/data

# 设置界面语言
./fatalder-cli --language zh

# 显示版本信息
./fatalder-cli --version

# GUI 同样支持参数（复用 cmd/args）
./fatalder-gui --storage-path /path/to/data --language zh
```

#### 3. 编程式使用

```go
package main

import (
    "github.com/Yeah114/Fatalder/control"
    "github.com/Yeah114/Fatalder/control/task"
    "github.com/Yeah114/Fatalder/defines"
)

func main() {
    // 创建控制器
    ctrl := control.NewControl(defines.ControlConfig{
        AuthServerAddress:    "https://your-auth-server.com",
        AuthServerToken:      "your-token",
        RentalServerCode:     "server-code",
        RentalServerPasscode: "server-password",
    })

    // 连接服务器
    if err := ctrl.EnterRentalServer(); err != nil {
        panic(err)
    }

    // 创建建筑任务
    buildTask := task.BuildTask{
        TaskType:         task.TaskTypeBuild,
        FilePath:         "your-building.schematic",
        StartPos:         defines.BlockPos{X: 100, Y: 64, Z: 100},
        Speed:            5000,
        AutoCleanBlock:   true,
        AutoCleanItem:    true,
        VerifyAfterChunk: 1,
        VerifyChunkLevel: 1,
    }

    // 执行任务
    ctrl.AddTask(&buildTask)
    ctx := context.Background()
    if err := ctrl.RunAllTask(ctx, true); err != nil {
        panic(err)
    }
}
```

## 📚 功能模块

### 🏗️ 建筑系统
- **自动构建** - 支持从 schematic 文件自动构建
- **坐标定位** - 精确的三维坐标定位系统
- **构建验证** - 多级别的构建结果验证
- **速度控制** - 可调节的构建速度

### 📁 文件支持
- **Schematic 格式** - 支持标准 Minecraft schematic 文件
- **NBT 解析** - 完整的 NBT 数据解析和处理
- **地图艺术** - 支持地图艺术的生成和导入

### 🔧 配置管理
- **服务器配置** - 灵活的服务器连接配置
- **任务配置** - 详细的任务参数配置
- **用户偏好** - 个性化的用户设置

### 🌐 国际化
- **多语言界面** - 支持中文、英文、日文
- **动态切换** - 运行时语言切换
- **易于扩展** - 简单的语言包添加机制

## 📖 详细文档

### 配置文件格式

服务器配置示例：
```json
{
    "server_number": "12345",
    "server_password": "password",
    "auth_server_address": "https://auth.example.com",
    "auth_server_token": "your-token-here"
}
```

### 任务类型

- **BuildTask** - 建筑构建任务
- **CommandTask** - 命令执行任务
- **SleepTask** - 延时任务

### 验证级别

- **0 - 无验证** - 不进行构建验证
- **1 - 空气验证** - 仅验证空气方块
- **2 - 相似验证** - 验证相似方块类型
- **3 - 完全验证** - 完全匹配验证

## 🛠️ 开发

### 项目结构

```
Fatalder/
├── bot/                    # 机器人核心功能
│   ├── core/              # 核心逻辑
│   ├── game_control/      # 游戏控制
│   ├── mapping/           # 地图相关
│   └── std_server/        # 标准服务器
├── cmd/                    # 可执行子命令
│   ├── args/              # 参数与运行环境初始化（无自动更新）
│   ├── cli/               # 终端交互式界面（包含自动更新）
│   └── gui/               # 图形界面（Electron，无自动更新）
├── control/               # 控制系统
├── defines/               # 类型定义
├── localization/          # 国际化
├── modules/               # 外部模块
│   ├── WaterStructure/    # 结构处理
│   └── blocks/            # 方块定义
├── test/                  # 测试文件
└── utils/                 # 工具函数

### 自动更新说明

- CLI（`cmd/cli`）在启动时会执行自动更新，使用 `utils/auto_updater/client`，并从框架配置中读取 GitHub 基础 URL（可配置镜像）。
- 参数与环境包（`cmd/args`）不再包含自动更新逻辑，避免对其他子命令的启动造成影响。
- GUI（`cmd/gui`）当前暂不包含自动更新，每个 `cmd/...` 可独立实现自己的更新策略。
```

### 贡献指南

1. Fork 本仓库
2. 创建功能分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

### 代码规范

- 遵循 Go 官方代码规范
- 添加适当的注释
- 编写单元测试
- 保持代码简洁易读

## 📄 许可证

本项目基于 [GNU Affero General Public License v3.0](LICENSE) 许可证开源。

## 👥 作者

- **Yeah114** - *主要开发者* - [GitHub](https://github.com/Yeah114)

## 🤝 致谢

感谢所有为本项目做出贡献的开发者和用户。

特别感谢以下开源项目：
- [gophertunnel](https://github.com/sandertv/gophertunnel) - Minecraft 网络协议支持
- [gin](https://github.com/gin-gonic/gin) - HTTP 服务框架
- [pterm](https://github.com/pterm/pterm) - 终端 UI 库

## 📞 支持

如果你遇到问题或有任何建议，请：

1. 查看 [Issues](https://github.com/Yeah114/Fatalder/issues)
2. 创建新的 Issue
3. 参与讨论

---

**注意**: 本工具仅供学习和研究使用，请遵守相关服务器的使用条款和规则。

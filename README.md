# Proloader

[![Offline checks](https://github.com/adfawrgvaegw/proloader/actions/workflows/ci.yml/badge.svg)](https://github.com/adfawrgvaegw/proloader/actions/workflows/ci.yml)

网易《我的世界》基岩版租赁服建筑导入器，供协议学习和自己有权限的建筑施工使用。
Python管理账号、连接与任务；施工规划直接调用仓库内的Fatalder原版Go代码。

## 开始使用

```sh
git clone https://github.com/adfawrgvaegw/proloader.git
cd proloader
```

在源码目录中双击 **proloader启动器.command**，或运行：

```sh
python3 -m proloader
```

菜单有两个入口：

- **管理机器人**：手机号登录或导入已有Cookie文件，验证身份、移除本机身份。
- **开始导入**：选择机器人、服务器、建筑、起点与维度，选择校验和速度，核对覆盖范围后开始。

机器人进服后先用只读命令检查命令权限；未确认时保持在线等待，并显示机器人名称。
将它设为操作员（OP）后，检查通过就自动开始导入；等待期间可按 Ctrl+C 暂停。

本机需要 Python 3.9+、Go 1.25.5+（含原生编译器工具链）；首次构建需要网络下载 Go 模块。
建议在项目独立环境安装：`python3 -m venv .venv`，然后 `source .venv/bin/activate`。启动器会检查实际依赖，
缺依赖时创建项目`.venv`安装。首次规划或源码变化时会重新构建原生助手。
手动安装可运行：

```sh
python3 -m pip install -e .
python3 -m proloader doctor
```

项目当前按**源码checkout**运行，尚未完成独立wheel的原生资源打包。

## 命令行

无参数打开菜单；带参数的`import`使用真实租赁服。旧模拟工具在`simulate`下。
以下位置、机器人与服务器均为示例，应换成你已授权的测试范围。

```sh
# 查看账号与历史任务
python3 -m proloader bots
python3 -m proloader tasks

# 离线读取建筑、生成计划，不登录服务器
python3 -m proloader plan path/to/building.schem --to=0,80,0

# 真实导入：显示计划并确认，密码隐藏输入
python3 -m proloader import path/to/building.schem \
  --server "$SERVER_CODE" --bot "建筑助手" --to=0,80,0

# 继续兼容版本的中断任务
python3 -m proloader resume "$TASK_ID"

# 查看某个任务或输出JSON
python3 -m proloader tasks "$TASK_ID" --json
```

负坐标可写成`--to=-85,78,351`。默认主世界，可用
`--dimension nether`或`--dimension the_end`选择维度。

发送配置默认`--rate 4000`，平滑突发最多10条，避免等待后瞬间灌入大量命令。
实际吞吐还受服务器、传送、ACK背压与世界校验影响，配置速率不代表实际落地速度。
菜单可选4000、2000、1000或自定义1～5000；命令行也可以指定`--rate`。
关闭方块校验时，每256条异步发送处理探针，最多768条调度窗口，达到窗口上限才等待；换区/结束前收齐探针。探针超时记录原批次并降速至250，不把它当作已落地证明。真实连接错误保存断点并按原策略恢复。
开启抽样/逐块校验仍保留每32条的方块检查，速度较慢；手动repair使用流水发送。位置确认通过后的固定等待从1秒缩短为0.25秒。

`.mcworld` 可读取主世界已保存区块的全部主层非空气方块，自动裁掉上下空白高度，
保留新版方块名称和states；使用文件映射避免展开巨型JSON输入。世界解压副本和索引保存在本机cache。
目前不搬运实体、普通方块实体NBT、次层液体和其他维度；命令方块内部数据已接入下述实验性暂存流程。
也不清除目标位置对应空气处的旧方块。
预检会显示尺寸、高度限制与未导入的数据，不能把它当作完整世界克隆。

### 命令方块内部数据（离线验证，实服待验收）

`.mcstructure` 和 `.mcworld` 中的基岩版命令方块可规划内部指令、名称、模式、条件、
输出跟踪与延迟；方块朝向随主层放置，并在数据写入时严格检查。
含命令方块的导入默认停止，菜单须明确选择处理方式，命令行使用：

```sh
# 暂存内部指令并读回核对；需要红石、关闭首刻执行
python3 -m proloader import path/to/building.mcstructure \
  --server "$SERVER_CODE" --to=0,80,0 --command-blocks stage
```

暂存保留源自动执行设置用于计划校验，但不恢复自动运行；**外部红石仍可能触发指令**，
因此首次实服验证须使用明确的隔离区域。`--command-blocks skip` 表示只放外壳，任务明确记录内部数据未导入。
Java `.schem` 命令文本尚未转换，不能选择暂存。`--verify none` 仅关闭普通方块校验，
显式选择暂存后仍须逐个读回命令方块数据；数据阶段有独立断点，`repair` 后会重新核对内部数据。
NBT历史输出、执行计数、容器和旗帜等尚未恢复；当前不提供源自动模式的激活操作。
真实网易权限、服务器开关、实际写入与触发行为仍待实服验证，不能据离线通过宣传已实测可用。

校验方式：

- `--verify sample`：默认，施工时有分批检查，结束后核对最多64个方块名称。
- `--verify complete`：核对全部目标方块名称，耗时较长。
- `--verify none`：确认可靠发送队列排空后结束，结果为“已发送、未验收”。

开启校验时，失败先记录并继续发送；全部命令发送后统一补齐，再校验，最多补齐两轮。
批次检查失败会记录整批可疑命令；结束时抽样或逐块检查发现的差异也会补齐。
仍未补齐的坐标与命令保存到任务记录，可从断点继续；断线或传送失败仍暂停保存。
关闭校验时跳过上述检查与补齐，仅保留必要的位置确认和命令处理背压。

以上名称检查都不等于完整states/NBT验证。兼容性警告会在预检中显示。
`--json`逐行输出计划、进度与最终结果。非交互运行应使用`--yes`确认覆盖范围，
并用`--password-stdin`从标准输入读取一行密码；不支持把密码放在命令参数中。

## 游戏内进度与完成控制

正常CLI导入会发送“proloader_bot：欢迎使用proloader开源导入器”，并用原生actionbar
每约2秒显示建筑放置命令的已发/总条数、最近15秒实测速率与预计剩余时间。
`fill`可能覆盖多个方块，速率单位是命令条/秒；聊天/传送等辅助指令不计入总数。

发送完成后机器人保留原连接，提示“proloader_bot：输入repair开始修复，输入done彻底完成操作”。
在游戏普通聊天或私聊中完整输入`repair`或`done`（忽略首尾空白/大小写）：

- `repair`重发整套建筑主层命令，修复结束后继续留服等待；它不是一次完整世界校验。
- `done`结束并退出。初始发送过程中输入不会提前取消；修复中输入则等当前修复发送完成后退出。
- 忽略系统/JSON通知和机器人自身消息。当前服务器玩家均可控制该导入机器人。

命令负回执和处理确认超时记录到任务，不因单次异常中断整套发送。
网易负回执可能误报，记录标为未确认，不能据此宣称方块一定缺失。
明确断线会保存断点并自动重连，最多5次；仍无法连接则显示暂停并保留全部记录，不冒称完成。
Ctrl+C仍可主动暂停。自动化只需发送后退出时加`--exit-after-send`。

建筑列表支持↑↓滚动、N/P或PageUp/PageDown翻页、Home/End首尾；数字选择当前页。

## 覆盖与恢复

建筑起点是最小X/Y/Z。跳过空气，保留建筑空白处原有内容；目标非空气块会覆盖对应位置。
每条fill检查范围和体积，施工窗口按蛇形分组，命令坐标始终锚定计划原点。

Ctrl+C暂停并保存断点。任务保存在`~/.proloader/imports`，身份在`~/.proloader/bots`。
续传需匹配原文件、位置、机器人、服务器、引擎、计划指纹及命令映射版本。
旧版或已变化的计划会拒绝按旧下标继续。完成/已发送任务不能当作中断任务续传。

## 已接通与限制

- 交互导入超出限高时会给出建议起始Y和合法范围，可采用建议或手动修改，保留服务器、建筑、密码及X/Z后继续。建筑自身过高时可更换维度或文件。非交互/JSON模式返回结构化高度建议，不会擅自移动施工位置；断点续传仍严格使用原坐标。
- 租赁服认证链、RakNet、加密、游戏初始化、单身份锁与退出清理。
- `.mcstructure`、Sponge `.schem` v2/v3、MCEdit Alpha `.schematic` 读取和 Java→Bedrock 方块转换。
  `.schematic` 可直接在菜单选择或拖入终端，支持 gzip/原始NBT与旧版ID/附加值；模组扩展ID会明确拒绝，Java方块实体附加数据尚未转换。
- Fatalder原版fill规划、48×48蛇形施工组、可调限速、传输背压及任务记录。
- 连续10轮首次连接实验已通过，单轮完整初始化、只读探针、保持在线至少30秒并正常退出。
  这不保证外部版本服务或任何未来网络请求都不会失败。
- 当前为实验性源码版本。大型建筑的全量验收尚未完成；请先在备份过的测试世界验证。

完整Fatalder源码、FunShuttler、WaterStructure等已纳入`third_party/fatalder`，
来源见`third_party/fatalder-source.json`。命令方块更新包和子区块NBT读取通过Python唯一会话接入，
原生助手只做离线编解码。其余NBT console/assigner、完整导出等上游能力尚未接通；
旗帜图案、容器内容等附加数据不能宣称已经完整导入。
原版CLI虽可编译，也不能和Python使用同一身份并连。

## 维护与发布

维护者先读[MAINTAINERS.md](MAINTAINERS.md)，并使用仓库技能
[proloader-maintenance](.agents/skills/proloader-maintenance/SKILL.md)。
技能包含版本升级的定位入口、验收方法和独立发布目录导出脚本。

**本机保留私有凭据。** 发布不删除或移动本机Cookie、PE、机器人和诊断材料；
另建干净的发布目录，审查后再上传。不要直接打包整个工作目录。

```sh
python3 -m pip install -e '.[dev]'
python3 -m pytest -q
python3 -m proloader simulate --help
```

组合分发采用 **AGPL-3.0-only**；原项目MIT条款保留于`LICENSE-MIT`。
上游及依赖说明见[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

### AI 命令方块开发 CLI

`python3 -m proloader ai --help` 提供网易手机版指令规范查询、保守静态检查、mcfunction蛇形布局、JSON→mcstructure、常驻假人命令/批量控制、NBT读回与热修改。先看[使用说明](docs/AI_CLI.md)，实服观察仅覆盖项目内置的保守子集；静态检查不代表任意服务器均支持。当前AI通过本机CLI调用，不内置模型或API Key。


## 开源目录与账号隐私

本源码包不附带账号、Cookie、PE token、私人建筑或开发者 Git 历史。运行时仍默认把新身份与任务保存到 `~/.proloader`；如与其他副本在同一系统账号下运行，会共享该目录和身份锁。请勿使用同一身份同时启动两个连接。源码副本并不自动隔离运行时账号。

本次整理范围与离线验证结果见 [RELEASE_REVIEW.md](RELEASE_REVIEW.md)。报告只说明此源码快照，不代表线上完整验收。


### Go dependency mirror

The pinned nemc-tan-lobby-solver v0.2.5 module is unavailable from the default Go proxy. Native helper builds fall back to the public goproxy.cn mirror and keep go.sum verification enabled. Set GOPROXY explicitly to override this build-local default; no global Go or network settings are changed.

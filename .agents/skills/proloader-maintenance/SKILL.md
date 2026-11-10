---
name: proloader-maintenance
description: "Agent-operated Proloader maintenance after NetEase/Minecraft updates: official version discovery, exact authentication and protocol adaptation, static artifact analysis, offline regression and read-only live acceptance. Also supports requested Fatalder synchronization and sanitized source releases."
---

# Proloader 版本维护

由 AI 执行维护、定位、适配、验收和记录。不要让用户代跑命令。只要求改 skill/交接时不连服。

## 开始与完成标准

1. 读仓库 AGENTS、当前维护记录和本 skill。本机存在`docs/HANDOFF.md`时读取顶部最新状态。保留 dirty 文件、原身份、建筑和旧任务；不要 reset。先确定本轮范围：版本适配默认包含已有授权服务器上的只读连接验收，不包含施工、重放任务、启用命令链、发布或改代理。
2. 先运行内部入口，读取一行 JSON；失败状态是转入人工分析的路由，不是任务终点：
   ```sh
   python3 .agents/skills/proloader-maintenance/scripts/maintain.py --online
   ```
3. `current` 只说明官方精确配对已在配置中，不等于本轮重新验收。若用户要求排障/复验，使用 `--audit` 并按下方流程继续；无变化且没有复验需求则结束。
4. 新版本必须完成：官方来源与摘要 → 静态差异审阅 → 精确配方 → 离线回归 → 一次60秒只读长连 → 同身份顺序10轮首次连接（每轮30秒、两次探针、0重连）→ socket/任务/租约清理 → 交接与过程记录。长连用于发现曾在31–44秒发生的动态校验断线，不由10轮短保持代替。
5. 成功后才推进认证静态基线；失败保留证据并修复具体阶段，再开始新的固定源码验收序列。不能混合修复前后的成功轮次。存在未解决阻塞就明确说明最后成功阶段，不能把已配置称为已适配。

## 按状态继续

| 状态/原因 | 下一步 |
|---|---|
| `accepted` | 核对ONLINE、源码摘要及清理，再完成60秒长连与记录；脚本目前只内置10轮。 |
| `current_verified` | 只完成静态核对；是否需要连接由本轮任务决定。 |
| `needs_configuration` / `needs_password` | 读[自动化说明](references/automation.md)，复用本机身份和已授权服务器；密码仅stdin，不落盘。 |
| `busy` | 查施工、共享常驻假人和租约；不能抢锁或并开同身份。活任务结束再继续。 |
| `ready_to_apply` | 维护已授权则加`--online`执行，不重复询问。 |
| `needs_analysis` / 新引擎 / 模块变化 | 按[完整适配手册](references/adaptation.md)分析；新引擎必须核对原生库、证书、签名参数和协议，不能只换版本号。 |
| `rolled_back` / `error` / 超时 | 按[故障分流](references/troubleshooting.md)读最小日志，修复、测试，再回到验收。检查已回滚什么、PE是否已更新、进程是否仍在。 |

## 必须保持的约束

- 正式下载引擎、patch、资源hash、库hash和签名参数必须来自同一已审阅配方。灰度比例只是证据；不因存在灰度就切换，100%后仍核官方正式入口。
- 同引擎仅当三个选定模块全部规范化指纹、四个认证方法一致，RN/overlay/原生资源条件满足时自动追加精确配对。新引擎即使Python方法相同也必须审阅原生调用。旧配方按引擎保留，拒绝跨引擎拼接。
- downloaded pyc、MCP及服务端动态校验只静态解析，不执行。固定解析器来源、许可和摘要；未知模板停止于分析，不回退旧固定校验。
- 真实游戏协议以当前 owning source 和握手证据为准；旧AGENTS或旧报告中的2168/686/860不能覆盖当前runtime配置。读取CURRENT_RENTAL_PROTOCOL及最新验收报告，不在skill固定当前协议号；升级必须核对对应GameVersion和wire格式。
- Python唯一持有游戏连接；Fatalder原生助手不使用同身份另连。维护默认不写世界、不重发短信、不发布、不改系统网络。
- 成功发送、PE成功、进入世界、长连、完整建筑/NBT验收是不同结论。只读维护不声称建筑验收通过。
- 历史发送速率、窗口、等待和修复算法不固化在此skill。涉及这些改动才读取当前代码及最新专项交接，尤其不要恢复已移除的0.25秒等待或用旧游标接新计划。

## 按需展开

- [自动化入口、配置和证据文件](references/automation.md)：首次绑定、缺依赖、脚本行为和回滚边界。
- [完整适配手册](references/adaptation.md)：从官方发现到新引擎原生分析、配方、动态MCP、协议和验收的完整操作路径。
- [故障分流与收尾](references/troubleshooting.md)：下载、工具、PE、Link、地址启动、RakNet、spawn、延迟断线、回归失败与清理。
- [代码定位](references/version-updates.md)：具体文件入口和协议易错点。
- [本次完整案例](references/case-20261103.md)：3.9→3.10实际步骤、证据、失败与最终结果；不是未来版本可直接套用的参数。
- Fatalder同步：核`third_party/fatalder-source.json`及组合许可，比较目标提交，重建受影响助手并跑对应测试；计划变化不沿旧下标续传。只在用户要求时同步上游。
- [发布流程](references/releases.md)：仅用户要求发布时，另建干净目录，保留本机私有材料。组合许可AGPL-3.0-only，原MIT保留；不打包整个本机项目。

每次维护必须新增日期化过程文档，记录开始状态、来源/工具、决策依据、文件改动、每次失败、测试与排除项、实服轮次、摘要、回滚/凭据/租约状态；在HANDOFF顶部链接。敏感原始证据留cache，文档/skill不写Cookie、token、JWT、密码或私有响应正文。具体记录格式见故障分流末尾。

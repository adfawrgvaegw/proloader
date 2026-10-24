---
name: proloader-maintenance
description: Agent-operated Proloader maintenance after NetEase/Minecraft updates. Check official versions, prove compatible patches, configure exact metadata, test and validate connections; route changed authentication/protocols to focused analysis. Also maintain Fatalder integration and clean source exports.
---

# Proloader AI维护

用户只需让AI读取本skill并提出维护需求，不需要运行终端命令或双击.command。AI负责执行、读取结果、按需分析适配并汇报；不要把下面的内部脚本命令当作操作步骤交还用户。本次若只要求编辑skill/交接，不启动在线维护。

先遵守仓库AGENTS；本机HANDOFF只读顶部最新状态，不展开历史。接到实际版本维护任务后，由AI在项目根目录调用内部自动化入口：

```sh
python3 .agents/skills/proloader-maintenance/scripts/maintain.py --online
```

只读stdout的一行JSON；大清单、源码和日志不进入上下文。没有变化即结束，不重复下载/测试/验收。
脚本自动完成：正式源核对→校验MCP→三个认证模块静态指纹→精确配对配置→离线测试→10轮只读连接；失败回滚本次配置。已有灰度版本只报告，不自动切换。

## 按结果处理

- `current` / `current_verified` / `accepted`：报告结论与证据位置。
- `needs_configuration` / `needs_password`：按[自动化说明](references/automation.md)补齐本机参数后重跑；密码仅stdin/隐藏输入，不写配置。
- `busy`：已有施工或维护正在运行，先查状态，不能开第二连接或抢锁。
- `ready_to_apply`：缺少在线验收选项，若用户授权版本维护，使用上面`--online`完成；不另问已授权事项。
- `needs_analysis` / `rolled_back` / `error`：只读JSON列出的REPORT/NEXT和相关日志；按[自动化说明](references/automation.md)与[版本定位](references/version-updates.md)处理具体变化，修复后回到内部自动化入口。不把非零退出统一当程序崩溃。

同引擎只有**全部三个选定认证模块**语义指纹一致、RN/原生资源未变，才可自动增加精确patch/hash配对。新引擎或算法变化需要agent分析；不能删版本校验、混配3.9/3.10或伪造证据。脚本不执行下载的pyc。

保留本机凭据和断点；已有身份优先复用，失败不靠重发短信解决。默认一键流程不写世界、不发布GitHub、不改系统代理。请求版本维护包含必要的只读连接验收，按已有服务器授权使用本机配置；配置文件本身不扩大授权。

仅在任务涉及下列内容时展开相应资料：

- Fatalder同步：核对third_party/fatalder-source.json，重建原生助手，计划变化不能沿旧下标续传。Python是唯一游戏连接所有者。
- 发送策略：用户选定默认4000条/秒、突发10；关闭方块校验/手动repair每256条异步探针、最多768条调度窗口，换区/结束前drain。探针未确认记录原批次并降速250；位置确认后等待0.25秒。开启方块校验保留32条批次检查。断点回退保存的完整窗口，禁止退回32条。
- 建筑校验：差异记录→全量发送→最多两轮补齐；关闭校验时跳过检查/补齐，仍保留位置确认与处理背压。发送成功不是完整世界验收，NBT/次层液体缺口见MAINTAINERS。正常CLI默认留服等待玩家repair/done，repair重发全套主层命令；自动化可显式--exit-after-send。断线最多5次自动重连须单独披露，十轮首次连接验收不启用此恢复。
- 开源发布：等用户要求的测试通过，按[发布流程](references/releases.md)另建干净目录，保留本机私有材料。组合许可AGPL-3.0-only；上传仍需用户授权。

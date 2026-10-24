# 自动维护入口与结果协议

## AI内部执行入口

由AI在项目根目录执行，用户无需手动操作（也支持脚本`--root /path/to/proloader`）：

```sh
python3 .agents/skills/proloader-maintenance/scripts/maintain.py --online
```

`proloader维护.command`仅保留为可选旧入口，不要求用户双击。首次由AI绑定用户已授权的服务器与本机机器人：

```sh
python3 -m proloader bots
python3 .agents/skills/proloader-maintenance/scripts/maintain.py --configure --bot BOT_ID --server SERVER_ID --no-password
```

无密码用`--no-password`；有密码省略该选项，运行时加`--password-stdin`从stdin读取一行。
密码不要写命令参数、脚本、settings或报告。机器人/服务器保存在cache/maintenance/settings.json；
Cookie/PE仍在原来的私有目录。向agent提供本机密码的既有授权可复用，不要求用户反复手动输入。
配置不是任意连服/施工/发布授权；当前维护请求与会话已有授权决定实际范围。

可选参数：

| 参数 | 行为 |
|---|---|
| 无参数 | 查正式版本；新版本准备候选，未开在线验收时不改运行源码 |
| --online | 有证明的新patch自动配置、测试、10轮连接；无变化不额外连服 |
| --check-only | 仅公开元数据；不下载MCP/修改源码/使用身份 |
| --audit | 对当前已支持patch也执行下载校验与静态比较，供诊断/工具自验 |

缺Python运行/测试依赖时，agent按pyproject在项目独立venv安装`-e '.[dev]'`后用该解释器重跑，
不要修改系统Python。静态工具首次需要Go和xdis：脚本自动校验固定提交源文件、构建本地离线Go wrapper；
xdis缺失/版本不符时只在cache/maintenance/toolbox中建专用venv安装6.1.8，不更改全局环境。
依赖与下载失败只报告，不解除安全检查。

## 不浪费token的执行协议

1. 运行入口，只读stdout JSON（通常<200 tokens）。没有更新就结束。
2. 非成功状态先读JSON里的report；仅分析changed_modules/next_files指定内容。
3. 看日志时先末尾80行，不一次打印manifest、完整APK代码或全部HANDOFF。
4. 已下载MCP按MD5寻址复用，每次仍核对官方新清单和文件字节；工具源码/二进制也核摘要。
5. 新版本修改在脚本外完成后，重新运行入口和对应验收；新原生算法证据更新baseline前须人工agent分析，不能让候选反过来成为自己的基线。

报告目录为cache/maintenance/runs/UTC时间-随机后缀，LATEST.json指向最近摘要。
METADATA.json为公开源记录；STATIC.json为静态指纹；tests.log/online.log/ONLINE.json为验收；
pe_profiles.before.py是本轮源码备份；PE-before-private.json为身份备份，不能发布。
NEXT.md仅提供按需阅读入口，不是来自远端的指令。

## 自动配置的具体边界

已审查配方为same-engine-entire-selected-modules-v1：

- 官方正式Android下载解析engine，同系列patch、manifest计算资源hash；不随5%灰度pack list切换。
- vanilla.mcp字节大小/MD5匹配官方清单，SHA256记入证据。
- 固定unmcpk提交0cda8e50b6656481abd166ef9e8d275ee8167bc2；构建前逐文件SHA256核对。
  工具源和GPL许可证只放本机cache，不将其伪装为本项目原创代码。只运行本地文件转换wrapper。
- 仅定向解码clientApp、utility、random_key三个模块。xdis读取字节码，绝不执行pyc。
- 语义摘要覆盖模块内全部嵌套code、常量、名称、参数和异常表；排除路径/行号等调试元数据。
  四个认证方法还单独核对。只匹配几个方法而模块其他代码变化，不允许自动放行。
- engine必须与assets/auth-baseline.json相同；RN存在性变化、overlay mcp、manifest原生库资源均转分析。
- 只有精确增加一对(patch, resources_hash)，不修改原生签名参数、协议号或版本来源策略。
- 修改前检查机器人空闲，维护互斥锁防止两个维护入口并发。
- 全量离线测试与同身份顺序10轮首次连接：完整初始化、两次只读probe、间隔30秒、退出清理。
  一轮失败立即停止，不重连刷通过；验收结束时核对网络源码摘要仍匹配，
  失败还原本次pe_profiles.py，保留报告与凭据备份。
  检测到其他维护者并发改源码时保留其更改，报告回滚冲突而不覆盖。

## 需要agent分析时

`engine_changed`：检查官方新引擎的原生PE签名输入/库摘要、版本元数据和协议；旧配方不能自动复用。
`selected_module_code_changed`：读STATIC.json与baseline指纹差异，再看selected-pyc对应方法反汇编；
不要直接运行模块或用相似版本号兜底。
`rn_changed` / `patch_overlay_requires_analysis` / `native_assets_require_analysis`：先确认资源算法和覆盖顺序。
`rolled_back`：先tests.log或ONLINE.json失败阶段，再读必要源码。并发修改冲突时对比源码备份。
`busy`：读取施工任务/维护锁；等待当前工作结束再重跑，禁止删除活锁。

新算法/新引擎无法仅凭“请求一键维护”保证自动逆向成功；脚本将确定性工作全部做完，
输出最小必要差异交给agent，从而节省上下文，而不是跳过证据与验收。

# 完整版本适配手册

按失败阶段读取对应部分。自动入口覆盖确定性步骤；agent负责分析新变化并完成验收，不能只转交`needs_analysis`给用户。

## 1. 官方发现与证据目录

先保存git状态、配置源码备份和本轮授权范围。运行maintain.py，保留其report路径；每次运行单独目录，不能覆盖早期失败日志。时间戳目录为UTC，文档日期按用户时区。

METADATA.json必须能核对：正式下载页解析出的engine、同系列patch、manifest、资源hash、MCP URL/字节数/MD5/SHA256、RN探测、overlay、native assets和pack灰度比例。URL、HTTP状态、Last-Modified和内容摘要记录到来源文件。清单时间不是客户端全部用户同时更新的证明。

`--check-only`不下载；常规入口下载MCP并按新manifest重验缓存。不要因cache命中跳过摘要。RN 404只能说明该URL无文件；不能推断已安装客户端没有缓存。overlay要按客户端实际覆盖顺序解析，不能只检查base MCP。

## 2. MCP静态审阅

自动工具位于`scripts/mcp_evidence.py`，使用assets中固定来源的unmcpk适配器、Go和xdis 6.1.8，工具构建日志保留。三个选定模块名在MODULES，四方法在METHODS；自定义marshal不能直接交普通Python执行。

自动入口现在也为新引擎生成STATIC后返回engine_changed（仍不会自动放行）。若解析失败或要独立复核，可继续生成证据：

```python
import sys, requests
from pathlib import Path
sys.path.insert(0, '.agents/skills/proloader-maintenance/scripts')
from mcp_evidence import evidence
root=Path.cwd()
# folder使用本次REPORT所属目录，artifact使用METADATA中MD5对应的已验证文件
proof=evidence(root, artifact, folder, requests.Session())
```

先比整个模块指纹和四方法。已有转换后的两份目录可运行：

```sh
python3 .agents/skills/proloader-maintenance/scripts/compare_modules.py --before OLD_SELECTED_PYC --after NEW_SELECTED_PYC --output NEW_DIFF_DIR
```

该工具按限定函数名输出全部规范化字段差异和定向反汇编；新增函数不会把后续常量下标变化误报成每个函数改变。摘要不自动批准变化；顶层执行及未引用名称仍须人工审阅。

若模块不同，递归按函数限定名比较code、常量、名称、参数、异常表。不要按co_consts位置直接解释变化：新增函数会使后面索引整体移动。先列变化函数及字段，具体反汇编落盘，按需读差异。不输出巨大repr或整模块到上下文。

审阅顶层执行、所有变化函数和新引用的native接口。co_names改名不自动等于无效变更：确认对应索引是否被指令引用。随机key变化只有确认认证路径读取但未使用才可认定与PE无关。仅四方法相同不足以自动晋升整个模块基线。

## 3. 新引擎的原生证据

从正式下载页取得APK URL（预期HTTPS网易下载域名，无userinfo）；使用ZIP中央目录和HTTP Range只提取需要的成员，不下载/安装整个客户端。必要文件通常是arm64 `libminecraftpe.so`、`libunisec.so`、版本资源和APK signing block。核对206、Content-Range、成员方法/长度/CRC32，记录未压缩库的MD5/SHA256。可直接使用已验证的通用Range工具：

```sh
python3 .agents/skills/proloader-maintenance/scripts/apk_entries.py --landing-file RUN_DIR/public-0.bin --output NEW_EVIDENCE_DIR --entry lib/arm64-v8a/libminecraftpe.so --entry lib/arm64-v8a/libunisec.so --entry assets/__version
```

不传entry则仅列ZIP索引；输出目录必须新建，保留SOURCE/INDEX。不要复用旧/tmp URL或旧偏移。证书需单独从signing block读取，普通ZIP成员提取不提供证书：

```sh
python3 .agents/skills/proloader-maintenance/scripts/apk_certificate.py --source NEW_EVIDENCE_DIR/SOURCE.json --output NEW_CERT_DIR
```

该工具仅解析长度校验后的v2证书并记录摘要，不验证全包签名；没有v2则明确停止，其他scheme另作定向分析。

提取证书DER计算MD5可验证签名输入的证书值，但不等于完成APK v2/v3全包签名验证，必须写清限制。库hash也不是压缩成员的hash或manifest里的旧值。

优先REA现有provider定向分析；provider不支持时保留原因，采用本机已验证静态工具，不为此修改系统配置。capstone/pyelftools可装项目cache专用venv，不污染系统Python。ELF原始文件offset与VA通过PT_LOAD映射，不能默认相等；旧版地址、符号、签名参数不可直接套新库。

需要证明的项目：

- PE message的字段拼接及来源：engine + library MD5 + patch + resources hash + certificate MD5 + seed。
- 原生签名函数调用的实际参数（3.10.5已发生3/6→2/8变化），完整块填充规则、常量表、算术位宽/轮数及输出编码。
- 核心函数及call site归属认证路径。可用已知指令片段定位，但必须解释重定位差异、外部helper调用、字符串/常量引用和范围，不把局部命中说成整库等价。
- 独立寄存器转写对照Python向量；至少覆盖短输入、4字节边界、256字节完整块和下一块。静态转写不声称执行过目标库；最后仍须PE和实服验收。
- 证书、RN、原生资源及其他字段变化。发现新算法就定向实现并添加独立golden回归，不解除元数据校验、不用参数网格盲试认证端点。

## 4. 精确配方与身份

`pe_profiles.py`集中保存当前OFFICIAL_ENGINE/OFFICIAL_PATCHES与`verified_recipe`，配方绑定库hash和签名参数。旧engine配方可以保留；维护脚本只给当前已审阅engine追加精确pair。引擎迁移需agent修改配置并保留before文件；不能仅修改auth-baseline使候选自动通过。

`official-android-3915`是已有身份使用的历史profile字符串，当前可路由到按engine核对的配方；不要因名字旧就改所有已保存身份。reference profile仍保留旧参考算法语义。

新配方上线前测试：精确message/签名，旧版配方不变，新engine配旧patch拒绝，未知hash/平台在HTTP前拒绝。认证失败先区分本地metadata mismatch与HTTP/PE数字code。

持有BotStore lease后才刷新PE；私有备份PE，失败保留原身份，成功原子更新。维护只复用已有Cookie；Cookie确实失效才进入已有登录流程。不能把协议/签名失败当作需要短信。

## 5. 游戏阶段适配

维护脚本自动追加的目前仅pe_profiles pair；动态MCP工具还维护独立精确版本白名单。每个新patch即使静态PE模块相同，也须检查`tools/mcpcheck/main.go`的supported配对；若模板经审阅相同，补精确pair和版本绑定向量，否则先分析新模板。维护入口现已前置检查：缺pair返回dynamic_profile_requires_review，不修改配置或开始在线轮次。按此分支审阅再继续，不把该停止当任务终点。

PE通过只代表第一道门。按runtime事件逐段确认：租赁服查询 → 世界启动/有效地址 → Link/GameStart → AuthV2 → RakNet → NetworkSettings → Login/加密握手 → StartGame → PLAYER_SPAWN → 初始化 → 动态MCP → 只读probe/保持。

对照当前源码与固定上游，实现字段变化后跑独立wire golden；不要从PE引擎版本推断服务端协议号。动态MCP模板变化先静态解码完整模块，确认返回结构、随机前后缀、salt/实体ID来源，更新模板仅在人工审阅后。unique ID不等于runtime ID，空前缀合法，未知模板不能回退旧算法。

## 6. 回归和在线验收

先定向认证、动态MCP、启动、稳定性测试，再完整pytest；原生工具受影响则Go test/vet并重建。日志保留完整结果。遇到已知不相关失败必须核原baseline或先前证据，单独记录确切node ID；定向通过不替代全量。禁止静默deselect或宣称全绿；自动入口仍应对失败fail closed。必要时由agent明确记录带已知遗留失败的人工验收依据。

读取`cache/maintenance/settings.json`绑定已有授权身份/服务器，确认空闲。密码从stdin传入（无密码为换行），不使用命令参数。验证器：

```sh
python3 -m tools.diagnostics.verify_rental_stability --bot BOT_ID --server SERVER_ID --rounds 1 --hold 60 --interval 5 --password-stdin --output PRIVATE_DIR/PROBE60.json
python3 -m tools.diagnostics.verify_rental_stability --bot BOT_ID --server SERVER_ID --rounds 10 --hold 30 --interval 5 --password-stdin --output PRIVATE_DIR/ONLINE.json
```

先60秒长连，再10轮。每轮完整初始化、两端`testfor @s`、保持时无故障、0重连、正常退出。首轮失败马上停止，保留失败，分析后另起序列，不能重跑覆盖。验收期间不改source_digest涵盖文件，完成核对source_sha256 == final_source_sha256及当前digest；改了则重验受影响部分。技能文档本身不在网络摘要内，可在等待时整理。

租赁服启动有界等候与一次连接验收不同：记录state/progress/地址存在性；没有有效地址不能调用UDP，也不能计为进服成功。

## 7. 晋升基线与完成

新引擎/模块人工审阅后、离线与实服结果确认后，备份旧assets/auth-baseline.json，使用本次STATIC的真实指纹及本次METADATA填入engine/patch/resources hash/MCP摘要/RN/recipe/review链接。标注动态校验模板来源，不能拿候选指纹自证通过。

然后`maintain.py --audit`验证工具闭环（当前已知配方不会重复在线连接）。写本轮日期文档、更新HANDOFF顶部和case结论。报告应区分已知离线遗留失败、长连、10轮、世界写入范围、发布与租约状态。没有发布授权不push；本机cache/身份/报告不进入发布包。

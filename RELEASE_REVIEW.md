# 开源源码整理记录

Prepared 2026-10-24. The owner has authorized public GitHub upload. Remote publication requires an authenticated GitHub account.

## 收录内容

- Python regression: 471 passed in 23.30 seconds, with an isolated temporary HOME.
- Fatalder 及其本地子模块源码、Go 锁文件、原生适配器、构建资源。
- 使用说明、公开维护技能、AGPL-3.0、原 MIT 和上游许可。
- 全部文件校验值见 RELEASE_MANIFEST.json；该清单自身不参与哈希。

## 隐私与独立性

按允许列表导出；不携带原 Git 历史、开发者个人 AGENTS、私有交接与诊断文档、运行缓存、账号库、登录 Cookie、PE 会话、抓包、日志、本机二进制或用户建筑。上游自带测试样本和公开示例仍保留。

个人服号、联系方式、服主昵称和绝对用户路径已替换或改成动态路径。认证测试里的手机号、token 等为合成测试数据。另清除了上游两个文件中的 GitHub Token 字面量，改用环境变量；上游公开的协议 PEM 常量保留，说明见 SECURITY.md。

已用当前身份与本机历史 Cookie 缓存提取的23个身份/认证特征扫描源文件，并对可识别的 gzip/ZIP 内容展开核对：未匹配到当前私有材料。另检查了个人标识、JWT、GitHub/AWS/API token 及敏感赋值。此检查不对所有专有二进制格式作完整语义证明。

原工作目录、私有数据目录的文件大小/修改时间，以及原机器人文件的 SHA-256 前后核对一致。未启动游戏连接、发送短信或写入世界。测试使用临时 HOME，未接触真实账号库。

源码目录独立，但运行时默认身份路径仍为 `~/.proloader`，在同一系统用户下启动两个副本会共享身份锁；请勿同身份并连。

## 验证结果

- Python regression: 471 passed in 23.30 seconds, with an isolated temporary HOME.
- Fixed three existing malformed mcstructure cases in this public copy: truncated primary layer, invalid palette reference, and duplicate block entity coordinates. Existing regression tests now pass; the original working copy is unchanged.
- 原生编译通过：proloader-plan、proloader-nbt、proloader-world、上游 CLI，以及 tools/mpay_login。
- 原生测试：proloader-nbt 通过；proloader-world 无测试文件。
- CLI 帮助与雪球菜单离线打包通过。
- 修复 tools/mpay_login 的 Go replace，使其使用随包附带的 g79client，不依赖开发者 cache。
- iOS 兼容路径改为当前用户主目录；旧分析脚本改用相对路径。

未重新执行线上验收；大型建筑全量校验、普通方块实体 NBT、次层液体和其他 README 列出的能力缺口仍存在。当前按源码 checkout 使用，独立 wheel 资源打包尚未完成。

Publication is authorized. Public repository: https://github.com/adfawrgvaegw/proloader. Preserve all upstream notices and dependency source references.

## Fresh-runner portability follow-up

The initial GitHub runner exposed an unavailable default-proxy dependency and a Python 3.11 test event-loop assumption. Native helper builds and CI now use a public mirror fallback without disabling Go checksum verification or changing global settings; explicit GOPROXY environment overrides remain supported. A clean module cache retrieved the pinned solver with the exact locked h1 checksum. The CLI test now restores an absent default loop correctly. Local Python regressions remain 471 passed. Current GitHub CI status is visible at https://github.com/adfawrgvaegw/proloader/actions/workflows/ci.yml.

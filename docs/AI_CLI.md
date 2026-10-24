# AI 控制假人和命令方块

AI（包括当前聊天的助手）通过本地 CLI 写源文件、部署、读取和修改。第一版不内置模型调用或 API Key；支持能运行本机命令的 AI。游戏连接仍由一个 Python 进程持有，复用 BotStore 身份锁。不要同时使用同一身份启动普通导入器。

先查规范，再写代码：

```sh
python3 -m proloader ai spec
python3 -m proloader ai spec --topic execute
python3 -m proloader ai lint examples/ai/counter.json
```

内置规范在 `proloader/ai/spec.py`，区分本项目网易手机版实服观察与国际版基岩参考。不是完整网易语法解析器，不能由静态检查通过推断服务器支持。网易启动器/补丁版本、引擎版本、网络协议号不同；不要把国际版最新版文档直接当网易规范。Java 语法常见误用会被拒绝，未知命令不会凭空认定支持。OP、命令方块开关、实验功能、插件也会影响结果。

启动常驻连接（另一个终端；无密码可省略 stdin 参数）：

```sh
python3 -m proloader ai serve --server 服务器编号
python3 -m proloader ai status
python3 -m proloader ai command 'testfor @s'
python3 -m proloader ai stop
```

有密码使用 `--password-stdin`，密码只从标准输入读取。多机器人用 `--bot ID或唯一名称`。`serve`前台运行，可由进程管理器或`nohup`留后台。控制端点默认 `~/.proloader/ai/control.sock`，目录0700、socket0600，只提供本地 Unix socket。`--socket 路径`写在操作前。真实连接错误最多连续重试5次，稳定在线60秒后重置计数；主动stop不重连。认证错误不盲目重试。电脑休眠、网络或服务器离线仍可能使假人掉线。

源文件示例：

```json
{
  "format": "proloader-ai-v1",
  "edition": "bedrock",
  "dimension": "overworld",
  "origin": [12000, 284, 12014],
  "blocks": [
    {
      "at": [0, 0, 0],
      "type": "impulse",
      "facing": "east",
      "command": "scoreboard players add pulse plai_test 1",
      "auto": false,
      "conditional": false,
      "delay": 0,
      "first_tick": false,
      "track_output": true,
      "name": "AI pulse"
    }
  ]
}
```

`at`为相对origin的非负整数坐标。类型impulse/chain/repeat，六个朝向down/up/north/south/west/east。conditional为条件连锁，auto=true为保持开启，delay单位游戏刻（20刻约1秒），first_tick为首次执行设置。命令为单行字符串。导出保留源设置；在线stage强制需要红石及关闭首刻执行。JSON不能夹带任意Python或shell代码。当前仅接收这组已定义字段，拼写错误会拒绝。

离线生成文件，交给原导入器使用：

```sh
python3 -m proloader ai compile examples/ai/counter.json --output cache/counter.mcstructure
python3 -m proloader import cache/counter.mcstructure --server 服务器编号 --to 12000,284,12014 --command-blocks stage
```

原导入器的目标坐标与JSON origin需明确对应；目标维度也需要通过`--dimension`传入。普通导入器stage后不会自动启用。常驻AI进程占用身份时不能同时运行此import，应先stop，或直接走下述deploy路径。

直接使用当前常驻连接调试：

```sh
python3 -m proloader ai deploy examples/ai/counter.json --id counter-deploy-001
python3 -m proloader ai verify examples/ai/counter.json
python3 -m proloader ai arm examples/ai/counter.json --id counter-arm-001
python3 -m proloader ai inspect 12000 284 12014
python3 -m proloader ai disarm examples/ai/counter.json --id counter-disarm-001
```

deploy先检查全部目标为空气，才放外壳、写内部数据并逐块验证；不清空包围盒，也不覆盖已有块。arm先核对全部暂存数据，再开启链尾与首块。verify默认检查暂存配置；`--active`检查源启用配置。inspect传世界坐标，返回新鲜NBT，包括Command、SuccessCount、LastOutput等实际服务器字段。检查操作会将假人传送到目标附近，以便加载区块。

修改JSON后用`write`热改内部指令，随后verify、arm再测试；write会暂存为需要红石。类型/朝向/条件变化要求明确停机并重建外壳，write不会偷偷换块。批量激活或写入并非世界事务，中途异常可能留下部分结果；返回已确认数量，先inspect再决定下一步。生产玩法应先关闭自己的计分板总开关。disarm只是需要红石，外部电源仍能触发。示例counter需先建立`plai_test dummy`计分板；它是开发样例，不是自动安装脚本。

大量命令可从`.mcfunction`自动生成蛇形链，箭头自动连接下一块：

```sh
python3 -m proloader ai layout gameplay.mcfunction --origin 12000 288 12000 --width 12 --head repeat --delay 20 --output cache/gameplay.json
python3 -m proloader ai lint cache/gameplay.json
python3 -m proloader ai compile cache/gameplay.json --output cache/gameplay.mcstructure
```

忽略空行和以#开头的注释。源上限10万块、包围盒200万格；常驻单请求最多4MiB，较大程序需分拆。离线验证包含1024块蛇形链及读写保真；未做千块线上性能验收，在线逐块NBT核对速度不能套用普通fill导入速率。

即时批量命令用JSON字符串数组：`python3 -m proloader ai batch commands.json --id batch-001`。每条等待回执，最多10000条；首条未确认就停止，检查`all_confirmed`和已提交数量。`ok=true`只表示RPC处理完成，单命令还需看`confirmed`，批量需看`all_confirmed`。不会自动重发非幂等写操作。

所有控制操作输出JSON。也支持`rpc request.json`，请求为`{"id":"唯一id","op":"command","command":"testfor @s"}`。同一次常驻进程最近512项请求按id与请求正文去重（跨自动重连保留），重试使用相同id和内容。busy时尚未接收新请求；status在忙时可返回进度。进程重启、结果被淘汰后去重不再有效，连接丢失不能随意换id重发加分/give等命令。客户端被中断后服务器端操作可能仍继续，先status和inspect。

AI每次操作应遵循：确认服务器/区域→查spec→生成源文件→lint与离线预览→deploy暂存→检查NBT→按用户本次授权arm→实际效果检查→修改→重测。不要在未知区域放自动循环链。不要以写入成功、空字符串回执或发送完毕冒充玩法验收。

命令发送回执不等于游戏刻中的红石/连锁逻辑已执行。首次自动化试验曾在放红石后立即查分得到未确认，随后NBT与只读查分确认成功；效果测试应等待游戏刻并有界重复只读探针，不能重放加分命令。

Codex现已配置名为proloader的MCP服务，提供10个proloader_工具。新开聊天加载后可直接说“查询假人状态”“检查网易指令规范”“编写玩法并在授权区域暂存”。MCP入口为tools/proloader_mcp.py，和CLI共享同一控制进程，不会创建第二个游戏连接。当前工具清单可能需要新聊天/重载才能更新。Trae接入已按用户要求取消。

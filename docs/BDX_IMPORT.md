# BDX 导入

支持菜单、plan/import、交互文件选择及 Web 上传。先离线预检：

```sh
python3 -m proloader plan path/to/building.bdx --to=1000,80,1000
```

首次读取需 Go 构建 tools/bdxreader；解码复用随包上游源码，Python 唯一持有游戏连接。支持字符串/位移编码、117运行时池、旧名称/附加值、状态、命令方块及通用NBT读取。建筑包围盒最小角对应 --to；重复坐标以后一次为准。BDX 的字面 structure_void 支撑保留，空气与 structure_block 不施工。

未知操作码、损坏/截断、未知方块、未定义常量与未知运行时池会停止预检；不验证签名尾部的真实性。解压限制256MiB、1600万操作、6400万格包围盒。

命令方块需明确选择暂存或仅放外壳，不自动启用源自动指令链。容器与告示牌仅处理预检支持字段，见[其他NBT](OTHER_NBT.md)。本次轻量在线验收使用 mcstructure 小屋，不能据此认为任意 BDX 或新 NBT 优化均已实服验收，见[验收说明](ACCEPTANCE_20261028.md)。

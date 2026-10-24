#!/usr/bin/env python3
"""端到端测试：cookie → PE认证 → Link+GameStart → JWT chain → Bedrock 连接 → 建筑导入

完整网易租赁服登录链路（7 步）+ Bedrock 协议连接（第 7 步）+ 建筑导入（aiCommand setblock）。

用法: python3 -m tools.test_netease_bedrock
"""
import asyncio
import json
import logging
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from proloader.net.bedrock.crypto import generate_keypair, serialize_public_key_b64
from proloader.net.bedrock.session import BedrockSession
from proloader.net.netease.client import NeteaseClient
from proloader.net.raknet.connection import handshake
from proloader.net.raknet.session import RakNetSession
from proloader.parser import load_structure

logging.basicConfig(
    level=logging.INFO,
    format="\033[36m[%(name)s]\033[0m %(message)s",
)
logger = logging.getLogger("test_bedrock")


# 网易 RakNet 协议版本（fatalder conn.go: protocolVersion = 8）
NETEASE_RAKNET_PROTOCOL = 8
# 网易 Bedrock 协议版本
# ⚠️ 抓包确认网易客户端发 860。测试验证：发 686 服务器返回 LOGIN_FAILED_CLIENT
#    （版本过旧），故必须用 860。fatalder 源码的 686 已过时。
NETEASE_BEDROCK_PROTOCOL = 860


async def main():
    # ════════════════════════════════════════════════════════════════════════
    # Part 1：HTTPS 登录链路（cookie → PE认证 → Link+GameStart → JWT chain）
    # ════════════════════════════════════════════════════════════════════════
    print("=" * 70)
    print("Part 1: HTTPS 登录链路")
    print("=" * 70)

    # 1. 读取 cookie
    cookie_path = Path(__file__).resolve().parents[1] / "cache" / "netease" / "cookie.json"
    if not cookie_path.exists():
        print(f"❌ Cookie 文件不存在: {cookie_path}")
        sys.exit(1)
    cookie_str = cookie_path.read_text()
    print(f"✅ Cookie 已加载 ({len(cookie_str)} 字符)")

    # 2. PE 认证 + 搜索租赁服 + 进入世界 + Link+GameStart + JWT chain
    print("\n--- PE 认证 + 租赁服链路 ---")
    client = NeteaseClient(platform="ios")
    client.bootstrap()
    client.authenticate_with_cookie(cookie_str)
    print(f"  ✅ user_id: {client.user_id}")

    server_code = "12345678"
    server_entity = client.search_rental_server(server_code)
    print(f"  ✅ serverID: {server_entity.entity_id}")

    host, port = client.enter_rental_server_world(server_entity.entity_id, "")
    print(f"  ✅ IP:port = {host}:{port}")

    client.connect_link_and_send_game_start(server_code, server_entity)
    print(f"  ✅ Link+GameStart 完成")

    # 生成 ECDH 密钥对（Bedrock Login 用）
    priv = generate_keypair()
    client_key = serialize_public_key_b64(priv)
    authv2 = client.generate_rental_game_auth_v2(server_entity.entity_id, client_key)
    jwt_chain = client.send_auth_v2_request(authv2)
    print(f"  ✅ JWT chain ({len(jwt_chain)} 字符)")

    # ════════════════════════════════════════════════════════════════════════
    # Part 2：Bedrock 协议连接（RakNet → Login → 游戏初始化 → 放方块）
    # ════════════════════════════════════════════════════════════════════════
    print("\n" + "=" * 70)
    print(f"Part 2: Bedrock 协议连接 → {host}:{port}")
    print("=" * 70)

    # 3. RakNet 握手（网易协议版本 8）
    print("\n--- RakNet 握手 ---")
    try:
        conn = await handshake(
            host=host,
            port=port,
            timeout=15.0,
            protocol=NETEASE_RAKNET_PROTOCOL,
        )
        print(f"  ✅ RakNet 握手成功 (protocol={NETEASE_RAKNET_PROTOCOL})")
    except Exception as e:
        print(f"  ❌ RakNet 握手失败: {e}")
        import traceback
        traceback.print_exc()
        return

    # 4. Bedrock 登录 + 游戏初始化
    try:
        raknet = RakNetSession(conn)
        await raknet.start()
        await raknet.connect()

        bedrock = BedrockSession(
            raknet,
            netease_mode=True,
            netease_jwt_chain=jwt_chain,
            netease_uid=int(client.user_id),
            netease_protocol=NETEASE_BEDROCK_PROTOCOL,
            priv_key=priv,  # ⚠️ 必须传入与 authv2 请求相同的私钥
        )

        print("\n--- Bedrock 登录 ---")
        await bedrock.login(server_addr=f"{host}:{port}", timeout=20.0)
        print("  ✅ Bedrock 登录成功 (LOGIN_SUCCESS)")

        print("\n--- 游戏初始化 ---")
        start_game = await bedrock.initialize_game(timeout=30.0)
        print(f"  ✅ 游戏初始化完成 (runtime_id={start_game.entity_runtime_id})")

        # 等待 MCP challenge 到达并处理（fatalder 在发命令前先解决 MCP）
        print("\n--- 等待 MCP challenge（8秒）---")
        await asyncio.sleep(8.0)

        # ════════════════════════════════════════════════════════════════════════
        # Part 3：aiCommand 通道诊断 + 建筑导入测试
        # ════════════════════════════════════════════════════════════════════════
        print("\n" + "=" * 70)
        print("Part 3: aiCommand 通道诊断（绕过 MCP 的魔法指令通道）")
        print("=" * 70)

        # ── 3.1 通道诊断：list 命令（不需要 OP 权限，任何玩家都能用）──
        # 如果 list 返回 executeResult=True → 通道可用，setblock 失败是权限/格式问题
        # 如果 list 返回 executeResult=False → MCP/通道问题
        print("\n--- 3.1 通道诊断：list 命令（不需 OP 权限）---")
        try:
            result = await bedrock.run_command_ai("list", timeout=8.0)
            if result is not None:
                print(f"  ✅ list 命令成功! 输出: {str(result)[:200]}")
                print("  → aiCommand 通道可用")
            else:
                print("  ❌ list 命令无响应或失败")
                print("  → aiCommand 通道可能被 MCP 封锁，或 executeResult=False")
        except Exception as e:
            print(f"  ❌ list 命令异常: {e}")
            import traceback
            traceback.print_exc()

        await asyncio.sleep(1.0)

        # ── 3.2 通道诊断：help 命令（不需要 OP 权限）──
        print("\n--- 3.2 通道诊断：help 命令（不需 OP 权限）---")
        try:
            result = await bedrock.run_command_ai("help", timeout=8.0)
            if result is not None:
                print(f"  ✅ help 命令成功! 输出: {str(result)[:200]}")
            else:
                print("  ❌ help 命令无响应或失败")
        except Exception as e:
            print(f"  ❌ help 命令异常: {e}")

        await asyncio.sleep(1.0)

        # ── 3.3 快速验证：gamemode + 相对坐标 setblock ──
        # 已确认结论：绝对坐标被 MCP 拒绝，相对坐标 + dimensional 可行
        print("\n--- 3.3 快速验证（gamemode + 相对坐标 setblock）---")
        BOT_NAME = "废魂魄街车"
        try:
            result = await bedrock.run_command_ai(
                "gamemode creative", timeout=8.0, dimensional=True, bot_name=BOT_NAME,
            )
            print(f"  gamemode creative: {'✅' if result is not None else '❌'}")
        except Exception as e:
            print(f"  gamemode ❌: {e}")
        await asyncio.sleep(0.5)

        try:
            result = await bedrock.run_command_ai(
                "setblock ~ ~ ~ minecraft:gold_block []",
                timeout=8.0, dimensional=True, bot_name=BOT_NAME,
            )
            print(f"  setblock ~ ~ ~: {'✅' if result is not None else '❌'}")
        except Exception as e:
            print(f"  setblock ~ ~ ~ ❌: {e}")
        await asyncio.sleep(0.5)

        await asyncio.sleep(1.0)

        # ── 3.4 解析雪景球 .schem 文件（Sponge Schematic v2）──
        print("\n--- 3.4 解析雪景球 .schem ---")
        import gzip
        import numpy as np
        from proloader.model.block import Block, BlockState
        from proloader.model.palette import BlockPalette
        from proloader.model.region import Region
        from proloader.model.structure import Structure
        from proloader.parser.nbt import parse_nbt_auto

        schem_path = Path(__file__).resolve().parents[1] / "雪景球1.schem"
        with open(schem_path, "rb") as f:
            raw = f.read()
        root = parse_nbt_auto(gzip.decompress(raw))
        SX, SY, SZ = int(root["Width"]), int(root["Height"]), int(root["Length"])

        # 解析 Palette: "minecraft:stone_bricks[facing=east]" -> (name, states_dict)
        def _parse_state_val(v: str):
            if v == "true": return True
            if v == "false": return False
            try: return int(v)
            except ValueError: return v

        mc_palette = {}  # mc_idx -> (name, states)
        for key, mc_idx in root["Palette"].items():
            if "[" in key:
                name = key[:key.index("[")]
                states_str = key[key.index("[")+1:key.index("]")]
                states = {}
                for pair in states_str.split(","):
                    if "=" in pair:
                        k, v = pair.split("=", 1)
                        states[k] = _parse_state_val(v)
            else:
                name = key
                states = {}
            mc_palette[int(mc_idx)] = (name, states)

        # 构建 our palette
        _pal = BlockPalette()
        mc_to_our = {}
        for mc_idx, (name, states) in mc_palette.items():
            if name == "minecraft:air":
                mc_to_our[mc_idx] = _pal.air_id
            else:
                mc_to_our[mc_idx] = _pal.add(Block(BlockState(name, states), 0))

        # 解析 BlockData (varint 编码的 palette index 数组)
        block_data = root["BlockData"]
        indices = []
        pos = 0
        while pos < len(block_data):
            val = 0; shift = 0
            while True:
                b = block_data[pos]; pos += 1
                val |= (b & 0x7F) << shift
                if (b & 0x80) == 0: break
                shift += 7
            indices.append(val)

        # 构建 3D blocks 数组 (顺序 x→z→y，同 .mcstructure)
        _blk = np.full((SX, SY, SZ), _pal.air_id, dtype=np.uint32)
        for i, mc_idx in enumerate(indices):
            x = i % SX
            rem = i // SX
            z = rem % SZ
            y = rem // SZ
            _blk[x, y, z] = mc_to_our.get(mc_idx, _pal.air_id)

        structure = Structure(
            name="snow_globe", size=(SX, SY, SZ), palette=_pal,
            region=Region(palette=_pal, blocks=_blk, offset=(0, 0, 0)),
            block_entities={}, format_version="2", is_netease=False,
        )
        print(f"  ✅ 雪景球已解析: {structure.size}")
        print(f"     非空气方块: {structure.block_count}")
        print(f"     调色板: {len(structure.palette)} 种方块")

        # ── 3.5a fill 命令验证 ──
        OFFSET = (300, 100, 300)
        print("\n--- 3.5a fill 命令验证 ---")
        try:
            await bedrock.run_command_ai(
                "fill ~5 ~0 ~5 ~7 ~0 ~7 minecraft:netherrack []",
                timeout=8.0, await_response=False, dimensional=True, bot_name=BOT_NAME,
            )
            await asyncio.sleep(1.0)
            r = await bedrock.run_command_ai(
                "testforblock ~5 ~0 ~5 minecraft:netherrack []",
                timeout=8.0, dimensional=True, bot_name=BOT_NAME,
            )
            print(f"  {'✅' if r is not None else '⚠️'} fill 验证{'通过' if r is not None else '无响应'}")
        except Exception as e:
            print(f"  ⚠️ fill 验证异常: {e}")

        # ── 3.5b 完整雪景球导入（合并 fill）──
        print(f"\n--- 3.5b 完整雪景球导入（合并 fill，偏移 {OFFSET}）---")
        fast_cmds = []
        def fast_cb(current, total, cmd_type, name):
            fast_cmds.append((current, cmd_type, name))
            if current <= 5 or current % 100 == 0 or current == total:
                print(f"  [{current}/{total}] {cmd_type} → {name}")

        t0 = time.time()
        try:
            covered = await bedrock.import_structure_fast(
                structure, offset=OFFSET, delay=0.03,
                progress_callback=fast_cb, bot_name=BOT_NAME,
            )
            elapsed = time.time() - t0
            print(f"  ✅ 完整导入完成: 覆盖 {covered} 块，{len(fast_cmds)} 条命令，耗时 {elapsed:.1f}s")
        except Exception as e:
            print(f"  ❌ 导入失败: {e}")
            import traceback; traceback.print_exc()
            await bedrock.close(); return

        # ── 3.6 验证方块（从建筑中取5个非空气方块验证）──
        print("\n--- 3.6 验证方块（testforblock 5个位置）---")
        await asyncio.sleep(2.0)
        verify_list = []
        air_id = structure.palette.air_id
        for y in range(SY):
            for z in range(SZ):
                for x in range(SX):
                    if len(verify_list) >= 5: break
                    bid = int(structure.region.blocks[x, y, z])
                    if bid != air_id:
                        blk = structure.palette.get(bid)
                        verify_list.append((x, y, z, blk.state.name))
                if len(verify_list) >= 5: break
            if len(verify_list) >= 5: break

        ok_count = 0
        for x, y, z, blk_name in verify_list:
            rel = f"~{x} ~{y} ~{z}"
            try:
                result = await bedrock.run_command_ai(
                    f"testforblock {rel} {blk_name} []",
                    timeout=8.0, dimensional=True, bot_name=BOT_NAME,
                )
                if result is not None:
                    print(f"  ✅ {rel} {blk_name}: 通过")
                    ok_count += 1
                else:
                    print(f"  ⚠️ {rel} {blk_name}: 无响应")
            except Exception as e:
                print(f"  ❌ {rel} {blk_name}: {e}")
        print(f"  验证结果: {ok_count}/{len(verify_list)} 通过")

        await bedrock.close()
    except Exception as e:
        print(f"  ❌ Bedrock 连接失败: {e}")
        import traceback
        traceback.print_exc()
        try:
            await bedrock.close()
        except Exception:
            pass
        return

    print("\n" + "=" * 70)
    print("🎉 端到端测试完成！")
    print("=" * 70)


if __name__ == "__main__":
    asyncio.run(main())

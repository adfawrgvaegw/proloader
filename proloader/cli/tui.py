"""proloader 交互式主菜单（对齐 fatalder cmd/cli/main.go）。

主菜单项（对齐 fatalder 6 项）：
    1. 开始构建建筑 → 选/创服务器配置 → 选建筑文件 → 选位置 → 执行
    2. 恢复中断的导入 → 列出 interrupted 会话 → 选会话 → 连接 → 从断点继续
    3. 开始导出建筑   （暂留 stub，后续实现）
    4. 任务组配置管理   （暂留 stub，后续实现）
    5. 配置管理 → 服务器配置管理 / 框架配置
    6. 建筑操作工具     （暂留 stub，后续实现）
    7. 退出

与原 argparse CLI 的关系：
    - `tui.py` 是交互式菜单（默认入口，无参数运行时）
    - `main.py` (argparse) 保留为脚本接口（CI / 高级用户）
"""

from __future__ import annotations

import asyncio
import json
import sys
from pathlib import Path
from typing import Any, List, Optional, Tuple

from proloader import __version__
from proloader.config import (
    COOKIE_FILE,
    ControlConfig,
    auto_refresh_cookie,
    check_cookie_valid,
    ensure_storage_dirs,
    list_server_configs,
    load_last_server,
    load_server_config,
    save_last_server,
    save_server_config,
)
from proloader.control import (
    BuildReport,
    BuildTask,
    Control,
    VERIFY_LEVEL_AIR,
    VERIFY_LEVEL_COMPLETE,
    VERIFY_LEVEL_NONE,
    VERIFY_LEVEL_SIMILAR,
    list_interrupted_sessions,
    list_all_sessions,
    delete_session,
    load_any_structure,
)


# ═══════════════════════════════════════════════════════════════════════════════
# 简易 TUI 原语（不依赖外部库，保证零依赖）
# ═══════════════════════════════════════════════════════════════════════════════
_COLORS = {
    "reset": "\033[0m", "red": "\033[31m", "green": "\033[32m",
    "yellow": "\033[33m", "blue": "\033[34m", "cyan": "\033[36m",
    "bold": "\033[1m", "dim": "\033[2m",
}


def _c(text: str, color: str) -> str:
    return f"{_COLORS.get(color, '')}{text}{_COLORS['reset']}"


def _print(msg: str = "") -> None:
    print(msg, flush=True)


def _input(prompt: str = "") -> str:
    try:
        return input(prompt).strip()
    except EOFError:
        return ""


def _select(prompt: str, options: List[str], default: Optional[int] = None) -> int:
    """显示选项菜单，返回选中的索引。Ctrl+C / 空输入 → default 或 退出。"""
    _print(_c(prompt, "cyan"))
    for i, opt in enumerate(options):
        marker = _c(f"  [{i+1}]", "yellow")
        suffix = _c("  (默认)", "dim") if default == i else ""
        _print(f"{marker} {opt}{suffix}")
    while True:
        raw = _input(_c("请选择编号: ", "bold"))
        if raw == "" and default is not None:
            _print(_c(f"  → 使用默认: {options[default]}", "dim"))
            return default
        try:
            idx = int(raw) - 1
            if 0 <= idx < len(options):
                return idx
        except ValueError:
            pass
        if raw.lower() in ("q", "quit", "exit"):
            return -1
        _print(_c("  ⚠️  无效输入，请输入编号 (q 取消)", "red"))


def _input_str(prompt: str, default: str = "", allow_empty: bool = False) -> str:
    while True:
        suffix = f" (默认: {default})" if default else ""
        raw = _input(_c(f"{prompt}{suffix}: ", "bold"))
        if raw:
            return raw
        if default:
            return default
        if allow_empty:
            return ""
        _print(_c("  ⚠️  不能为空", "red"))


def _input_int(prompt: str, default: Optional[int] = None) -> int:
    while True:
        suffix = f" (默认: {default})" if default is not None else ""
        raw = _input(_c(f"{prompt}{suffix}: ", "bold"))
        if raw == "" and default is not None:
            return default
        try:
            return int(raw)
        except ValueError:
            _print(_c("  ⚠️  必须是整数", "red"))


def _confirm(prompt: str, default: bool = True) -> bool:
    marker = "[Y/n]" if default else "[y/N]"
    while True:
        raw = _input(_c(f"{prompt} {marker}: ", "bold")).lower()
        if raw in ("y", "yes"): return True
        if raw in ("n", "no"): return False
        if raw == "": return default
        _print(_c("  ⚠️  请输入 y 或 n", "red"))


def _pwd(prompt: str) -> str:
    """读取密码，不回显。"""
    import getpass
    try:
        return getpass.getpass(_c(f"{prompt}: ", "bold"))
    except (EOFError, getpass.GetPassWarning):
        return ""


# ═══════════════════════════════════════════════════════════════════════════════
# 主入口
# ═══════════════════════════════════════════════════════════════════════════════
def run_tui() -> None:
    """TUI 主循环。"""
    ensure_storage_dirs()
    _show_banner()
    # 启动时自动刷新 Cookie
    _auto_refresh_cookie_step()

    try:
        while True:
            idx = _show_main_menu()
            if idx < 0:
                break
            if idx == 0:
                asyncio.run(_handle_start_build())
            elif idx == 1:
                asyncio.run(_handle_resume_import())
            elif idx == 2:
                _print("\n" + _c("ℹ️  导出功能（从租赁服导出建筑为 .mcstructure）后续版本实现。", "yellow"))
            elif idx == 3:
                _print("\n" + _c("ℹ️  任务组配置管理（多建筑批量导入）后续版本实现。", "yellow"))
            elif idx == 4:
                _handle_manage_config()
            elif idx == 5:
                _print("\n" + _c("ℹ️  建筑操作工具（修补/填充/清理等）后续版本实现。", "yellow"))
            elif idx == 6:
                _print(_c("\n👋 再见！", "green"))
                return
            _print()
    except KeyboardInterrupt:
        _print(_c("\n\n👋 已中断，再见。", "yellow"))


# ═══════════════════════════════════════════════════════════════════════════════
# Banner + 主菜单
# ═══════════════════════════════════════════════════════════════════════════════
def _show_banner() -> None:
    _print()
    _print(_c("═" * 70, "blue"))
    _print(_c(f"  proloader v{__version__}  —  网易租赁服开源建筑导入器", "bold"))
    _print(_c("  (学习用途 | 对齐 fatalder CLI 界面 | 底层使用 proloader 开源实现)", "dim"))
    _print(_c("═" * 70, "blue"))
    _print()


def _show_main_menu() -> int:
    # 检查是否有中断的导入会话
    interrupted = list_interrupted_sessions()
    resume_label = "恢复中断的导入"
    if interrupted:
        resume_label += f"（{len(interrupted)} 个待恢复）"

    options = [
        "开始构建（导入建筑到租赁服）",
        resume_label,
        "开始导出（从租赁服导出建筑）",
        "任务组配置管理",
        "配置管理（服务器配置 / 框架配置）",
        "建筑操作工具",
        "退出",
    ]
    return _select("=== 主菜单 ===", options)


# ═══════════════════════════════════════════════════════════════════════════════
# 菜单 4: 配置管理
# ═══════════════════════════════════════════════════════════════════════════════
def _handle_manage_config() -> None:
    idx = _select(
        "\n=== 配置管理 ===",
        ["服务器配置管理", "框架配置 / 自动 Cookie"],
    )
    if idx == 0:
        _manage_server_configs()
    elif idx == 1:
        _show_framework_config()


def _manage_server_configs() -> None:
    options = [
        "创建新服务器配置",
        "选择已有的服务器配置",
        "查看所有已保存的服务器配置",
        "返回",
    ]
    while True:
        idx = _select("\n=== 服务器配置管理 ===", options)
        if idx == 0:
            cfg = _create_server_config(remember=False)
            if cfg is not None:
                _save_server_config_dialog(cfg)
        elif idx == 1:
            cfg = _select_existing_server_config()
            if cfg is not None:
                _print(_c(f"\n✅ 已加载: rental_server_code={cfg.rental_server_code}", "green"))
        elif idx == 2:
            _list_all_server_configs()
        else:
            return


def _create_server_config(*, remember: bool) -> Optional[ControlConfig]:
    code = _input_str("租赁服编号")
    if not code:
        return None
    password = _pwd("租赁服密码（空则无密码）")
    cfg = ControlConfig(
        rental_server_code=code,
        rental_server_passcode=password,
    )
    if remember:
        save_last_server(cfg)
    return cfg


def _save_server_config_dialog(cfg: ControlConfig) -> None:
    if _confirm("保存这个服务器配置？", True):
        default_name = cfg.rental_server_code or "server"
        name = _input_str("配置文件名", default_name)
        path = save_server_config(cfg, name)
        _print(_c(f"✅ 已保存: {path}", "green"))


def _select_existing_server_config() -> Optional[ControlConfig]:
    names = list_server_configs()
    if not names:
        _print(_c("⚠️  没有已保存的服务器配置", "yellow"))
        return None
    names = sorted(names)
    idx = _select(f"选择服务器配置（共 {len(names)} 个）", names)
    if idx < 0 or idx >= len(names):
        return None
    cfg = load_server_config(names[idx])
    if cfg is None:
        _print(_c("❌ 加载失败", "red"))
        return None
    save_last_server(cfg)
    return cfg


def _list_all_server_configs() -> None:
    names = list_server_configs()
    if not names:
        _print(_c("⚠️  没有已保存的服务器配置", "yellow"))
        return
    _print(_c(f"\n共 {len(names)} 个服务器配置:", "bold"))
    for n in sorted(names):
        cfg = load_server_config(n)
        if cfg is None: continue
        pwd = "✅" if cfg.rental_server_passcode else _c("(空)", "dim")
        _print(f"  · {n:30s}  服号={cfg.rental_server_code}  密码={pwd}")


def _show_framework_config() -> None:
    _print("\n=== 框架配置 / 自动 Cookie ===")
    options = [
        "立即刷新 Cookie（自动提取）",
        "查看当前 Cookie 状态",
        "查看 Cookie 文件路径",
        "返回",
    ]
    while True:
        idx = _select("\n框架配置", options)
        if idx == 0:
            _auto_refresh_cookie_step(force=True)
        elif idx == 1:
            _check_cookie_status()
        elif idx == 2:
            _print(_c(f"\n  Cookie 文件: {COOKIE_FILE}", "cyan"))
        else:
            return


def _auto_refresh_cookie_step(*, force: bool = False) -> None:
    """启动时/手动触发的自动 Cookie 刷新。"""
    if not force:
        _print(_c("\n[Cookie] 正在从网易客户端沙盒自动提取登录信息...", "dim"))
    try:
        cookie = auto_refresh_cookie()
    except Exception as e:
        _print(_c(f"  ⚠️  Cookie 刷新异常: {e}", "red"))
        return
    ok, fields = check_cookie_valid(cookie)
    sauth_len = len(cookie.sauth_json)
    if ok:
        _print(_c(f"  ✅ Cookie OK (sauth_json {sauth_len} chars)", "green"))
        if force:
            _print(_c(f"     关键字段: {fields}", "dim"))
    else:
        if sauth_len == 0:
            _print(_c("  ⚠️  无法从客户端提取 Cookie。请先：", "yellow"))
            _print(_c("     1. 在 Mac 上打开『我的世界』App 并登录网易账号", "yellow"))
            _print(_c("     2. 进入租赁服一次（触发 sauth_json 写入 Cache.db）", "yellow"))
            _print(_c("     3. 回到这里选择“立即刷新 Cookie”", "yellow"))
        else:
            _print(_c(f"  ⚠️  Cookie 字段不完整: {fields}", "yellow"))


def _check_cookie_status() -> None:
    from proloader.config import CookieData
    if not COOKIE_FILE.exists():
        _print(_c("  ⚠️  Cookie 文件不存在（尚未登录过）", "yellow"))
        return
    try:
        cookie = CookieData.from_json(COOKIE_FILE.read_text())
    except Exception as e:
        _print(_c(f"  ❌ Cookie 文件损坏: {e}", "red"))
        return
    ok, fields = check_cookie_valid(cookie)
    _print(_c(f"\n  sauth_json: {len(cookie.sauth_json)} 字符", "cyan"))
    _print(f"  字段完整性: {_c('✅' if ok else '❌', 'green' if ok else 'red')}")
    for k, v in fields.items():
        mark = _c("✅", "green") if v else _c("❌", "red")
        _print(f"    {k:18s}: {mark}")


# ═══════════════════════════════════════════════════════════════════════════════
# 菜单 2: 恢复中断的导入（断点续传）
# ═══════════════════════════════════════════════════════════════════════════════
async def _handle_resume_import() -> None:
    """恢复中断的导入。"""
    _print(_c("\n" + "=" * 70, "blue"))
    _print(_c("  恢复中断的导入（断点续传）", "bold"))
    _print(_c("=" * 70, "blue"))

    sessions = list_interrupted_sessions()
    if not sessions:
        _print(_c("\nℹ️  没有中断的导入会话。", "yellow"))
        return

    _print(f"\n找到 {len(sessions)} 个中断的导入会话：\n")
    for i, s in enumerate(sessions):
        progress = 100.0 * s.sent_commands / max(1, s.total_commands)
        fname = Path(s.file_path).name
        _print(f"  [{i}] {s.session_id}  {fname}")
        _print(f"      进度: {s.sent_commands}/{s.total_commands} ({progress:.0f}%)"
               f"  位置: {s.start_pos}")
        _print(f"      更新: {s.updated_at[:19]}")
        _print()

    choice = _select("选择要恢复的会话", [f"{s.session_id} ({Path(s.file_path).name})"
                                            for s in sessions] + ["返回"])
    if choice < 0 or choice >= len(sessions):
        return

    session = sessions[choice]
    _print(f"\n恢复会话 {session.session_id}...")

    # 选服务器配置
    server_cfg = _select_or_create_server_config()
    if server_cfg is None:
        return

    # 连接租赁服
    ctl = Control(server_cfg)
    try:
        _print(_c("\n正在连接租赁服...", "dim"))
        rep = await ctl.bot_online()
        _print(_c(f"✅ 假人 {rep.user_name} 已上线", "green"))

        # 恢复导入
        _print(_c(f"\n开始恢复导入 ({session.sent_commands}/{session.total_commands} 已发送)...", "dim"))
        report = await ctl.resume_import(session.session_id)
        _print(_c("\n✅ 恢复完成！", "green"))
        _print(report.summary())

    except KeyboardInterrupt:
        _print(_c("\n\n⚠️  再次中断！进度已保存，可下次继续恢复。", "yellow"))
    except Exception as e:
        _print(_c(f"\n❌ 恢复失败: {e}", "red"))
    finally:
        try:
            await ctl.bot_offline()
        except Exception:
            pass


# ═══════════════════════════════════════════════════════════════════════════════
# 菜单 1: 开始构建
# ═══════════════════════════════════════════════════════════════════════════════
async def _handle_start_build() -> None:
    """完整的建筑导入流程（对齐 fatalder handleStartBuild）。"""
    _print(_c("\n" + "=" * 70, "blue"))
    _print(_c("  开始构建  —  建筑导入流程", "bold"))
    _print(_c("=" * 70, "blue"))

    # Step 1: 选服务器配置
    _print("\n" + _c("── 步骤 1/5: 选择服务器配置 ──", "cyan"))
    cfg: Optional[ControlConfig] = None
    last_cfg = load_last_server()
    if last_cfg and last_cfg.rental_server_code:
        _print(f"  检测到上次使用的服务器: {last_cfg.rental_server_code}")
        if _confirm("使用上次的服务器配置？", True):
            cfg = last_cfg
    if cfg is None:
        opts = ["创建新服务器配置", "从已保存的配置选择"]
        idx = _select("选择服务器配置方式", opts)
        if idx == 0:
            cfg = _create_server_config(remember=True)
            if cfg is not None and _confirm("保存这个配置以便复用？", True):
                _save_server_config_dialog(cfg)
        elif idx == 1:
            cfg = _select_existing_server_config()
    if cfg is None or not cfg.rental_server_code:
        _print(_c("❌ 未选择有效服务器配置，已取消", "red"))
        return

    # Step 2: 选建筑文件
    _print("\n" + _c("── 步骤 2/5: 选择建筑文件 ──", "cyan"))
    file_path = _select_building_file()
    if not file_path:
        _print(_c("❌ 未选择建筑文件，已取消", "red"))
        return
    try:
        st = load_any_structure(file_path)
    except Exception as e:
        _print(_c(f"❌ 解析建筑失败: {e}", "red"))
        return
    _print(_c(f"  ✅ 加载成功: {st.size[0]}×{st.size[1]}×{st.size[2]}, "
              f"{st.block_count} 方块, {len(st.palette)} 种", "green"))

    # Step 3: 选起始位置 + 速度 + 校验等级
    _print("\n" + _c("── 步骤 3/5: 配置导入参数 ──", "cyan"))
    pos = _input_position()
    speed = _input_int("目标速度（块/秒，5000 适合一般服）", 5000)
    levels = [
        f"Level 1: 不校验 ({VERIFY_LEVEL_NONE})",
        f"Level 2: 空气级——只抽样关键非空气（推荐，最平衡） ({VERIFY_LEVEL_AIR})",
        f"Level 3: 相似级——抽样 10% 随机方块 ({VERIFY_LEVEL_SIMILAR})",
        f"Level 4: 完全级——逐方块校验（非常慢） ({VERIFY_LEVEL_COMPLETE})",
    ]
    lv_idx = _select("完整性校验等级", levels, default=1)
    verify_level = [VERIFY_LEVEL_NONE, VERIFY_LEVEL_AIR, VERIFY_LEVEL_SIMILAR, VERIFY_LEVEL_COMPLETE][lv_idx]

    # Step 4: 确认
    _print("\n" + _c("── 步骤 4/5: 配置确认 ──", "cyan"))
    _print(f"  服务器:   {cfg.rental_server_code}")
    _print(f"  建筑:     {Path(file_path).name} ({st.block_count} 方块)")
    _print(f"  位置:     ({pos[0]}, {pos[1]}, {pos[2]})")
    _print(f"  速度:     {speed} 块/s")
    _print(f"  校验:     Level {verify_level}")
    if not _confirm("开始执行？", True):
        _print(_c("已取消", "yellow"))
        return

    # Step 5: 执行连接 + 导入
    _print("\n" + _c("── 步骤 5/5: 执行 ──", "cyan"))

    # 5a: 确保 cookie 有效
    from proloader.config import auto_refresh_cookie, check_cookie_valid
    cookie = auto_refresh_cookie()
    ok, _fields = check_cookie_valid(cookie)
    if not ok:
        _print(_c("❌ Cookie 无效，请先在网易客户端登录并进入租赁服后再试。", "red"))
        return

    # 5b: 连接
    _print(_c("\n[1/3] 连接租赁服 + Bedrock 登录...", "cyan"))
    ctl = Control(cfg)
    user_name = ""
    try:
        rep = await ctl.enter_rental_server(cookie)
        user_name = rep.user_name
        _print(_c(f"  ✅ 连接成功! 假人: {user_name}, runtime_id={rep.runtime_id}, "
                  f"耗时 {rep.elapsed_sec:.1f}s", "green"))
    except Exception as e:
        _print(_c(f"  ❌ 连接失败: {e}", "red"))
        import traceback; traceback.print_exc()
        return

    # 5c: 导入
    _print(_c("\n[2/3] 执行建筑导入（合并 fill 算法）...", "cyan"))
    def prog_cb(cur, total, cmd_type, name):
        pct = 100.0 * cur / max(1, total)
        mark = _c(f"[{cur}/{total}]", "yellow")
        if cur <= 5 or cur % max(1, total // 10) == 0 or cur == total:
            _print(f"  {mark} {pct:5.1f}%  {cmd_type:8s} → {name}")

    task = BuildTask(
        file_path=file_path, start_pos=pos,
        speed=speed, verify_level=verify_level,
        progress_cb=prog_cb,
    )
    try:
        report: BuildReport = await task.run(ctl.bedrock_session, bot_name=user_name)
    except Exception as e:
        _print(_c(f"  ❌ 导入失败: {e}", "red"))
        import traceback; traceback.print_exc()
        try:
            await ctl.close()
        except Exception:
            pass
        return

    # 5d: 结果
    _print(_c("\n[3/3] 导入完成!", "cyan"))
    _print("-" * 60)
    _print(report.summary())
    _print("-" * 60)
    if report.verify.get("failed", 0) > 0:
        _print(_c("  ⚠️  校验发现失败方块，建议提升 Level 到 4 再次执行或手动检查。", "yellow"))
    else:
        _print(_c("  ✅ 校验通过。", "green"))

    try:
        await ctl.close()
    except Exception:
        pass
    _print(_c("\n👷 构建流程结束。", "green"))


def _select_building_file() -> Optional[str]:
    """建筑文件选择：先列 buildings/ 目录 + 当前目录的 .mcstructure/.schem。"""
    candidates: List[Path] = []
    roots = [
        Path.cwd(),
        Path.cwd() / "buildings",
        Path.home() / ".proloader" / "buildings",
        Path(__file__).resolve().parents[2] / "buildings",
    ]
    for root in roots:
        if not root.exists():
            continue
        for p in sorted(root.iterdir()):
            if p.suffix.lower() in (".bdx", ".mcstructure", ".schem", ".schematic", ".litematic"):
                if p not in candidates:
                    candidates.append(p)
    opts = [f"{p.name}  ({p.stat().st_size // 1024} KB)  [{p.parent}]" for p in candidates]
    opts.append("手动输入路径...")
    idx = _select("选择建筑文件", opts)
    if idx < 0:
        return None
    if idx == len(candidates):
        path = _input_str("建筑文件绝对/相对路径")
        return path if path else None
    if idx < len(candidates):
        return str(candidates[idx])
    return None


def _input_position() -> Tuple[int, int, int]:
    _print(_c("  输入建筑放置原点（世界坐标），例如 200,100,200", "dim"))
    default = "200,100,200"
    while True:
        raw = _input_str("位置 (x,y,z)", default=default)
        parts = [p.strip() for p in raw.split(",")]
        if len(parts) == 3:
            try:
                return (int(parts[0]), int(parts[1]), int(parts[2]))
            except ValueError:
                pass
        _print(_c("  ⚠️  格式应为 x,y,z（三个整数，逗号分隔）", "red"))


# ═══════════════════════════════════════════════════════════════════════════════
# 直接运行入口
# ═══════════════════════════════════════════════════════════════════════════════
def run_tui():
    """Compatibility entry: the current two-action wizard lives in wizard.py."""
    from proloader.cli.wizard import run_tui as current
    return current()


if __name__ == "__main__":
    run_tui()

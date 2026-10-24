"""proloader 命令行入口（argparse 实现，无额外依赖）。

命令：
    proloader import <file> --to x,y,z    导入建筑
    proloader diff <file> --to x,y,z      仅查看差异（不放置）
    proloader resume <session-id>         断点续传
    proloader rollback <session-id>       回滚
    proloader list                        列出会话
    proloader preview <file>              启动 Web 可视化预览
    proloader gen-example <file>          生成示例建筑文件
"""

from __future__ import annotations

import argparse
import asyncio
import sys
from pathlib import Path
from typing import List, Tuple

from proloader import __version__


def _parse_coord(s: str) -> Tuple[int, int, int]:
    """解析 'x,y,z' 坐标。"""
    parts = s.split(",")
    if len(parts) != 3:
        raise argparse.ArgumentTypeError(f"坐标格式应为 x,y,z，得到 {s}")
    return (int(parts[0]), int(parts[1]), int(parts[2]))


def _print(msg: str) -> None:
    print(msg, flush=True)


# ── import 命令 ───────────────────────────────────────────────────────────────
async def _cmd_import(args: argparse.Namespace) -> None:
    from proloader.backend.simulated import SimulatedWorld
    from proloader.parser import load_structure
    from proloader.pipeline import make_simulated_backends, run_import
    from proloader.session import ImportOptions, ImportSession
    from proloader.snapshot.checkpoint import CheckpointManager

    structure = load_structure(args.file)
    _print(f"建筑: {structure}")

    world = SimulatedWorld(size=tuple(args.world_size))
    backends = make_simulated_backends(args.bots, world, place_delay=args.place_delay)

    cm = CheckpointManager()
    options = ImportOptions(
        num_bots=args.bots,
        cell_size=args.cell_size,
        use_diff=not args.no_diff,
        include_removes=not args.no_removes,
        place_delay=args.place_delay,
    )
    session = ImportSession(
        structure=structure,
        origin=args.to,
        backends=backends,
        options=options,
        checkpoint_manager=cm,
    )

    _print(f"\n开始导入（{args.bots} 假人, 差异={'开' if not args.no_diff else '关'}）...")
    report = await run_import(session)
    _print("\n" + "=" * 60)
    _print(report.summary())
    _print("=" * 60)
    _print(f"会话 ID: {report.session_id}")
    _print(f"  resume:  proloader resume {report.session_id}")
    _print(f"  rollback: proloader rollback {report.session_id}")


# ── diff 命令 ─────────────────────────────────────────────────────────────────
async def _cmd_diff(args: argparse.Namespace) -> None:
    from proloader.backend.simulated import SimulatedBackend, SimulatedWorld
    from proloader.diff import DiffEngine, DiffPlanner, RegionScanner, PlannerOptions
    from proloader.parser import load_structure

    structure = load_structure(args.file)
    _print(f"建筑: {structure}")

    world = SimulatedWorld(size=tuple(args.world_size))
    bot = SimulatedBackend(world, "scanner")
    await bot.connect()

    structure_region = structure.to_region(args.to)
    scanner = RegionScanner()
    target = await scanner.scan(bot, structure_region.bbox)

    if target is None:
        _print("后端不支持读区域，无法计算差异")
        return

    plan = DiffEngine().compute(target, structure_region)
    _print("\n" + "=" * 60)
    _print("差异分析结果（创新点1）")
    _print("=" * 60)
    _print(plan.stats.summary())
    _print(f"\nADD(新增): {plan.stats.adds}")
    _print(f"CHANGE(替换): {plan.stats.changes}")
    _print(f"REMOVE(破坏): {plan.stats.removes}")
    _print(f"SKIP(跳过): {plan.stats.skipped}")
    _print(f"节省比例: {plan.stats.saved_ratio:.1%}")


# ── resume 命令 ──────────────────────────────────────────────────────────────
async def _cmd_resume(args: argparse.Namespace) -> None:
    from proloader.parser import load_structure
    from proloader.pipeline import make_simulated_backends, resume_import
    from proloader.session import ImportOptions
    from proloader.snapshot.checkpoint import CheckpointManager

    cm = CheckpointManager()
    meta = cm.load_meta(args.session_id)
    _print(f"恢复会话 {args.session_id}: {meta.structure_name}, 状态 {meta.status.value}")

    structure = load_structure(args.file)
    world = SimulatedWorld(size=tuple(args.world_size))
    backends = make_simulated_backends(meta.num_bots, world)
    options = ImportOptions(num_bots=meta.num_bots, cell_size=16)

    from proloader.backend.simulated import SimulatedWorld
    report = await resume_import(
        args.session_id, structure, tuple(meta.origin), backends, options, cm
    )
    _print("\n" + "=" * 60)
    _print(report.scheduler_report.summary())
    _print("=" * 60)


# ── rollback 命令 ────────────────────────────────────────────────────────────
async def _cmd_rollback(args: argparse.Namespace) -> None:
    from proloader.backend.simulated import SimulatedBackend, SimulatedWorld
    from proloader.pipeline import rollback_session
    from proloader.snapshot.checkpoint import CheckpointManager

    cm = CheckpointManager()
    meta = cm.load_meta(args.session_id)
    _print(f"回滚会话 {args.session_id}: {meta.structure_name}")

    world = SimulatedWorld(size=tuple(args.world_size))
    bot = SimulatedBackend(world, "rollback-bot")
    report = await rollback_session(args.session_id, bot, cm)
    _print("\n" + "=" * 60)
    _print(report.summary())
    _print("=" * 60)


# ── list 命令 ────────────────────────────────────────────────────────────────
def _cmd_list(args: argparse.Namespace) -> None:
    from proloader.snapshot.checkpoint import CheckpointManager

    cm = CheckpointManager()
    sessions = cm.list_sessions()
    if not sessions:
        _print("暂无会话")
        return
    _print(f"共 {len(sessions)} 个会话:")
    _print("-" * 70)
    for m in sessions:
        _print(f"  {m.session_id}  {m.status.value:10}  {m.structure_name:20}  ops={m.total_ops}")


# ── preview 命令 ─────────────────────────────────────────────────────────────
async def _cmd_preview(args: argparse.Namespace) -> None:
    from proloader.parser import load_structure
    structure = load_structure(args.file)
    _print(f"建筑: {structure}")
    try:
        from proloader.web.app import run_web
        await run_web(structure, host=args.host, port=args.port)
    except ImportError:
        _print("Web 可视化需要安装 web 依赖: pip install -e \".[web]\"")
        _print(f"建筑摘要: {structure.summary()}")


# ── gen-example 命令 ─────────────────────────────────────────────────────────
def _cmd_gen_example(args: argparse.Namespace) -> None:
    from proloader.examples.gen_buildings import generate_house
    path = Path(args.output)
    generate_house(path)
    _print(f"示例建筑已生成: {path}")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="proloader",
        description="世界最好的网易我的世界租赁服建筑导入器（学习用途）",
    )
    parser.add_argument("-V", "--version", action="version", version=f"proloader {__version__}")
    sub = parser.add_subparsers(dest="command", required=True)

    # import
    p = sub.add_parser("import", help="导入建筑到（模拟）世界")
    p.add_argument("file", help="建筑文件路径（.mcstructure）")
    p.add_argument("--to", type=_parse_coord, required=True, help="放置原点 x,y,z")
    p.add_argument("--backend", default="simulated", help="后端类型（默认 simulated）")
    p.add_argument("--bots", type=int, default=3, help="假人数量（默认 3）")
    p.add_argument("--cell-size", type=int, default=16, help="切块大小（默认 16）")
    p.add_argument("--no-diff", action="store_true", help="禁用差异增量导入")
    p.add_argument("--no-removes", action="store_true", help="不执行破坏操作")
    p.add_argument("--place-delay", type=float, default=0.0, help="模拟放置延迟秒数")
    p.add_argument("--world-size", type=int, nargs=3, default=[256, 256, 256], help="模拟世界大小")
    p.set_defaults(func=_cmd_import, is_async=True)

    # diff
    p = sub.add_parser("diff", help="仅查看差异（不放置）")
    p.add_argument("file", help="建筑文件路径")
    p.add_argument("--to", type=_parse_coord, required=True, help="放置原点 x,y,z")
    p.add_argument("--world-size", type=int, nargs=3, default=[256, 256, 256])
    p.set_defaults(func=_cmd_diff, is_async=True)

    # resume
    p = sub.add_parser("resume", help="断点续传")
    p.add_argument("session_id", help="会话 ID")
    p.add_argument("file", help="建筑文件路径（需与首次一致）")
    p.add_argument("--world-size", type=int, nargs=3, default=[256, 256, 256])
    p.set_defaults(func=_cmd_resume, is_async=True)

    # rollback
    p = sub.add_parser("rollback", help="回滚到快照")
    p.add_argument("session_id", help="会话 ID")
    p.add_argument("--world-size", type=int, nargs=3, default=[256, 256, 256])
    p.set_defaults(func=_cmd_rollback, is_async=True)

    # list
    p = sub.add_parser("list", help="列出所有会话")
    p.set_defaults(func=_cmd_list, is_async=False)

    # preview
    p = sub.add_parser("preview", help="启动 Web 可视化预览")
    p.add_argument("file", help="建筑文件路径")
    p.add_argument("--host", default="127.0.0.1")
    p.add_argument("--port", type=int, default=8000)
    p.set_defaults(func=_cmd_preview, is_async=True)

    # gen-example
    p = sub.add_parser("gen-example", help="生成示例建筑文件")
    p.add_argument("output", help="输出路径（.mcstructure）")
    p.set_defaults(func=_cmd_gen_example, is_async=False)

    return parser


def app() -> None:
    """CLI 主入口（pyproject [project.scripts] 指向此处）。

    无参数时启动交互式 TUI（对齐 fatalder 主菜单）；
    有参数时走 argparse 模式。
    """
    if len(sys.argv) == 1:
        # 无参数 → 启动 TUI 交互菜单
        from proloader.cli.wizard import run_tui
        run_tui()
        return

    parser = build_parser()
    args = parser.parse_args()
    if not hasattr(args, "func"):
        parser.print_help()
        sys.exit(1)
    if getattr(args, "is_async", False):
        asyncio.run(args.func(args))
    else:
        args.func(args)


if __name__ == "__main__":
    app()

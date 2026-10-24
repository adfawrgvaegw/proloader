"""Generate an offline, relocatable deployment bundle for the user's final menu.

Run from the project root: python3 -m tools.package_snow_menu --help
No connection, credentials or world changes are made by this command.
"""
import argparse
import copy
import hashlib
import json
from pathlib import Path

from proloader.ai.program import compile_program, validate

ROOT = Path(__file__).resolve().parents[1]
TEMPLATE = ROOT / 'templates/snow_menu_v3/program.json'
HOME_COMMAND = 'tp @a[tag=!pl_sm_bot,scores={菜单=1},rx=-89] -68 118 246'


def bundle(server, home, origin, template=TEMPLATE):
    if not str(server).isascii() or not str(server).isdigit():
        raise ValueError('服务器编号必须为数字')
    data = json.loads(Path(template).read_text())
    program = copy.deepcopy(data)
    program['origin'] = list(origin)
    if program.get('dimension') != 'overworld':
        raise ValueError('此菜单模板只支持主世界部署')
    changed = 0
    for block in program['blocks']:
        if HOME_COMMAND in block['command']:
            block['command'] = block['command'].replace(
                HOME_COMMAND, HOME_COMMAND.rsplit(' ', 3)[0] + ' ' + ' '.join(map(str, home)))
            changed += 1
    if changed != 1:
        raise ValueError('模板必须有且只有一条已识别的回城命令；新菜单须先整理模板')
    program = validate(program)
    x, y, z = origin
    if y < -63:
        raise ValueError('指令区下方须留一层平台空间')
    if len(home) != 3 or any(type(v) is not int for v in home) or not -64 <= home[1] <= 319:
        raise ValueError('主城必须为主世界合法整数坐标')
    maxx = x + max(b['at'][0] for b in program['blocks'])
    maxz = z + max(b['at'][2] for b in program['blocks'])
    if any(abs(v) > 29_999_000 for v in (*home, x, z, maxx, maxz)):
        raise ValueError('坐标超过世界边界')
    # A platform immediately below all chains. Keep mode never clears terrain.
    platform = f'fill {x} {y-1} {z} {maxx} {y-1} {maxz} smooth_stone 0 keep'
    ticking = f'tickingarea add {x} {y} {z} {maxx} {y} {maxz} proloader_snow_menu true'
    setup = [f'scoreboard objectives add {name} dummy'
             for name in ('菜单', '互传', 'dh', 'dh1', 'plsm_state')]
    manifest = dict(format='proloader-snow-menu-install-v1', server=str(server),
        home=list(home), origin=list(origin), dimension='overworld', blocks=len(program['blocks']),
        template_sha256=hashlib.sha256(Path(template).read_bytes()).hexdigest(),
        platform_bounds=[[x, y-1, z], [maxx, y-1, maxz]],
        setup=setup,
        prepare=['scoreboard players set plsm_enabled plsm_state 0',
                 'tag @s add pl_sm_bot',
                 'gamerule commandblockoutput false',
                 'gamerule sendcommandfeedback false',
                 'gamerule commandblocksenabled true', ticking],
        platform=platform,
        start=['scoreboard players set plsm_enabled plsm_state 1'],
        stop=['scoreboard players set plsm_enabled plsm_state 0'],
        workflow=str(ROOT / 'docs/SNOW_MENU_INSTALL.md'))
    return program, manifest


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--server', required=True)
    parser.add_argument('--home', type=int, nargs=3, required=True, metavar=('X', 'Y', 'Z'))
    parser.add_argument('--origin', type=int, nargs=3, default=[12000, 280, 12000])
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args(argv)
    program, manifest = bundle(args.server, args.home, args.origin)
    args.output.mkdir(parents=True, exist_ok=False)
    for name, value in [('snow-menu.ai.json', program), ('install.json', manifest)]:
        (args.output / name).write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')
    compile_program(program, args.output / 'snow-menu.mcstructure')
    print(json.dumps(dict(output=str(args.output.resolve()), blocks=len(program['blocks']),
                         online=False), ensure_ascii=False))


if __name__ == '__main__':
    main()

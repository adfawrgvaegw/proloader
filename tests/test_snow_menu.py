import json
from tools.build_snow_menu import make_program,write_program
from proloader.parser import load_structure
from proloader.command_blocks import plan_command_blocks

def source():
 return '作者：肆陵\nexecute as @a[scores={菜单=1..6}] at @s run titleraw @s actionbar '+json.dumps({'rawtext':[{'text':'菜单'},{'score':{'name':'@s','objective':'在线时间'}},{'selector':'@s[scores={菜单=2}]'}]},ensure_ascii=False)

def test_program_layout_chain_faces_next_and_groups_cannot_leak():
 p=make_program(source(),home=(10,80,-10))
 assert len({tuple(e['position']) for e in p['entries']})==len(p['entries'])
 for group in ('main','seconds','minutes'):
  entries=[e for e in p['entries'] if e['group']==group]
  assert entries[0]['block']=='minecraft:repeating_command_block'
  for a,b in zip(entries,entries[1:]):
   dx,dy,dz={3:(0,0,1),4:(-1,0,0),5:(1,0,0)}[a['facing']]
   assert [a['position'][0]+dx,a['position'][1]+dy,a['position'][2]+dz]==b['position']
  assert all(e['command'].startswith('execute if score plsm_enabled plsm_state matches 1 run ') for e in entries)
 assert [e['delay'] for e in p['entries'] if e['index']==0]==[0,20,1200]
 assert p['disable']=='scoreboard players set plsm_enabled plsm_state 0'
 assert any('tp @a[tag=!pl_sm_bot,scores={plsm_menu=1},rx=-89] 10 80 -10' in e['command'] for e in p['entries'])
 assert not any('scores={plsm_menu=1..,plsm_menu=' in e['command'] for e in p['entries'])

def test_artifact_preserves_each_instruction_and_source_activation(tmp_path):
 p=make_program(source());write_program(p,tmp_path)
 s=load_structure(tmp_path/'snow-menu.mcstructure')
 ops=plan_command_blocks(s,tuple(p['origin']))
 assert len(ops)==len(p['entries'])
 by_position={o.position:o for o in ops}
 for e in p['entries']:
  o=by_position[tuple(e['position'])]
  assert o.command==e['command'] and o.tick_delay==e['delay'] and not o.needs_redstone
 assert (tmp_path/'stop.mcfunction').read_text().strip()==p['disable']

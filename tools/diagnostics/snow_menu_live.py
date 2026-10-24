"""Focused live probes called by the single-owner local session controller."""
import asyncio,base64,json,time
from pathlib import Path
from proloader.bots import private_json

async def run(session,name,request):
 action=request.get('action','inspect')
 if action=='deploy':return await deploy(session,name,request)
 if action=='inspect':
  original=session._dispatch_packet;packets=[]
  def dispatch(packet):
   pid,_=session._read_packet_id(packet)
   if pid==174:packets.append(packet)
   return original(packet)
  session._dispatch_packet=dispatch
  try:
   try:nbt=await session.read_block_entity(tuple(request['position']),timeout=12);error=None
   except Exception as exc:nbt=None;error=dict(type=type(exc).__name__,message=str(exc))
  finally:session._dispatch_packet=original
  private_json(Path('cache/snow_subchunk_probe.json'),dict(packets=[base64.b64encode(p).decode() for p in packets],error=error,nbt=nbt))
  return dict(captured=[dict(size=len(p),prefix=p[:60].hex()) for p in packets],error=error,nbt=nbt)
 raise ValueError('Unknown probe')

async def deploy(session,name,request):
 from types import SimpleNamespace
 from proloader.importer import WorldCommands
 from proloader.command_blocks import MODES
 from proloader.net.bedrock.command_block import build_command_block_update
 program=json.loads(Path(request['program']).read_text())
 origin=tuple(program['origin']);report=dict(phase='preflight',staged=[],activated=[],errors=[])
 output=Path('cache/snow_menu_deploy_20261021.json')
 def save():private_json(output,report)
 def progress(phase,**kw):
  report.update(phase=phase,**kw);save()
  print(json.dumps(dict(deploy_phase=phase,staged=len(report['staged']),activated=len(report['activated']),**kw),ensure_ascii=False),flush=True)
 world=WorldCommands(session,name,SimpleNamespace(origin=origin,dimension='overworld'))
 async def command(cmd):
  report['last_command']=cmd[:120];save()
  response=await world.send(cmd)
  if response is None:raise RuntimeError('Command not confirmed: '+cmd[:100])
  return response
 async def check(cmd):
  return await session.run_command_ai(cmd,timeout=8,dimensional=True,bot_name=name) is not None
 async def place(c):
  await world.place(c,wait=False);await command('testfor @s')
 async def update(entry,auto,track=True):
  p=tuple(entry['position'])
  payload=dict(position=p,mode=MODES[entry['block']],needs_redstone=not auto,
     conditional=entry['conditional'],command=entry['command'],name='雪球菜单 '+entry['group']+' '+str(entry['index']+1),
     track_output=track,tick_delay=entry['delay'],execute_on_first_tick=False)
  await session.send_packet(build_command_block_update(**payload));await command('testfor @s')
  nbt=await session.read_block_entity(p)
  expected=dict(Command=payload['command'],CustomName=payload['name'],auto=int(auto),TrackOutput=int(track),TickDelay=payload['tick_delay'],ExecuteOnFirstTick=0)
  differences=[k for k,v in expected.items() if nbt.get(k)!=v]
  if differences:raise RuntimeError('Configuration mismatch: '+','.join(differences))
  block=SimpleNamespace(position=p,block_name=entry['block'],conditional=entry['conditional'],states=dict(conditional_bit=entry['conditional'],facing_direction=entry['facing']))
  if not await world.check_command_block(block):raise RuntimeError('Post-update shell state mismatch')
  return nbt
 async def near(entry):
  nonlocal current
  p=entry['position']
  if max(abs(p[0]-current[0]),abs(p[2]-current[2]))>3:
   current=(p[0],p[1]+2,p[2]);await world.teleport(*current)
 try:
  if not await check('testfor @s[tag=pl_sm_bot]'):await command('tag @s add pl_sm_bot')
  current=(origin[0]+5,origin[1]+2,origin[2]+5);await world.teleport(*current)
  if not request.get('resume'):
   # Compare the entire untouched command/platform footprint to a sky reference.
   for x,z in ((32,32),(43,43),(37,37)):
    if not await check(world.context(f'testforblock ~{x} ~20 ~{z} air []')):raise RuntimeError('Sky reference is not confirmed air')
   if not await check(world.context('testforblocks ~ ~-1 ~ ~11 ~ ~11 ~32 ~20 ~32 all')):raise RuntimeError('Command-zone footprint is not confirmed empty')
  for setup in program['setup']:
   # Existing objective creation may return false on a deliberate resume.
   if setup.startswith('scoreboard objectives add '):
    await session.run_command_ai(setup,timeout=8,dimensional=True,bot_name=name)
   else:await command(setup)
  await command('gamerule commandblockoutput false');await command('gamerule sendcommandfeedback false')
  # Area creation was accepted by the first single-block probe. A repeat only
  # checks presence; failure cannot erase another ticking area.
  await command('tickingarea list all-dimensions')
  platform=('fill',origin[0],origin[1]-1,origin[2],origin[0]+11,origin[1]-1,origin[2]+11,'minecraft:quartz_block',{})
  if not request.get('resume'):
   await place(platform)
   progress('platform')
   for e in program['entries']:
    p=tuple(e['position']);c=('setblock',*p,*p,e['block'],dict(conditional_bit=False,facing_direction=e['facing']))
    await near(e);await place(c)
  for e in program['entries']:
   await near(e)
   block=SimpleNamespace(position=tuple(e['position']),block_name=e['block'],conditional=False,states=dict(conditional_bit=False,facing_direction=e['facing']))
   if not await world.check_command_block(block):raise RuntimeError('Shell state mismatch')
   nbt=await update(e,False,True)
   report['staged'].append(dict(position=e['position'],block=e['block'],auto=nbt['auto']))
   if len(report['staged'])%8==0:progress('staging')
  progress('staged_all')
  # Keep the scoreboard master gate at zero while all chain blocks are armed.
  for e in reversed(program['entries']):
   await near(e);nbt=await update(e,True,True)
   report['activated'].append(dict(position=e['position'],block=e['block'],auto=nbt['auto']))
   if len(report['activated'])%8==0:progress('arming')
  progress('armed_master_off')
  await command(program['enable'])
  await asyncio.sleep(2)
  errors=[];results=[]
  for e in program['entries']:
   await near(e);nbt=await session.read_block_entity(tuple(e['position']))
   last=nbt.get('LastOutput','')
   row=dict(group=e['group'],index=e['index'],position=e['position'],last_output=last,params=nbt.get('LastOutputParams'),success=nbt.get('SuccessCount'),last_execution=nbt.get('LastExecution'))
   results.append(row)
   if any(word in last.lower() for word in ('syntax','unknown','permission','disabled','invalid')):errors.append(row)
  report['runtime']=results;report['errors']=errors
  if errors:
   await command(program['disable']);progress('runtime_error_master_off');return report
  progress('enabled',runtime_errors=0)
  return dict(phase=report['phase'],staged=len(report['staged']),activated=len(report['activated']),runtime_errors=errors)
 except BaseException as exc:
  report['error_type']=type(exc).__name__;report['detail']=str(exc)[:200]
  try:await command(program['disable'])
  except Exception:pass
  progress('failed_master_off');raise


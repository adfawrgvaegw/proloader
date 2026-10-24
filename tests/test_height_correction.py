import json
from types import SimpleNamespace
from unittest.mock import AsyncMock

import numpy as np
import pytest

from proloader.importer import HeightLimitError, prepare_import
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure


@pytest.mark.parametrize('dimension,height,y,expected',[
    ('overworld',128,250,192),('overworld',128,-80,-64),
    ('nether',64,100,64),('the_end',128,200,128),
    ('overworld',384,0,-64),
])
def test_real_preflight_suggests_nearest_valid_origin_before_planning(tmp_path,monkeypatch,dimension,height,y,expected):
    import proloader.importer as importer
    palette=BlockPalette()
    st=Structure('test',(1,height,1),palette,Region(palette,np.zeros((1,height,1),dtype=np.uint32)))
    monkeypatch.setattr(importer,'load_structure',lambda _:st)
    source=tmp_path/'test.mcstructure';source.write_bytes(b'fixture')
    with pytest.raises(HeightLimitError) as caught:
        prepare_import(source,(12,y,-34),dimension,engine='legacy')
    exc=caught.value
    assert exc.suggested_origin==(12,expected,-34)
    plan=prepare_import(source,exc.suggested_origin,dimension,engine='legacy')
    assert plan.origin==exc.suggested_origin
    assert plan.origin[1]+height-1<=exc.details['world_y_range'][1]


def test_building_too_tall_has_no_false_recommendation():
    exc=HeightLimitError((0,80,0),385,'overworld',-64,319)
    assert exc.suggested_origin is None
    assert exc.details['valid_start_y_range'] is None
    assert '仅调整Y无法' in str(exc)


def test_manual_bad_then_still_out_of_range_can_retry_without_restarting(monkeypatch):
    from proloader.cli import commands
    calls=[]
    def planner(file,origin,dimension,**kwargs):
        calls.append((file,origin,dimension))
        if origin[1]>192:raise HeightLimitError(origin,128,dimension,-64,319)
        return SimpleNamespace(origin=origin)
    choices=iter((1,1,0));answers=iter(('bad','300'))
    monkeypatch.setattr(commands.ui,'choose',lambda *a,**k:next(choices))
    monkeypatch.setattr(commands.ui,'ask',lambda *a,**k:next(answers))
    monkeypatch.setattr(commands.ui,'notice',lambda *a,**k:None)
    plan=commands.prepare_with_height_correction('palace.schematic',(12,250,-34),'overworld',planner=planner)
    assert plan.origin==(12,192,-34)
    assert all(file=='palace.schematic' and dimension=='overworld' for file,_,dimension in calls)


def test_impossible_height_can_change_dimension_in_place(monkeypatch):
    from proloader.cli import commands
    def planner(file,origin,dimension,**kwargs):
        if dimension=='nether':raise HeightLimitError(origin,200,dimension,0,127)
        return SimpleNamespace(origin=origin,dimension=dimension)
    choices=iter((0,0))
    monkeypatch.setattr(commands.ui,'choose',lambda *a,**k:next(choices))
    monkeypatch.setattr(commands.ui,'notice',lambda *a,**k:None)
    plan=commands.prepare_with_height_correction('city',(1,80,3),'nether',planner=planner)
    assert plan.dimension=='overworld'


def test_json_suggestion_is_structured_and_does_not_prompt(monkeypatch,capsys):
    from proloader.cli import commands
    exc=HeightLimitError((1,250,3),128,'overworld',-64,319)
    def planner(*a,**k):raise exc
    monkeypatch.setattr(commands.ui,'choose',lambda *a,**k:pytest.fail('must not prompt'))
    with pytest.raises(HeightLimitError):
        commands.prepare_with_height_correction('city',(1,250,3),'overworld',interactive=False,planner=planner)
    commands.print_error(exc,as_json=True)
    result=json.loads(capsys.readouterr().out)
    assert result['details']['suggested_origin']==[1,192,3]


def test_wizard_height_retry_preserves_server_and_password(tmp_path,monkeypatch):
    from proloader.cli import wizard
    from proloader.examples.gen_buildings import generate_house
    source=tmp_path/'house.mcstructure';generate_house(source)
    bot=SimpleNamespace(id='fixture',label='fixture')
    monkeypatch.setattr(wizard,'pending_jobs',lambda:[])
    monkeypatch.setattr(wizard,'preferences',lambda:{})
    monkeypatch.setattr(wizard,'select_file',lambda:source)
    answers=iter(('12345678','PRIVATE-PASSWORD','12 318 -34'))
    monkeypatch.setattr(wizard.ui,'ask',lambda *a,**k:next(answers))
    choices=iter((0,0,0,0))  # dimension, suggested height, verification, rate
    monkeypatch.setattr(wizard.ui,'choose',lambda *a,**k:next(choices))
    monkeypatch.setattr(wizard.ui,'screen',lambda *a,**k:None)
    monkeypatch.setattr(wizard.ui,'notice',lambda *a,**k:None)
    seen=[]
    monkeypatch.setattr(wizard,'review',lambda *a:seen.append(a) or False)
    perform=AsyncMock();monkeypatch.setattr(wizard,'perform',perform)
    wizard.start_import(SimpleNamespace(list=lambda:[bot]))
    plan=seen[0][0]
    assert plan.origin==(12,320-plan.structure.size[1],-34)
    assert seen[0][2]=='12345678'
    perform.assert_not_called()

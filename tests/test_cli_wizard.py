import io
from pathlib import Path

import pytest
from rich.console import Console

from proloader.cli.ui import home, THEME
from proloader.cli.wizard import parse_coordinates, input_path


@pytest.mark.parametrize('value', ['-85,78,351','-85 78 351',' -85，78，351 '])
def test_coordinate_inputs(value):
    assert parse_coordinates(value)==(-85,78,351)


@pytest.mark.parametrize('value',['1 2','1 2 3 4','a,2,3','1.5,2,3'])
def test_coordinate_error_is_actionable(value):
    with pytest.raises(ValueError,match='三个整数'):
        parse_coordinates(value)


def test_dragged_or_quoted_path_with_spaces(tmp_path):
    path=tmp_path/'airport name.schem'
    path.write_bytes(b'')
    assert input_path(str(path))==path
    assert input_path('"'+str(path)+'"')==path
    assert input_path(str(path).replace(' ','\\ '))==path


@pytest.mark.parametrize('width',[56,96])
def test_home_is_two_real_actions_without_legacy_stubs(width):
    output=io.StringIO()
    console=Console(file=output,width=width,theme=THEME,color_system=None)
    console.print(home(1,0))
    text=output.getvalue()
    assert '管理机器人' in text and '开始导入' in text
    assert '暂留' not in text and '配置管理' not in text
    assert 'sessionid' not in text and 'user_token' not in text


def test_review_cancel_never_connects(tmp_path,monkeypatch):
    from types import SimpleNamespace
    from unittest.mock import AsyncMock
    from proloader.cli import wizard
    from proloader.examples.gen_buildings import generate_house
    source=tmp_path/'house.mcstructure';generate_house(source)
    bot=SimpleNamespace(id='fixture',label='fixture')
    store=SimpleNamespace(list=lambda:[bot])
    monkeypatch.setattr(wizard,'pending_jobs',lambda:[])
    monkeypatch.setattr(wizard,'preferences',lambda:{})
    monkeypatch.setattr(wizard,'select_file',lambda:source)
    answers=iter(('12345678','PRIVATE-PASSWORD','0 80 0'))
    monkeypatch.setattr(wizard.ui,'ask',lambda *a,**k:next(answers))
    choices=iter((0,0,0))  # overworld, sample verification, default speed
    monkeypatch.setattr(wizard.ui,'choose',lambda *a,**k:next(choices))
    monkeypatch.setattr(wizard.ui,'screen',lambda *a,**k:None)
    reviewed=[]
    monkeypatch.setattr(wizard,'review',lambda *args:reviewed.append(args) or False)
    perform=AsyncMock();monkeypatch.setattr(wizard,'perform',perform)
    wizard.start_import(store)
    assert reviewed[0][-1]==4000
    assert reviewed[0][0].origin==(0,80,0)
    perform.assert_not_called()


def test_placement_error_shows_coordinate_and_block(monkeypatch):
    from proloader.cli import wizard
    from proloader.importer import PlacementCheckError
    seen=[]
    monkeypatch.setattr(wizard.ui,'notice',lambda *args:seen.append(args))
    monkeypatch.setattr(wizard,'pause',lambda:None)
    wizard.show_error(PlacementCheckError(448,('setblock',125,110,-96,125,110,-96,'minecraft:stonebrick',{'stone_brick_type':'cracked'})))
    assert '125 110 -96' in seen[0][1]
    assert 'cracked_stone_bricks' in seen[0][1]

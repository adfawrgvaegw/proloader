import pytest

from proloader.errors import ParseError
from proloader.parser.java_mapper import map_java_palette


@pytest.mark.parametrize('axis',['x','y','z'])
def test_iron_chain_rename_preserves_exact_chain_axis(axis):
    old=f'minecraft:chain[axis={axis},waterlogged=false]'
    new=f'minecraft:iron_chain[axis={axis},waterlogged=false]'
    result=map_java_palette([old,new])
    assert result[old]==result[new]
    assert result[new]['found'] is True
    assert result[new]['name']=='minecraft:chain'
    assert result[new]['states']=={'pillar_axis':axis}


def test_unknown_chain_variant_still_fails_without_air_replacement():
    with pytest.raises(ParseError,match='无法转换'):
        map_java_palette(['minecraft:not_a_real_iron_chain[axis=y]'])

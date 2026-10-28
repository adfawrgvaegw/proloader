"""CommandBlockUpdate/SubChunkRequest, aligned with Fatalder's packet Marshal."""
import struct

from proloader.net.raknet.wire import ByteWriter, ByteReader
from .packets import PacketID, wrap_packet, unwrap_packet


def build_command_block_update(position, *, mode, needs_redstone, conditional,
                               command, name='', track_output=True,
                               tick_delay=0, execute_on_first_tick=False, protocol=860):
    if protocol not in (686,860):raise ValueError('Unsupported command block protocol')
    if len(position) != 3 or any(type(v) is not int or not -(1<<31) <= v < 1<<31 for v in position):
        raise ValueError('Invalid command block position')
    if type(mode) is not int or mode not in (0, 1, 2):
        raise ValueError('Invalid command block mode')
    if type(tick_delay) is not int or not 0 <= tick_delay < 1<<31:
        raise ValueError('Invalid command block delay')
    if not isinstance(command, str) or not isinstance(name, str):
        raise ValueError('Invalid command block text')
    w = ByteWriter()
    w.write_bool(True)
    w.write_varint(position[0]); w.write_unsigned_varint(position[1] & 0xffffffff); w.write_varint(position[2])
    w.write_unsigned_varint(mode); w.write_bool(needs_redstone); w.write_bool(conditional)
    w.write_string(command); w.write_string(''); w.write_string(name)
    # Modern servers require FilteredName. The old Fatalder 686 codec omits it.
    if protocol == 860:w.write_string('')
    w.write_bool(track_output); w.write(struct.pack('<i', tick_delay)); w.write_bool(execute_on_first_tick)
    return wrap_packet(PacketID.COMMAND_BLOCK_UPDATE, w.to_bytes())


def build_subchunk_request(position, dimension, heights=None, offsets=None):
    if dimension not in (0, 1, 2) or len(position) != 3:
        raise ValueError('Invalid subchunk request')
    x, y, z = position
    if offsets is not None:
        if heights is not None:raise ValueError('Conflicting subchunk offsets')
        offsets=list(offsets)
        if not offsets or len(offsets)>1024 or len(set(offsets))!=len(offsets) or any(len(v)!=3 or any(type(n) is not int or not -128<=n<=127 for n in v) for v in offsets):
            raise ValueError('Invalid subchunk offsets')
    heights=[y//16] if heights is None else list(heights)
    if not heights or len(heights)>32 or len(set(heights))!=len(heights) or any(type(h) is not int or not -128<=h<=127 for h in heights):
        raise ValueError('Invalid subchunk height')
    w = ByteWriter()
    w.write_varint(dimension)
    for v in (x//16, 0, z//16):w.write_varint(v)
    offsets=offsets if offsets is not None else [(0,h,0) for h in heights]
    w.write_u32(len(offsets))
    for offset in offsets:
        for v in offset:w.write_byte(v & 255)
    return wrap_packet(PacketID.SUB_CHUNK_REQUEST, w.to_bytes())


def subchunk_key(packet):
    pid, body = unwrap_packet(packet)
    if pid != PacketID.SUB_CHUNK:
        raise ValueError('Not a subchunk response')
    r = ByteReader(body)
    r.read_bool()
    return (r.read_varint(), r.read_varint(), r.read_varint(), r.read_varint())

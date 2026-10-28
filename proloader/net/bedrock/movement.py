"""Teleport ACK: legacy Fatalder686 and pinned, live-validated NEMC860 layouts."""
import math
import struct
from proloader.net.raknet.wire import ByteReader,ByteWriter
from .packets import unwrap_packet,wrap_packet


def read_player_teleport(packet):
    pid,body=unwrap_packet(packet)
    if pid!=19:raise ValueError('Not MovePlayer')
    r=ByteReader(body);runtime_id=r.read_unsigned_varlong()
    position=struct.unpack('<fff',r.read(12))
    r.read(12)  # pitch/yaw/head yaw
    mode=r.read_byte()
    if mode not in (0,1,2,3):raise ValueError('Invalid player move mode')
    r.read_bool();r.read_unsigned_varlong()
    if mode==2:r.read(8)  # teleport cause/source entity type
    r.read_unsigned_varlong()
    if any(not math.isfinite(v) for v in position):raise ValueError('Invalid player position')
    return runtime_id,position,mode


def build_teleport_ack(position,protocol=860):
    if protocol not in (686,860):raise ValueError('Unsupported NetEase teleport acknowledgement')
    if len(position)!=3 or any(not math.isfinite(v) for v in position):raise ValueError('Invalid player position')
    w=ByteWriter();w.write(bytes(8));w.write(struct.pack('<fff',*position))
    w.write(bytes(12))  # move vector and head yaw
    w.write_unsigned_varlong(1<<37)  # InputFlagHandledTeleport
    w.write_unsigned_varint(1 if protocol==860 else 0)  # mouse input for 860
    w.write_unsigned_varint(0);w.write_unsigned_varint(0)
    if protocol==860:w.write(bytes(8))  # interact pitch/yaw
    w.write_unsigned_varlong(0)  # upstream echo uses tick zero
    w.write(bytes(12))  # delta
    w.write_bool(False)  # NetEase bypass move vector / legacy Unknown3
    w.write(bytes(8))  # analogue move vector
    if protocol==860:
        w.write(bytes(20))  # camera orientation, raw move vector
        w.write_bool(False);w.write(bytes(8))  # camera departed, two camera floats
        w.write_bool(False);w.write_bool(True);w.write_bool(False)  # dirty, grounded, reset
    else:
        w.write_bool(False);w.write(bytes(8))
        w.write_bool(False);w.write_bool(False);w.write_byte(0)
    return wrap_packet(144,w.to_bytes())

"""Text packet fields aligned with Gophertunnel v1.48.1 (protocol 860); NetEase adds trailing fields, ignored here."""
from dataclasses import dataclass
from proloader.net.raknet.wire import ByteReader


@dataclass(frozen=True)
class ChatMessage:
    source: str
    message: str
    xuid: str = ''


def parse_chat(packet):
    reader=ByteReader(packet)
    if reader.read_unsigned_varint()&0x3ff!=9:return None
    kind=reader.read_byte();reader.read_byte()  # NeedsTranslation
    if kind not in (1,7):return None  # Ignore raw/system/translated JSON commands.
    source=reader.read_string();message=reader.read_string()
    xuid=reader.read_string();reader.read_string();reader.read_string()
    if not source or len(source)>256 or len(message)>4096:return None
    return ChatMessage(source,message,xuid)

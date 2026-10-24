"""Audit an existing PCAP Login offline, printing only credential-free checks."""
import argparse
import hashlib
import json
import struct
from pathlib import Path

from proloader.net.bedrock.login_audit import audit_connection_request
from proloader.net.bedrock.batch import NeteaseDecompressor
from proloader.net.raknet.frames import FrameSet
from tools.extraction.extract_game_version import (
    PcapAnalyzer, parse_login_packet, read_pcap, process_batch_body, read_varuint,
)


def audit_capture(path):
    data = path.read_bytes()
    endian = '<' if data[:4] == b'\xd4\xc3\xb2\xa1' else '>'
    capture_time = struct.unpack_from(endian + 'I', data, 24)[0]

    class Analyzer(PcapAnalyzer):
        result = None

        def __init__(self):
            super().__init__()
            self.streams = {}

        def process_frame(self, body, direction, pkt_idx):
            decoder = self.streams.setdefault(direction, NeteaseDecompressor()) if self.netease_active else None
            for method, packet in process_batch_body(body, direction, decoder):
                pid, _ = read_varuint(packet, 0)
                self.handle_packet(pid, packet, direction, pkt_idx, method)

        def handle_packet(self, pid, packet, direction, pkt_idx, method):
            if pid == 143:
                self.netease_active = True
            if pid == 1 and self.result is None:
                login = parse_login_packet(packet)
                if login is None:
                    return
                cert, raw = login['chain'], login['raw_token']
                request = struct.pack('<i', len(cert)) + cert + struct.pack('<i', len(raw)) + raw
                self.result = dict(protocol=login['protocol'],
                                   checks=audit_connection_request(request, now=capture_time))

    analyzer = Analyzer()
    for i, (src, dst, payload) in enumerate(read_pcap(path)):
        if not payload or payload[0] & 0xf0 != 0x80:
            continue
        try:
            frames = FrameSet.decode(payload).frames
        except (ValueError, EOFError, IndexError, struct.error):
            continue
        for frame in frames:
            direction = f'{src}:{dst}'  # Internal only; never included in the result.
            if frame.is_split:
                key = (direction, frame.split_id)
                analyzer.split_groups[key][frame.split_index] = frame.body
                if len(analyzer.split_groups[key]) != frame.split_count:
                    continue
                body = b''.join(analyzer.split_groups[key][j] for j in range(frame.split_count))
                del analyzer.split_groups[key]
            else:
                body = frame.body
            analyzer.process_frame(body, direction, i)
            if analyzer.result is not None:
                return dict(pcap_sha256=hashlib.sha256(data).hexdigest(),
                            time_basis='first_pcap_record', **analyzer.result)
    raise ValueError('no decodable Login found in capture')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('pcap', type=Path)
    args = parser.parse_args()
    print(json.dumps(audit_capture(args.pcap), ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()

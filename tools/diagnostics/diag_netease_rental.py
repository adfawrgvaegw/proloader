"""使用已有成功 PE 结果诊断租赁服；仅登录，不发送命令。"""
import argparse
import asyncio
import getpass
import json
import logging
import os
import time
from collections import Counter
from pathlib import Path

from proloader.net.netease.client import NeteaseClient
from proloader.net.bedrock.crypto import generate_keypair, serialize_public_key_b64
from proloader.net.bedrock.session import BedrockSession, BedrockLoginError
from proloader.net.bedrock.login import parse_play_status, parse_network_settings, build_login_from_jwt_chain
from proloader.net.bedrock.login_audit import audit_connection_request
from proloader.net.raknet.client import RakNetClient
from proloader.net.raknet.connection import handshake, RakNetHandshakeError
from proloader.net.raknet.session import RakNetSession
from proloader.net.raknet.frames import FrameSet, parse_ack, parse_nack
from proloader.net.raknet import connection as raknet_connection


async def run(args, password):
    # Reserve before any API/game request, avoiding a late save race after testing.
    output_fd = os.open(args.output, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    events = []
    counts = Counter()
    transport_counts = Counter()
    started = time.monotonic()
    def report(**event):
        event['elapsed_s'] = round(time.monotonic() - started, 3)
        events.append(event)
        print(json.dumps(event, ensure_ascii=False), flush=True)

    class ObservedSession(BedrockSession):
        def _build_connection_request(self):
            request = super()._build_connection_request()
            if args.device_os is not None:
                self._client_data.DeviceOS = args.device_os
            if args.prepend_client_jwt or args.device_os is not None:
                request = build_login_from_jwt_chain(self._netease_jwt_chain, self._client_data,
                                                    self._priv, prepend_client_jwt=True if args.prepend_client_jwt else None,
                                                    protocol=self._netease_protocol)
            # Only booleans, sizes and fixed status labels enter the persisted log.
            report(stage='login_audit', **audit_connection_request(request))
            return request

        async def _send_packet(self, packet):
            pid, _ = self._read_packet_id(packet)
            report(stage='send_packet', packet_id=pid, size=len(packet))
            await super()._send_packet(packet)

        def _dispatch_packet(self, packet):
            try:
                pid, _ = self._read_packet_id(packet)
                counts[pid] += 1
                if pid == 2:
                    report(stage='play_status', status=parse_play_status(packet))
                elif pid == 143:
                    threshold, algorithm = parse_network_settings(packet)
                    report(stage='network_settings', threshold=threshold, algorithm=algorithm)
            except (ValueError, IndexError, EOFError):
                pass
            return super()._dispatch_packet(packet)

    class ObservedRakNet(RakNetSession):
        def _dispatch(self, data):
            transport_counts[data[0]] += 1
            return super()._dispatch(data)

        def _handle_frame(self, frame):
            if args.trace_transport:
                report(stage='recv_frame', size=len(frame.body), reliability=frame.reliability,
                       reliable_index=frame.frame_index, order=frame.order_index,
                       channel=frame.order_channel, split=frame.is_split,
                       split_count=frame.split_count, split_index=frame.split_index,
                       split_id=frame.split_id,
                       body_id=None if frame.is_split or not frame.body else frame.body[0])
            if frame.body[:1] == b'\x15':
                report(stage='remote_transport_disconnect')
            return super()._handle_frame(frame)

        def _handle_frame_set(self, data):
            if args.trace_transport:
                try:
                    frame_set = FrameSet.decode(data)
                    report(stage='recv_datagram', size=len(data), sequence=frame_set.sequence_number,
                           frames=len(frame_set.frames))
                except Exception as exc:
                    report(stage='recv_datagram_decode_error', size=len(data), error=type(exc).__name__)
            return super()._handle_frame_set(data)

        def _handle_ack(self, data):
            if args.trace_transport:
                try:
                    report(stage='recv_ack', ranges=[[r.start, r.start if r.is_single else r.end]
                                                    for r in parse_ack(data)])
                except Exception as exc:
                    report(stage='recv_ack_decode_error', error=type(exc).__name__)
            return super()._handle_ack(data)

        def _handle_nack(self, data):
            if args.trace_transport:
                try:
                    report(stage='recv_nack', ranges=[[r.start, r.start if r.is_single else r.end]
                                                     for r in parse_nack(data)])
                except Exception as exc:
                    report(stage='recv_nack_decode_error', error=type(exc).__name__)
            return super()._handle_nack(data)

        def _deliver_body(self, body):
            if args.trace_transport:
                report(stage='deliver_message', size=len(body), body_id=body[0] if body else None)
            return super()._deliver_body(body)

        async def _enqueue_frame(self, frame):
            if args.trace_transport:
                report(stage='enqueue_message', size=len(frame.body),
                       body_id=frame.body[0] if frame.body else None, reliability=frame.reliability,
                       next_reliable=self._frame_index, next_order=self._order_index)
            return await super()._enqueue_frame(frame)

    client = NeteaseClient(pe_auth_profile='official-android-3915')
    original_reply1 = raknet_connection.parse_open_connection_reply_1
    original_reply2 = raknet_connection.parse_open_connection_reply_2
    if args.trace_transport:
        def observed_reply1(data):
            reply = original_reply1(data)
            report(stage='reply1_shape', size=len(data), security=reply.use_security, mtu=reply.mtu)
            return reply
        def observed_reply2(data):
            reply = original_reply2(data)
            report(stage='reply2_shape', size=len(data), address_version=reply.client_addr.version,
                   security=reply.use_encryption, mtu=reply.mtu)
            return reply
        raknet_connection.parse_open_connection_reply_1 = observed_reply1
        raknet_connection.parse_open_connection_reply_2 = observed_reply2
    bedrock = None
    stage = 'load'
    try:
        data = json.loads(args.pe_result.read_text())
        if data.get('ok') is not True or data.get('stage') != 'PE' or data['response']['code'] != 0:
            raise ValueError('successful PE required')
        if data.get('auth_profile') != 'official-android-3915':
            raise ValueError('profile mismatch')
        grant = data['credentials']
        if not grant['user_id'].isdigit() or int(grant['user_id']) <= 0 or not grant['user_token']:
            raise ValueError('invalid credentials')
        if grant['user_detail']['entity_id'] != grant['user_id']:
            raise ValueError('identity mismatch')
        client.user_id = grant['user_id']
        client.user_token = grant['user_token']
        client.seed = grant['seed']
        client.user_detail = grant['user_detail']
        if args.show_player_name:
            report(stage='player', name=client._get_display_name())
        stage = 'bootstrap'
        client.bootstrap(engine_source='official-download')
        if (client.engine_version != data['metadata']['engine_version'] or
                client.g79_latest_version != data['metadata']['patch_version']):
            raise ValueError('metadata changed')
        stage = 'search'
        server = client.search_rental_server(args.server_code)
        report(stage=stage, ok=True)
        stage = 'enter'
        host, port = client.enter_rental_server_world(server.entity_id, password)
        report(stage=stage, ok=True)
        stage = 'ping'
        try:
            info = await RakNetClient().ping(host, port, timeout=5)
            report(stage=stage, protocol=info.protocol_version)
        except Exception as exc:
            report(stage=stage, error=type(exc).__name__)
        stage = 'link'
        client.connect_link_and_send_game_start(args.server_code, server)
        report(stage=stage, ok=True)
        stage = 'chain'
        key = generate_keypair()
        chain = client.send_auth_v2_request(client.generate_rental_game_auth_v2(
            server.entity_id, serialize_public_key_b64(key)))
        parsed_chain = json.loads(chain)
        report(stage='chain_shape', count=len(parsed_chain if isinstance(parsed_chain, list) else parsed_chain['chain']))
        bedrock = ObservedSession(None, netease_mode=True, netease_jwt_chain=chain,
                                 netease_uid=int(client.user_id), netease_protocol=args.protocol, priv_key=key)
        address = f'{host}:{port}'
        bedrock._client_data.ServerAddress = address
        bedrock._build_connection_request()
        report(stage=stage, ok=True)
        stage = 'raknet_offline'
        conn = await handshake(host, port, timeout=8, protocol=8, max_retries=0)
        report(stage=stage, ok=True, mtu=conn.mtu)
        raknet = ObservedRakNet(conn)
        bedrock._raknet = raknet
        stage = 'raknet_online'
        await raknet.start()
        await raknet.connect(timeout=15)
        report(stage=stage, ok=True)
        stage = 'bedrock_login'
        await bedrock.login(address, timeout=20)
        report(stage=stage, ok=True)
        stage = 'initialize'
        await bedrock.initialize_game(timeout=30)
        report(stage=stage, ok=True, encrypted=bedrock._crypto is not None,
               start_game_received=bedrock._start_game is not None,
               runtime_id_present=bedrock._entity_runtime_id > 0)
        if args.hold_seconds:
            stage = 'hold'
            before_packets = sum(counts.values())
            before_udp = sum(transport_counts.values())
            until = time.monotonic() + args.hold_seconds
            while time.monotonic() < until:
                await asyncio.sleep(min(1.0, until - time.monotonic()))
                bedrock._raise_if_failed()
                if raknet._closed:
                    raise RuntimeError('transport closed during hold')
            report(stage=stage, ok=True, seconds=args.hold_seconds,
                   game_packets=sum(counts.values()) - before_packets,
                   udp_packets=sum(transport_counts.values()) - before_udp)
        return 0
    except Exception as exc:
        detail = type(exc).__name__
        # 本地 BedrockLoginError 不含任意服务端文本；RakNet 错误仅分类。
        if isinstance(exc, BedrockLoginError):
            detail = str(exc)
        elif isinstance(exc, RakNetHandshakeError):
            text = str(exc)
            detail = next((label for word, label in [('Reply1', 'no_reply1'),
                ('Reply2', 'no_reply2'), ('协议版本', 'raknet_protocol_mismatch'),
                ('AlreadyConnected', 'already_connected'), ('ConnectionAttemptFailed', 'attempt_rejected')]
                if word in text), type(exc).__name__)
        report(stage=stage, ok=False, error=detail)
        return 1
    finally:
        raknet_connection.parse_open_connection_reply_1 = original_reply1
        raknet_connection.parse_open_connection_reply_2 = original_reply2
        if bedrock is not None and bedrock._raknet is not None:
            report(stage='transport_pending', reliable_frames=len(bedrock._raknet._pending_acks),
                   udp_counts=dict(transport_counts), closed=bedrock._raknet._closed,
                   frames=[dict(size=len(p.frame.body), split=p.frame.is_split,
                                split_count=p.frame.split_count, order=p.frame.order_index)
                           for p in bedrock._raknet._pending_acks.values()])
            if args.trace_transport:
                report(stage='receive_buffers',
                       ordered={ch: dict(expected=bedrock._raknet._order_recv.get(ch, 0),
                                         pending=sorted(pending))
                                for ch, pending in bedrock._raknet._order_buffer.items()},
                       splits=[dict(id=sid, expected=group.split_count,
                                    received=sorted(group.fragments))
                               for sid, group in bedrock._raknet._split_buffer.items()])
        if bedrock is not None:
            await bedrock.close()
        client._session.close()
        report(stage='received_packet_counts', counts=dict(counts))
        with os.fdopen(output_fd, 'w') as out:
            json.dump(events, out, ensure_ascii=False)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--pe-result', type=Path, required=True)
    p.add_argument('--server-code', required=True)
    p.add_argument('--output', type=Path, required=True)
    p.add_argument('--protocol', type=int, choices=(686, 860), default=860)
    p.add_argument('--connect', action='store_true')
    p.add_argument('--trace-transport', action='store_true', help='脱敏分片/ACK/排序轨迹，不记录正文')
    p.add_argument('--hold-seconds', type=int, choices=range(0, 7201), default=0,
                   metavar='0..7200', help='初始化后保持连接，只收包/心跳，不发游戏命令')
    p.add_argument('--show-player-name', action='store_true', help='显示游戏昵称以便用户授予权限')
    p.add_argument('--prepend-client-jwt', action='store_true', help='显式对照参考 Encode 的客户端桥接JWT')
    p.add_argument('--device-os', type=int, choices=(1, 2), help='仅诊断：显式对照 Android/iOS DeviceOS')
    args = p.parse_args()
    if not args.connect:
        print('离线模式；--connect 才请求服务器。')
        return 0
    if args.output.exists():
        p.error('输出已存在')
    logging.disable(logging.CRITICAL)
    return asyncio.run(run(args, getpass.getpass('租赁服密码（隐藏输入）: ')))


if __name__ == '__main__':
    raise SystemExit(main())

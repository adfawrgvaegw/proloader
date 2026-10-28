"""Reusable, verified rental connection lifecycle for the real CLI/importer."""
from __future__ import annotations
import asyncio
import time
from collections import deque
from contextlib import asynccontextmanager

from proloader.bots import BotStore, BotProfile
from .client import NeteaseClient, RentalServerEntity, RentalWorldPending, RentalWorldEnterError
from .pe_profiles import PEMetadataMismatchError, PEAuthenticationError
import requests
from proloader.net.bedrock.crypto import generate_keypair, serialize_public_key_b64
from proloader.net.bedrock.session import BedrockSession
from proloader.net.raknet.connection import handshake, RakNetHandshakeError
from proloader.net.raknet.session import RakNetSession


class RentalError(RuntimeError):
    def __init__(self, stage, code=None, reason=None):
        self.stage, self.code, self.reason = stage, code, reason
        super().__init__(f'{stage}失败' + (f'（网易错误码 {code}）' if type(code) is int else '')
                         + (f'：{reason}' if reason else ''))


async def _enter_world(client, server_id, password, stage, event, *, wait_seconds=60):
    """Poll only an explicitly starting world, within one identity lease.

    HTTP failures/rejections do not cause credential refresh or a second game
    connection. Each request retains its existing bounded HTTP timeout.
    """
    deadline = time.monotonic() + wait_seconds
    while True:
        try:
            try:
                endpoint = await asyncio.to_thread(client.enter_rental_server_world, server_id, password)
            finally:
                status = getattr(client, '_last_world_enter_status', None)
                if isinstance(status,dict):event('world_enter_response', **status)
            return endpoint
        except RentalWorldPending as exc:
            event('world_starting', state=exc.state, progress=exc.progress, total=exc.total)
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise RentalError('等待租赁服启动', reason='服务器尚未返回游戏地址，已停止等待；身份和断点保留') from None
            progress = f'（{exc.progress}/{exc.total}）' if exc.progress is not None and exc.total else ''
            stage('等待租赁服启动' + progress)
            await asyncio.sleep(min(2, remaining))
        except RentalWorldEnterError as exc:
            raise RentalError('进入租赁服', exc.code, reason='服务器未提供有效游戏地址') from None
        except requests.Timeout:
            raise RentalError('进入租赁服', reason='地址请求超时') from None
        except requests.HTTPError as exc:
            status = exc.response.status_code if exc.response is not None else None
            raise RentalError('进入租赁服', reason=f'地址服务HTTP错误 {status}') from None
        except requests.ConnectionError:
            raise RentalError('进入租赁服', reason='无法连接地址服务') from None


def _refresh(client, store, bot):
    try:
        client.authenticate_with_cookie(store.cookie(bot))
    except PEMetadataMismatchError as exc:
        raise RentalError('机器人身份验证',reason=f'客户端/补丁尚未适配（{exc.engine} / {exc.patch}），请更新导入器') from None
    except PEAuthenticationError as exc:
        raise RentalError('机器人身份验证',exc.code) from None
    except requests.Timeout:
        raise RentalError('机器人身份验证',reason='认证请求超时，请稍后重试') from None
    except requests.HTTPError as exc:
        status=exc.response.status_code if exc.response is not None else None
        raise RentalError('机器人身份验证',reason=f'认证服务HTTP错误 {status}') from None
    except requests.ConnectionError:
        raise RentalError('机器人身份验证',reason='无法连接认证服务器') from None
    except Exception as exc:
        raise RentalError('机器人身份验证',reason=f'本机材料或响应检查未通过（{type(exc).__name__}）') from None
    data = dict(ok=True, stage='PE', auth_profile='official-android-3915', response={'code': 0},
                metadata=dict(engine_version=client.engine_version, patch_version=client.g79_latest_version),
                credentials=dict(user_id=client.user_id, user_token=client.user_token,
                                 seed=client.seed, user_detail=client.user_detail))
    store.save_pe(bot, data)


def verify_bot(store, bot):
    with store.lease(bot):
        client = NeteaseClient(pe_auth_profile='official-android-3915')
        try:
            client.bootstrap(engine_source='official-download')
            _refresh(client, store, bot)
            return client._get_display_name()
        finally:
            client._session.close()


@asynccontextmanager
async def _rental_session(store: BotStore, bot: BotProfile, server_code: str, password: str,
                         on_stage=None, on_event=None):
    """Reuse a PE identity; refresh once only on missing/mismatched data or code10."""
    client = NeteaseClient(pe_auth_profile='official-android-3915')
    bedrock = raknet = None
    started = time.monotonic()
    def event(kind, **data):
        if on_event:
            on_event(dict(event=kind, elapsed_s=round(time.monotonic()-started, 3), **data))

    trace = deque(maxlen=256)
    class ObservedRakNet(RakNetSession):
        async def _enqueue_frame(self, frame):
            trace.append(dict(direction='send', body_id=frame.body[0] if frame.body else None,
                              size=len(frame.body), reliable=self._frame_index, order=self._order_index))
            return await super()._enqueue_frame(frame)
        def _handle_frame(self, frame):
            trace.append(dict(direction='recv', body_id=None if frame.is_split or not frame.body else frame.body[0],
                              size=len(frame.body), reliable=frame.frame_index, order=frame.order_index,
                              reliability=frame.reliability, split=frame.is_split))
            return super()._handle_frame(frame)
        def _handle_ack(self, data):
            from proloader.net.raknet.frames import parse_ack
            trace.append(dict(direction='ack', ranges=[[r.start, r.start if r.is_single else r.end] for r in parse_ack(data)]))
            return super()._handle_ack(data)
        async def _resend_frame(self, frame):
            trace.append(dict(direction='resend', reliable=frame.frame_index, order=frame.order_index, size=len(frame.body)))
            return await super()._resend_frame(frame)

    class ObservedBedrock(BedrockSession):
        async def _send_packet(self, packet):
            pid, _ = self._read_packet_id(packet)
            if pid in (1, 4, 8, 193, 113):
                event('sent', packet_id=pid, encrypted=self._crypto is not None)
            return await super()._send_packet(packet)

        def _dispatch_packet(self, packet):
            pid, _ = self._read_packet_id(packet)
            if pid in (2, 3, 5, 6, 7, 11, 70, 143):
                data = dict(packet_id=pid)
                if pid == 2:
                    from proloader.net.bedrock.login import parse_play_status
                    data['status'] = parse_play_status(packet)
                event('received', **data)
            return super()._dispatch_packet(packet)


    def stage(text):
        event('stage', name=text)
        if on_stage:
            on_stage(text)

    try:
        stage('检查机器人身份')
        await asyncio.to_thread(client.bootstrap, engine_source='official-download')
        from .mcp_check import binary as mcp_check_binary
        await asyncio.to_thread(mcp_check_binary)
        pe = store.pe(bot)
        if pe and pe.get('metadata', {}).get('engine_version') == client.engine_version and pe.get('metadata', {}).get('patch_version') == client.g79_latest_version:
            grant = pe['credentials']
            client.user_id, client.user_token = grant['user_id'], grant['user_token']
            client.seed, client.user_detail = grant['seed'], grant['user_detail']
        else:
            event('pe_refresh', reason='missing_or_metadata_changed')
            await asyncio.to_thread(_refresh, client, store, bot)
        stage('查找租赁服')

        def search():
            return client._api_request_plain(client.web_server_url, '/rental-server/query/search-by-name',
                                             {'server_name': server_code})
        result = await asyncio.to_thread(search)
        if result.get('code') == 10:
            event('pe_refresh', reason='code10')
            stage('更新已失效的身份凭据')
            await asyncio.to_thread(_refresh, client, store, bot)
            result = await asyncio.to_thread(search)
        if result.get('code', 0) != 0 or not result.get('entities'):
            raise RentalError('查询服务器', result.get('code'))
        entity = result['entities'][0]
        server = RentalServerEntity(str(entity['entity_id']), str(entity.get('owner_id', '')),
                                    int(entity.get('min_level', 0) or 0))
        stage('获取租赁服游戏地址')
        host, port = await _enter_world(client, server.entity_id, password, stage, event)
        stage('准备游戏认证')
        await asyncio.to_thread(client.connect_link_and_send_game_start, server_code, server)
        key = generate_keypair()
        request = client.generate_rental_game_auth_v2(server.entity_id, serialize_public_key_b64(key))
        chain = await asyncio.to_thread(client.send_auth_v2_request, request)
        stage('连接游戏服务器')
        conn = await handshake(host, port, timeout=8, protocol=8, max_retries=0)
        event('raknet_offline', mtu=conn.mtu)
        raknet = ObservedRakNet(conn)
        await raknet.start()
        await raknet.connect(timeout=15)
        event('raknet_online')
        bedrock = ObservedBedrock(raknet, netease_mode=True, netease_jwt_chain=chain,
                                 netease_uid=int(client.user_id), netease_protocol=860, priv_key=key)
        bedrock._netease_engine_version = client.engine_version
        bedrock._netease_patch_version = client.g79_latest_version
        bedrock._diagnostic_event = event
        stage('完成加密登录')
        await bedrock.login(f'{host}:{port}', timeout=20)
        stage('加载世界与校验协议')
        await bedrock.initialize_game(timeout=30)
        await asyncio.sleep(5)
        bedrock._raise_if_failed()
        event('initialized', encrypted=bedrock._crypto is not None,
              start_game=bedrock._start_game is not None, runtime_id=bedrock._entity_runtime_id > 0)
        stage('机器人已就绪')
        yield bedrock, client._get_display_name()
    except Exception as exc:
        from proloader.net.bedrock.session import BedrockLoginError, BedrockDisconnectedError
        detail = dict(error_type=type(exc).__name__)
        if isinstance(exc, BedrockDisconnectedError):
            detail['reason'] = exc.reason
        elif isinstance(exc, RentalError):
            detail['stage'] = exc.stage
            if type(exc.code) is int:detail['code'] = exc.code
        elif isinstance(exc, BedrockLoginError):
            # These locally generated errors have stage labels; no JWT/server free text.
            text = str(exc)
            if any(word in text for word in ('等待', '超时', '校验和', '解码', '状态码', '版本不匹配')):
                detail['detail'] = text[:200]
        event('failure', **detail)
        event('transport_trace', entries=list(trace))
        raise
    finally:
        try:
            if raknet:
                event('transport_summary', pending=len(raknet._pending_acks),
                      queued=raknet._send_queue.qsize(), closed=raknet._closed,
                      close_reason=getattr(raknet, 'close_reason', None),
                      accepted=raknet._accepted is not None,
                      pending_frames=[dict(index=k, size=len(v.frame.body), order=v.frame.order_index,
                                           age_s=round(time.monotonic()-v.send_time,3))
                                      for k,v in raknet._pending_acks.items()],
                      ordered={str(ch): dict(expected=raknet._order_recv.get(ch, 0), pending=sorted(buf))
                               for ch, buf in raknet._order_buffer.items()})
            if bedrock:
                await bedrock.close()
            elif raknet:
                await raknet.stop()
        finally:
            client._session.close()
            event('cleanup', socket_closed=raknet is None or raknet._sock.fileno() == -1,
                  tasks_finished=raknet is None or all(t.done() for t in raknet._tasks))


@asynccontextmanager
async def rental_session(store,bot,server_code,password,on_stage=None, *, on_event=None):
    with store.lease(bot):
        async with _rental_session(store,bot,server_code,password,on_stage,on_event) as connection:
            yield connection

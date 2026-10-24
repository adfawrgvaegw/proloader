"""校验已有 chain/私钥，或仅登录并初始化；不发游戏命令。

python3 -m tools.diagnostics.diag_netease_session --help
默认仅离线校验，--connect 才建立会话。私钥文件应为未加密 PEM。
"""
from __future__ import annotations

import argparse
import asyncio
import json
import logging
from pathlib import Path

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ec

from proloader.net.bedrock.session import BedrockLoginError, BedrockSession
from proloader.net.raknet.connection import handshake
from proloader.net.raknet.session import RakNetSession


def parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--chain-file', required=True, type=Path, help='认证 API 返回的 chain JSON 文件')
    p.add_argument('--private-key', required=True, type=Path, help='申请该 chain 时的 P-384 PEM 私钥文件')
    p.add_argument('--host', required=True, help='已通过进入世界 API 获取的地址')
    p.add_argument('--port', required=True, type=int)
    p.add_argument('--uid', required=True, type=int, help='PE 认证的用户 ID')
    p.add_argument('--protocol', choices=(686, 860), type=int, default=860)
    p.add_argument('--timeout', type=float, default=20)
    p.add_argument('--connect', action='store_true', help='连接真实服务器，仅完成登录与初始化后退出')
    return p


async def run(args: argparse.Namespace) -> int:
    stage = '本地认证材料校验'
    bedrock = raknet = None
    try:
        if not 0 < args.port < 65536 or args.uid <= 0 or args.timeout <= 0:
            raise ValueError('端口、UID、超时必须有效')
        key = serialization.load_pem_private_key(args.private_key.read_bytes(), password=None)
        if not isinstance(key, ec.EllipticCurvePrivateKey) or not isinstance(key.curve, ec.SECP384R1):
            raise ValueError('需要 P-384 私钥')
        # 本地预检不创建 socket。复用会话的编码校验，避免两套规则漂移。
        chain = args.chain_file.read_text()
        bedrock = BedrockSession(
            None, netease_mode=True, netease_jwt_chain=chain,
            netease_uid=args.uid, netease_protocol=args.protocol, priv_key=key,
        )
        addr = f'{args.host}:{args.port}'
        bedrock._client_data.ServerAddress = addr
        bedrock._build_connection_request()
        print(json.dumps({'stage': stage, 'ok': True, 'protocol': args.protocol}, ensure_ascii=False))
        if not args.connect:
            return 0
        stage = 'RakNet 离线握手'
        conn = await handshake(args.host, args.port, timeout=args.timeout, protocol=8)
        raknet = RakNetSession(conn)
        bedrock._raknet = raknet
        stage = 'RakNet 在线握手'
        await raknet.start()
        await raknet.connect(timeout=args.timeout)
        stage = 'Bedrock Login'
        await bedrock.login(addr, timeout=args.timeout)
        print(json.dumps({'stage': stage, 'ok': True}, ensure_ascii=False))
        stage = '游戏初始化'
        game = await bedrock.initialize_game(timeout=args.timeout)
        print(json.dumps({'stage': stage, 'ok': True,
                          'entity_unique_id': game.entity_unique_id,
                          'entity_runtime_id': game.entity_runtime_id}, ensure_ascii=False))
        return 0
    except Exception as exc:
        # 不回显认证材料或任意底层异常正文；会话错误仅输出已知的状态/等待阶段。
        detail = str(exc) if isinstance(exc, BedrockLoginError) else type(exc).__name__
        print(json.dumps({'stage': stage, 'ok': False, 'error': detail}, ensure_ascii=False))
        return 1
    finally:
        if raknet is not None:
            try:
                await bedrock.close()
            except Exception as exc:
                print(json.dumps({'stage': '清理连接', 'ok': False,
                                  'error': type(exc).__name__}, ensure_ascii=False))


def main() -> int:
    args = parser().parse_args()
    # 现有协议模块日志可能包含 JWT/挑战内容，诊断默认只输出阶段结果。
    logging.disable(logging.CRITICAL)
    return asyncio.run(run(args))


if __name__ == '__main__':
    raise SystemExit(main())

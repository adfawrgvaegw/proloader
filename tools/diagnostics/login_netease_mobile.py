"""本机手机号短信登录；默认离线。复用同游戏设备，不创建游客。"""
import argparse
import getpass
import json
import logging
import os
import sys
from pathlib import Path

from proloader.net.netease.mobile_login import MobileCookieProvider, ClassicMobileCookieProvider, MobileLoginError


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--login', action='store_true')
    p.add_argument('--recovery', type=Path)
    p.add_argument('--template-cookie', type=Path)
    p.add_argument('--output', type=Path)
    p.add_argument('--diagnostic', type=Path, help='仅本机保存失败响应（0600，不覆盖）')
    p.add_argument('--route', choices=('mobile2', 'classic'), default='mobile2')
    a = p.parse_args()
    logging.disable(logging.CRITICAL)
    if not a.login:
        print(json.dumps(dict(mode='offline', plan='local phone input; one SMS; no game installed')))
        return 0
    if not all((a.recovery, a.template_cookie, a.output)):
        p.error('--login requires --recovery, --template-cookie and --output')
    if not sys.stdin.isatty():
        print(json.dumps(dict(ok=False, error='interactive_terminal_required')))
        return 1
    provider = None
    fd = diagnostic_fd = None
    reserved = saved = False
    try:
        # Reserve the destination before sending SMS; never overwrite a prior identity.
        fd = os.open(a.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        reserved = True
        if a.diagnostic:
            diagnostic_fd = os.open(a.diagnostic, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        provider_class = ClassicMobileCookieProvider if a.route == 'classic' else MobileCookieProvider
        provider = provider_class(json.loads(a.recovery.read_text())['recovery'],
                                        a.template_cookie.read_text())
        print('\n网易手机号登录：1. 输入手机号 → 2. 确认发送短信 → 3. 输入验证码', flush=True)
        print('隐藏输入时屏幕不会显示字符，输入后按回车。', flush=True)
        phone = getpass.getpass('[1/3] 已有网易账号的手机号（隐藏输入）：').strip()
        if input('将发送一条官方登录短信。输入 SEND 发送：').strip() != 'SEND':
            return 1
        print('[2/3] 正在查询账号的短信登录方式，通过后发送一条短信……', flush=True)
        provider.send_sms(phone)
        phone = ''
        print('短信接口已接受请求，请查看手机收到的验证码。', flush=True)
        code = getpass.getpass('[3/3] 短信验证码（隐藏输入）：').strip()
        print('正在验证短信登录……', flush=True)
        provider.verify_sms(code)
        code = ''
        with os.fdopen(fd, 'w') as f:
            fd = None
            f.write(provider.fetch_cookie().cookie)
        saved = True
        print(json.dumps(dict(ok=True, stage='identity', saved=str(a.output), game_authenticated=False)))
        return 0
    except MobileLoginError as e:
        if e.stage == 'sms_info':
            print(f'发送前查询失败：HTTP {e.status}，网易错误码 {e.code}。尚未请求发送短信。')
        elif e.stage == 'sms_existing_account_required':
            print('服务器未确认此号码是已有账号，已停止；未发送短信或注册账号。')
        elif e.stage == 'sms_method_unavailable':
            print('服务器未提供短信登录方式，已停止；未发送短信。')
        elif e.stage == 'sms_activation_required':
            print('账号需要额外激活流程，当前尚未支持；未发送短信。')
        elif e.stage == 'sms_account_selection_required':
            print('官方要求选择关联账号，当前尚未接入选择步骤；未自动选择或完成登录。')
        elif e.stage == 'sms_send':
            print(f'短信请求失败：HTTP {e.status}，网易错误码 {e.code}。尚未进入验证码输入步骤。')
            print('程序已停止，不会自动重发。错误码的具体原因需根据服务器详细响应确认。')
        else:
            print(f'登录失败：阶段 {e.stage}，HTTP {e.status}，网易错误码 {e.code}。')
        if e.code == 1006:
            print('官方SDK将此响应交给排队处理流程；当前终端尚不支持该流程，已停止。')
        elif e.code == 1373:
            print('官方要求手机主动发送短信验证；当前终端尚不支持该流程，已停止。')
        if diagnostic_fd is not None:
            with os.fdopen(diagnostic_fd, 'w') as f:
                diagnostic_fd = None
                json.dump(dict(report=e.report(), response=e.diagnostic), f, ensure_ascii=False)
            print(f'详细错误已仅保存在本机：{a.diagnostic}（请勿分享文件内容）')
        print(json.dumps(e.report()))
        if e.verify_url:
            print('请在本机浏览器完成官方验证（链接含凭据，请勿分享）：')
            print(e.verify_url)
            input('完成后按回车结束；不会自动重发短信或提交登录：')
        return 1
    except (OSError, ValueError, KeyError, TypeError, EOFError, KeyboardInterrupt) as e:
        print(json.dumps(dict(ok=False, error=type(e).__name__)))
        return 1
    finally:
        if provider:
            provider.close()
        if fd is not None:
            os.close(fd)
        if diagnostic_fd is not None:
            os.close(diagnostic_fd)
            a.diagnostic.unlink(missing_ok=True)
        if reserved and not saved:
            a.output.unlink(missing_ok=True)


if __name__ == '__main__':
    raise SystemExit(main())

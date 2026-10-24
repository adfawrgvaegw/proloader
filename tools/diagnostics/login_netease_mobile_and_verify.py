"""本机短信登录，然后依次验证游戏认证和同身份再次认证。"""
import json
import logging
import sys
from datetime import datetime
from pathlib import Path
from zoneinfo import ZoneInfo
from . import login_netease_mobile
from .diag_netease_pe import diagnose


def main():
    logging.disable(logging.CRITICAL)
    root=Path(__file__).resolve().parents[2]
    stamp=datetime.now(ZoneInfo('Asia/Shanghai')).strftime('%Y%m%d_%H%M%S')
    cookie=root/'cache'/f'mobile_cookie_{stamp}.json'
    sys.argv=['mobile','--login','--route','classic','--recovery',str(root/'cache/mpay_guest_20261017_failure.json'),
              '--template-cookie',str(root/'cache/mpay_guest_verified_cookie.json'),'--output',str(cookie),
              '--diagnostic',str(root/'cache'/f'mobile_failure_{stamp}.json')]
    if login_netease_mobile.main()!=0:
        return 1
    first=diagnose(cookie,root/'cache'/f'mobile_pe_{stamp}.json',engine_source='official-download')
    print(json.dumps(first,ensure_ascii=False))
    if not first['ok']:
        return 1
    second=diagnose(cookie,root/'cache'/f'mobile_pe_reuse_{stamp}.json',engine_source='official-download')
    print(json.dumps(dict(second,same_identity_reauthenticated=second['ok']),ensure_ascii=False))
    return 0 if second['ok'] else 1


if __name__=='__main__':
    raise SystemExit(main())

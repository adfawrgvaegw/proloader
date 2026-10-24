"""Official MPay mobile2 SMS flow, transcribed from Android 3.9.15.297907.

See docs/netease-identity-investigation.md for the exact first-party classes.
No device registration, retries, real-name submission, or guest creation here.
"""
from __future__ import annotations

import base64
import json
import re
import secrets
import time
import uuid
from dataclasses import dataclass, field
from urllib.parse import urlsplit

import requests

from .user_center import CookieGrant

ORIGIN = 'https://service.mkey.163.com'
INFO = '/mpay/api/users/login/mobile2/info'
SEND = '/mpay/api/users/login/mobile2/send_verify_sms'
LOGIN = '/mpay/api/users/login/mobile2/login_by_sms'


def create_classic_mobile_provider(*, session=None):
    """Create one MPay device for an explicit add-bot wizard; never creates a guest.

    Device request fields follow g79client/account/mpay/{client,device}.go;
    version fields use the same inspected Android3.9 profile as the SMS flow.
    """
    session = session or requests.Session()
    timestamp = int(time.time())
    udid, mac = secrets.token_hex(8), secrets.token_hex(16)
    transaction = f'{udid}_{timestamp}114_307044629'
    transid = f'{udid}_{timestamp}10114_399039478'
    form = dict(mac=mac, urs_udid=secrets.token_hex(20), init_urs_device='0',
                unique_id=f'{uuid.uuid4()}{timestamp}20114', brand='Redmi', device_name='MuMu',
                device_type='tablet', device_model='23117RK66C', resolution='900*1600',
                system_name='Android', system_version='12', udid=udid, app_channel='netease',
                ext_ci=secrets.token_hex(32), ci_code='3', game_id='aecfrxodyqaaaajp-g-x19',
                gv='840297907', gvn='3.9.15.297907', cv='a5.16.0', sv='32',
                app_type='games', app_mode='2', mcount_app_key='EEkEEXLymcNjM42yLY3Bn6AO15aGy4yq',
                mcount_transaction_id=transaction, transid=transid, _cloud_extra_base64='e30=')
    try:
        response = session.post(ORIGIN + '/mpay/games/aecfrxodyqaaaajp-g-x19/devices', data=form,
                    headers={'User-Agent':'com.netease.x19/840297907 NeteaseMobileGame/a5.16.0 (23117RK66C;32)'},
                    timeout=20, allow_redirects=False)
        payload = response.json()
        if response.status_code != 201 or not payload.get('device',{}).get('id'):
            raise MobileLoginError('device', code=payload.get('code'), status=response.status_code,
                    verify_url=official_verification_url(payload.get('verify_url')))
        device = dict(payload['device'], udid=udid, transid=transid,
                      mcount_transaction_id=transaction)
        sauth = dict(gameid='x19', deviceid=device['id'], udid=udid, sdkuid='', sessionid='',
                     app_channel='netease', login_channel='netease', source_app_channel='netease',
                     platform='ad', source_platform='ad', sdk_version='5.16.0', is_unisdk_guest=0,
                     aim_info='{"aim":"127.0.0.1","country":"CN","tz":"+0800","tzid":""}',
                     get_access_token='1', ip='127.0.0.1', gas_token='', client_login_sn='')
        template = json.dumps(dict(sauth_json=json.dumps(sauth), mac_addr=mac, ram='8034369536',rom='128849018880'))
        return ClassicMobileCookieProvider(device, template, session)
    except MobileLoginError:
        session.close()
        raise
    except (requests.RequestException, ValueError, KeyError, TypeError):
        session.close()
        raise MobileLoginError('device') from None


class MobileLoginError(RuntimeError):
    def __init__(self, stage, *, code=None, status=None, verify_url=None, diagnostic=None):
        self.stage, self.code, self.status = stage, code, status
        self.verify_url = verify_url
        # Contains private server details; never include in report()/exception text.
        self.diagnostic = diagnostic
        super().__init__('mobile_login_failed')

    def report(self):
        return dict(ok=False, stage=self.stage, code=self.code, http_status=self.status,
                    verification_required=bool(self.verify_url))


def official_verification_url(value):
    if not isinstance(value, str):
        return None
    try:
        p = urlsplit(value)
        port = p.port
    except ValueError:
        return None
    if (p.scheme == 'https' and p.hostname and
            (p.hostname == '163.com' or p.hostname.endswith('.163.com')) and
            port in (None, 443) and not p.username and not p.password):
        return value
    return None


@dataclass
class MobileCookieProvider:
    """One in-memory SMS session; fetch_cookie only consumes a completed login."""
    device: dict = field(repr=False)
    template: str = field(repr=False)
    session: requests.Session = field(default_factory=requests.Session, repr=False)
    _phone: str = field(default='', init=False, repr=False)
    _state: str = field(default='new', init=False)
    _cookie: str = field(default='', init=False, repr=False)

    def __post_init__(self):
        sauth = json.loads(json.loads(self.template)['sauth_json'])
        if (sauth.get('gameid') != 'x19' or
                sauth.get('deviceid') != self.device.get('id') or
                not all(isinstance(self.device.get(k), str) and self.device[k]
                        for k in ('id', 'udid', 'transid', 'mcount_transaction_id'))):
            raise ValueError('invalid_device_or_game')

    def _form(self):
        # k.a common fields; APK versionCode/versionName, MPay MpayApi.getVersion.
        return dict(game_id='aecfrxodyqaaaajp-g-x19', jf_game_id='x19',
                    gv='840297907', gvn='3.9.15.297907', cv='a5.16.0', sv='32',
                    app_type='games', app_mode='2', app_channel='netease',
                    mcount_app_key='EEkEEXLymcNjM42yLY3Bn6AO15aGy4yq',
                    mcount_transaction_id=self.device['mcount_transaction_id'],
                    transid=self.device['transid'], _cloud_extra_base64='e30=', sc='1',
                    device_id=self.device['id'])

    def _request(self, stage, method, path, form, *, query=None):
        try:
            r = self.session.request(method, ORIGIN + path, params=form if method == 'GET' else query,
                                     data=form if method == 'POST' else None,
                                     headers={'User-Agent': 'com.netease.x19/840297907 NeteaseMobileGame/a5.16.0 (23117RK66C;32)'},
                                     timeout=20, allow_redirects=False)
            data = r.json()
            if not isinstance(data, dict):
                raise ValueError('invalid_response')
        except (requests.RequestException, ValueError):
            raise MobileLoginError(stage) from None
        code = data.get('code')
        # Official k treats the presence of code as an error, including zero.
        if not 200 <= r.status_code < 300 or 'code' in data:
            raise MobileLoginError(stage, code=code if type(code) is int else None,
                                   status=r.status_code,
                                   verify_url=official_verification_url(data.get('verify_url')),
                                   diagnostic=data)
        return data

    def send_sms(self, phone):
        if self._state != 'new':
            raise ValueError('sms_already_attempted')
        if not re.fullmatch(r'1[3-9][0-9]{9}', phone):
            raise ValueError('invalid_mobile_number')
        self._phone, self._state = phone, 'send_attempted'
        form = self._form()
        form.update(mobile_num=phone, login_for='1')
        # request/login/x runs before y. The server determines allowed methods.
        info = self._request('sms_info', 'GET', INFO, form)
        if info.get('registered') is not True:
            raise MobileLoginError('sms_existing_account_required')
        methods = info.get('allow_methods')
        if not isinstance(methods, list) or not any(type(m) is int and m == 1 for m in methods):
            raise MobileLoginError('sms_method_unavailable')
        form = self._form()
        # SDK f$e labels method 1 other_mobile_sms and 2 other_mobile_vcode;
        # task Z's success callback labels 2 vvc_send_success (voice).
        form.update(mobile_num=phone, method='1')
        # request/login/y: GET, device_id/mobile_num/method. No automatic retry.
        self._request('sms_send', 'GET', SEND, form)
        self._state = 'sent'

    def verify_sms(self, code):
        if self._state != 'sent':
            raise ValueError('sms_not_sent')
        if not re.fullmatch(r'[0-9]{4,8}', code):
            raise ValueError('invalid_sms_code')
        self._state = 'verify_attempted'
        form = self._form()
        form.update(mobile_num=self._phone, sms_code=code,
                    opt_fields='nickname,avatar,realname_status,mobile_bind_status')
        query = {'un': base64.b64encode(self._phone.encode()).decode()}
        data = self._request('sms_login', 'POST', LOGIN, form, query=query)
        self._accept_identity(data)

    def _accept_identity(self, data):
        user = data.get('user')
        if not isinstance(user, dict) or not all(isinstance(user.get(k), str) and user[k]
                                                for k in ('id', 'token')):
            raise MobileLoginError('sms_identity')
        if user.get('login_channel', 'netease') != 'netease':
            raise MobileLoginError('unsupported_login_channel')
        outer = json.loads(self.template)
        sauth = json.loads(outer['sauth_json'])
        sauth.update(sdkuid=user['id'], sessionid=user['token'], sdk_version='5.16.0',
                     platform='ad', source_platform='ad', is_unisdk_guest=0,
                     udid=user.get('udid') or self.device['udid'],
                     client_login_sn=secrets.token_hex(16).upper())
        outer['sauth_json'] = json.dumps(sauth, separators=(',', ':'))
        self._cookie = json.dumps(outer, separators=(',', ':'))
        self._phone, self._state = '', 'authenticated'

    def fetch_cookie(self):
        if self._state != 'authenticated':
            raise ValueError('mobile_login_not_complete')
        return CookieGrant(self._cookie, False)

    def close(self):
        self._phone = self._cookie = ''
        self._state = 'closed'
        self.session.close()


class ClassicMobileCookieProvider(MobileCookieProvider):
    """SDK login type 7: mobile/user_info → get_sms → verify_sms → finish.

    Uses existing accounts only. Related-account selection and other verification
    branches stop explicitly; never guess an identity or register one.
    """

    def send_sms(self, phone):
        if self._state != 'new':
            raise ValueError('sms_already_attempted')
        if not re.fullmatch(r'1[3-9][0-9]{9}', phone):
            raise ValueError('invalid_mobile_number')
        self._phone, self._state = phone, 'send_attempted'
        form = self._form()
        form.update(oversea_user='0', device_udid=self.device['udid'], country_codes_hash='')
        listing = self._request('login_methods', 'GET',
                                '/mpay/games/' + form['game_id'] + '/login_methods', form)
        entrance = listing.get('entrance')
        if (not isinstance(entrance, list) or not any(
                isinstance(row, list) and any(isinstance(item, dict) and
                    type(item.get('type')) is int and item['type'] == 7 for item in row)
                for row in entrance)):
            raise MobileLoginError('sms_method_unavailable')
        form = self._form()
        form.update(mobile=phone, login_for='1')
        info = self._request('sms_info', 'POST', '/mpay/api/users/login/mobile/user_info', form)
        if info.get('registered') is not True:
            raise MobileLoginError('sms_existing_account_required')
        if type(info.get('pre_active')) is not int or info['pre_active'] != 0:
            raise MobileLoginError('sms_activation_required')
        form = self._form()
        form.update(mobile=phone, urs_udid=self.device['udid'])
        self._request('sms_send', 'POST', '/mpay/api/users/login/mobile/get_sms', form)
        self._state = 'sent'

    def verify_sms(self, code):
        if self._state != 'sent':
            raise ValueError('sms_not_sent')
        if not re.fullmatch(r'[0-9]{4,8}', code):
            raise ValueError('invalid_sms_code')
        self._state = 'verify_attempted'
        form = self._form()
        form.update(mobile=self._phone, smscode=code, up_content='', login_for='1',
                    urs_udid=self.device['udid'])
        response = self._request('sms_verify', 'POST', '/mpay/api/users/login/mobile/verify_sms', form)
        ticket = response.get('ticket')
        if not isinstance(ticket, str) or not ticket:
            raise MobileLoginError('sms_ticket_missing')
        related = response.get('related_accounts', [])
        if not isinstance(related, list) or related:
            raise MobileLoginError('sms_account_selection_required')
        form = self._form()
        form.update(ticket=ticket, login_for='1', urs_udid=self.device['udid'],
                    opt_fields='nickname,avatar,realname_status,mobile_bind_status,exit_popup_info,'
                               'mask_related_mobile,related_login_status,detect_is_new_user')
        query = {'un': base64.b64encode(self._phone.encode()).decode()}
        data = self._request('sms_finish', 'POST', '/mpay/api/users/login/mobile/finish', form, query=query)
        self._accept_identity(data)

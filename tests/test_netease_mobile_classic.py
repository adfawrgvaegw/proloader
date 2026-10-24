import json
from unittest.mock import Mock
import pytest
from proloader.net.netease.mobile_login import ClassicMobileCookieProvider, MobileLoginError
from tests.test_netease_mobile_login import PHONE, DEVICE, TEMPLATE

METHODS = {'entrance': [[{'type': 7, 'name': '手机账号'}]]}
INFO = {'registered': True, 'pre_active': 0}


def provider(*responses):
    session = Mock()
    session.request.side_effect = [Mock(status_code=status, json=Mock(return_value=body))
                                   for status, body in responses]
    return ClassicMobileCookieProvider(DEVICE, TEMPLATE, session)


def test_classic_existing_account_flow():
    p = provider((200, METHODS), (200, INFO), (200, {}),
                 (200, {'ticket': 'PRIVATE_TICKET', 'related_accounts': []}),
                 (200, {'user': {'id': 'UID', 'token': 'TOKEN'}}))
    p.send_sms(PHONE)
    p.verify_sms('123456')
    calls = p.session.request.call_args_list
    assert [c.args[0] for c in calls] == ['GET', 'POST', 'POST', 'POST', 'POST']
    assert [c.args[1].rsplit('/', 1)[1] for c in calls] == [
        'login_methods', 'user_info', 'get_sms', 'verify_sms', 'finish']
    assert calls[1].kwargs['data']['mobile'] == PHONE
    assert calls[2].kwargs['data']['urs_udid'] == DEVICE['udid']
    assert calls[3].kwargs['data']['smscode'] == '123456'
    assert calls[4].kwargs['data']['ticket'] == 'PRIVATE_TICKET'
    assert all(not c.kwargs['allow_redirects'] for c in calls)
    assert json.loads(json.loads(p.fetch_cookie().cookie)['sauth_json'])['sdkuid'] == 'UID'
    with pytest.raises(ValueError):
        p.send_sms(PHONE)
    with pytest.raises(ValueError):
        p.verify_sms('123456')
    p.close()


@pytest.mark.parametrize('methods', [{}, {'entrance': []}, {'entrance': [[{'type': '7'}]]},
                                     {'entrance': [[{'type': 25}]]}])
def test_unlisted_mobile_route_has_no_phone_query_or_sms(methods):
    p = provider((200, methods))
    with pytest.raises(MobileLoginError, match='mobile_login_failed'):
        p.send_sms(PHONE)
    assert p.session.request.call_count == 1


@pytest.mark.parametrize('info', [{}, {'registered': False, 'pre_active': 0},
    {'registered': True, 'pre_active': 1}, {'registered': True, 'pre_active': False}])
def test_no_registration_or_activation(info):
    p = provider((200, METHODS), (200, info))
    with pytest.raises(MobileLoginError):
        p.send_sms(PHONE)
    assert p.session.request.call_count == 2


def test_challenge_stops_without_retry_or_sms():
    p = provider((200, METHODS), (400, {'code': 1351,
        'verify_url': 'https://service.mkey.163.com/private', 'reason': 'PRIVATE'}))
    with pytest.raises(MobileLoginError) as error:
        p.send_sms(PHONE)
    assert error.value.verify_url and error.value.stage == 'sms_info'
    assert 'PRIVATE' not in json.dumps(error.value.report())
    assert p.session.request.call_count == 2
    with pytest.raises(ValueError):
        p.send_sms(PHONE)


@pytest.mark.parametrize('body', [{}, {'ticket': ''}, {'ticket': 'T', 'related_accounts': [{}]},
                                 {'ticket': 'T', 'related_accounts': None}])
def test_verify_does_not_guess_ticket_or_related_account(body):
    p = provider((200, METHODS), (200, INFO), (200, {}), (200, body))
    p.send_sms(PHONE)
    with pytest.raises(MobileLoginError):
        p.verify_sms('123456')
    assert p.session.request.call_count == 4
    with pytest.raises(ValueError):
        p.fetch_cookie()

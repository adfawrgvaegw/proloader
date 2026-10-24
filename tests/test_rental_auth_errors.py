from unittest.mock import Mock
import pytest
import requests
from proloader.net.netease.runtime import _refresh, RentalError
from proloader.net.netease.pe_profiles import PEMetadataMismatchError, PEAuthenticationError


@pytest.mark.parametrize('error,expected',[
    (PEMetadataMismatchError('3.10','3.10.1'),'尚未适配'),
    (PEAuthenticationError(2001),'2001'),
    (requests.Timeout('SECRET RESPONSE'),'超时'),
    (requests.HTTPError('SECRET RESPONSE',response=Mock(status_code=503)),'503'),
    (requests.ConnectionError('SECRET RESPONSE'),'无法连接'),
])
def test_auth_error_is_actionable_and_does_not_leak_secret_or_save_bad_pe(error,expected):
    client=Mock();client.authenticate_with_cookie.side_effect=error
    store=Mock();store.cookie.return_value='PRIVATE COOKIE'
    with pytest.raises(RentalError) as caught:_refresh(client,store,object())
    message=str(caught.value)
    assert expected in message
    assert 'SECRET' not in message and 'PRIVATE' not in message
    store.save_pe.assert_not_called()

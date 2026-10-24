"""只输出已知错误分类和数字码，避免把任意服务端字段带入日志。"""


def summarize_response(payload: dict) -> dict:
    result = {}
    if type(payload.get('code')) is int:
        result['code'] = payload['code']
    entity = payload.get('entity')
    if not isinstance(entity, dict):
        return result
    for source, target in (('code', 'service_code'), ('subcode', 'service_subcode')):
        if type(entity.get(source)) is int:
            result[target] = entity[source]
    if entity.get('status') == 'invalid param: sdkuid':
        result['reason'] = 'sdkuid_rejected'
    return result

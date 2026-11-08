export async function api(method, path, body, cb) {
  let data;
  try {
    const headers = { Accept: 'application/json' };
    if (body != null) { headers['Content-Type'] = 'application/json; charset=utf-8'; }
    const response = await fetch(path, {
      method,
      headers,
      body: body == null ? undefined : JSON.stringify(body),
    });
    try {
      data = JSON.parse((await response.text()) || '{}');
      if (!data || typeof data !== 'object' || Array.isArray(data)) { throw new Error('invalid response'); }
    } catch {
      data = { ok: false, message: '管理台返回内容无法读取' };
    }
    if (!response.ok) {
      data.ok = false;
      data.message = data.message || '管理台请求失败';
    }
  } catch {
    data = { ok: false, message: '管理台连接失败，请重新打开服务端 GUI' };
  }
  cb(data);
}

import { responseText } from './format.js';

export function api(method, path, body, cb) {
  var xhr = new XMLHttpRequest();
  xhr.open(method, path, true);
  xhr.setRequestHeader('Accept', 'application/json');
  if (body != null) { xhr.setRequestHeader('Content-Type', 'application/json; charset=utf-8'); }
  xhr.onreadystatechange = function () {
    if (xhr.readyState !== 4) { return; }
    var data = null;
    try { data = JSON.parse(xhr.responseText || '{}'); } catch (e) { data = { ok: false, message: '管理台返回内容无法读取' }; }
    if (xhr.status < 200 || xhr.status >= 300) {
      data.ok = false;
      data.message = data.message || '管理台请求失败';
    }
    cb(data);
  };
  xhr.onerror = function () { cb({ ok: false, message: '管理台连接失败，请重新打开服务端 GUI' }); };
  xhr.send(body == null ? null : JSON.stringify(body));
}

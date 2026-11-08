import assert from 'node:assert/strict';
import test from 'node:test';

import { api } from '../modules/api.js';

test('admin API keeps request and error behavior', async () => {
  const originalFetch = globalThis.fetch;
  try {
    globalThis.fetch = async (path, options) => {
      assert.equal(path, '/api/state');
      assert.equal(options.method, 'GET');
      assert.equal(options.headers.Accept, 'application/json');
      assert.equal(options.body, undefined);
      return { ok: true, text: async () => '{"ok":true,"config":{}}' };
    };
    let result;
    await api('GET', '/api/state', null, (data) => { result = data; });
    assert.deepEqual(result, { ok: true, config: {} });

    globalThis.fetch = async (_path, options) => {
      assert.equal(options.headers['Content-Type'], 'application/json; charset=utf-8');
      assert.equal(options.body, '{"ip":"127.0.0.1"}');
      return { ok: false, text: async () => '{"message":"请求已拒绝"}' };
    };
    await api('POST', '/api/security/unblock', { ip: '127.0.0.1' }, (data) => { result = data; });
    assert.deepEqual(result, { ok: false, message: '请求已拒绝' });

    globalThis.fetch = async () => ({ ok: true, text: async () => '<html>error</html>' });
    await api('GET', '/api/state', null, (data) => { result = data; });
    assert.deepEqual(result, { ok: false, message: '管理台返回内容无法读取' });

    globalThis.fetch = async () => { throw new Error('connection refused'); };
    await api('GET', '/api/state', null, (data) => { result = data; });
    assert.deepEqual(result, { ok: false, message: '管理台连接失败，请重新打开服务端 GUI' });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

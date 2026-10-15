import assert from 'node:assert/strict';
import test from 'node:test';
import {
  bytesText, rateText, numberText, isFailureRecord, eventResultClass,
  eventActorText, requestTargetText, responseText, timeText,
} from '../modules/format.js';

test('traffic formatting keeps zero and unit boundaries', () => {
  assert.equal(numberText(null), '0');
  assert.equal(bytesText(0), '0 B');
  assert.equal(bytesText(1023), '1023 B');
  assert.equal(bytesText(1024), '1.00 KB');
  assert.equal(bytesText(1024 ** 2), '1.00 MB');
  assert.equal(bytesText(1024 ** 3), '1.00 GB');
  assert.equal(rateText(2048), '2.00 KB/s');
});

test('log classification accepts request and business records', () => {
  assert.equal(isFailureRecord({ result: 'blocked' }), true);
  assert.equal(isFailureRecord({ response: { code: 'auth_failed' } }), true);
  assert.equal(isFailureRecord({ code: 'target_unreachable' }), true);
  assert.equal(isFailureRecord({ result: 'ok' }), false);
  assert.equal(isFailureRecord(null), false);
  assert.equal(eventResultClass('connected'), 'ok');
  assert.equal(eventResultClass('denied'), 'warn');
  assert.equal(eventResultClass('future-result'), 'muted');
});

test('diagnostic fields remain visible without evaluating their content', () => {
  assert.equal(eventActorText({ client_id: '<script>client</script>' }), '<script>client</script>');
  assert.equal(requestTargetText({ request: { forward_name: 'rdp' } }), 'rdp');
  assert.equal(responseText({ ok: false, code: 'auth_failed', message: 'denied' }), 'fail / auth_failed / denied');
  assert.equal(timeText('invalid-time'), 'invalid-time');
  assert.equal(timeText(null), '-');
});

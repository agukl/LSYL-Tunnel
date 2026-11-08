import assert from 'node:assert/strict';
import test from 'node:test';

import { actionButton } from '../modules/dom.js';

test('action button keeps table command behavior', () => {
  const originalDocument = globalThis.document;
  const button = {};
  try {
    globalThis.document = { createElement: (tag) => {
      assert.equal(tag, 'button');
      return button;
    } };
    let clicked = false;
    assert.equal(actionButton('删除', 'danger', () => { clicked = true; }), button);
    assert.equal(button.type, 'button');
    assert.equal(button.className, 'btn small danger');
    assert.equal(button.textContent, '删除');
    button.onclick();
    assert.equal(clicked, true);
  } finally {
    globalThis.document = originalDocument;
  }
});

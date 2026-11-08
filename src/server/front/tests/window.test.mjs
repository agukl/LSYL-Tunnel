import assert from 'node:assert/strict';
import test from 'node:test';

import { bindWindowControls } from '../modules/window.js';

test('window controls call the Wails window API', async () => {
  const originals = {
    document: globalThis.document,
    window: globalThis.window,
    wails: globalThis.wails,
  };
  const elements = Object.fromEntries(['windowMinimize', 'windowMaximize', 'windowClose'].map((id) => [id, {
    setAttribute(name, value) { this[name] = value; },
  }]));
  const calls = [];
  let maximized = false;
  try {
    globalThis.document = { getElementById: (id) => elements[id] };
    globalThis.window = { addEventListener: () => {} };
    globalThis.wails = { Window: {
      Minimise: () => calls.push('minimize'),
      ToggleMaximise: async () => { maximized = !maximized; calls.push('maximize'); },
      IsMaximised: async () => maximized,
      Close: () => calls.push('close'),
    } };

    bindWindowControls();
    elements.windowMinimize.onclick();
    await elements.windowMaximize.onclick();
    elements.windowClose.onclick();

    assert.deepEqual(calls, ['minimize', 'maximize', 'close']);
    assert.equal(elements.windowMaximize.title, '还原');
  } finally {
    globalThis.document = originals.document;
    globalThis.window = originals.window;
    globalThis.wails = originals.wails;
  }
});

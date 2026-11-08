import { $ } from './dom.js';

export function updateLogHorizontalScroll(tableID, barID, trackID) {
  var table = $(tableID);
  var bar = $(barID);
  var track = $(trackID);
  if (!table || !bar || !track) { return; }
  var wrap = table.parentNode;
  if (!wrap.clientWidth) { return; }
  var width = Math.max(table.scrollWidth, table.offsetWidth);
  bar.style.display = width > wrap.clientWidth ? 'block' : 'none';
  // Both scrollbars must have the same range, including any vertical scrollbar space.
  track.style.width = Math.max(0, width + bar.clientWidth - wrap.clientWidth) + 'px';
  bar.scrollLeft = wrap.scrollLeft;
}

export function bindLogHorizontalScroll(tableID, barID, trackID) {
  var table = $(tableID);
  var bar = $(barID);
  var wrap = table.parentNode;
  bar.onscroll = function () { wrap.scrollLeft = bar.scrollLeft; };
  wrap.onscroll = function () { bar.scrollLeft = wrap.scrollLeft; };
  updateLogHorizontalScroll(tableID, barID, trackID);
}

export function updateLogScrollbars() {
  updateLogHorizontalScroll('businessTable', 'businessScrollBar', 'businessScrollTrack');
  updateLogHorizontalScroll('requestTable', 'requestScrollBar', 'requestScrollTrack');
}

function stopMouseEvent(evt) {
  evt.preventDefault();
  evt.stopPropagation();
  return false;
}

function tableWidthStorageKey(table) {
  return 'lsyl.table.widths.' + table.id;
}

function getLocalStorage() {
  try {
    return window.localStorage || null;
  } catch (e) {
    return null;
  }
}

function loadTableColumnWidths(table) {
  var storage = getLocalStorage();
  if (!storage || !table.id) { return null; }
  try {
    var text = storage.getItem(tableWidthStorageKey(table));
    if (!text) { return null; }
    var widths = JSON.parse(text);
    return widths && widths.length ? widths : null;
  } catch (e) {
    return null;
  }
}

function saveTableColumnWidths(table, widths) {
  var storage = getLocalStorage();
  if (!storage || !table.id) { return; }
  try {
    storage.setItem(tableWidthStorageKey(table), JSON.stringify(widths));
  } catch (e) {}
}

function getTableHeaders(table) {
  var thead = table.tHead;
  if (!thead) { return []; }
  var rows = thead.getElementsByTagName('tr');
  if (!rows.length) { return []; }
  return rows[0].getElementsByTagName('th');
}

function ensureTableColGroup(table, count) {
  var groups = table.getElementsByTagName('colgroup');
  var group = groups.length ? groups[0] : null;
  if (!group) {
    group = document.createElement('colgroup');
    table.insertBefore(group, table.firstChild);
  }
  while (group.childNodes.length < count) {
    group.appendChild(document.createElement('col'));
  }
  while (group.childNodes.length > count) {
    group.removeChild(group.lastChild);
  }
  return group;
}

function currentTableColumnWidths(table, headers) {
  var widths = [];
  var group = ensureTableColGroup(table, headers.length);
  var cols = group.getElementsByTagName('col');
  for (var i = 0; i < headers.length; i++) {
    var width = parseInt(cols[i].style.width, 10);
    if (!width && headers[i].offsetWidth) {
      width = headers[i].offsetWidth;
    }
    if (!width) { return null; }
    widths.push(width);
  }
  return widths;
}

function applyTableColumnWidths(table, widths) {
  var group = ensureTableColGroup(table, widths.length);
  var cols = group.getElementsByTagName('col');
  var total = 0;
  for (var i = 0; i < widths.length; i++) {
    var width = Math.max(56, parseInt(widths[i], 10) || 56);
    cols[i].style.width = width + 'px';
    total += width;
  }
  if (total > 0) {
    table.style.width = total + 'px';
    table.style.minWidth = total + 'px';
    var handles = table.querySelectorAll('.column-resizer');
    for (var j = 0; j < handles.length; j++) {
      var pixels = Math.max(56, parseInt(widths[j], 10) || 56);
      handles[j].setAttribute('aria-valuenow', Math.round(pixels / total * 100));
      handles[j].setAttribute('aria-valuetext', pixels + ' 像素');
    }
  }
  updateLogScrollbars();
}

function beginColumnResize(table, headers, index, evt) {
  var startX = evt.clientX;
  var startWidths = currentTableColumnWidths(table, headers);
  if (!startWidths) { return stopMouseEvent(evt); }
  document.body.classList.add('column-resizing');

  var moveHandler = function (moveEvt) {
    var next = [];
    for (var i = 0; i < startWidths.length; i++) { next.push(startWidths[i]); }
    next[index] = Math.max(56, startWidths[index] + (moveEvt.clientX - startX));
    applyTableColumnWidths(table, next);
    stopMouseEvent(moveEvt);
  };

  var upHandler = function (upEvt) {
    document.removeEventListener('mousemove', moveHandler, true);
    document.removeEventListener('mouseup', upHandler, true);
    document.body.classList.remove('column-resizing');
    var finalWidths = currentTableColumnWidths(table, headers);
    if (finalWidths) { saveTableColumnWidths(table, finalWidths); }
    stopMouseEvent(upEvt);
  };

  document.addEventListener('mousemove', moveHandler, true);
  document.addEventListener('mouseup', upHandler, true);
  return stopMouseEvent(evt);
}

function bindResizableTable(table) {
  if (!table || !table.id || table.getAttribute('data-resizable-bound') === '1') { return; }
  var headers = getTableHeaders(table);
  if (!headers.length) { return; }
  table.classList.add('resizable-table');
  for (let i = 0; i < headers.length; i++) {
    var handle = document.createElement('span');
    handle.className = 'column-resizer';
    handle.title = '拖拽或按左右方向键调整列宽';
    handle.tabIndex = 0;
    handle.setAttribute('role', 'separator');
    handle.setAttribute('aria-orientation', 'vertical');
    handle.setAttribute('aria-valuemin', '0');
    handle.setAttribute('aria-valuemax', '100');
    handle.setAttribute('aria-label', '调整' + headers[i].textContent + '列宽');
    handle.onmousedown = function (evt) {
      return beginColumnResize(table, headers, i, evt);
    };
    handle.onkeydown = function (evt) {
      if (evt.key !== 'ArrowLeft' && evt.key !== 'ArrowRight') { return; }
      var widths = currentTableColumnWidths(table, headers);
      if (!widths) { return; }
      widths[i] = Math.max(56, widths[i] + (evt.key === 'ArrowRight' ? 12 : -12));
      applyTableColumnWidths(table, widths);
      saveTableColumnWidths(table, widths);
      evt.preventDefault();
    };
    headers[i].appendChild(handle);
  }
  table.setAttribute('data-resizable-bound', '1');
}

export function initResizableTable(table) {
  var headers = getTableHeaders(table);
  if (!headers.length) { return; }
  bindResizableTable(table);
  var stored = loadTableColumnWidths(table);
  if (stored && stored.length === headers.length) {
    applyTableColumnWidths(table, stored);
    table.setAttribute('data-resizable-sized', '1');
    return;
  }
  if (table.getAttribute('data-resizable-sized') !== '1' && table.offsetWidth) {
    var widths = currentTableColumnWidths(table, headers);
    if (widths) {
      applyTableColumnWidths(table, widths);
      table.setAttribute('data-resizable-sized', '1');
    }
  }
}

export function initResizableTables() {
  var tables = document.querySelectorAll('table.data-table');
  for (var i = 0; i < tables.length; i++) {
    initResizableTable(tables[i]);
  }
}

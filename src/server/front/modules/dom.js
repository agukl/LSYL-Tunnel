export function $(id) { return document.getElementById(id); }

export function setText(id, text) {
  var node = $(id);
  if (node) { node.textContent = text == null || text === '' ? '-' : String(text); }
}

export function hiddenInput(value, name) {
  var input = document.createElement('input');
  input.type = 'hidden';
  input.value = value;
  input.setAttribute('data-name', name);
  return input;
}

export function preventEvent(event) {
  if (!event) { return; }
  if (event.preventDefault) { event.preventDefault(); }
  event.returnValue = false;
}

export function hasClass(node, className) {
  return (' ' + (node.className || '') + ' ').indexOf(' ' + className + ' ') >= 0;
}

export function appendPlainCell(row, text, className) {
  var td = document.createElement('td');
  if (className) { td.className = className; }
  td.textContent = text == null || text === '' ? '-' : String(text);
  td.title = td.textContent;
  row.appendChild(td);
}

export function appendBadgeCell(row, text, className) {
  var td = document.createElement('td');
  var span = document.createElement('span');
  span.className = 'event-badge ' + (className || '');
  span.textContent = text || '-';
  td.appendChild(span);
  row.appendChild(td);
}

export function appendEmptyRow(tbody, colspan, text) {
  var tr = document.createElement('tr');
  tr.className = 'empty-row-wrap';
  var td = document.createElement('td');
  td.colSpan = colspan;
  td.className = 'empty-row';
  td.textContent = text;
  tr.appendChild(td);
  tbody.appendChild(tr);
}

export function removeEmptyRows(tbody) {
  if (!tbody) { return; }
  var rows = tbody.querySelectorAll('.empty-row-wrap');
  for (var i = rows.length - 1; i >= 0; i--) {
    rows[i].parentNode.removeChild(rows[i]);
  }
}

export function hasDataRows(tbody) {
  if (!tbody) { return false; }
  var rows = tbody.getElementsByTagName('tr');
  for (var i = 0; i < rows.length; i++) {
    if (rows[i].className !== 'empty-row-wrap') { return true; }
  }
  return false;
}

export function setClassEnabled(node, className, enabled) {
  if (!node) { return; }
  var current = ' ' + (node.className || '') + ' ';
  var token = ' ' + className + ' ';
  var hasClass = current.indexOf(token) >= 0;
  if (enabled && !hasClass) {
    node.className = (node.className ? node.className + ' ' : '') + className;
  } else if (!enabled && hasClass) {
    node.className = current.replace(token, ' ').replace(/^\s+|\s+$/g, '').replace(/\s+/g, ' ');
  }
}

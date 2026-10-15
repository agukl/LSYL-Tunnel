import { $, appendPlainCell, appendBadgeCell, appendEmptyRow } from './dom.js';
import { updateLogHorizontalScroll, initResizableTable } from './table.js';
import {
  eventKindText, eventResultText, eventResultClass, eventActorText, eventTargetText,
  eventReasonText, trafficDurationText, eventPeriodText, requestTypeText,
  requestResultText, requestActorText, requestTargetText, responseText, compactJSON, timeText,
} from './format.js';

export function renderBusinessLogs(items) {
  var tbody = $('businessTable').getElementsByTagName('tbody')[0];
  tbody.innerHTML = '';
  initResizableTable($('businessTable'));
  if (!items || items.length === 0) {
    appendEmptyRow(tbody, 7, '暂无业务记录');
    updateLogHorizontalScroll('businessTable', 'businessScrollBar', 'businessScrollTrack');
    return;
  }
  for (var i = 0; i < items.length; i++) {
    var ev = items[i] || {};
    var tr = document.createElement('tr');
    appendPlainCell(tr, eventPeriodText(ev), 'ellipsis-cell');
    appendPlainCell(tr, eventKindText(ev.kind));
    appendBadgeCell(tr, eventResultText(ev.result), eventResultClass(ev.result));
    appendPlainCell(tr, eventActorText(ev));
    appendPlainCell(tr, eventTargetText(ev), 'ellipsis-cell');
    appendPlainCell(tr, trafficDurationText(ev), 'ellipsis-cell');
    appendPlainCell(tr, eventReasonText(ev), 'ellipsis-cell');
    tbody.appendChild(tr);
  }
  updateLogHorizontalScroll('businessTable', 'businessScrollBar', 'businessScrollTrack');
}

export function renderRequestLogs(items) {
  var tbody = $('requestTable').getElementsByTagName('tbody')[0];
  tbody.innerHTML = '';
  initResizableTable($('requestTable'));
  if (!items || items.length === 0) {
    appendEmptyRow(tbody, 8, '暂无请求记录');
    updateLogHorizontalScroll('requestTable', 'requestScrollBar', 'requestScrollTrack');
    return;
  }
  for (var i = 0; i < items.length; i++) {
    var item = items[i] || {};
    var req = item.request || {};
    var resp = item.response || {};
    var tr = document.createElement('tr');
    appendPlainCell(tr, timeText(item.time));
    appendPlainCell(tr, item.remote_addr || item.remote_ip || '-');
    appendPlainCell(tr, requestTypeText(req.type));
    appendBadgeCell(tr, requestResultText(item), eventResultClass(item.result));
    appendPlainCell(tr, requestActorText(item), 'ellipsis-cell');
    appendPlainCell(tr, requestTargetText(item), 'ellipsis-cell');
    appendPlainCell(tr, responseText(resp), 'ellipsis-cell');
    appendPlainCell(tr, compactJSON(item), 'raw-json-cell');
    tbody.appendChild(tr);
  }
  updateLogHorizontalScroll('requestTable', 'requestScrollBar', 'requestScrollTrack');
}

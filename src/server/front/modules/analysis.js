import { $, setText, appendPlainCell, appendEmptyRow } from './dom.js';
import { api } from './api.js';
import { numberText, bytesText, dateTimeLocalValue, analysisDurationText, timeText } from './format.js';

let analysisLoaded = false;

export function hasLoadedAnalysis() { return analysisLoaded; }

export function initAnalysisDefaults() {
  var start = $('analysisStart');
  var end = $('analysisEnd');
  if (!start || !end) { return; }
  var now = new Date();
  var weekAgo = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);
  if (!start.value) { start.value = dateTimeLocalValue(weekAgo); }
  if (!end.value) { end.value = dateTimeLocalValue(now); }
}

export function loadAnalysis() {
  initAnalysisDefaults();
  var start = $('analysisStart') ? $('analysisStart').value : '';
  var end = $('analysisEnd') ? $('analysisEnd').value : '';
  setAnalysisBusy(true);
  setAnalysisMessage('正在分析日志并查询归属地...', false);
  api('GET', '/api/log-analysis?start=' + encodeURIComponent(start) + '&end=' + encodeURIComponent(end), null, function (res) {
    setAnalysisBusy(false);
    if (!res || !res.ok) {
      setAnalysisMessage((res && res.message) || '日志分析失败', true);
      return;
    }
    analysisLoaded = true;
    renderLogAnalysis(res);
  });
}

function setAnalysisBusy(isBusy) {
  var btn = $('analysisRunBtn');
  if (!btn) { return; }
  btn.disabled = !!isBusy;
  btn.textContent = isBusy ? '分析中' : '分析日志';
}

function setAnalysisMessage(text, isError) {
  var box = $('analysisMessage');
  if (!box) { return; }
  box.textContent = text || '';
  box.className = 'hint-card' + (isError ? ' error' : '') + (text ? '' : ' hidden');
}

function renderLogAnalysis(res) {
  var summary = res.summary || {};
  var success = res.success_sources || [];
  var blocked = res.blocked_sources || [];
  setText('analysisSuccessIPCount', success.length);
  setText('analysisBlockedIPCount', blocked.length);
  setText('analysisBusinessEventCount', summary.business_events || 0);
  setText('analysisRequestEventCount', summary.request_events || 0);
  var paths = res.paths || {};
  setText('analysisPathsView', '业务日志 ' + (paths.business_log || '-') + ' / 请求日志 ' + (paths.request_log || '-'));
  if (res.geo_lookup_error) {
    setAnalysisMessage('分析完成，但归属地查询失败：' + res.geo_lookup_error, true);
  } else {
    setAnalysisMessage('分析完成，时间范围：' + timeText(res.start) + ' - ' + timeText(res.end), false);
  }
  renderAnalysisSuccess(success);
  renderAnalysisBlocked(blocked);
}

function renderAnalysisSuccess(items) {
  var tbody = $('analysisSuccessTable').getElementsByTagName('tbody')[0];
  tbody.innerHTML = '';
  if (!items || items.length === 0) {
    appendEmptyRow(tbody, 6, '所选时间内没有成功来源');
    return;
  }
  for (var i = 0; i < items.length; i++) {
    var item = items[i] || {};
    var tr = document.createElement('tr');
    appendPlainCell(tr, item.ip || '-');
    appendPlainCell(tr, analysisNetworkText(item), 'ellipsis-cell');
    appendPlainCell(tr, '成功 ' + numberText(item.success_events) + ' / 请求 ' + numberText(item.requests));
    appendPlainCell(tr, analysisActorList(item), 'ellipsis-cell');
    appendPlainCell(tr, analysisTargetTrafficText(item), 'ellipsis-cell');
    appendPlainCell(tr, analysisFirstLastText(item), 'ellipsis-cell');
    tbody.appendChild(tr);
  }
}

function renderAnalysisBlocked(items) {
  var tbody = $('analysisBlockedTable').getElementsByTagName('tbody')[0];
  tbody.innerHTML = '';
  if (!items || items.length === 0) {
    appendEmptyRow(tbody, 6, '所选时间内没有封禁来源');
    return;
  }
  for (var i = 0; i < items.length; i++) {
    var item = items[i] || {};
    var tr = document.createElement('tr');
    appendPlainCell(tr, item.ip || '-');
    appendPlainCell(tr, analysisNetworkText(item), 'ellipsis-cell');
    appendPlainCell(tr, '封禁 ' + numberText(item.blocked_events) + ' / 失败请求 ' + numberText(item.failed_requests));
    appendPlainCell(tr, analysisBlockedTypeText(item), 'ellipsis-cell');
    appendPlainCell(tr, analysisReasonText(item), 'ellipsis-cell');
    appendPlainCell(tr, analysisFirstLastText(item), 'ellipsis-cell');
    tbody.appendChild(tr);
  }
}

function analysisNetworkText(item) {
  item = item || {};
  var parts = [];
  if (item.location) { parts.push(item.location); }
  if (item.network_type) { parts.push(item.network_type); }
  if (item.network) { parts.push(item.network); }
  if (item.as_name && (!item.network || item.network.indexOf(item.as_name) < 0)) { parts.push(item.as_name); }
  return parts.length ? parts.join(' / ') : '-';
}

function analysisActorList(item) {
  item = item || {};
  var parts = [];
  if (item.users && item.users.length) { parts.push('用户 ' + item.users.join(', ')); }
  if (item.clients && item.clients.length) { parts.push('客户端 ' + item.clients.join(', ')); }
  return parts.length ? parts.join(' / ') : '-';
}

function analysisTargetTrafficText(item) {
  item = item || {};
  var parts = [];
  if (item.targets && item.targets.length) { parts.push(item.targets.join(', ')); }
  if ((item.bytes_up || item.bytes_down) > 0) {
    parts.push('上行 ' + bytesText(item.bytes_up || 0));
    parts.push('下行 ' + bytesText(item.bytes_down || 0));
  }
  if (item.duration_ms) { parts.push(analysisDurationText(item.duration_ms)); }
  return parts.length ? parts.join(' / ') : '-';
}

function analysisBlockedTypeText(item) {
  item = item || {};
  var parts = [];
  if (item.auth_blocked_events) { parts.push('认证封禁 ' + item.auth_blocked_events); }
  if (item.permanent_blocked_events) { parts.push('永久封禁 ' + item.permanent_blocked_events); }
  if (!parts.length && item.blocked_events) { parts.push('封禁 ' + item.blocked_events); }
  return parts.length ? parts.join(' / ') : '-';
}

function analysisReasonText(item) {
  item = item || {};
  var parts = [];
  if (item.codes && item.codes.length) { parts.push(item.codes.join(', ')); }
  if (item.messages && item.messages.length) { parts.push(item.messages.join(' | ')); }
  return parts.length ? parts.join(' / ') : '-';
}

function analysisFirstLastText(item) {
  item = item || {};
  return timeText(item.first_seen) + ' / ' + timeText(item.last_seen);
}

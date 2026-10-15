export function numberText(value) {
  if (value == null) { return '0'; }
  return String(value);
}

export function bytesText(value) {
  value = Number(value || 0);
  if (value >= 1024 * 1024 * 1024) { return (value / 1024 / 1024 / 1024).toFixed(2) + ' GB'; }
  if (value >= 1024 * 1024) { return (value / 1024 / 1024).toFixed(2) + ' MB'; }
  if (value >= 1024) { return (value / 1024).toFixed(2) + ' KB'; }
  return value + ' B';
}

export function rateText(value) {
  return bytesText(value) + '/s';
}

export function dateTimeLocalValue(date) {
  function pad(value) { return value < 10 ? '0' + value : String(value); }
  return date.getFullYear() + '-' +
    pad(date.getMonth() + 1) + '-' +
    pad(date.getDate()) + 'T' +
    pad(date.getHours()) + ':' +
    pad(date.getMinutes());
}

export function analysisDurationText(ms) {
  ms = Number(ms || 0);
  if (ms >= 3600000) { return (ms / 3600000).toFixed(2) + ' 小时'; }
  if (ms >= 60000) { return (ms / 60000).toFixed(2) + ' 分钟'; }
  if (ms > 0) { return Math.ceil(ms / 1000) + ' 秒'; }
  return '-';
}

export function isFailureRecord(ev) {
  ev = ev || {};
  var result = String(ev.result || '').toLowerCase();
  var code = String(ev.code || (ev.response && ev.response.code) || '').toLowerCase();
  if (result === 'failed' || result === 'denied' || result === 'blocked' || result === 'error') {
    return true;
  }
  return code.indexOf('failed') >= 0 ||
    code.indexOf('denied') >= 0 ||
    code.indexOf('blocked') >= 0 ||
    code.indexOf('unreachable') >= 0 ||
    code.indexOf('timeout') >= 0 ||
    code.indexOf('bad_request') >= 0 ||
    code.indexOf('expired') >= 0;
}

export function eventKindText(kind) {
  switch (kind) {
  case 'compatibility': return '版本兼容';
  case 'auth': return '认证';
  case 'login': return '登录';
  case 'request': return '请求';
  case 'forward_check': return '转发检查';
  case 'open': return '正向连接';
  case 'reverse_listen': return '被动监听';
  case 'reverse_stream': return '被动流';
  case 'reverse_inbound': return '被动接入';
  default: return kind || '-';
  }
}

export function eventResultText(result) {
  switch (result) {
  case 'ok': return '成功';
  case 'connected': return '已连接';
  case 'closed': return '已关闭';
  case 'activated': return '已激活';
  case 'failed': return '失败';
  case 'denied': return '拒绝';
  case 'blocked': return '封禁';
  default: return result || '-';
  }
}

export function eventResultClass(result) {
  switch (result) {
  case 'ok':
  case 'connected':
  case 'activated':
    return 'ok';
  case 'closed':
    return 'muted';
  case 'failed':
  case 'denied':
  case 'blocked':
    return 'warn';
  default:
    return 'muted';
  }
}

export function eventActorText(ev) {
  var parts = [];
  if (ev.username) { parts.push(ev.username); }
  if (ev.remote_ip) { parts.push(ev.remote_ip); }
  if (!parts.length && ev.client_id) { parts.push(ev.client_id); }
  return parts.length ? parts.join(' / ') : '-';
}

export function eventTargetText(ev) {
  var parts = [];
  if (ev.forward_name) { parts.push(ev.forward_name); }
  if (ev.listen_addr) { parts.push('监听 ' + ev.listen_addr); }
  if (ev.target) { parts.push('目标 ' + ev.target); }
  if (!parts.length && ev.direction) { parts.push(directionText(ev.direction)); }
  return parts.length ? parts.join(' / ') : '-';
}

function directionText(direction) {
  return direction === 'server_to_client' ? '服务端到客户端' : '客户端到服务端';
}

export function eventReasonText(ev) {
  ev = ev || {};
  var parts = [];
  if (ev.code) { parts.push(ev.code); }
  if (ev.message) { parts.push(ev.message); }
  return parts.length ? parts.join(' / ') : '-';
}

export function trafficDurationText(ev) {
  ev = ev || {};
  var parts = [];
  if (ev.bytes_up || ev.bytes_down) {
    parts.push('上行 ' + bytesText(ev.bytes_up || 0));
    parts.push('下行 ' + bytesText(ev.bytes_down || 0));
  }
  if (ev.duration_ms) { parts.push(analysisDurationText(ev.duration_ms)); }
  return parts.length ? parts.join(' / ') : '-';
}

export function eventPeriodText(ev) {
  ev = ev || {};
  var start = ev.started_at || ev.time || '';
  var end = ev.ended_at || '';
  if (start && end) { return periodText(start, end); }
  if (start && isActiveBusinessResult(ev.result)) { return timeText(start) + ' - 进行中'; }
  if (start) { return timeText(start); }
  return '-';
}

function isActiveBusinessResult(result) {
  return result === 'connected' || result === 'activated';
}

function periodText(startValue, endValue) {
  var start = new Date(startValue);
  var end = new Date(endValue);
  if (isNaN(start.getTime()) || isNaN(end.getTime())) {
    return timeText(startValue) + ' - ' + timeText(endValue);
  }
  var startDate = dateOnlyText(start);
  var endDate = dateOnlyText(end);
  if (startDate === endDate) {
    return startDate + ' ' + clockText(start) + ' - ' + clockText(end);
  }
  return timeText(startValue) + ' - ' + timeText(endValue);
}

function dateOnlyText(date) {
  return date.getFullYear() + '/' + (date.getMonth() + 1) + '/' + date.getDate();
}

function clockText(date) {
  function pad(value) { return value < 10 ? '0' + value : String(value); }
  return pad(date.getHours()) + ':' + pad(date.getMinutes()) + ':' + pad(date.getSeconds());
}

export function requestTypeText(type) {
  if (type === 'reverse_listen' || type === 'reverse') { return '被动监听'; }
  if (type === 'reverse_stream') { return '被动流'; }
  if (type === 'open') { return '正向连接'; }
  if (type === 'login') { return '登录'; }
  return type || '-';
}

export function requestResultText(item) {
  item = item || {};
  var resp = item.response || {};
  if (item.result) { return eventResultText(item.result); }
  if (resp.ok === true) { return '成功'; }
  if (resp.ok === false) { return '失败'; }
  return '-';
}

export function requestActorText(item) {
  var req = (item || {}).request || {};
  var parts = [];
  if (req.username) { parts.push(req.username); }
  if (req.client_id) { parts.push(req.client_id); }
  if (item.auth_result) { parts.push('auth=' + item.auth_result); }
  return parts.length ? parts.join(' / ') : '-';
}

export function requestTargetText(item) {
  var req = (item || {}).request || {};
  var parts = [];
  if (req.forward_name) { parts.push(req.forward_name); }
  if (req.listen_addr) { parts.push('监听 ' + req.listen_addr); }
  if (req.target) { parts.push('目标 ' + req.target); }
  if (req.stream_id) { parts.push('stream ' + req.stream_id); }
  return parts.length ? parts.join(' / ') : '-';
}

export function responseText(resp) {
  resp = resp || {};
  var parts = [];
  if (resp.ok === true || resp.ok === false) { parts.push(resp.ok ? 'ok' : 'fail'); }
  if (resp.code) { parts.push(resp.code); }
  if (resp.message) { parts.push(resp.message); }
  return parts.length ? parts.join(' / ') : '-';
}

export function compactJSON(value) {
  try { return JSON.stringify(value); } catch (e) { return '-'; }
}

export function timeText(value) {
  if (!value) { return '-'; }
  var date = new Date(value);
  if (isNaN(date.getTime())) { return String(value); }
  return date.toLocaleString();
}

export function remainText(seconds) {
  seconds = Number(seconds || 0);
  if (seconds <= 0) { return '-'; }
  if (seconds >= 3600) { return Math.ceil(seconds / 3600) + ' 小时'; }
  if (seconds >= 60) { return Math.ceil(seconds / 60) + ' 分钟'; }
  return Math.ceil(seconds) + ' 秒';
}

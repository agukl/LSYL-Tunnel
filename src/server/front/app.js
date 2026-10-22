import {
  $, setText, hiddenInput, appendPlainCell, appendBadgeCell, appendEmptyRow,
  removeEmptyRows, hasDataRows, setClassEnabled,
} from './modules/dom.js';
import { api } from './modules/api.js';
import { numberText, bytesText, rateText, isFailureRecord, timeText, remainText } from './modules/format.js';
import {
  addCustomSelect, renderCustomSelect, closeAllCustomSelects, bindCustomSelectDismissal,
} from './modules/select.js';
import { renderBusinessLogs, renderRequestLogs } from './modules/logs.js';
import { bindLogHorizontalScroll, updateLogScrollbars, initResizableTables } from './modules/table.js';
import { initAnalysisDefaults, loadAnalysis, hasLoadedAnalysis } from './modules/analysis.js';

let state = null;
let dirty = false;
let editRevision = 0;
let busy = false;
let activeTab = 'overview';
let resetPasswordRow = null;
const lockedSecurityIds = [
  'listenAddr', 'monitorAddr', 'logLevel', 'tlsCert', 'tlsKey', 'tlsMinVersion',
  'sealKeyID', 'sealPrivate', 'sealPublic', 'sealExpires',
];
const editableSecurityIds = [
  'handshakeTimeout', 'dialTimeout', 'maxHandshakeBytes', 'maxConcurrentConnections',
  'maxConcurrentConnectionsPerIP', 'connectionRateWindow', 'maxNewConnectionsPerIPWindow',
  'maxTrackedConnectionIPs', 'connectionLimiterCleanupSec', 'maxTrackedFailureIPs',
  'failureTrackerCleanupSec', 'entryTrafficLogQueueSize', 'maxConcurrentStreamsPerUser',
  'streamRateLimitBytesPerSec', 'authFailWindow', 'authFailThreshold', 'authFailBlock',
];

function showToast(message, isError) {
  var box = $('toast');
  box.textContent = message || '';
  box.className = 'toast' + (isError ? ' error' : '') + (message ? '' : ' hidden');
  if (message) {
    window.setTimeout(function () {
      if (box.textContent === message) { box.className = 'toast hidden'; }
    }, 4200);
  }
}

function formatIssues(issues) {
  if (!issues || !issues.length) { return ''; }
  var lines = [];
  for (var i = 0; i < issues.length; i++) {
    var issue = issues[i] || {};
    var text = issue.message || '';
    if (issue.field) { text = issue.field + ': ' + text; }
    if (issue.level) { text = '[' + issue.level + '] ' + text; }
    if (text) { lines.push(text); }
  }
  return lines.join('\n');
}

function showValidationIssues(issues) {
  var text = formatIssues(issues);
  if (!text) { return; }
  var validation = $('validationBox');
  validation.textContent = text;
  validation.className = 'hint-card error';
}

function markDirty() {
  dirty = true;
  editRevision++;
  setText('summaryText', '配置有未保存修改');
}

function setBusy(next) {
  busy = next;
  updateButtons();
}

function updateButtons() {
  var svc = state && state.service ? state.service : {};
  var running = !!svc.running;
  var blockedBySavedValidation = state && state.validation && !dirty;
  var blockedByPermission = state && state.config_writable === false;
  setActionDisabled('restart', busy || !!blockedBySavedValidation);
  setActionDisabled('save', busy || blockedByPermission);
  setActionDisabled('refresh', busy);
  setUnblockButtonsDisabled(busy);
  setPermanentBlockDeleteButtonsDisabled(busy);
  setConfigEditingEnabled(!blockedByPermission);
}

function setActionDisabled(action, disabled) {
  var buttons = document.querySelectorAll('[data-action="' + action + '"]');
  for (var i = 0; i < buttons.length; i++) {
    buttons[i].disabled = disabled;
  }
}

function loadState(fillForms) {
  var requestedRevision = editRevision;
  api('GET', '/api/state', null, function (res) {
    if (!res || !res.config) {
      showToast(res && res.message ? res.message : '状态读取失败', true);
      return;
    }
    if (fillForms && requestedRevision !== editRevision) {
      renderState(res, false);
      showToast('刷新期间配置发生修改，未覆盖当前编辑内容。', true);
      return;
    }
    renderState(res, !!fillForms);
  });
}

function renderState(nextState, fillForms) {
  state = nextState;
  var cfg = state.config || {};
  var svc = state.service || {};
  var mon = state.monitor || {};

  setText('pathConfig', state.paths ? state.paths.config : '-');
  var pill = $('servicePill');
  var pillClass = 'status-pill';
  if (svc.running) { pillClass += ' running'; }
  else if (svc.installed) { pillClass += ' stopped'; }
  pill.className = pillClass;
  pill.getElementsByTagName('strong')[0].textContent = svc.status || '未知';

  setText('summaryText', dirty && !fillForms ? '配置有未保存修改' : (state.message || (svc.running ? '服务正在运行' : '服务未运行')));
  setText('listenAddrView', (mon && mon.listen_addr) || cfg.listen_addr || '-');
  setText('activeConnectionsView', numberText(mon.active_connections));
  setText('activeStreamsView', numberText(mon.active_streams));
  setText('connectionsRejectedView', numberText(mon.connections_rejected));
  var connectionRejections = (mon && mon.connection_rejections) || {};
  setText('connectionConcurrentRejectsView', numberText((connectionRejections.global_concurrent || 0) + (connectionRejections.per_ip_concurrent || 0)));
  setText('connectionRateRejectsView', numberText(connectionRejections.per_ip_new_connection_rate || 0));
  setText('userStreamLimitRejectedView', numberText(mon.user_stream_limit_rejected));
  setText('authOKView', numberText(mon.auth_ok));
  setText('authFailedView', numberText(mon.auth_failed));
  setText('policyRejectedView', numberText(mon.policy_rejected));
  setText('dialFailedView', numberText(mon.dial_failed));
  setText('trafficView', bytesText((mon.bytes_up || 0) + (mon.bytes_down || 0)));

  var validation = $('validationBox');
  if (state.validation) {
    validation.textContent = state.validation;
    validation.className = 'hint-card error';
  } else if (state.config_write_hint) {
    validation.textContent = state.config_write_hint;
    validation.className = 'hint-card error';
  } else if (state.monitor_error) {
    validation.textContent = '监控接口暂不可读：' + state.monitor_error;
    validation.className = 'hint-card';
  } else if (svc.running) {
    validation.textContent = '服务运行中。保存配置后需要重启服务才会生效。';
    validation.className = 'hint-card';
  } else {
    validation.textContent = '服务未运行。确认配置无误后可以重启 Windows 服务，系统会自动注册并启动服务。';
    validation.className = 'hint-card';
  }
  renderConfigLockBanner();
  renderActivityDetails(state.business_logs || [], state.request_logs || [], (mon && mon.blocked_ips) || [], state.permanent_blocked_ips || []);
  renderUserConcurrency(cfg, mon || {});

  if (fillForms) {
    fillSecurity(cfg);
    renderUsers(cfg.users || [], cfg.forwards || []);
    renderForwards(cfg.forwards || []);
    dirty = false;
  }

  updateButtons();
}

function configLockMessage() {
  if (state && state.config_write_hint) { return state.config_write_hint; }
  return '\u914d\u7f6e\u6587\u4ef6\u4e0d\u53ef\u5199\uff0c\u8bf7\u4ee5\u7ba1\u7406\u5458\u8eab\u4efd\u8fd0\u884c\u670d\u52a1\u7aef GUI \u6216\u68c0\u67e5\u5b89\u88c5\u76ee\u5f55\u6743\u9650\u3002';
}

function renderConfigLockBanner() {
  var banner = $('configLockBanner');
  if (!banner) { return; }
  if (state && state.config_writable === false) {
    banner.textContent = configLockMessage();
    banner.className = 'hint-card error';
  } else {
    banner.textContent = '';
    banner.className = 'hint-card error hidden';
  }
}

function setConfigEditingEnabled(enabled) {
  var selectors = [
    '#usersTable input',
    '#usersTable button',
    '#forwardsTable input',
    '#forwardsTable select',
    '#forwardsTable button',
    '#addUserBtn',
    '#addForwardBtn'
  ];
  var nodes = document.querySelectorAll(selectors.join(','));
  for (var i = 0; i < nodes.length; i++) {
    nodes[i].disabled = !enabled;
    if (!enabled) {
      nodes[i].title = configLockMessage();
    } else {
      nodes[i].title = '';
    }
  }
  if (enabled) {
    var rows = document.querySelectorAll('#forwardsTable tbody tr');
    for (var r = 0; r < rows.length; r++) {
      updateForwardRowForDirection(rows[r]);
    }
  }
  var customControls = document.querySelectorAll('#forwardsTable input[type="hidden"][data-name]');
  for (var k = 0; k < customControls.length; k++) {
    renderCustomSelect(customControls[k]);
  }
  if (!enabled) { closeAllCustomSelects(); }
  for (var j = 0; j < editableSecurityIds.length; j++) {
    var input = $(editableSecurityIds[j]);
    if (!input) { continue; }
    input.disabled = !enabled;
    input.title = enabled ? '' : configLockMessage();
  }
}

function fillSecurity(cfg) {
  var tls = cfg.tls || {};
  var sec = cfg.security || {};
  var seal = cfg.credential_seal || {};
  $('listenAddr').value = cfg.listen_addr || '';
  $('monitorAddr').value = cfg.monitor_addr || '';
  $('logLevel').value = cfg.log_level || '';
  $('tlsCert').value = tls.cert_file || '';
  $('tlsKey').value = tls.key_file || '';
  $('tlsMinVersion').value = tls.min_version || '';
  $('handshakeTimeout').value = sec.handshake_timeout_sec || '';
  $('dialTimeout').value = sec.dial_timeout_sec || '';
  $('maxHandshakeBytes').value = sec.max_handshake_bytes || '';
  $('maxConcurrentConnections').value = sec.max_concurrent_connections || '';
  $('maxConcurrentConnectionsPerIP').value = sec.max_concurrent_connections_per_ip || '';
  $('connectionRateWindow').value = sec.connection_rate_window_sec || '';
  $('maxNewConnectionsPerIPWindow').value = sec.max_new_connections_per_ip_window || sec.max_connections_per_ip_per_window || '';
  $('maxTrackedConnectionIPs').value = sec.max_tracked_connection_ips || '';
  $('connectionLimiterCleanupSec').value = sec.connection_limiter_cleanup_sec || '';
  $('maxTrackedFailureIPs').value = sec.max_tracked_failure_ips || '';
  $('failureTrackerCleanupSec').value = sec.failure_tracker_cleanup_sec || '';
  $('entryTrafficLogQueueSize').value = sec.entry_traffic_log_queue_size || '';
  $('maxConcurrentStreamsPerUser').value = sec.max_concurrent_streams_per_user || '';
  $('streamRateLimitBytesPerSec').value = sec.stream_rate_limit_bytes_per_sec || '';
  $('authFailWindow').value = sec.auth_fail_window_sec || '';
  $('authFailThreshold').value = sec.auth_fail_threshold || '';
  $('authFailBlock').value = sec.auth_fail_block_sec || '';
  $('sealKeyID').value = seal.key_id || '';
  $('sealPrivate').value = seal.private_key_file || '';
  $('sealPublic').value = seal.public_key_file || '';
  $('sealExpires').value = seal.expires_at || '';
}

function renderUserConcurrency(cfg, mon) {
  cfg = cfg || {};
  mon = mon || {};
  var sec = cfg.security || {};
  var limits = mon.user_stream_limits || {};
  var maxStreams = Number(sec.max_concurrent_streams_per_user || limits.max_concurrent_streams_per_user || 0);
  var rateLimit = Number(sec.stream_rate_limit_bytes_per_sec || limits.stream_rate_limit_bytes_per_sec || 0);
  var activeByUser = limits.active_by_user || {};
  var users = cfg.users || [];
  var forwards = cfg.forwards || [];

  setText('userStreamLimitView', maxStreams > 0 ? maxStreams : '不限');
  setText('streamRateLimitView', rateLimit > 0 ? rateText(rateLimit) : '不限');
  setText('userStreamActiveView', numberText(limits.active || mon.active_streams || 0));
  setText('userConcurrencyRejectedView', numberText(mon.user_stream_limit_rejected || limits.user_stream_limit_rejections_total || 0));

  var tbody = $('userConcurrencyTable').getElementsByTagName('tbody')[0];
  tbody.innerHTML = '';
  var seen = {};
  for (var i = 0; i < users.length; i++) {
    var user = users[i] || {};
    var username = (user.username || '').replace(/^\s+|\s+$/g, '');
    if (!username || seen[username]) { continue; }
    seen[username] = true;
    appendUserConcurrencyRow(tbody, username, !!user.disabled, boundPortsForUser(username, forwards), activeByUser[username] || 0, maxStreams, rateLimit);
  }
  var extras = [];
  for (var name in activeByUser) {
    if (Object.prototype.hasOwnProperty.call(activeByUser, name) && !seen[name]) {
      extras.push(name);
    }
  }
  extras.sort();
  for (var j = 0; j < extras.length; j++) {
    appendUserConcurrencyRow(tbody, extras[j], false, [], activeByUser[extras[j]] || 0, maxStreams, rateLimit);
  }
  if (!hasDataRows(tbody)) {
    appendEmptyRow(tbody, 6, '暂无用户并发数据');
  }
}

function appendUserConcurrencyRow(tbody, username, disabled, ports, active, maxStreams, rateLimit) {
  var tr = document.createElement('tr');
  appendPlainCell(tr, username || '-');
  appendBadgeCell(tr, disabled ? '禁用' : '启用', disabled ? 'warn' : 'ok');
  appendPlainCell(tr, ports && ports.length ? ports.join('，') : '未绑定');
  appendPlainCell(tr, numberText(active));
  appendPlainCell(tr, maxStreams > 0 ? String(maxStreams) : '不限');
  appendPlainCell(tr, rateLimit > 0 ? rateText(rateLimit) : '不限');
  tbody.appendChild(tr);
}

function bindSecurityFields() {
  lockSecurityFields();
  for (var i = 0; i < editableSecurityIds.length; i++) {
    $(editableSecurityIds[i]).onchange = markDirty;
    $(editableSecurityIds[i]).onkeyup = markDirty;
  }
}

function lockSecurityFields() {
  for (var i = 0; i < lockedSecurityIds.length; i++) {
    var input = $(lockedSecurityIds[i]);
    if (!input) { continue; }
    input.readOnly = true;
    input.tabIndex = -1;
    input.setAttribute('aria-readonly', 'true');
    input.setAttribute('data-locked', 'true');
    input.title = '此项由配置文件或部署脚本维护';
    if (input.className.indexOf('locked-input') < 0) {
      input.className = (input.className ? input.className + ' ' : '') + 'locked-input';
    }
    input.onfocus = function () { this.blur(); };
    input.onmousedown = function (event) {
      event = event || window.event;
      if (event.preventDefault) { event.preventDefault(); }
      event.returnValue = false;
      return false;
    };
    input.onkeydown = function (event) {
      event = event || window.event;
      if (event.preventDefault) { event.preventDefault(); }
      event.returnValue = false;
      return false;
    };
  }
}

function renderUsers(users, forwards) {
  var tbody = $('usersTable').getElementsByTagName('tbody')[0];
  tbody.innerHTML = '';
  for (var i = 0; i < users.length; i++) {
    addUserRow(users[i], boundPortsForUser(users[i].username, forwards || []));
  }
  if (users.length === 0) { appendEmptyRow(tbody, 4, '\u6682\u65e0\u7528\u6237\uff0c\u70b9\u51fb\u201c\u65b0\u589e\u7528\u6237\u201d\u6dfb\u52a0\u8d26\u53f7'); }
}

function addUserRow(user, boundPorts) {
  var tbody = $('usersTable').getElementsByTagName('tbody')[0];
  removeEmptyRows(tbody);
  var tr = document.createElement('tr');
  var usernameCell = textCell(user.username || '', 'username', function () {
    refreshForwardOwnerOptions();
    refreshUserBoundPorts();
  });
  usernameCell.appendChild(hiddenInput(user.password_hash || '', 'password_hash'));
  tr.appendChild(usernameCell);
  tr.appendChild(boundPortsCell(boundPorts || []));
  tr.appendChild(checkCell(!!user.disabled, 'disabled', refreshForwardOwnerOptions));
  tr.appendChild(userActionCell(tr));
  tbody.appendChild(tr);
}

function boundPortsCell(ports) {
  var td = document.createElement('td');
  td.className = 'readonly-cell';
  var text = ports && ports.length ? ports.join('，') : '未绑定';
  var span = document.createElement('span');
  span.className = ports && ports.length ? 'bound-ports' : 'bound-ports empty';
  span.textContent = text;
  span.title = text;
  td.appendChild(span);
  return td;
}

function boundPortsForUser(username, forwards) {
  username = (username || '').replace(/^\s+|\s+$/g, '');
  if (!username) { return []; }
  var ports = [];
  var seen = {};
  for (var i = 0; i < forwards.length; i++) {
    var fwd = forwards[i] || {};
    if (!userInForward(username, fwd)) { continue; }
    var port = forwardPortValue(fwd);
    if (!port || seen[port]) { continue; }
    seen[port] = true;
    ports.push(port);
  }
  return ports;
}

function refreshUserBoundPorts() {
  var forwards = state && state.config ? state.config.forwards || [] : [];
  if ($('forwardsTable')) {
    forwards = collectForwards();
  }
  var rows = $('usersTable').getElementsByTagName('tbody')[0].getElementsByTagName('tr');
  for (var i = 0; i < rows.length; i++) {
    var usernameNode = field(rows[i], 'username');
    var boundNode = rows[i].querySelectorAll('.bound-ports')[0];
    if (!usernameNode || !boundNode) { continue; }
    var ports = boundPortsForUser(usernameNode.value, forwards);
    var text = ports.length ? ports.join('，') : '未绑定';
    boundNode.textContent = text;
    boundNode.title = text;
    boundNode.className = ports.length ? 'bound-ports' : 'bound-ports empty';
  }
}

function renderForwards(forwards) {
  var tbody = $('forwardsTable').getElementsByTagName('tbody')[0];
  tbody.innerHTML = '';
  for (var i = 0; i < forwards.length; i++) { addForwardRow(forwards[i]); }
  if (forwards.length === 0) { appendEmptyRow(tbody, 5, '\u6682\u65e0\u8f6c\u53d1\u7aef\u53e3\uff0c\u70b9\u51fb\u201c\u65b0\u589e\u8f6c\u53d1\u201d\u6dfb\u52a0\u89c4\u5219'); }
}

function addForwardRow(fwd) {
  var tbody = $('forwardsTable').getElementsByTagName('tbody')[0];
  removeEmptyRows(tbody);
  var tr = document.createElement('tr');
  tr.appendChild(directionCell(fwd.direction || '', tr));
  tr.appendChild(allowedUsersCell(allowedUsersForForward(fwd)));
  tr.appendChild(serverHostCell(forwardHostValue(fwd)));
  tr.appendChild(portCell(forwardPortValue(fwd)));
  tr.appendChild(actionCell(tr));
  tbody.appendChild(tr);
  updateForwardRowForDirection(tr);
}

function textCell(value, name, afterChange) {
  var td = document.createElement('td');
  var input = document.createElement('input');
  input.type = 'text';
  input.value = value;
  input.setAttribute('data-name', name);
  input.onchange = function () {
    markDirty();
    if (afterChange) { afterChange(); }
  };
  input.onkeyup = input.onchange;
  td.appendChild(input);
  return td;
}

function checkCell(value, name, afterChange) {
  var td = document.createElement('td');
  td.className = 'check-cell';
  var input = document.createElement('input');
  input.type = 'checkbox';
  input.checked = !!value;
  input.setAttribute('data-name', name);
  input.onchange = function () {
    markDirty();
    if (afterChange) { afterChange(); }
  };
  td.appendChild(input);
  return td;
}

function directionCell(value, row) {
  var td = document.createElement('td');
  td.className = 'select-cell';
  addCustomSelect(td, 'direction', value === 'server_to_client' || value === 'client_to_server' ? value : '', [
    { value: '', text: '\u8bf7\u9009\u62e9\u65b9\u5411' },
    { value: 'client_to_server', text: '正向代理' },
    { value: 'server_to_client', text: '反向代理' }
  ], function () {
    markDirty();
    updateForwardRowForDirection(row);
  });
  return td;
}

function allowedUsersCell(selectedUsers) {
  var td = document.createElement('td');
  td.className = 'select-cell';
  var control = addCustomSelect(td, 'allowed_users', '', [], function () {
    setSelectedUsersData(control, selectedAllowedUsers(control));
    markDirty();
    refreshUserBoundPorts();
  });
  setSelectedUsersData(control, firstUserOnly(selectedUsers || []));
  refreshAllowedUserSelect(control);
  return td;
}

function setSelectedUsersData(control, users) {
  var selected = firstUserOnly(users || []);
  control.setAttribute('data-selected-users', selected.join(','));
  control.value = selected.length ? selected[0] : '';
}

function selectedAllowedUsers(control) {
  if (!control) { return []; }
  return splitUserList(control.value);
}

function refreshAllowedUserSelect(control) {
  if (!control) { return; }
  var selected = firstUserOnly(splitUserList(control.getAttribute('data-selected-users') || ''));
  var selectedMap = {};
  for (var i = 0; i < selected.length; i++) { selectedMap[selected[i]] = true; }
  var names = currentUserNames();
  var known = {};
  for (var j = 0; j < names.length; j++) { known[names[j]] = true; }
  for (var k = 0; k < selected.length; k++) {
    if (!known[selected[k]]) {
      names.push(selected[k]);
      known[selected[k]] = true;
    }
  }
  var options = [{ value: '', text: names.length ? '\u8bf7\u9009\u62e9\u7528\u6237' : '\u8bf7\u5148\u65b0\u589e\u7528\u6237', disabled: !names.length }];
  for (var n = 0; n < names.length; n++) {
    options.push({ value: names[n], text: names[n] });
  }
  control._customOptions = options;
  control._customEmpty = !names.length;
  setSelectedUsersData(control, names.length ? selected : []);
  renderCustomSelect(control);
}

function firstUserOnly(users) {
  var list = [];
  for (var i = 0; i < (users || []).length; i++) {
    var parts = splitUserList(users[i]);
    for (var j = 0; j < parts.length; j++) {
      if (parts[j]) { return [parts[j]]; }
    }
  }
  return list;
}

function currentUserNames() {
  var out = [];
  var seen = {};
  var table = $('usersTable');
  if (!table) { return out; }
  var rows = table.getElementsByTagName('tbody')[0].getElementsByTagName('tr');
  for (var i = 0; i < rows.length; i++) {
    var node = field(rows[i], 'username');
    var name = node ? node.value.replace(/^\s+|\s+$/g, '') : '';
    if (!name || seen[name]) { continue; }
    seen[name] = true;
    out.push(name);
  }
  return out;
}

function serverHostCell(value) {
  var td = document.createElement('td');
  td.className = 'target-cell';
  var input = document.createElement('input');
  input.type = 'text';
  input.value = value || '';
  input.setAttribute('data-name', 'server_host');
  input.onchange = function () {
    markDirty();
    updateForwardRowForDirection(input.parentNode.parentNode);
    refreshUserBoundPorts();
  };
  input.onkeyup = input.onchange;
  td.appendChild(input);
  return td;
}

function portCell(value) {
  var td = document.createElement('td');
  td.className = 'port-cell';
  var input = document.createElement('input');
  input.type = 'text';
  input.value = value || '';
  input.maxLength = 5;
  input.setAttribute('data-name', 'port');
  input.onchange = function () {
    markDirty();
    refreshUserBoundPorts();
  };
  input.onkeyup = input.onchange;
  td.appendChild(input);
  return td;
}

function userInForward(username, fwd) {
  var users = allowedUsersForForward(fwd);
  for (var i = 0; i < users.length; i++) {
    if (users[i] === username) { return true; }
  }
  return false;
}

function allowedUsersForForward(fwd) {
  if (!fwd) { return []; }
  var source = [];
  if (fwd.allowed_users && fwd.allowed_users.length) {
    source = fwd.allowed_users;
  } else if (fwd.owner) {
    source = [fwd.owner];
  }
  var out = [];
  var seen = {};
  for (var i = 0; i < source.length; i++) {
    var parts = splitUserList(source[i]);
    for (var j = 0; j < parts.length; j++) {
      if (seen[parts[j]]) { continue; }
      seen[parts[j]] = true;
      out.push(parts[j]);
    }
  }
  return out;
}

function splitUserList(text) {
  var raw = String(text || '').split(/[,\s;]+/);
  var out = [];
  for (var i = 0; i < raw.length; i++) {
    var item = raw[i].replace(/^\s+|\s+$/g, '');
    if (item) { out.push(item); }
  }
  return out;
}

function forwardPortValue(fwd) {
  if (fwd && fwd.port) { return fwd.port; }
  return hostPortPort(fwd && fwd.server_target);
}

function forwardHostValue(fwd) {
  var direction = fwd && fwd.direction === 'server_to_client' ? 'server_to_client' : 'client_to_server';
  return direction === 'server_to_client' ? '127.0.0.1' : hostPortHost(fwd && fwd.server_target);
}

function hostPortPort(value) {
  value = (value || '').replace(/^\s+|\s+$/g, '');
  var match = /:(\d+)$/.exec(value);
  return match ? match[1] : '';
}

function hostPortHost(value) {
  value = (value || '').replace(/^\s+|\s+$/g, '');
  var bracketed = /^\[([^\]]+)\]:(\d+)$/.exec(value);
  if (bracketed) { return bracketed[1]; }
  var basic = /^([^:\s]+):(\d+)$/.exec(value);
  return basic ? basic[1] : '';
}

function joinHostPort(host, port) {
  host = (host || '').replace(/^\s+|\s+$/g, '');
  if (host.indexOf(':') >= 0 && host.charAt(0) !== '[') { host = '[' + host + ']'; }
  return host + ':' + port;
}

function updateForwardRowForDirection(row) {
  if (!row) { return; }
  var direction = field(row, 'direction');
  var host = field(row, 'server_host');
  var port = field(row, 'port');
  if (!direction || !host || !port) { return; }
  if (direction.value === 'server_to_client') {
    host.value = '127.0.0.1';
    host.disabled = true;
    host.placeholder = '';
    port.placeholder = '18080';
    return;
  }
  host.disabled = false;
  host.placeholder = '127.0.0.1 或 192.168.1.10';
  port.placeholder = '3389';
}

function refreshForwardOwnerOptions() {
  var table = $('forwardsTable');
  if (table) {
    var selects = table.querySelectorAll('[data-name="allowed_users"]');
    for (var i = 0; i < selects.length; i++) {
      refreshAllowedUserSelect(selects[i]);
    }
  }
  refreshUserBoundPorts();
}

function actionCell(row) {
  var td = document.createElement('td');
  var btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'btn small delete-btn';
  btn.textContent = '删除';
  btn.onclick = function () {
    row.parentNode.removeChild(row);
    ensureForwardsEmptyRow();
    refreshUserBoundPorts();
    markDirty();
  };
  td.appendChild(btn);
  return td;
}

function userActionCell(row) {
  var td = document.createElement('td');
  var wrap = document.createElement('div');
  wrap.className = 'row-actions';
  var deleteBtn = document.createElement('button');
  deleteBtn.type = 'button';
  deleteBtn.className = 'btn small delete-btn';
  deleteBtn.textContent = '删除';
  deleteBtn.onclick = function () {
    var username = field(row, 'username').value.replace(/^\s+|\s+$/g, '') || '未命名用户';
    if (!window.confirm('确定删除用户“' + username + '”？删除后，该用户关联的转发端口需要重新分配。')) {
      return;
    }
    row.parentNode.removeChild(row);
    ensureUsersEmptyRow();
    refreshForwardOwnerOptions();
    markDirty();
  };
  var resetBtn = document.createElement('button');
  resetBtn.type = 'button';
  resetBtn.className = 'btn small reset-btn';
  resetBtn.textContent = '重置密码';
  resetBtn.onclick = function () { openResetPassword(row); };
  wrap.appendChild(deleteBtn);
  wrap.appendChild(resetBtn);
  td.appendChild(wrap);
  return td;
}

function openResetPassword(row) {
  var username = field(row, 'username').value.replace(/^\s+|\s+$/g, '');
  if (!username) {
    showToast('请先填写用户名，再重置密码', true);
    return;
  }
  resetPasswordRow = row;
  $('resetUserName').textContent = username;
  $('resetPassword').value = '';
  $('resetPasswordConfirm').value = '';
  $('resetModal').className = 'modal';
  window.setTimeout(function () { $('resetPassword').focus(); }, 0);
}

function closeResetPassword() {
  resetPasswordRow = null;
  $('resetPassword').value = '';
  $('resetPasswordConfirm').value = '';
  $('resetModal').className = 'modal hidden';
}

function confirmResetPassword() {
  if (!resetPasswordRow) {
    closeResetPassword();
    return;
  }
  var password = $('resetPassword').value;
  var confirmPassword = $('resetPasswordConfirm').value;
  if (!password) {
    showToast('新密码不能为空', true);
    return;
  }
  if (password !== confirmPassword) {
    showToast('两次输入的新密码不一致', true);
    return;
  }
  setBusy(true);
  api('POST', '/api/password/hash', { password: password }, function (res) {
    setBusy(false);
    if (!res.ok || !res.password_hash) {
      showToast(res.message || '密码重置失败', true);
      return;
    }
    field(resetPasswordRow, 'password_hash').value = res.password_hash;
    closeResetPassword();
    markDirty();
    showToast('密码已重置，请保存配置后生效', false);
  });
}

function collectConfig() {
  return {
    listen_addr: $('listenAddr').value,
    monitor_addr: $('monitorAddr').value,
    log_level: $('logLevel').value,
    tls: {
      cert_file: $('tlsCert').value,
      key_file: $('tlsKey').value,
      min_version: $('tlsMinVersion').value
    },
    users: collectUsers(),
    forwards: collectForwards(),
    security: {
      handshake_timeout_sec: toInt($('handshakeTimeout').value),
      dial_timeout_sec: toInt($('dialTimeout').value),
      max_handshake_bytes: toInt($('maxHandshakeBytes').value),
      max_concurrent_connections: toInt($('maxConcurrentConnections').value),
      max_concurrent_connections_per_ip: toInt($('maxConcurrentConnectionsPerIP').value),
      connection_rate_window_sec: toInt($('connectionRateWindow').value),
      max_new_connections_per_ip_window: toInt($('maxNewConnectionsPerIPWindow').value),
      max_tracked_connection_ips: toInt($('maxTrackedConnectionIPs').value),
      connection_limiter_cleanup_sec: toInt($('connectionLimiterCleanupSec').value),
      max_tracked_failure_ips: toInt($('maxTrackedFailureIPs').value),
      failure_tracker_cleanup_sec: toInt($('failureTrackerCleanupSec').value),
      entry_traffic_log_queue_size: toInt($('entryTrafficLogQueueSize').value),
      max_concurrent_streams_per_user: toInt($('maxConcurrentStreamsPerUser').value),
      stream_rate_limit_bytes_per_sec: toInt($('streamRateLimitBytesPerSec').value),
      auth_fail_window_sec: toInt($('authFailWindow').value),
      auth_fail_threshold: toInt($('authFailThreshold').value),
      auth_fail_block_sec: toInt($('authFailBlock').value)
    },
    credential_seal: {
      key_id: $('sealKeyID').value,
      private_key_file: $('sealPrivate').value,
      public_key_file: $('sealPublic').value,
      expires_at: $('sealExpires').value,
      active: true
    }
  };
}

function collectUsers() {
  var rows = $('usersTable').getElementsByTagName('tbody')[0].getElementsByTagName('tr');
  var out = [];
  for (var i = 0; i < rows.length; i++) {
    if (!field(rows[i], 'username')) { continue; }
    out.push({
      username: field(rows[i], 'username').value,
      password_hash: field(rows[i], 'password_hash').value,
      disabled: field(rows[i], 'disabled').checked
    });
  }
  return out;
}

function collectForwards() {
  var rows = $('forwardsTable').getElementsByTagName('tbody')[0].getElementsByTagName('tr');
  var out = [];
  for (var i = 0; i < rows.length; i++) {
    var directionNode = field(rows[i], 'direction');
    var hostNode = field(rows[i], 'server_host');
    var portNode = field(rows[i], 'port');
    var allowedNode = field(rows[i], 'allowed_users');
    if (!directionNode || !hostNode || !portNode || !allowedNode) { continue; }
    var direction = directionNode.value;
    var host = hostNode.value.replace(/^\s+|\s+$/g, '');
    var port = portNode.value.replace(/^\s+|\s+$/g, '');
    var isReverse = direction === 'server_to_client';
    var allowedUsers = selectedAllowedUsers(allowedNode);
    out.push({
      direction: direction,
      owner: allowedUsers.length ? allowedUsers[0] : '',
      allowed_users: allowedUsers,
      port: port,
      listen_addr: '',
      server_target: isReverse ? '' : joinHostPort(host, port)
    });
  }
  return out;
}

function validateUsers() {
  var rows = $('usersTable').getElementsByTagName('tbody')[0].getElementsByTagName('tr');
  var seen = {};
  for (var i = 0; i < rows.length; i++) {
    var usernameNode = field(rows[i], 'username');
    if (!usernameNode) { continue; }
    var passwordNode = field(rows[i], 'password_hash');
    var disabledNode = field(rows[i], 'disabled');
    var username = usernameNode ? usernameNode.value.replace(/^\s+|\s+$/g, '') : '';
    var passwordHash = passwordNode ? passwordNode.value.replace(/^\s+|\s+$/g, '') : '';
    var disabled = disabledNode ? disabledNode.checked : false;
    if (!username && !passwordHash && !disabled) { continue; }
    if (!username) {
      return '\u7528\u6237\u540d\u4e0d\u80fd\u4e3a\u7a7a\u3002';
    }
    if (seen[username]) {
      return '\u7528\u6237 "' + username + '" \u91cd\u590d\uff0c\u8bf7\u4fdd\u7559\u552f\u4e00\u4e00\u6761\u3002';
    }
    seen[username] = true;
    if (!disabled && !passwordHash) {
      return '\u7528\u6237 "' + username + '" \u8fd8\u6ca1\u6709\u5bc6\u7801\uff0c\u8bf7\u5148\u70b9\u51fb\u201c\u91cd\u7f6e\u5bc6\u7801\u201d\u3002';
    }
  }
  return '';
}

function validateForwards() {
  var rows = $('forwardsTable').getElementsByTagName('tbody')[0].getElementsByTagName('tr');
  for (var i = 0; i < rows.length; i++) {
    var name = '\u7b2c ' + (i + 1) + ' \u6761\u89c4\u5219';
    var directionNode = field(rows[i], 'direction');
    var allowedNode = field(rows[i], 'allowed_users');
    var hostNode = field(rows[i], 'server_host');
    var portNode = field(rows[i], 'port');
    if (!directionNode || !allowedNode || !hostNode || !portNode) { continue; }
    var direction = directionNode.value;
    var allowedUsers = selectedAllowedUsers(allowedNode);
    var host = hostNode.value.replace(/^\s+|\s+$/g, '');
    var port = portNode.value.replace(/^\s+|\s+$/g, '');
    if (!host && !port && !allowedUsers.length && !direction) { continue; }
    if (!direction) {
      return '\u8f6c\u53d1\u7aef\u53e3 "' + name + '" \u9700\u8981\u9009\u62e9\u4ee3\u7406\u65b9\u5411\u3002';
    }
    if (!port || !isValidPort(port)) {
      return '\u8f6c\u53d1\u7aef\u53e3 "' + name + '" \u7684\u670d\u52a1\u7aef\u7aef\u53e3\u5fc5\u987b\u662f 1-65535 \u7684\u6570\u5b57\u3002';
    }
    if (direction === 'server_to_client') {
      if (!allowedUsers.length) {
        return '\u8f6c\u53d1\u7aef\u53e3 "' + name + '" \u9700\u8981\u586b\u5199\u653e\u901a\u7528\u6237\u3002';
      }
      continue;
    }
    if (!host || !isValidHostPort(joinHostPort(host, port))) {
      return '\u8f6c\u53d1\u7aef\u53e3 "' + name + '" \u7684\u670d\u52a1\u7aef IP \u5730\u5740\u4e0d\u6b63\u786e\u3002';
    }
    if (!allowedUsers.length) {
      return '\u8f6c\u53d1\u7aef\u53e3 "' + name + '" \u9700\u8981\u586b\u5199\u653e\u901a\u7528\u6237\u3002';
    }
  }
  return '';
}

function isValidPort(port) {
  if (!/^\d+$/.test(port || '')) { return false; }
  var n = parseInt(port, 10);
  return n > 0 && n <= 65535;
}

function isValidHostPort(value) {
  var match = /^(?:[^:\s]+|\[[^\]\s]+\]):(\d+)$/.exec(value || '');
  return !!match && isValidPort(match[1]);
}

function field(row, name) {
  var nodes = row.querySelectorAll('[data-name="' + name + '"]');
  return nodes[0];
}

function toInt(value) {
  var n = parseInt(value, 10);
  return isNaN(n) ? 0 : n;
}

function saveConfig(done) {
  if (state && state.config_writable === false) {
    showToast(configLockMessage(), true);
    if (done) { done(false); }
    return;
  }
  var userError = validateUsers();
  if (userError) {
    showToast(userError, true);
    if (done) { done(false); }
    return;
  }
  var forwardError = validateForwards();
  if (forwardError) {
    showToast(forwardError, true);
    if (done) { done(false); }
    return;
  }
  setBusy(true);
  api('POST', '/api/config', collectConfig(), function (res) {
    setBusy(false);
    if (!res.ok) {
      if (res.state) { renderState(res.state, false); }
      if (res.issues) { showValidationIssues(res.issues); }
      showToast(res.message || '保存失败', true);
      if (done) { done(false); }
      return;
    }
    dirty = false;
    if (res.state) { renderState(res.state, true); }
    showToast(res.message || '配置已保存', false);
    if (done) { done(true); }
  });
}

function serviceAction(path, successMessage) {
  setBusy(true);
  api('POST', path, {}, function (res) {
    setBusy(false);
    if (res.state) { renderState(res.state, false); }
    showToast(res.message || successMessage, !res.ok);
  });
}

function unblockIP(ip) {
  if (!ip || busy) { return; }
  setBusy(true);
  api('POST', '/api/security/unblock', { ip: ip }, function (res) {
    setBusy(false);
    if (res.state) { renderState(res.state, false); }
    showToast((res && res.message) || ('已解封 ' + ip), !(res && res.ok));
  });
}

function deletePermanentBlock(ip) {
  if (!ip || busy) { return; }
  if (!window.confirm('确认删除永久封禁 IP：' + ip + '？')) { return; }
  setBusy(true);
  api('POST', '/api/security/permanent-block/delete', { ip: ip }, function (res) {
    setBusy(false);
    if (res.state) { renderState(res.state, false); }
    showToast((res && res.message) || ('已删除永久封禁 ' + ip), !(res && res.ok));
  });
}

function restartService() {
  if (dirty) {
    saveConfig(function (ok) {
      if (ok) { serviceAction('/api/service/restart', '服务已重启'); }
    });
    return;
  }
  serviceAction('/api/service/restart', '服务已重启');
}

function renderActivityDetails(businessLogs, requestLogs, blocked, permanentBlocked) {
  businessLogs = businessLogs || [];
  requestLogs = requestLogs || [];
  blocked = blocked || [];
  permanentBlocked = permanentBlocked || [];
  var businessFailed = 0;
  for (var i = 0; i < businessLogs.length; i++) {
    if (isFailureRecord(businessLogs[i])) { businessFailed++; }
  }
  var requestFailed = 0;
  var requestAuthOK = 0;
  for (var j = 0; j < requestLogs.length; j++) {
    if (isFailureRecord(requestLogs[j])) { requestFailed++; }
    if ((requestLogs[j] || {}).auth_result === 'ok') { requestAuthOK++; }
  }
  setText('businessFailedCountView', businessFailed);
  setText('businessCountView', businessLogs.length);
  setText('requestCountView', requestLogs.length);
  setText('requestFailedCountView', requestFailed);
  setText('requestAuthOKView', requestAuthOK);
  renderBlockDetails(blocked, permanentBlocked);
  renderBusinessLogs(businessLogs);
  renderRequestLogs(requestLogs);
}

function renderBlockDetails(temporary, permanent) {
  temporary = temporary || [];
  permanent = permanent || [];
  setText('temporaryBlockedCountView', temporary.length);
  setText('permanentBlockedCountView', permanent.length);
  renderTemporaryBlockedIPs(temporary);
  renderPermanentBlockedIPs(permanent);
}

function renderTemporaryBlockedIPs(items) {
  var tbody = $('temporaryBlockedTable').getElementsByTagName('tbody')[0];
  tbody.innerHTML = '';
  if (!items || items.length === 0) {
    appendEmptyRow(tbody, 4, '暂无封禁 IP');
    return;
  }
  for (var i = 0; i < items.length; i++) {
    var item = items[i] || {};
    var tr = document.createElement('tr');
    appendPlainCell(tr, item.ip || '-');
    appendPlainCell(tr, remainText(item.remaining_sec), 'strong-text');
    appendPlainCell(tr, timeText(item.blocked_until));
    appendUnblockCell(tr, item.ip || '');
    tbody.appendChild(tr);
  }
}

function renderPermanentBlockedIPs(items) {
  var tbody = $('permanentBlockedTable').getElementsByTagName('tbody')[0];
  tbody.innerHTML = '';
  if (!items || items.length === 0) {
    appendEmptyRow(tbody, 5, '暂无永久封禁 IP');
    return;
  }
  for (var i = 0; i < items.length; i++) {
    var item = items[i] || {};
    var tr = document.createElement('tr');
    appendPlainCell(tr, item.ip || '-');
    appendPlainCell(tr, '永久封禁', 'strong-text');
    appendPlainCell(tr, permanentBlockSourceText(item), 'ellipsis-cell');
    appendPlainCell(tr, '重启服务后仍然生效', 'ellipsis-cell');
    appendPermanentBlockDeleteCell(tr, item.ip || '');
    tbody.appendChild(tr);
  }
}

function permanentBlockSourceText(item) {
  item = item || {};
  if (!item.source) { return '-'; }
  if (item.line) { return item.source + ':' + item.line; }
  return item.source;
}

function appendUnblockCell(row, ip) {
  var td = document.createElement('td');
  td.className = 'row-actions';
  var btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'btn small danger';
  btn.textContent = '解封';
  btn.setAttribute('data-unblock-ip', ip);
  btn.disabled = busy || !ip;
  btn.onclick = function () { unblockIP(ip); };
  td.appendChild(btn);
  row.appendChild(td);
}

function appendPermanentBlockDeleteCell(row, ip) {
  var td = document.createElement('td');
  td.className = 'row-actions';
  var btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'btn small danger';
  btn.textContent = '删除';
  btn.setAttribute('data-delete-permanent-ip', ip);
  btn.disabled = busy || !ip;
  btn.onclick = function () { deletePermanentBlock(ip); };
  td.appendChild(btn);
  row.appendChild(td);
}

function setUnblockButtonsDisabled(disabled) {
  var buttons = document.querySelectorAll('[data-unblock-ip]');
  for (var i = 0; i < buttons.length; i++) {
    buttons[i].disabled = disabled;
  }
}

function setPermanentBlockDeleteButtonsDisabled(disabled) {
  var buttons = document.querySelectorAll('[data-delete-permanent-ip]');
  for (var i = 0; i < buttons.length; i++) {
    buttons[i].disabled = disabled;
  }
}

function ensureUsersEmptyRow() {
  var tbody = $('usersTable').getElementsByTagName('tbody')[0];
  if (!hasDataRows(tbody)) {
    appendEmptyRow(tbody, 4, '\u6682\u65e0\u7528\u6237\uff0c\u70b9\u51fb\u201c\u65b0\u589e\u7528\u6237\u201d\u6dfb\u52a0\u8d26\u53f7');
    updateButtons();
  }
}

function ensureForwardsEmptyRow() {
  var tbody = $('forwardsTable').getElementsByTagName('tbody')[0];
  if (!hasDataRows(tbody)) {
    appendEmptyRow(tbody, 5, '\u6682\u65e0\u8f6c\u53d1\u7aef\u53e3\uff0c\u70b9\u51fb\u201c\u65b0\u589e\u8f6c\u53d1\u201d\u6dfb\u52a0\u89c4\u5219');
    updateButtons();
  }
}

function setActiveTab(tab) {
  activeTab = tab || 'overview';
  var navItems = document.querySelectorAll('.nav-item');
  for (var i = 0; i < navItems.length; i++) {
    var isActive = navItems[i].getAttribute('data-tab') === activeTab;
    navItems[i].className = isActive ? 'nav-item active' : 'nav-item';
  }
  var panels = document.querySelectorAll('.tab-panel');
  for (var j = 0; j < panels.length; j++) {
    var panelActive = panels[j].id === activeTab;
    setClassEnabled(panels[j], 'active', panelActive);
  }
  if (activeTab === 'analysis' && !hasLoadedAnalysis()) {
    loadAnalysis();
  }
  initResizableTables();
  updateLogScrollbars();
}

function bindTabs() {
  var navItems = document.querySelectorAll('.nav-item');
  for (var i = 0; i < navItems.length; i++) {
    navItems[i].onclick = function () {
      setActiveTab(this.getAttribute('data-tab'));
    };
  }
}

function bindActions() {
  var buttons = document.querySelectorAll('[data-action]');
  for (var i = 0; i < buttons.length; i++) {
    buttons[i].onclick = function () {
      var action = this.getAttribute('data-action');
      if (action === 'refresh') {
        if (!dirty) {
          loadState(true);
        } else {
          var dialog = $('refreshDialog');
          dialog.returnValue = 'cancel';
          dialog.showModal();
        }
      }
      else if (action === 'save') { saveConfig(); }
      else if (action === 'restart') { restartService(); }
    };
  }
  $('addUserBtn').onclick = function () {
    addUserRow({}, []);
    refreshForwardOwnerOptions();
    markDirty();
  };
  $('addForwardBtn').onclick = function () {
    addForwardRow({});
    markDirty();
  };
  $('resetCancelBtn').onclick = closeResetPassword;
  $('resetConfirmBtn').onclick = confirmResetPassword;
  $('refreshDialog').addEventListener('close', function () {
    if (this.returnValue === 'discard') { loadState(true); }
  });
  if ($('analysisRunBtn')) {
    $('analysisRunBtn').onclick = loadAnalysis;
  }
}

function init() {
  bindTabs();
  bindSecurityFields();
  bindActions();
  bindCustomSelectDismissal();
  bindLogHorizontalScroll('businessTable', 'businessScrollBar', 'businessScrollTrack');
  bindLogHorizontalScroll('requestTable', 'requestScrollBar', 'requestScrollTrack');
  window.addEventListener('resize', updateLogScrollbars);
  initAnalysisDefaults();
  setActiveTab(activeTab);
  loadState(true);
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}

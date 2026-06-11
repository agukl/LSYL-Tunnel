(function () {
  var state = null;
  var dirty = false;
  var busy = false;
  var activeTab = 'overview';
  var resetPasswordRow = null;
  var analysisLoaded = false;
  var serverLoopbackHost = '127.0.0.1';
  var lockedSecurityIds = ['listenAddr', 'monitorAddr', 'logLevel', 'tlsCert', 'tlsKey', 'tlsMinVersion', 'sealKeyID', 'sealPrivate', 'sealPublic', 'sealExpires'];
  var editableSecurityIds = ['handshakeTimeout', 'dialTimeout', 'maxHandshakeBytes', 'maxConcurrentConnections', 'maxConcurrentConnectionsPerIP', 'connectionRateWindow', 'maxNewConnectionsPerIPWindow', 'maxConcurrentStreamsPerUser', 'streamRateLimitBytesPerSec', 'authFailWindow', 'authFailThreshold', 'authFailBlock'];

  function $(id) { return document.getElementById(id); }

  function setText(id, text) {
    var node = $(id);
    if (node) { node.textContent = text == null || text === '' ? '-' : String(text); }
  }

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

  function api(method, path, body, cb) {
    var xhr = new XMLHttpRequest();
    xhr.open(method, path, true);
    xhr.setRequestHeader('Accept', 'application/json');
    if (body != null) { xhr.setRequestHeader('Content-Type', 'application/json; charset=utf-8'); }
    xhr.onreadystatechange = function () {
      if (xhr.readyState !== 4) { return; }
      var data = null;
      try { data = JSON.parse(xhr.responseText || '{}'); } catch (e) { data = { ok: false, message: '管理台返回内容无法读取' }; }
      if (xhr.status < 200 || xhr.status >= 300) {
        data.ok = false;
        data.message = data.message || '管理台请求失败';
      }
      cb(data);
    };
    xhr.onerror = function () { cb({ ok: false, message: '管理台连接失败，请重新打开服务端 GUI' }); };
    xhr.send(body == null ? null : JSON.stringify(body));
  }

  function markDirty() {
    dirty = true;
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
    setConfigEditingEnabled(!blockedByPermission);
  }

  function setActionDisabled(action, disabled) {
    var buttons = document.querySelectorAll('[data-action="' + action + '"]');
    for (var i = 0; i < buttons.length; i++) {
      buttons[i].disabled = disabled;
    }
  }

  function loadState(fillForms) {
    api('GET', '/api/state', null, function (res) {
      if (!res || !res.config) {
        showToast(res && res.message ? res.message : '状态读取失败', true);
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

    setText('summaryText', state.message || (svc.running ? '服务正在运行' : '服务未运行'));
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
    for (var j = 0; j < editableSecurityIds.length; j++) {
      var input = $(editableSecurityIds[j]);
      if (!input) { continue; }
      input.disabled = !enabled;
      input.title = enabled ? '' : configLockMessage();
    }
  }

  function numberText(value) {
    if (value == null) { return '0'; }
    return String(value);
  }

  function bytesText(value) {
    value = Number(value || 0);
    if (value >= 1024 * 1024 * 1024) { return (value / 1024 / 1024 / 1024).toFixed(2) + ' GB'; }
    if (value >= 1024 * 1024) { return (value / 1024 / 1024).toFixed(2) + ' MB'; }
    if (value >= 1024) { return (value / 1024).toFixed(2) + ' KB'; }
    return value + ' B';
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

  function rateText(value) {
    return bytesText(value) + '/s';
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
    if (forwards.length === 0) { appendEmptyRow(tbody, 4, '\u6682\u65e0\u8f6c\u53d1\u7aef\u53e3\uff0c\u70b9\u51fb\u201c\u65b0\u589e\u8f6c\u53d1\u201d\u6dfb\u52a0\u89c4\u5219'); }
  }

  function addForwardRow(fwd) {
    var tbody = $('forwardsTable').getElementsByTagName('tbody')[0];
    removeEmptyRows(tbody);
    var tr = document.createElement('tr');
    tr.appendChild(directionCell(fwd.direction || '', tr));
    tr.appendChild(allowedUsersCell(allowedUsersForForward(fwd)));
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

  function hiddenInput(value, name) {
    var input = document.createElement('input');
    input.type = 'hidden';
    input.value = value;
    input.setAttribute('data-name', name);
    return input;
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
    var select = document.createElement('select');
    select.setAttribute('data-name', 'direction');
    select.className = 'pretty-select';
    var opt0 = document.createElement('option');
    opt0.value = '';
    opt0.text = '\u8bf7\u9009\u62e9\u65b9\u5411';
    var opt1 = document.createElement('option');
    opt1.value = 'client_to_server';
    opt1.text = '正向代理';
    var opt2 = document.createElement('option');
    opt2.value = 'server_to_client';
    opt2.text = '反向代理';
    select.add(opt0);
    select.add(opt1);
    select.add(opt2);
    select.value = value === 'server_to_client' || value === 'client_to_server' ? value : '';
    select.onchange = function () {
      markDirty();
      updateForwardRowForDirection(row);
    };
    td.appendChild(selectShell(select));
    return td;
  }

  function allowedUsersCell(selectedUsers) {
    var td = document.createElement('td');
    td.className = 'select-cell';
    var select = document.createElement('select');
    select.className = 'pretty-select user-select';
    select.setAttribute('data-name', 'allowed_users');
    setSelectedUsersData(select, firstUserOnly(selectedUsers || []));
    select.onchange = function () {
      setSelectedUsersData(select, selectedAllowedUsers(select));
      markDirty();
      refreshUserBoundPorts();
    };
    td.appendChild(selectShell(select));
    refreshAllowedUserSelect(select);
    return td;
  }

  function setSelectedUsersData(select, users) {
    select.setAttribute('data-selected-users', firstUserOnly(users || []).join(','));
  }

  function selectedAllowedUsers(control) {
    if (!control) { return []; }
    if (String(control.tagName || '').toLowerCase() === 'select') {
      if (!control.multiple) {
        return control.value ? [control.value] : [];
      }
      var out = [];
      for (var i = 0; i < control.options.length; i++) {
        if (control.options[i].selected && control.options[i].value) {
          out.push(control.options[i].value);
        }
      }
      if (!out.length) {
        return splitUserList(control.getAttribute('data-selected-users') || '');
      }
      return out;
    }
    return splitUserList(control.value);
  }

  function refreshAllowedUserSelect(select) {
    if (!select) { return; }
    var selected = firstUserOnly(splitUserList(select.getAttribute('data-selected-users') || ''));
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
    select.innerHTML = '';
    var placeholder = document.createElement('option');
    placeholder.value = '';
    placeholder.text = names.length ? '\u8bf7\u9009\u62e9\u7528\u6237' : '\u8bf7\u5148\u65b0\u589e\u7528\u6237';
    placeholder.disabled = !names.length;
    select.add(placeholder);
    if (!names.length) {
      select.value = '';
      setSelectedUsersData(select, []);
      return;
    }
    for (var n = 0; n < names.length; n++) {
      var opt = document.createElement('option');
      opt.value = names[n];
      opt.text = names[n];
      opt.selected = !!selectedMap[names[n]];
      select.add(opt);
    }
    select.value = selected.length ? selected[0] : '';
    setSelectedUsersData(select, selectedAllowedUsers(select));
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

  function selectShell(select) {
    var wrap = document.createElement('span');
    wrap.className = 'select-shell';
    var arrow = document.createElement('span');
    arrow.className = 'select-arrow';
    arrow.innerHTML = '&#9662;';
    wrap.appendChild(select);
    wrap.appendChild(arrow);
    return wrap;
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
      updateForwardRowForDirection(input.parentNode.parentNode);
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

  function forwardAllowedUsersText(fwd) {
    return allowedUsersForForward(fwd).join(', ');
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
    var direction = fwd && fwd.direction === 'server_to_client' ? 'server_to_client' : 'client_to_server';
    return hostPortPort(direction === 'server_to_client' ? fwd.listen_addr : fwd.server_target);
  }

  function hostPortPort(value) {
    value = (value || '').replace(/^\s+|\s+$/g, '');
    var match = /:(\d+)$/.exec(value);
    return match ? match[1] : '';
  }

  function serverLoopbackAddr(port) {
    return port ? serverLoopbackHost + ':' + port : '';
  }

  function updateForwardRowForDirection(row) {
    if (!row) { return; }
    var direction = field(row, 'direction');
    var port = field(row, 'port');
    if (!direction || !port) { return; }
    port.placeholder = '';
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
      var portNode = field(rows[i], 'port');
      var allowedNode = field(rows[i], 'allowed_users');
      if (!directionNode || !portNode || !allowedNode) { continue; }
      var direction = directionNode.value;
      var port = portNode.value.replace(/^\s+|\s+$/g, '');
      var addr = serverLoopbackAddr(port);
      var allowedUsers = selectedAllowedUsers(allowedNode);
      out.push({
        direction: direction,
        owner: allowedUsers.length ? allowedUsers[0] : '',
        allowed_users: allowedUsers,
        port: port,
        listen_addr: direction === 'server_to_client' ? addr : '',
        server_target: direction === 'server_to_client' ? '' : addr
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
      var portNode = field(rows[i], 'port');
      if (!directionNode || !allowedNode || !portNode) { continue; }
      var direction = directionNode.value;
      var allowedUsers = selectedAllowedUsers(allowedNode);
      var port = portNode.value.replace(/^\s+|\s+$/g, '');
      if (!port && !allowedUsers.length && !direction) { continue; }
      if (!direction) {
        return '\u8f6c\u53d1\u7aef\u53e3 "' + name + '" \u9700\u8981\u9009\u62e9\u4ee3\u7406\u65b9\u5411\u3002';
      }
      if (!port) {
        return '\u8f6c\u53d1\u7aef\u53e3 "' + name + '" \u9700\u8981\u586b\u5199\u7aef\u53e3\u53f7\u3002';
      }
      if (!isValidPort(port)) {
        return '\u8f6c\u53d1\u7aef\u53e3 "' + name + '" \u7684\u7aef\u53e3\u53f7\u4e0d\u6b63\u786e\uff0c\u8bf7\u586b\u5199 1-65535\u3002';
      }
      if (!allowedUsers.length) {
        return '\u8f6c\u53d1\u7aef\u53e3 "' + name + '" \u9700\u8981\u586b\u5199\u653e\u901a\u7528\u6237\u3002';
      }
      if (!isValidHostPort(serverLoopbackAddr(port))) {
        return '\u8f6c\u53d1\u7aef\u53e3 "' + name + '" \u751f\u6210\u7684\u670d\u52a1\u7aef\u5730\u5740\u4e0d\u6b63\u786e\u3002';
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
    var match = /^127\.0\.0\.1:(\d+)$/.exec(value || '');
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
      appendEmptyRow(tbody, 4, '暂无永久封禁 IP');
      return;
    }
    for (var i = 0; i < items.length; i++) {
      var item = items[i] || {};
      var tr = document.createElement('tr');
      appendPlainCell(tr, item.ip || '-');
      appendPlainCell(tr, '永久封禁', 'strong-text');
      appendPlainCell(tr, permanentBlockSourceText(item), 'ellipsis-cell');
      appendPlainCell(tr, '重启服务后仍然生效', 'ellipsis-cell');
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

  function setUnblockButtonsDisabled(disabled) {
    var buttons = document.querySelectorAll('[data-unblock-ip]');
    for (var i = 0; i < buttons.length; i++) {
      buttons[i].disabled = disabled;
    }
  }

  function renderBusinessLogs(items) {
    var tbody = $('businessTable').getElementsByTagName('tbody')[0];
    tbody.innerHTML = '';
    if (!items || items.length === 0) {
      appendEmptyRow(tbody, 7, '暂无业务记录');
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
  }

  function renderRequestLogs(items) {
    var tbody = $('requestTable').getElementsByTagName('tbody')[0];
    tbody.innerHTML = '';
    if (!items || items.length === 0) {
      appendEmptyRow(tbody, 8, '暂无请求记录');
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
  }

  function initAnalysisDefaults() {
    var start = $('analysisStart');
    var end = $('analysisEnd');
    if (!start || !end) { return; }
    var now = new Date();
    var weekAgo = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);
    if (!start.value) { start.value = dateTimeLocalValue(weekAgo); }
    if (!end.value) { end.value = dateTimeLocalValue(now); }
  }

  function dateTimeLocalValue(date) {
    function pad(value) { return value < 10 ? '0' + value : String(value); }
    return date.getFullYear() + '-' +
      pad(date.getMonth() + 1) + '-' +
      pad(date.getDate()) + 'T' +
      pad(date.getHours()) + ':' +
      pad(date.getMinutes());
  }

  function loadAnalysis() {
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

  function analysisDurationText(ms) {
    ms = Number(ms || 0);
    if (ms >= 3600000) { return (ms / 3600000).toFixed(2) + ' 小时'; }
    if (ms >= 60000) { return (ms / 60000).toFixed(2) + ' 分钟'; }
    if (ms > 0) { return Math.ceil(ms / 1000) + ' 秒'; }
    return '-';
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

  function appendPlainCell(row, text, className) {
    var td = document.createElement('td');
    if (className) { td.className = className; }
    td.textContent = text == null || text === '' ? '-' : String(text);
    td.title = td.textContent;
    row.appendChild(td);
  }

  function appendBadgeCell(row, text, className) {
    var td = document.createElement('td');
    var span = document.createElement('span');
    span.className = 'event-badge ' + (className || '');
    span.textContent = text || '-';
    td.appendChild(span);
    row.appendChild(td);
  }

  function appendEmptyRow(tbody, colspan, text) {
    var tr = document.createElement('tr');
    tr.className = 'empty-row-wrap';
    var td = document.createElement('td');
    td.colSpan = colspan;
    td.className = 'empty-row';
    td.textContent = text;
    tr.appendChild(td);
    tbody.appendChild(tr);
  }

  function removeEmptyRows(tbody) {
    if (!tbody) { return; }
    var rows = tbody.querySelectorAll('.empty-row-wrap');
    for (var i = rows.length - 1; i >= 0; i--) {
      rows[i].parentNode.removeChild(rows[i]);
    }
  }

  function hasDataRows(tbody) {
    if (!tbody) { return false; }
    var rows = tbody.getElementsByTagName('tr');
    for (var i = 0; i < rows.length; i++) {
      if (rows[i].className !== 'empty-row-wrap') { return true; }
    }
    return false;
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
      appendEmptyRow(tbody, 4, '\u6682\u65e0\u8f6c\u53d1\u7aef\u53e3\uff0c\u70b9\u51fb\u201c\u65b0\u589e\u8f6c\u53d1\u201d\u6dfb\u52a0\u89c4\u5219');
      updateButtons();
    }
  }

  function isFailureRecord(ev) {
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

  function eventKindText(kind) {
    switch (kind) {
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

  function eventResultText(result) {
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

  function eventResultClass(result) {
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

  function eventActorText(ev) {
    var parts = [];
    if (ev.username) { parts.push(ev.username); }
    if (ev.remote_ip) { parts.push(ev.remote_ip); }
    if (!parts.length && ev.client_id) { parts.push(ev.client_id); }
    return parts.length ? parts.join(' / ') : '-';
  }

  function eventTargetText(ev) {
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

  function eventReasonText(ev) {
    ev = ev || {};
    var parts = [];
    if (ev.code) { parts.push(ev.code); }
    if (ev.message) { parts.push(ev.message); }
    return parts.length ? parts.join(' / ') : '-';
  }

  function trafficDurationText(ev) {
    ev = ev || {};
    var parts = [];
    if (ev.bytes_up || ev.bytes_down) {
      parts.push('上行 ' + bytesText(ev.bytes_up || 0));
      parts.push('下行 ' + bytesText(ev.bytes_down || 0));
    }
    if (ev.duration_ms) { parts.push(analysisDurationText(ev.duration_ms)); }
    return parts.length ? parts.join(' / ') : '-';
  }

  function eventPeriodText(ev) {
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

  function requestTypeText(type) {
    if (type === 'reverse_listen' || type === 'reverse') { return '被动监听'; }
    if (type === 'reverse_stream') { return '被动流'; }
    if (type === 'open') { return '正向连接'; }
    if (type === 'login') { return '登录'; }
    return type || '-';
  }

  function requestResultText(item) {
    item = item || {};
    var resp = item.response || {};
    if (item.result) { return eventResultText(item.result); }
    if (resp.ok === true) { return '成功'; }
    if (resp.ok === false) { return '失败'; }
    return '-';
  }

  function requestActorText(item) {
    var req = (item || {}).request || {};
    var parts = [];
    if (req.username) { parts.push(req.username); }
    if (req.client_id) { parts.push(req.client_id); }
    if (item.auth_result) { parts.push('auth=' + item.auth_result); }
    return parts.length ? parts.join(' / ') : '-';
  }

  function requestTargetText(item) {
    var req = (item || {}).request || {};
    var parts = [];
    if (req.forward_name) { parts.push(req.forward_name); }
    if (req.listen_addr) { parts.push('监听 ' + req.listen_addr); }
    if (req.target) { parts.push('目标 ' + req.target); }
    if (req.stream_id) { parts.push('stream ' + req.stream_id); }
    return parts.length ? parts.join(' / ') : '-';
  }

  function responseText(resp) {
    resp = resp || {};
    var parts = [];
    if (resp.ok === true || resp.ok === false) { parts.push(resp.ok ? 'ok' : 'fail'); }
    if (resp.code) { parts.push(resp.code); }
    if (resp.message) { parts.push(resp.message); }
    return parts.length ? parts.join(' / ') : '-';
  }

  function compactJSON(value) {
    try { return JSON.stringify(value); } catch (e) { return '-'; }
  }

  function timeText(value) {
    if (!value) { return '-'; }
    var date = new Date(value);
    if (isNaN(date.getTime())) { return String(value); }
    return date.toLocaleString();
  }

  function remainText(seconds) {
    seconds = Number(seconds || 0);
    if (seconds <= 0) { return '-'; }
    if (seconds >= 3600) { return Math.ceil(seconds / 3600) + ' 小时'; }
    if (seconds >= 60) { return Math.ceil(seconds / 60) + ' 分钟'; }
    return Math.ceil(seconds) + ' 秒';
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
    if (activeTab === 'analysis' && !analysisLoaded) {
      loadAnalysis();
    }
  }

  function setClassEnabled(node, className, enabled) {
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
        if (action === 'refresh') { loadState(true); }
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
    if ($('analysisRunBtn')) {
      $('analysisRunBtn').onclick = loadAnalysis;
    }
  }

  function init() {
    bindTabs();
    bindSecurityFields();
    bindActions();
    initAnalysisDefaults();
    setActiveTab(activeTab);
    loadState(true);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();

var busy = false;
var lastRunning = null;
var lastState = null;
var forceNextForm = false;
var savedPasswordMask = '********';
var lastProfilesSignature = '';
var lastStateSignature = '';
var refreshTimer = null;
var refreshInFlight = false;
function el(id){ return document.getElementById(id); }
function now(){ return new Date().getTime(); }
function stopEvent(evt){
  evt = evt || window.event;
  if(evt){
    if(evt.stopPropagation) evt.stopPropagation();
    if(evt.preventDefault) evt.preventDefault();
    evt.cancelBubble = true;
    evt.returnValue = false;
  }
  return false;
}
function suppressContextMenu(){
  var block = function(evt){ return stopEvent(evt); };
  window.oncontextmenu = block;
  document.oncontextmenu = block;
  if(document.body) document.body.oncontextmenu = block;
  if(document.documentElement) document.documentElement.oncontextmenu = block;
  document.addEventListener('contextmenu', block, false);
}
function isWindowButton(target){
  while(target){
    if(target.className && (String(target.className).indexOf('win-btn') >= 0 || String(target.className).indexOf('top-refresh') >= 0)) return true;
    target = target.parentNode;
  }
  return false;
}
function isLogoTarget(target){
  while(target){
    if(target.id === 'logoMenuBtn') return true;
    target = target.parentNode;
  }
  return false;
}
function isLogoMenuTarget(target){
  while(target){
    if(target.id === 'logoMenu') return true;
    if(target.id === 'logoMenuBtn') return true;
    target = target.parentNode;
  }
  return false;
}
function beginDrag(evt){
  evt = evt || window.event;
  var target = evt.target || evt.srcElement;
  if(isWindowButton(target)) return true;
  if(isLogoTarget(target)) return true;
  if(evt && evt.preventDefault) evt.preventDefault();
  api('/api/window/drag', {}, function(){});
  return false;
}
function minimizeWindow(evt){
  stopEvent(evt);
  api('/api/window/minimize', {}, function(){});
  return false;
}
function closeWindow(evt){
  stopEvent(evt);
  api('/api/window/close', {}, function(){});
  return false;
}
function api(path, body, cb, onError){
  var xhr = new XMLHttpRequest();
  var method = body ? 'POST' : 'GET';
  xhr.open(method, path + (!body ? '?_=' + now() : ''), true);
  xhr.setRequestHeader('Content-Type', 'application/json');
  xhr.onreadystatechange = function(){
    if(xhr.readyState !== 4) return;
    try { cb(JSON.parse(xhr.responseText)); }
    catch(e) {
      showMessage('界面请求失败，请重新打开客户端窗口后再试。', true);
      if(onError) onError();
    }
  };
  xhr.send(body ? JSON.stringify(body) : null);
}
function formData(){
  var password = el('password').value;
  if(el('password').getAttribute('data-saved') === '1' && password === savedPasswordMask){
    password = '';
  }
  return {
    server_addr: el('server_addr').value,
    username: el('username').value,
    password: password
  };
}
function setForm(f, force){
  f = f || {};
  if(force){
    el('server_addr').value = f.server_addr || '';
    el('username').value = f.username || '';
    el('password').value = f.password || '';
    el('password').setAttribute('data-saved', '0');
    return;
  }
  if(!el('server_addr').value) el('server_addr').value = f.server_addr || '';
  if(!el('username').value) el('username').value = f.username || '';
  if(!el('password').value) el('password').value = f.password || '';
}
function setSavedPasswordMask(enabled){
  var pwd = el('password');
  if(enabled){
    pwd.placeholder = '';
    if(pwd.getAttribute('data-saved') !== '1' || !pwd.value) pwd.value = savedPasswordMask;
    pwd.setAttribute('data-saved', '1');
    return;
  }
  if(pwd.getAttribute('data-saved') === '1') pwd.value = '';
  pwd.placeholder = '请输入密码';
  pwd.setAttribute('data-saved', '0');
}
function clearSavedPasswordMask(){
  var pwd = el('password');
  if(pwd.getAttribute('data-saved') === '1'){
    pwd.value = '';
    pwd.setAttribute('data-saved', '0');
  }
}
function clearMessages(){
  el('loginMsg').className = 'inline-message';
  el('loginMsg').innerText = '';
  el('runMsg').className = 'inline-message';
  el('runMsg').innerText = '';
}
function hideLogoMenu(){
  var menu = el('logoMenu');
  if(menu) menu.className = 'logo-menu';
  el('logoMenuBtn').setAttribute('aria-expanded', 'false');
}
function syncLogoMenu(state, running){
  var exportBtn = el('mobileExportMenuBtn');
  var empty = el('logoMenuEmpty');
  var profiles = (state && state.profiles) || [];
  var canSwitch = !running && profiles.length >= 2;
  if(exportBtn) exportBtn.style.display = running ? 'block' : 'none';
  if(empty) empty.style.display = (!running && !canSwitch) ? 'block' : 'none';
}
function showLogoMenu(evt){
  stopEvent(evt);
  var state = lastState || {};
  var running = !!state.running;
  renderProfiles(state, running);
  syncLogoMenu(state, running);
  var menu = el('logoMenu');
  if(menu) menu.className = 'logo-menu open';
  el('logoMenuBtn').setAttribute('aria-expanded', 'true');
  var items = menu.querySelectorAll('button:not(:disabled)');
  for(var i=0;i<items.length;i++){
    if(items[i].offsetParent !== null){ items[i].focus(); break; }
  }
  return false;
}
function handleLogoMenuKeyDown(evt){
  var code = evt.keyCode || evt.which;
  if(code === 13 || code === 32 || code === 40 || code === 93 || (code === 121 && evt.shiftKey)){
    return showLogoMenu(evt);
  }
  return true;
}
function handleLogoMenuMouseDown(evt){
  evt = evt || window.event;
  var target = evt.target || evt.srcElement;
  if(!isLogoMenuTarget(target)) hideLogoMenu();
}
function installLogoMenuDismiss(){
  document.addEventListener('mousedown', handleLogoMenuMouseDown, true);
  document.addEventListener('focusin', function(evt){
    if(!isLogoMenuTarget(evt.target)) hideLogoMenu();
  }, false);
  document.addEventListener('keydown', function(evt){
    if(el('logoMenu').className.indexOf('open') < 0) return;
    var code = evt.keyCode || evt.which;
    if(code === 27){
      hideLogoMenu();
      el('logoMenuBtn').focus();
      stopEvent(evt);
    } else if(code === 38 || code === 40){
      var nodes = el('logoMenu').querySelectorAll('button:not(:disabled)');
      var items = [];
      var index = -1;
      for(var i=0;i<nodes.length;i++){
        if(nodes[i].offsetParent === null) continue;
        if(nodes[i] === document.activeElement) index = items.length;
        items.push(nodes[i]);
      }
      if(items.length){
        index = index < 0 ? (code === 40 ? 0 : items.length - 1) : (index + (code === 40 ? 1 : -1) + items.length) % items.length;
        items[index].focus();
        stopEvent(evt);
      }
    }
  }, false);
}
function showMessage(message, bad){
  var running = el('app').className.indexOf('running') >= 0;
  var target = running ? el('runMsg') : el('loginMsg');
  target.className = bad ? 'inline-message show bad' : 'inline-message show';
  target.innerText = message || '';
}
function setStatus(running, state){
  var text = running ? (state.run_status || '后台值守中') : '未连接';
  var cls = statusClass(running, state.stats || {});
  el('topStatus').className = 'top-status ' + cls;
  el('topStatusText').innerText = text;
  el('topStatus').title = text;
  el('statusLogin').className = running ? 'status-chip ' + cls : 'status-chip';
  el('statusLogin').innerText = text;
  el('statusReconnect').className = 'status-chip ' + cls;
  el('statusReconnect').innerText = text;
  el('closeBtn').title = running ? '关闭到托盘，继续后台值守' : '关闭客户端';
}
function setVersion(state){
  var clientVersion = (state && state.version) || '';
  var serverVersion = (state && state.server_version) || '';
  var text = clientVersion ? ('C v' + clientVersion) : '';
  if(serverVersion) text += (text ? '  ' : '') + 'S v' + serverVersion;
  el('versionBadge').innerText = text;
}
function statusClass(running, stats){
  if(!running) return 'off';
  var health = (stats.health || {}).state || '';
  if(health === 'auth_error') return 'bad';
  if(health === 'server_unavailable' || health === 'checking') return 'warn';
  if(hasForwardIssue(stats)) return 'warn';
  return 'on';
}
function hasForwardIssue(stats){
  var items = (stats && stats.items) || [];
  for(var i=0;i<items.length;i++){
    if(isForwardIssue((items[i] || {}).state || '')) return true;
  }
  return false;
}
function isForwardIssue(state){
  return state === 'listen_failed' || state === 'retrying' || state === 'rejected';
}
function isReconnecting(stats){
  return ((stats && stats.health) || {}).state === 'server_unavailable';
}
function routeHTML(state){
  var stats = state.stats || {};
  var lines = [];
  var items = stats.items || [];
  if(items.length){
    for(var i=0;i<items.length;i++){
      var item = items[i] || {};
      var reverse = item.direction === 'server_to_client';
      var clientEndpoint = reverse
        ? compactAddr(item.server_target)
        : compactAddr(item.virtual_addr || item.listen_addr);
      var serverEndpoint = reverse
        ? compactAddr(item.listen_addr)
        : compactAddr(item.server_target);
      var hasIssue = hasRouteItemIssue(item);
      var line = (item.name || '转发') + ': ' + clientEndpoint;
      if(!reverse || item.listen_addr){
        line += (reverse ? ' ← ' : ' → ') + serverEndpoint;
      }
      if(hasIssue){
        var issue = item.message || item.last_error || '';
        if(issue) line += '，' + issue;
      }
      lines.push(routeLineHTML(line, hasIssue));
    }
  } else if(state.route) {
    lines.push(routeLineHTML(state.route, false));
  } else {
    lines.push(routeLineHTML('暂未配置访问入口', false));
  }
  return lines.join('');
}
function routeLineHTML(text, issue){
  return '<div class="route-line' + (issue ? ' issue' : '') + '">' + escapeHTML(text) + '</div>';
}
function hasRouteItemIssue(item){
  return isForwardIssue((item || {}).state || '') || !!((item || {}).last_error);
}
function escapeHTML(value){
  return String(value || '').replace(/[&<>"']/g, function(ch){
    if(ch === '&') return '&amp;';
    if(ch === '<') return '&lt;';
    if(ch === '>') return '&gt;';
    if(ch === '"') return '&quot;';
    return '&#39;';
  });
}
function reconnectText(state){
  var stats = state.stats || {};
  var health = stats.health || {};
  var lines = [];
  lines.push(health.message || '服务端暂时不可达，正在重连。');
  if(health.consecutive_failures){
    lines.push('连续失败 ' + Number(health.consecutive_failures || 0) + ' 次。');
  }
  if(health.retry_delay_sec){
    lines.push('下次自动重试约 ' + Number(health.retry_delay_sec || 0) + ' 秒后。');
  } else {
    lines.push('正在等待下一次重试。');
  }
  lines.push('如果服务端已恢复，可以点击“立即重试”。');
  return lines.join('\n');
}
function healthText(health){
  if(!health || !health.state) return '等待检查';
  if(health.state === 'ok') return '连接正常' + (health.last_ok ? '，最近成功 ' + shortTime(health.last_ok) : '');
  if(health.state === 'auth_error') return health.message || '认证异常';
  if(health.state === 'server_unavailable') return health.message || '服务端不可达';
  return health.message || '检查中';
}
function compactAddr(addr){
  addr = addr || '';
  var m = /^(?:127\.0\.0\.1|0\.0\.0\.0|\[::\]):(\d+)$/.exec(addr);
  if(m) return m[1];
  return addr || '-';
}
function shortTime(value){
  var d = new Date(value);
  if(isNaN(d.getTime())) return value;
  return d.toLocaleTimeString();
}
function renderState(state, force){
  state = state || {};
  lastState = state;
  var signature = '';
  try { signature = JSON.stringify(state); } catch(e) {}
  if(!force && signature && signature === lastStateSignature) return;
  lastStateSignature = signature;
  var running = !!state.running;
  var changed = lastRunning !== null && lastRunning !== running;
  var reconnecting = running && isReconnecting(state.stats || {});
  el('app').className = running ? (reconnecting ? 'app running reconnecting' : 'app running') : 'app';
  setStatus(running, state);
  setVersion(state);
  if(changed) clearMessages();
  lastRunning = running;
  el('loginBtn').disabled = running || busy;
  el('route').innerHTML = routeHTML(state);
  el('reconnectInfo').innerText = reconnectText(state);
  setForm(state.config, forceNextForm);
  forceNextForm = false;
  setSavedPasswordMask(!!state.has_password);
  renderProfiles(state, running);
  syncLogoMenu(state, running);
  if(!running && state.notice){
    showMessage(state.notice, !!state.notice_bad);
  }
}
function profileListSignature(profiles){
  var parts = [];
  for(var i=0;i<profiles.length;i++){
    var p = profiles[i] || {};
    parts.push([
      p.id || '',
      p.label || '',
      p.server_addr || '',
      p.current ? '1' : '0',
      p.available ? '1' : '0',
      p.message || ''
    ].join('|'));
  }
  return parts.join('||');
}
function renderProfiles(state, running){
  var box = el('profileSwitch');
  var list = el('profileList');
  var profiles = (state && state.profiles) || [];
  if(!box) return;
  if(running || profiles.length < 2){
    box.style.display = 'none';
    lastProfilesSignature = '';
    if(list) list.innerHTML = '';
    return;
  }
  box.style.display = 'block';
  var signature = profileListSignature(profiles);
  if(signature !== lastProfilesSignature){
    if(list) list.innerHTML = '';
    for(var i=0;i<profiles.length;i++){
      var p = profiles[i] || {};
      var item = document.createElement('button');
      item.type = 'button';
      item.className = 'profile-option' + (p.current ? ' current' : '');
      item.setAttribute('data-profile-id', p.id || '');
      item.setAttribute('data-profile-current', p.current ? '1' : '0');
      item.setAttribute('data-profile-available', p.available ? '1' : '0');
      item.setAttribute('role', 'menuitem');
      item.onclick = switchProfileOption;
      item.disabled = !p.available || p.current || busy;
      var main = document.createElement('span');
      main.className = 'profile-option-main';
      main.innerText = profileOptionMainText(p);
      var meta = document.createElement('span');
      meta.className = 'profile-option-meta';
      meta.innerText = profileOptionMetaText(p);
      item.appendChild(main);
      item.appendChild(meta);
      if(list) list.appendChild(item);
    }
    lastProfilesSignature = signature;
  }
  if(list){
    var buttons = list.getElementsByTagName('button');
    for(var j=0;j<buttons.length;j++){
      var b = buttons[j];
      b.disabled = running || busy || b.getAttribute('data-profile-current') === '1' || b.getAttribute('data-profile-available') !== '1';
    }
  }
}
function profileOptionMainText(profile){
  return profile.label || profile.server_addr || profile.id || '未命名配置';
}
function profileOptionMetaText(profile){
  if(!profile.available) return profile.message || '不可用';
  if(profile.current) return '当前使用';
  return '';
}
function switchProfileOption(evt){
  stopEvent(evt);
  var target = evt ? (evt.target || evt.srcElement) : null;
  while(target && !target.getAttribute('data-profile-id')) target = target.parentNode;
  if(!target || target.disabled) return false;
  return switchProfile(target.getAttribute('data-profile-id'));
}
function switchProfile(profileID){
  if(!profileID || profileID === '__current__') return false;
  busy = true;
  hideLogoMenu();
  showMessage('正在切换客户端配置。', false);
  api('/api/profile/switch', { id: profileID }, function(r){
    busy = false;
    lastProfilesSignature = '';
    if(r.ok){
      forceNextForm = true;
      clearMessages();
      if(r.state) renderState(r.state);
      showMessage(r.message || '已切换客户端配置。', false);
    } else {
      if(r.state) renderState(r.state);
      showMessage(r.message || '切换失败，请检查配置文件和证书。', true);
    }
    refresh();
  });
  return false;
}
function refreshDelay(state){
  if(state && state.window_hidden) return 30000;
  if(state && state.running && isReconnecting(state.stats || {})) return 2500;
  if(state && state.running) return 5000;
  return 15000;
}
function scheduleRefresh(delay){
  if(refreshTimer) window.clearTimeout(refreshTimer);
  refreshTimer = window.setTimeout(refresh, delay);
}
function refresh(){
  if(refreshInFlight) return;
  if(refreshTimer) window.clearTimeout(refreshTimer);
  refreshTimer = null;
  refreshInFlight = true;
  api('/api/state', null, function(state){
    refreshInFlight = false;
    renderState(state);
    scheduleRefresh(refreshDelay(state));
  }, function(){
    refreshInFlight = false;
    scheduleRefresh(15000);
  });
}
function refreshHealth(quiet){
  if(!quiet) showMessage('正在刷新状态。', false);
  api('/api/health/check', {}, function(r){
    if(r.state) renderState(r.state);
    else refresh();
    if(!quiet || !r.ok) showMessage(r.message || '状态已刷新。', !r.ok);
  });
}
function login(){
  busy = true;
  el('loginBtn').disabled = true;
  showMessage('正在登录，请稍候。', false);
  api('/api/start', formData(), function(r){
    busy = false;
    if(r.ok){
      clearMessages();
    } else {
      showMessage(r.message || '连接失败，请检查服务端地址、证书或账号密码。', true);
    }
    refresh();
  });
}
function stopClient(){
  showMessage('正在断开连接。', false);
  api('/api/stop', null, function(r){
    if(r.ok){
      clearMessages();
    } else {
      showMessage(r.message || '断开失败，请稍后重试，或从托盘菜单退出客户端。', true);
    }
    refresh();
  });
}
function hideWindow(){ api('/api/hide', null, function(){}); }
function quitApp(){
  showMessage('正在退出客户端。', false);
  api('/api/quit', null, function(){});
}
function exportMobileProfile(evt){
  stopEvent(evt);
  hideLogoMenu();
  if(!lastState || !lastState.running){
    showMessage('请先登录成功后再导出移动端配置。', true);
    return false;
  }
  if(!window.confirm('导出移动端配置到系统下载目录？\n文件名将使用用户名和凭据到期日期。')){
    return false;
  }
  showMessage('正在导出移动端配置。', false);
  api('/api/mobile/export', {}, function(r){
    if(r.state) renderState(r.state);
    showMessage(r.message || (r.ok ? '移动端配置已导出。' : '导出失败，请稍后重试。'), !r.ok);
  });
  return false;
}
suppressContextMenu();
installLogoMenuDismiss();
refresh();

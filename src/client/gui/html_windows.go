//go:build windows

package gui

const clientHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta http-equiv="X-UA-Compatible" content="IE=edge">
<title>LSYL Tunnel Client</title>
<link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Cdefs%3E%3ClinearGradient id='g' x1='10' y1='8' x2='54' y2='58'%3E%3Cstop stop-color='%23102A43'/%3E%3Cstop offset='.55' stop-color='%230D8F87'/%3E%3Cstop offset='1' stop-color='%2318B6A7'/%3E%3C/linearGradient%3E%3C/defs%3E%3Crect x='6' y='6' width='52' height='52' rx='14' fill='url(%23g)'/%3E%3Cpath d='M20 45 L32 18 L44 45' fill='none' stroke='white' stroke-width='6' stroke-linecap='round' stroke-linejoin='round'/%3E%3Cpath d='M25 35 H39' fill='none' stroke='%23D9FFF6' stroke-width='5' stroke-linecap='round'/%3E%3C/svg%3E">
<style>
* { box-sizing:border-box; }
html, body {
  width:100%;
  height:100%;
  margin:0;
  padding:0;
  overflow:hidden;
  -ms-overflow-style:none;
}
body {
  font-family:"Microsoft YaHei UI", "Segoe UI", sans-serif;
  color:#122b36;
  background:linear-gradient(180deg, #348f95 0, #56b9b4 60px, #eef8f7 60px, #dff1f2 100%);
  user-select:none;
}
input { user-select:text; }
button, input { font-family:"Microsoft YaHei UI", "Segoe UI", sans-serif; }
.app {
  width:100%;
  height:100%;
  overflow:hidden;
  background:transparent;
  position:relative;
}
.app:after {
  content:"";
  position:absolute;
  inset:0;
  pointer-events:none;
  z-index:20;
  box-shadow:none;
}
.titlebar {
  height:60px;
  padding:0 10px 0 16px;
  color:#fff;
  background:
    radial-gradient(circle at 0% 0%, rgba(255,255,255,.16), transparent 26%),
    radial-gradient(circle at 100% 0%, rgba(255,255,255,.08), transparent 24%),
    linear-gradient(120deg, #2f8e93 0%, #43a5a3 50%, #68c2bc 100%);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.18);
  border-bottom:1px solid rgba(230,248,245,.72);
  cursor:default;
  position:relative;
  overflow:hidden;
}
.titlebar:before {
  content:"";
  position:absolute;
  inset:0;
  background:linear-gradient(180deg, rgba(255,255,255,.08), rgba(255,255,255,0) 56%);
  pointer-events:none;
}
.title-left {
  float:left;
  height:60px;
  line-height:60px;
  font-size:15px;
  font-weight:800;
  letter-spacing:.2px;
  position:relative;
  z-index:1;
}
.mini-logo {
  display:inline-block;
  width:26px;
  height:26px;
  margin:17px 9px 0 0;
  vertical-align:top;
  border-radius:8px;
  background:linear-gradient(145deg, #073447 0%, #0c8d82 56%, #35c4ae 100%);
  box-shadow:0 10px 18px rgba(16,73,78,.24), inset 0 0 0 1px rgba(255,255,255,.24);
  position:relative;
  cursor:pointer;
}
.mini-logo:before {
  content:"";
  position:absolute;
  left:8px;
  top:6px;
  width:10px;
  height:13px;
  border-left:3px solid #fff;
  border-right:3px solid #fff;
  border-top:3px solid #fff;
  transform:skewX(-18deg);
  border-radius:2px;
}
.title-actions {
  float:right;
  height:60px;
  white-space:nowrap;
  position:relative;
  z-index:1;
}
.top-status {
  display:inline-block;
  height:34px;
  line-height:34px;
  margin:13px 10px 0 0;
  padding:0 13px;
  border-radius:999px;
  background:rgba(255,255,255,.17);
  color:#fff;
  font-size:12px;
  font-weight:800;
  vertical-align:top;
}
.top-status i {
  display:inline-block;
  width:8px;
  height:8px;
  margin-right:7px;
  border-radius:50%;
  background:#b8ccd0;
  box-shadow:0 0 0 5px rgba(255,255,255,.12);
}
.top-status.on i {
  background:#5df0a0;
  animation:pulseGreen 1.7s ease-out infinite;
}
.top-status.warn i {
  background:#f6c34a;
  box-shadow:0 0 0 5px rgba(246,195,74,.18);
}
.top-status.bad i {
  background:#f36f56;
  box-shadow:0 0 0 5px rgba(243,111,86,.18);
}
.win-btn,
.top-refresh {
  display:inline-flex;
  align-items:center;
  justify-content:center;
  width:44px;
  height:38px;
  margin:11px 0 0 5px;
  padding:0;
  border:0;
  border-radius:10px;
  color:#fff;
  background:rgba(255,255,255,.14);
  line-height:1;
  cursor:pointer;
  vertical-align:top;
  transition:background .16s ease, transform .16s ease;
}
.win-btn:hover,
.top-refresh:hover {
  background:rgba(255,255,255,.24);
  transform:none;
}
.win-btn:active,
.top-refresh:active { transform:scale(.97); }
.win-btn.close:hover { background:#176f73; }
.window-icon {
  display:block;
  width:18px;
  height:18px;
  pointer-events:none;
}
.top-refresh .window-icon,
.win-btn.close .window-icon {
  width:20px;
  height:20px;
}
.content {
  height:580px;
  padding:26px 24px 24px;
  overflow:hidden;
  background:
    radial-gradient(circle at 10% 3%, rgba(29,170,154,.14), transparent 28%),
    linear-gradient(180deg, #eef8f7 0%, #dff1f2 100%);
}
.main-card {
  height:100%;
  padding:22px;
  border-radius:30px;
  background:#fff;
  border:1px solid rgba(152,190,194,.34);
  box-shadow:0 18px 38px rgba(24,72,82,.14);
  position:relative;
  overflow:hidden;
}
.main-card:before {
  content:"";
  position:absolute;
  left:-80px;
  top:-90px;
  width:190px;
  height:190px;
  border-radius:50%;
  background:rgba(16,153,140,.08);
}
.panel { position:relative; z-index:1; }
.login-panel { display:block; }
.connected-panel { display:none; }
.reconnect-panel { display:none; }
.app.running .login-panel { display:none; }
.app.running .connected-panel { display:block; }
.app.running.reconnecting .connected-panel { display:none; }
.app.running.reconnecting .reconnect-panel { display:block; }
.top-refresh {
  display:none;
}
.app.running .top-refresh {
  display:inline-flex;
}
.status-line {
  height:35px;
  margin-bottom:14px;
}
.status-chip {
  display:inline-block;
  height:34px;
  line-height:34px;
  padding:0 14px;
  border-radius:999px;
  font-size:13px;
  font-weight:900;
  color:#536b77;
  background:#edf3f5;
}
.status-chip:before {
  content:"";
  display:inline-block;
  width:10px;
  height:10px;
  margin-right:8px;
  border-radius:50%;
  background:#8aa0ad;
  box-shadow:0 0 0 5px rgba(138,160,173,.14);
}
.status-chip.on {
  color:#087551;
  background:#ddf8ec;
}
.status-chip.on:before {
  background:#22c55e;
  box-shadow:0 0 0 5px rgba(34,197,94,.14);
  animation:pulseGreen 1.7s ease-out infinite;
}
.status-chip.warn {
  color:#815800;
  background:#fff7dc;
}
.status-chip.warn:before {
  background:#f6c34a;
  box-shadow:0 0 0 5px rgba(246,195,74,.14);
}
.status-chip.bad {
  color:#9a3929;
  background:#fff0ec;
}
.status-chip.bad:before {
  background:#f36f56;
  box-shadow:0 0 0 5px rgba(243,111,86,.14);
}
.field { margin-top:13px; }
label {
  display:block;
  margin-bottom:7px;
  color:#294a58;
  font-size:13px;
  font-weight:900;
}
input {
  width:100%;
  height:43px;
  padding:0 13px;
  border:1px solid #cbdfe2;
  border-radius:14px;
  outline:none;
  color:#102d3b;
  background:#fcffff;
  font-size:15px;
  box-shadow:inset 0 1px 0 rgba(255,255,255,.8);
}
input:focus {
  border-color:#10998c;
  box-shadow:0 0 0 4px rgba(16,153,140,.12);
}
button {
  border:0;
  border-radius:15px;
  height:44px;
  padding:0 17px;
  font-size:14px;
  font-weight:900;
  cursor:pointer;
  transition:transform .14s ease, box-shadow .14s ease, background .14s ease, opacity .14s ease;
}
button:hover { transform:translateY(-1px); }
button:active { transform:translateY(0); }
button:disabled { opacity:.48; cursor:not-allowed; transform:none; box-shadow:none; }
.primary {
  width:100%;
  margin-top:17px;
  color:#fff;
  background:linear-gradient(135deg, #0b7c75 0%, #14a093 100%);
  box-shadow:0 12px 22px rgba(12,130,121,.24);
}
.secondary {
  color:#1e5360;
  background:#edf6f7;
}
.secondary:hover { background:#e4f0f2; }
.danger {
  color:#915f00;
  background:#fff8e8;
  border:1px solid #f1d391;
}
.danger:hover { background:#fff1d0; }
.row { margin-top:16px; overflow:hidden; }
.row button { width:48%; }
.row button:first-child { float:left; }
.row button:last-child { float:right; }
.wide { width:100%; margin-top:14px; }
.connected-title {
  margin:3px 0 12px;
  color:#102d3b;
  font-size:24px;
  line-height:30px;
  font-weight:900;
}
.route {
  min-height:132px;
  max-height:270px;
  padding:14px 15px;
  border-radius:18px;
  color:#0f2f3a;
  background:#edf9f6;
  border-left:5px solid #0b9186;
  white-space:pre-wrap;
  line-height:1.55;
  font-weight:900;
  overflow:auto;
}
.route-line {
  margin:0 0 5px;
}
.route-line.issue {
  color:#c33a2f;
}
.route-line:last-child { margin-bottom:0; }
.note {
  margin-top:12px;
  color:#6a818d;
  line-height:1.65;
  font-size:13px;
}
.inline-message {
  display:none;
  margin-top:12px;
  padding:10px 12px;
  border-radius:13px;
  color:#0b766c;
  background:#eaf8f4;
  border:1px solid #bfe3dd;
  font-size:13px;
  font-weight:800;
  line-height:1.45;
}
.inline-message.show { display:block; }
.inline-message.bad {
  color:#9a5a00;
  background:#fffaf0;
  border-color:#f1d391;
}
@keyframes pulseGreen {
  0% { box-shadow:0 0 0 0 rgba(34,197,94,.34); }
  70% { box-shadow:0 0 0 9px rgba(34,197,94,0); }
  100% { box-shadow:0 0 0 0 rgba(34,197,94,0); }
}
</style>
</head>
<body scroll="no" oncontextmenu="return false">
<div id="app" class="app">
  <div id="titlebar" class="titlebar" onmousedown="return beginDrag(event)">
    <div class="title-left"><span id="mobileExportLogo" class="mini-logo" title="右键导出移动端配置" oncontextmenu="return exportMobileProfile(event)"></span>LSYL Tunnel</div>
    <div class="title-actions">
      <span id="topStatus" class="top-status off"><i></i><span id="topStatusText">未连接</span></span>
      <button class="top-refresh" title="刷新状态" aria-label="刷新状态" onclick="refreshHealth(true)"><svg class="window-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7.2 7.8A6.8 6.8 0 0 1 18.4 10" fill="none" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round"/><path d="M18.4 5.8V10h-4.2" fill="none" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round"/><path d="M16.8 16.2A6.8 6.8 0 0 1 5.6 14" fill="none" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round"/><path d="M5.6 18.2V14h4.2" fill="none" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round"/></svg></button>
      <button class="win-btn minimize" title="最小化" onclick="return minimizeWindow(event)"><svg class="window-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 12h10" fill="none" stroke="currentColor" stroke-width="2.35" stroke-linecap="round"/></svg></button>
      <button id="closeBtn" class="win-btn close" title="关闭" onclick="return closeWindow(event)"><svg class="window-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M8 8l8 8M16 8l-8 8" fill="none" stroke="currentColor" stroke-width="2.35" stroke-linecap="round"/></svg></button>
    </div>
  </div>

  <div class="content">
    <div class="main-card">
      <div class="panel login-panel">
        <div class="status-line"><span id="statusLogin" class="status-chip">未连接</span></div>
        <div class="field">
          <label for="server_addr">服务端地址</label>
          <input id="server_addr" autocomplete="off" placeholder="vpn.example.com:3443">
        </div>
        <div class="field">
          <label for="username">用户名</label>
          <input id="username" autocomplete="username" placeholder="请输入用户名">
        </div>
        <div class="field">
          <label for="password">密码</label>
          <input id="password" type="password" autocomplete="current-password" placeholder="请输入密码">
        </div>
        <button id="loginBtn" class="primary" onclick="login()">连接</button>
        <div id="loginMsg" class="inline-message"></div>
      </div>

      <div class="panel connected-panel">
        <div class="connected-title">正在后台值守</div>
        <div id="route" class="route">读取中...</div>
        <div class="note">关闭窗口会隐藏到托盘，不会断开连接。需要彻底退出时，请点击“退出客户端”。</div>
        <div class="row">
          <button class="secondary" onclick="hideWindow()">隐藏到托盘</button>
          <button class="danger" onclick="stopClient()">断开连接</button>
        </div>
        <button class="secondary wide" onclick="quitApp()">退出客户端</button>
        <div id="runMsg" class="inline-message"></div>
      </div>

      <div class="panel reconnect-panel">
        <div class="status-line"><span id="statusReconnect" class="status-chip warn">正在重连</span></div>
        <div class="connected-title">正在重连服务端</div>
        <div id="reconnectInfo" class="route">等待状态...</div>
        <div class="note">客户端会按指数退避自动重试。若连续重连失败，会取消当前连接状态并回到连接页。</div>
        <button class="secondary wide" onclick="refreshHealth()">立即重试</button>
        <button class="danger wide" onclick="stopClient()">取消连接</button>
        <button class="secondary wide" onclick="quitApp()">退出客户端</button>
      </div>
    </div>
  </div>
</div>
<script>
var busy = false;
var lastRunning = null;
var lastState = null;
var savedPasswordMask = '********';
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
  if(document.addEventListener) document.addEventListener('contextmenu', block, false);
  if(document.attachEvent) document.attachEvent('oncontextmenu', block);
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
    if(target.id === 'mobileExportLogo') return true;
    if(target.className && String(target.className).indexOf('mini-logo') >= 0) return true;
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
function api(path, body, cb){
  var xhr = new XMLHttpRequest();
  var method = body ? 'POST' : 'GET';
  xhr.open(method, path + (!body ? '?_=' + now() : ''), true);
  xhr.setRequestHeader('Content-Type', 'application/json');
  xhr.onreadystatechange = function(){
    if(xhr.readyState !== 4) return;
    try { cb(JSON.parse(xhr.responseText)); }
    catch(e) { showMessage('界面请求失败，请重新打开客户端窗口后再试。', true); }
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
function setForm(f){
  f = f || {};
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
  el('statusLogin').className = running ? 'status-chip ' + cls : 'status-chip';
  el('statusLogin').innerText = text;
  el('statusReconnect').className = 'status-chip ' + cls;
  el('statusReconnect').innerText = text;
  el('closeBtn').title = running ? '关闭到托盘，继续后台值守' : '关闭客户端';
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
      var direction = item.direction === 'server_to_client'
        ? '服务端 ' + compactAddr(item.listen_addr) + ' -> 本机 ' + compactAddr(item.server_target)
        : '本机 ' + compactAddr(item.listen_addr) + ' -> 服务端 ' + compactAddr(item.server_target);
      var hasIssue = hasRouteItemIssue(item);
      var line = (item.name || '转发') + ': ' + forwardStateText(item.state) + '，' + direction;
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
function forwardStateText(state){
  if(state === 'listening') return '正常监听';
  if(state === 'listen_failed') return '端口异常';
  if(state === 'reverse_waiting') return '等待被动连接';
  if(state === 'reverse_active') return '被动已激活';
  if(state === 'retrying') return '重试中';
  if(state === 'rejected') return '已暂停';
  if(state === 'starting') return '启动中';
  return state || '未知';
}
function compactAddr(addr){
  addr = addr || '';
  var m = /^(\[?)(127\.0\.0\.1|localhost|::1|0\.0\.0\.0)(\]?):(\d+)$/.exec(addr);
  if(m) return m[4];
  return addr || '-';
}
function shortTime(value){
  var d = new Date(value);
  if(isNaN(d.getTime())) return value;
  return d.toLocaleTimeString();
}
function renderState(state){
  state = state || {};
  lastState = state;
  var running = !!state.running;
  var changed = lastRunning !== null && lastRunning !== running;
  var reconnecting = running && isReconnecting(state.stats || {});
  el('app').className = running ? (reconnecting ? 'app running reconnecting' : 'app running') : 'app';
  setStatus(running, state);
  if(changed) clearMessages();
  lastRunning = running;
  el('loginBtn').disabled = running || busy;
  el('route').innerHTML = routeHTML(state);
  el('reconnectInfo').innerText = reconnectText(state);
  setForm(state.config);
  setSavedPasswordMask(!!state.has_password);
  if(!running && state.notice){
    showMessage(state.notice, !!state.notice_bad);
  }
}
function refresh(){ api('/api/state', null, renderState); }
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
refresh();
setInterval(refresh, 2500);
</script>
</body>
</html>`

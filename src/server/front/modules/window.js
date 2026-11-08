export function bindWindowControls() {
  const controls = globalThis.wails && globalThis.wails.Window;
  if (!controls) { return; }

  const maximize = document.getElementById('windowMaximize');
  const syncMaximize = async () => {
    try {
      const maximized = await controls.IsMaximised();
      maximize.textContent = maximized ? '\u2750' : '\u25a1';
      maximize.title = maximized ? '还原' : '最大化';
      maximize.setAttribute('aria-label', maximize.title);
    } catch {
      // The window can close while a state request is in flight.
    }
  };

  document.getElementById('windowMinimize').onclick = () => controls.Minimise();
  maximize.onclick = async () => {
    await controls.ToggleMaximise();
    await syncMaximize();
  };
  document.getElementById('windowClose').onclick = () => controls.Close();
  window.addEventListener('resize', syncMaximize);
  syncMaximize();
}

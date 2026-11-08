import { hiddenInput } from './dom.js';

let customSelectCount = 0;

export function addCustomSelect(td, name, value, options, onChange) {
  var control = hiddenInput(value, name);
  control._customOptions = options || [];
  control._customChange = onChange;
  control._customEmpty = false;

  var shell = document.createElement('div');
  shell.className = 'custom-select';
  var button = document.createElement('button');
  button.type = 'button';
  button.className = 'custom-select-button';
  button.setAttribute('aria-haspopup', 'listbox');
  button.setAttribute('aria-expanded', 'false');
  button.setAttribute('aria-label', name === 'direction' ? '转发方向' : '放通用户');
  var label = document.createElement('span');
  label.className = 'custom-select-label';
  var arrow = document.createElement('span');
  arrow.className = 'custom-select-arrow';
  arrow.innerHTML = '&#9662;';
  button.appendChild(label);
  button.appendChild(arrow);
  var list = document.createElement('div');
  list.className = 'custom-select-list';
  list.setAttribute('role', 'listbox');
  list.setAttribute('aria-label', name === 'direction' ? '转发方向' : '放通用户');
  list.id = 'select-options-' + (++customSelectCount);
  button.setAttribute('aria-controls', list.id);

  button.onclick = function () {
    if (!button.disabled) { toggleCustomSelect(control); }
  };
  button.onkeydown = function (event) {
    handleCustomSelectButtonKey(control, event);
  };
  shell.appendChild(button);
  shell.appendChild(list);
  td.appendChild(control);
  td.appendChild(shell);
  control._customShell = shell;
  control._customButton = button;
  control._customLabel = label;
  control._customList = list;
  renderCustomSelect(control);
  return control;
}

export function renderCustomSelect(control) {
  if (!control || !control._customShell) { return; }
  var options = control._customOptions || [];
  var selected = null;
  for (var i = 0; i < options.length; i++) {
    if (options[i].value === control.value) { selected = options[i]; break; }
  }
  if (!selected && options.length) { selected = options[0]; }
  control._customLabel.textContent = selected ? selected.text : '';
  control._customButton.title = control._customLabel.textContent;
  control._customButton.disabled = !!control.disabled || !!control._customEmpty;
  control._customShell.classList.toggle('disabled', control._customButton.disabled);
  closeCustomSelect(control);
  var list = control._customList;
  list.innerHTML = '';
  for (var n = 0; n < options.length; n++) {
    var option = options[n];
    var item = document.createElement('button');
    item.type = 'button';
    item.className = 'custom-select-option' + (option.value === control.value ? ' selected' : '') + (option.disabled ? ' disabled' : '');
    item.setAttribute('role', 'option');
    item.setAttribute('aria-selected', option.value === control.value ? 'true' : 'false');
    item.tabIndex = -1;
    item.textContent = option.text;
    item.disabled = !!option.disabled || !!control.disabled;
    item.onclick = customSelectOptionClick(control, option.value);
    item.onkeydown = customSelectOptionKey(control);
    list.appendChild(item);
  }
}

function customSelectOptionClick(control, value) {
  return function () {
    if (control.disabled) { return; }
    control.value = value;
    closeCustomSelect(control);
    renderCustomSelect(control);
    if (control._customChange) { control._customChange(); }
    control._customButton.focus();
  };
}

function customSelectOptionKey(control) {
  return function (event) {
    if (event.key === 'Tab') {
      closeCustomSelect(control);
      control._customButton.focus();
      return;
    }
    if (event.key === 'Escape') {
      closeCustomSelect(control);
      control._customButton.focus();
      event.preventDefault();
      return;
    }
    if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
      focusCustomOption(control, this, event.key === 'ArrowDown' ? 1 : -1);
      event.preventDefault();
    }
  };
}

function handleCustomSelectButtonKey(control, event) {
  if (event.key === 'Escape') {
    closeCustomSelect(control);
    event.preventDefault();
    return;
  }
  if (event.key === 'Enter' || event.key === ' ' || event.key === 'ArrowDown') {
    openCustomSelect(control);
    focusFirstCustomOption(control);
    event.preventDefault();
  }
}

function toggleCustomSelect(control) {
  if (control._customShell.classList.contains('open')) {
    closeCustomSelect(control);
    return;
  }
  openCustomSelect(control);
}

function openCustomSelect(control) {
  if (!control || control._customButton.disabled) { return; }
  closeAllCustomSelects(control);
  control._customShell.classList.add('open');
  control._customButton.setAttribute('aria-expanded', 'true');
  positionCustomSelect(control);
}

function positionCustomSelect(control) {
  var buttonRect = control._customButton.getBoundingClientRect();
  var list = control._customList;
  var below = window.innerHeight - buttonRect.bottom - 8;
  var above = buttonRect.top - 8;
  var openAbove = below < Math.min(list.scrollHeight, 238) && above > below;
  list.style.maxHeight = Math.min(238, Math.max(80, openAbove ? above : below)) + 'px';
  list.style.width = Math.min(buttonRect.width, window.innerWidth - 16) + 'px';
  list.style.left = Math.max(8, Math.min(buttonRect.left, window.innerWidth - list.offsetWidth - 8)) + 'px';
  list.style.top = (openAbove ? buttonRect.top - list.offsetHeight - 4 : buttonRect.bottom + 4) + 'px';
}

function closeCustomSelect(control) {
  if (!control || !control._customShell) { return; }
  control._customShell.classList.remove('open');
  control._customButton.setAttribute('aria-expanded', 'false');
}

export function closeAllCustomSelects(exceptControl) {
  var nodes = document.querySelectorAll('.custom-select');
  for (var i = 0; i < nodes.length; i++) {
    if (exceptControl && nodes[i] === exceptControl._customShell) { continue; }
    nodes[i].classList.remove('open');
    var button = nodes[i].querySelector('.custom-select-button');
    if (button) { button.setAttribute('aria-expanded', 'false'); }
  }
}

function focusFirstCustomOption(control) {
  var buttons = control._customList.getElementsByTagName('button');
  for (var selected = 0; selected < buttons.length; selected++) {
    if (!buttons[selected].disabled && buttons[selected].getAttribute('aria-selected') === 'true') {
      buttons[selected].focus();
      return;
    }
  }
  for (var i = 0; i < buttons.length; i++) {
    if (!buttons[i].disabled) { buttons[i].focus(); return; }
  }
}

function focusCustomOption(control, current, step) {
  var buttons = control._customList.getElementsByTagName('button');
  var index = -1;
  for (var i = 0; i < buttons.length; i++) {
    if (buttons[i] === current) { index = i; break; }
  }
  for (var offset = 1; offset <= buttons.length; offset++) {
    var next = (index + step * offset + buttons.length) % buttons.length;
    if (!buttons[next].disabled) { buttons[next].focus(); return; }
  }
}

export function bindCustomSelectDismissal() {
  var dismiss = function (event) {
    if (event.target.closest('.custom-select')) { return; }
    closeAllCustomSelects();
  };
  document.addEventListener('click', dismiss, false);
  document.addEventListener('scroll', function (event) {
    if (event.target.classList && event.target.classList.contains('custom-select-list')) { return; }
    var nodes = document.querySelectorAll('.custom-select.open');
    for (var i = 0; i < nodes.length; i++) {
      var control = nodes[i].querySelector('input[data-name]');
      if (control) { positionCustomSelect(control); }
    }
  }, true);
  window.addEventListener('resize', function () { closeAllCustomSelects(); });
}

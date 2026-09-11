// Shared keyboard, focus and browser-back lifecycle for the existing sheets.
export function createSheetController(close) {
  let dialog = null, opener = null, candidate = null, before = null;
  let historyToken = null, consumingBack = false;
  const inert = new Map();
  const selector = 'button, a[href], input, select, textarea, [tabindex]';
  const available = el => !el.disabled && el.tabIndex >= 0 && !el.closest('[inert]') && el.getClientRects().length;
  const focusables = () => dialog ? [...dialog.querySelectorAll(selector)].filter(available) : [];
  function focusDialog() {
    const title = dialog?.querySelector('h1, h2, h3') || dialog;
    if (title) { title.tabIndex = -1; title.focus({preventScroll:true}); }
  }
  function remember(el) {
    if (!el || el === document.body) return null;
    const attrs = ['id', 'data-action', 'data-home-chore-id', 'data-chore-id', 'data-log-id', 'data-schedule-id', 'data-nav'];
    const identity = attrs.filter(attr => el.hasAttribute(attr)).map(attr => [attr, el.getAttribute(attr)]);
    return {element:el, identity};
  }
  function restoreFocus() {
    let target = opener?.element;
    if (!target?.isConnected && opener?.identity.length) {
      target = [...document.querySelectorAll(selector)].find(el => opener.identity.every(([key,value]) => el.getAttribute(key) === value));
    }
    if (target?.isConnected && !target.closest('[inert]')) target.focus({preventScroll:true});
    opener = candidate = null;
  }
  function releaseBackground() {
    for (const [el, value] of inert) el.inert = value;
    inert.clear();
    document.body.classList.remove('sheet-open');
  }
  function isolate() {
    // Inert siblings at each ancestor, leaving the sheet's backdrop clickable.
    for (let node = dialog; node?.parentElement; node = node.parentElement) {
      for (const sibling of node.parentElement.children) {
        if (sibling === node || sibling.classList.contains('sheet-backdrop')) continue;
        if (!inert.has(sibling)) inert.set(sibling, sibling.inert);
        sibling.inert = true;
      }
      if (node.parentElement === document.body) break;
    }
    document.body.classList.add('sheet-open');
  }
  function pushHistory() {
    historyToken = crypto.randomUUID();
    history.pushState({...history.state, nabuSheet:historyToken}, '', location.href);
  }
  document.addEventListener('pointerdown', event => {
    if (!dialog) candidate = event.target.closest(selector);
  }, true);
  document.addEventListener('keydown', event => {
    if (!dialog) return;
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(); return; }
    if (event.key !== 'Tab') return;
    const items = focusables(), index = items.indexOf(document.activeElement);
    if (!items.length) { event.preventDefault(); focusDialog(); }
    else if (index < 0 || (!event.shiftKey && index === items.length-1) || (event.shiftKey && index === 0)) {
      event.preventDefault(); items[event.shiftKey ? items.length-1 : 0].focus();
    }
  }, true);
  document.addEventListener('focusin', event => {
    if (dialog?.isConnected && !dialog.contains(event.target)) focusDialog();
  });
  window.addEventListener('popstate', event => {
    if (consumingBack) {
      consumingBack = false;
      if (dialog) pushHistory();
      return;
    }
    if (dialog && event.state?.nabuSheet !== historyToken) {
      historyToken = null; close();
    }
  });
  // A reload of an open sheet returns to its page, with no lingering marker.
  if (history.state?.nabuSheet) {
    const {nabuSheet, ...rest} = history.state;
    history.replaceState(rest, '', location.href);
  }
  return {
    beforeRender() { before = remember(document.activeElement) || remember(candidate); releaseBackground(); },
    afterRender(root) {
      const next = root.querySelector('.bottom-sheet'), previous = dialog;
      releaseBackground();
      dialog = next;
      if (next) {
        if (!previous) {
          opener = before || remember(candidate);
          if (!consumingBack) pushHistory();
        }
        isolate();
        if (!next.contains(document.activeElement)) focusDialog();
      } else if (previous) {
        if (historyToken && history.state?.nabuSheet === historyToken) {
          consumingBack = true; history.back();
        }
        historyToken = null; restoreFocus();
      }
      before = null;
    },
    syncBackground() { if (dialog) { releaseBackground(); isolate(); } },
  };
}

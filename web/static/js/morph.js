// morph.js updates the DOM to match new HTML while keeping the nodes that
// stay, so focus, typed values, open <details> and scroll positions survive a
// render.
//
// Children are matched in order. A child with a key (data-key, or else id)
// keeps the node with the same key, wherever that node now is. Any other
// child is matched with the next existing child that looks the same: same
// tag and same first class. Looking one node ahead finds a child that was
// added or taken away, so the nodes after it are kept instead of rebuilt.
// Whitespace-only text never decides a match, so template indentation may
// differ between renders.

export function morphInnerHTML(root, html) {
  const template = document.createElement("template");
  template.innerHTML = html;
  // Use firstElementChild to skip any leading whitespace text nodes that
  // template literals may produce (e.g. a leading newline before the root <div>).
  const node = template.content.firstElementChild;

  if (!node) {
    root.replaceChildren();
    return;
  }

  if (root.firstElementChild) {
    const existing = root.firstElementChild;
    if (existing.nodeName === node.nodeName) {
      morphAttributes(existing, node);
      morphChildren(existing, node);
    } else {
      root.replaceChild(node.cloneNode(true), existing);
    }
  } else {
    root.appendChild(node.cloneNode(true));
  }
}

const ELEMENT_NODE = 1;
const TEXT_NODE = 3;

function keyOf(node) {
  if (node.nodeType !== ELEMENT_NODE) return null;
  return node.getAttribute("data-key") || node.getAttribute("id") || null;
}

function isBlank(node) {
  return node.nodeType === TEXT_NODE && !/\S/.test(node.data);
}

function firstClass(el) {
  return (el.getAttribute("class") || "").trim().split(/\s+/)[0];
}

// canBecome: an unkeyed node can be updated in place to the incoming one.
function canBecome(existing, incoming) {
  return existing.nodeType === incoming.nodeType && existing.nodeName === incoming.nodeName &&
    !keyOf(existing) && !keyOf(incoming);
}

// isSame: it most likely is the same node, so a mismatch next to it means a
// node was added or taken away.
function isSame(existing, incoming) {
  if (!existing || !incoming || !canBecome(existing, incoming)) return false;
  return existing.nodeType !== ELEMENT_NODE || firstClass(existing) === firstClass(incoming);
}

function nextMeaningful(node) {
  while (node && isBlank(node)) node = node.nextSibling;
  return node;
}

function morphChildren(existing, incoming) {
  // Keyed children the new markup no longer has go first, so they never
  // stand in the way of a match.
  const wanted = new Set();
  for (const child of incoming.childNodes) {
    const key = keyOf(child);
    if (key) wanted.add(key);
  }
  const keyed = new Map();
  for (const child of Array.from(existing.childNodes)) {
    const key = keyOf(child);
    if (!key) continue;
    if (wanted.has(key) && !keyed.has(key)) keyed.set(key, child);
    else existing.removeChild(child);
  }

  // cursor: the first existing child not yet matched.
  let cursor = existing.firstChild;
  let n = incoming.firstChild;
  while (n) {
    const next = n.nextSibling;
    const key = keyOf(n);

    if (key) {
      const match = keyed.get(key);
      keyed.delete(key);
      if (match && match.nodeName === n.nodeName) {
        cursor = placeAt(existing, match, cursor);
        morphNode(match, n);
      } else {
        if (match) {
          if (match === cursor) cursor = cursor.nextSibling;
          existing.removeChild(match);
        }
        existing.insertBefore(n.cloneNode(true), cursor);
      }
      n = next;
      continue;
    }

    // A keyed node waits for its own key, and the end of the list adds.
    if (!cursor || keyOf(cursor)) {
      existing.insertBefore(n.cloneNode(true), cursor);
      n = next;
      continue;
    }

    if (isSame(cursor, n)) {
      morphNode(cursor, n);
      cursor = cursor.nextSibling;
      n = next;
      continue;
    }

    // Whitespace never decides a match: drop or add it and look again.
    if (isBlank(cursor) && !isBlank(n)) {
      const after = cursor.nextSibling;
      existing.removeChild(cursor);
      cursor = after;
      continue;
    }
    if (isBlank(n)) {
      existing.insertBefore(n.cloneNode(true), cursor);
      n = next;
      continue;
    }

    // n was added before the cursor's node: add it, keep the cursor's node.
    if (isSame(cursor, nextMeaningful(next))) {
      existing.insertBefore(n.cloneNode(true), cursor);
      n = next;
      continue;
    }
    // The cursor's node was taken away: remove it and match n to the next.
    if (isSame(nextMeaningful(cursor.nextSibling), n)) {
      const after = cursor.nextSibling;
      existing.removeChild(cursor);
      cursor = after;
      continue;
    }

    // Otherwise n takes the cursor's place, updating it when it can.
    if (canBecome(cursor, n)) {
      morphNode(cursor, n);
      cursor = cursor.nextSibling;
    } else {
      const replacement = n.cloneNode(true);
      existing.replaceChild(replacement, cursor);
      cursor = replacement.nextSibling;
    }
    n = next;
  }

  // Whatever was not matched is gone.
  while (cursor) {
    const after = cursor.nextSibling;
    existing.removeChild(cursor);
    cursor = after;
  }
}

// placeAt puts a keyed node at the cursor and returns the next cursor. When
// the node holds focus, the nodes between move after it instead, because a
// moved node loses focus where moveBefore() is unavailable.
function placeAt(parent, node, cursor) {
  if (node === cursor) return node.nextSibling;
  if (typeof parent.moveBefore === "function") {
    try {
      parent.moveBefore(node, cursor);
      return cursor;
    } catch { /* fall back to a plain move */ }
  }
  const active = document.activeElement;
  if (active && node.contains(active)) {
    const after = node.nextSibling;
    let first = null;
    for (let child = cursor; child && child !== node;) {
      const following = child.nextSibling;
      parent.insertBefore(child, after);
      if (!first) first = child;
      child = following;
    }
    return first || node.nextSibling;
  }
  parent.insertBefore(node, cursor);
  return cursor;
}

function morphNode(existing, incoming) {
  if (existing.nodeType === ELEMENT_NODE) {
    morphAttributes(existing, incoming);
    morphChildren(existing, incoming);
  } else if (existing.data !== incoming.data) {
    existing.data = incoming.data;
  }
}

function morphAttributes(existing, incoming) {
  const existingAttrs = existing.attributes;
  const incomingAttrs = incoming.attributes;

  for (let i = existingAttrs.length - 1; i >= 0; i--) {
    const attr = existingAttrs[i];
    // A <details> someone opened stays open: like an input's value, its
    // "open" is their state, which markup without it doesn't undo.
    if (attr.name === "open" && existing.tagName === "DETAILS") continue;
    if (!incoming.hasAttribute(attr.name)) {
      existing.removeAttribute(attr.name);
    }
  }

  for (let i = 0; i < incomingAttrs.length; i++) {
    const attr = incomingAttrs[i];
    const existingValue = existing.getAttribute(attr.name);
    if (existingValue !== attr.value) {
      existing.setAttribute(attr.name, attr.value);
    }
  }

  if (existing.tagName === "INPUT" || existing.tagName === "TEXTAREA") {
    const input = existing;
    if (document.activeElement !== input) {
      input.value = incoming.value || "";
    }
    if (input.type === "checkbox") {
      input.checked = incoming.hasAttribute("checked");
    }
  }
}

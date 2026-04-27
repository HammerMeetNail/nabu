export function morphInnerHTML(root, html) {
  const template = document.createElement("template");
  template.innerHTML = html;
  const node = template.content.firstChild;
  if (node) {
    morphChildren(root, node);
  }
}

function morphChildren(existing, incoming) {
  const existingChildren = Array.from(existing.childNodes);
  const incomingChildren = Array.from(incoming.childNodes);

  const max = Math.max(existingChildren.length, incomingChildren.length);

  for (let i = 0; i < max; i++) {
    const e = existingChildren[i];
    const n = incomingChildren[i];

    if (!n) {
      existing.removeChild(e);
      continue;
    }

    if (!e) {
      existing.appendChild(n.cloneNode(true));
      continue;
    }

    if (e.nodeType !== n.nodeType || e.nodeName !== n.nodeName) {
      existing.replaceChild(n.cloneNode(true), e);
      continue;
    }

    if (e.nodeType === Node.TEXT_NODE) {
      if (e.textContent !== n.textContent) {
        e.textContent = n.textContent;
      }
      continue;
    }

    if (e.nodeType === Node.ELEMENT_NODE) {
      morphAttributes(e, n);
      morphChildren(e, n);
    }
  }
}

function morphAttributes(existing, incoming) {
  const existingAttrs = existing.attributes;
  const incomingAttrs = incoming.attributes;

  for (let i = existingAttrs.length - 1; i >= 0; i--) {
    const attr = existingAttrs[i];
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
  }
}

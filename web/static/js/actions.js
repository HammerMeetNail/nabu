// Every data-action handler, by the event that runs it. A feature module
// registers its own when it loads; app.js's listeners run the registered
// handler before falling back to their own switch.
const registry = { click: new Map(), submit: new Map(), change: new Map() };

export function registerActions(kind, handlers) {
  const table = registry[kind];
  for (const [name, handler] of Object.entries(handlers)) {
    if (table.has(name)) throw new Error(`action "${name}" is registered twice`);
    table.set(name, handler);
  }
}

// Runs the handler registered for name, and reports whether there was one.
export function runAction(kind, name, event, element) {
  const handler = registry[kind].get(name);
  if (!handler) return false;
  handler(event, element);
  return true;
}

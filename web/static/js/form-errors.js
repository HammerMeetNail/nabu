// The error line beside a form, found by its container's selector.
export function setError(containerId, message) {
  const el = document.querySelector(containerId);
  if (!el) return;
  el.textContent = message;
  el.classList.remove("hidden");
}

export function hideError(containerId) {
  const el = document.querySelector(containerId);
  if (!el) return;
  el.classList.add("hidden");
}

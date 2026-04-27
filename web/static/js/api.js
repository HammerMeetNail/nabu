export function getCSRFToken() {
  const match = document.cookie.match(/(?:^|;\s*)choresy_csrf=([^;]*)/);
  return match ? match[1] : "";
}

export async function apiFetch(path, options = {}) {
  const headers = new Headers(options.headers || {});
  headers.set("Content-Type", "application/json");

  const csrfToken = getCSRFToken();
  if (csrfToken) {
    headers.set("X-CSRF-Token", csrfToken);
  }

  const response = await fetch(path, {
    ...options,
    headers,
  });

  let data = null;
  const contentType = response.headers.get("Content-Type");
  if (contentType && contentType.includes("application/json")) {
    try {
      data = await response.json();
    } catch {}
  }

  return { response, data };
}

export async function apiMe() {
  const { data } = await apiFetch("/api/me");
  return data;
}

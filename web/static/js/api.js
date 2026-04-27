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

  const method = options.method || "GET";
  const isStateChanging = ["POST", "PATCH", "PUT", "DELETE"].includes(method);

  const response = await fetch(path, {
    ...options,
    method,
    headers,
  });

  let data = null;
  const contentType = response.headers.get("Content-Type");
  if (contentType && contentType.includes("application/json")) {
    data = await response.json();
  }

  return { response, data };
}

export async function apiMe() {
  return apiFetch("/api/me");
}

export async function apiLogin(email, password) {
  return apiFetch("/api/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export async function apiRegister(email, password) {
  return apiFetch("/api/auth/register", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export async function apiLogout() {
  return apiFetch("/api/auth/logout", { method: "POST" });
}

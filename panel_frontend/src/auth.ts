const TOKEN_KEY = "panel_token";
const SESSION_EXPIRED_EVENT = "panel-session-expired";

export function getPanelToken() {
  return localStorage.getItem(TOKEN_KEY);
}

export function setPanelToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearPanelToken() {
  localStorage.removeItem(TOKEN_KEY);
}

function decodeJwtPayload(token: string) {
  const [, payload] = token.split(".");
  if (!payload) {
    return null;
  }

  try {
    const normalized = payload.replace(/-/g, "+").replace(/_/g, "/");
    const padded = normalized.padEnd(normalized.length + ((4 - (normalized.length % 4)) % 4), "=");
    return JSON.parse(window.atob(padded)) as { exp?: number };
  } catch {
    return null;
  }
}

export function isTokenExpired(token: string) {
  const payload = decodeJwtPayload(token);
  if (!payload?.exp) {
    return true;
  }

  return payload.exp * 1000 <= Date.now();
}

export function getTokenExpiryTime(token: string) {
  const payload = decodeJwtPayload(token);
  return payload?.exp ? payload.exp * 1000 : null;
}

export function isAuthenticated() {
  const token = getPanelToken();
  if (!token) {
    return false;
  }

  if (isTokenExpired(token)) {
    clearPanelToken();
    return false;
  }

  return true;
}

export function expirePanelSession() {
  clearPanelToken();
  window.dispatchEvent(new CustomEvent(SESSION_EXPIRED_EVENT));
}

export function redirectToLogin() {
  const currentPath = `${window.location.pathname}${window.location.search}${window.location.hash}`;
  const loginUrl = currentPath && currentPath !== "/login"
    ? `/login?from=${encodeURIComponent(currentPath)}`
    : "/login";

  if (window.location.pathname !== "/login") {
    window.location.replace(loginUrl);
  }
}

export function notifySessionExpired() {
  expirePanelSession();
  redirectToLogin();
}

export function getSessionExpiredEventName() {
  return SESSION_EXPIRED_EVENT;
}

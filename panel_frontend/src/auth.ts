const TOKEN_KEY = "panel_token";

export function getPanelToken() {
  return localStorage.getItem(TOKEN_KEY);
}

export function setPanelToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearPanelToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export function isAuthenticated() {
  return Boolean(getPanelToken());
}

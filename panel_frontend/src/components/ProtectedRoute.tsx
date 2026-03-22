import { useEffect, useState } from "react";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import {
  getPanelToken,
  getSessionExpiredEventName,
  getTokenExpiryTime,
  isAuthenticated
} from "../auth";

export function ProtectedRoute() {
  const location = useLocation();
  const [authenticated, setAuthenticated] = useState(() => isAuthenticated());

  useEffect(() => {
    setAuthenticated(isAuthenticated());

    const token = getPanelToken();
    const expiresAt = token ? getTokenExpiryTime(token) : null;
    const timeoutMs = expiresAt ? Math.max(expiresAt - Date.now(), 0) : null;
    const timeoutId =
      timeoutMs !== null
        ? window.setTimeout(() => {
            setAuthenticated(isAuthenticated());
          }, timeoutMs)
        : null;

    const handleSessionExpired = () => {
      setAuthenticated(false);
    };

    const handleStorage = (event: StorageEvent) => {
      if (event.key === "panel_token") {
        setAuthenticated(isAuthenticated());
      }
    };

    window.addEventListener(getSessionExpiredEventName(), handleSessionExpired);
    window.addEventListener("storage", handleStorage);

    return () => {
      if (timeoutId !== null) {
        window.clearTimeout(timeoutId);
      }
      window.removeEventListener(getSessionExpiredEventName(), handleSessionExpired);
      window.removeEventListener("storage", handleStorage);
    };
  }, [location.key]);

  if (!authenticated) {
    return <Navigate to="/login" replace state={{ from: `${location.pathname}${location.search}${location.hash}` }} />;
  }

  return <Outlet />;
}

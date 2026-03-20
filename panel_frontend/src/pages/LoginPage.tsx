import { FormEvent, useState } from "react";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import api from "../api/client";
import { isAuthenticated, setPanelToken } from "../auth";

type LoginResponse = {
  token: string;
};

export function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const [credentials, setCredentials] = useState({ username: "admin", password: "admin" });
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  if (isAuthenticated()) {
    return <Navigate to="/" replace />;
  }

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    setError("");

    try {
      const response = await api.post<LoginResponse>("/auth/login", credentials);
      setPanelToken(response.data.token);
      const nextPath = typeof location.state?.from === "string" ? location.state.from : "/";
      navigate(nextPath, { replace: true });
    } catch {
      setError("Login failed. Check your admin username and password.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center px-4 py-6 sm:px-6 lg:px-8">
      <div className="grid w-full max-w-6xl gap-4 lg:grid-cols-[minmax(0,1.1fr),420px]">
        <section className="panel-surface hidden overflow-hidden p-8 lg:block xl:p-10">
          <p className="metric-kicker">Sing-box Control Plane</p>
          <h1 className="mt-4 max-w-xl font-display text-5xl font-bold leading-tight text-white">
            Operate your multi-node VPN panel from one admin workspace.
          </h1>
          <p className="mt-5 max-w-2xl text-base leading-8 text-slate-400">
            Sign in to manage VPN users, provision VPS nodes, sync access across the fleet, and generate client import
            links for sing-box subscriptions.
          </p>

          <div className="mt-10 grid gap-4 sm:grid-cols-3">
            {[
              ["Users", "Create VPN users, set bandwidth limits, and enable or disable access."],
              ["Nodes", "Bootstrap VPS nodes, monitor health, and reinstall node services."],
              ["Access", "Open QR codes and import links for subscription delivery."]
            ].map(([title, detail]) => (
              <div key={title} className="panel-subtle p-4">
                <p className="text-sm font-semibold text-white">{title}</p>
                <p className="mt-2 text-sm leading-6 text-slate-400">{detail}</p>
              </div>
            ))}
          </div>
        </section>

        <div className="panel-surface w-full p-6 sm:p-8">
          <p className="metric-kicker">Admin Login</p>
          <h2 className="mt-3 font-display text-4xl font-bold text-white">Access the control panel</h2>
          <p className="mt-3 text-sm leading-6 text-slate-400">
            Sign in to manage users, nodes, subscriptions, and admin settings for this VPN deployment.
          </p>

          <form className="mt-8 space-y-5" onSubmit={(event) => void handleSubmit(event)}>
            <label className="block">
              <span className="mb-2 block text-sm font-medium text-slate-300">Username</span>
              <input
                value={credentials.username}
                onChange={(event) => setCredentials((current) => ({ ...current, username: event.target.value }))}
                className="input-shell"
                placeholder="admin"
                autoComplete="username"
              />
            </label>

            <label className="block">
              <span className="mb-2 block text-sm font-medium text-slate-300">Password</span>
              <input
                type="password"
                value={credentials.password}
                onChange={(event) => setCredentials((current) => ({ ...current, password: event.target.value }))}
                className="input-shell"
                placeholder="admin"
                autoComplete="current-password"
              />
            </label>

            {error ? <p className="rounded-2xl border border-rose-400/20 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">{error}</p> : null}

            <button
              type="submit"
              disabled={submitting}
              className="btn-primary w-full py-3"
            >
              {submitting ? "Signing In..." : "Sign In"}
            </button>
          </form>

          <div className="mt-6 panel-subtle p-4 text-sm text-slate-400 lg:hidden">
            Manage VPN users, node provisioning, and subscription delivery from one protected panel.
          </div>
        </div>
      </div>
    </div>
  );
}

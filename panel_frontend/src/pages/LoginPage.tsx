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
    <div className="flex min-h-screen items-center justify-center px-4 py-8 md:px-8">
      <div className="w-full max-w-md rounded-[2rem] border border-white/70 bg-white/95 p-8 shadow-panel backdrop-blur">
        <p className="text-sm uppercase tracking-[0.25em] text-slate-500">Sing-box Control Plane</p>
        <h1 className="mt-3 font-display text-4xl font-bold text-ink">Admin Login</h1>
        <p className="mt-3 text-sm text-slate-500">Sign in before accessing the dashboard, users, nodes, and settings pages.</p>

        <form className="mt-8 space-y-5" onSubmit={(event) => void handleSubmit(event)}>
          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-600">Username</span>
            <input
              value={credentials.username}
              onChange={(event) => setCredentials((current) => ({ ...current, username: event.target.value }))}
              className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm text-slate-900 outline-none transition focus:border-sky-400"
              placeholder="admin"
              autoComplete="username"
            />
          </label>

          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-600">Password</span>
            <input
              type="password"
              value={credentials.password}
              onChange={(event) => setCredentials((current) => ({ ...current, password: event.target.value }))}
              className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm text-slate-900 outline-none transition focus:border-sky-400"
              placeholder="admin"
              autoComplete="current-password"
            />
          </label>

          {error ? <p className="rounded-2xl bg-rose-50 px-4 py-3 text-sm text-rose-600">{error}</p> : null}

          <button
            type="submit"
            disabled={submitting}
            className="w-full rounded-2xl bg-ink px-4 py-3 text-sm font-semibold text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-70"
          >
            {submitting ? "Signing In..." : "Sign In"}
          </button>
        </form>
      </div>
    </div>
  );
}

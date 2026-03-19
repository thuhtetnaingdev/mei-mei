import { useEffect, useState } from "react";
import api from "../api/client";
import { clearPanelToken, getPanelToken, setPanelToken } from "../auth";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { SectionCard } from "../components/SectionCard";
import { useNavigate } from "react-router-dom";

export function SettingsPage() {
  const navigate = useNavigate();
  const token = getPanelToken() ?? "";
  const [username, setUsername] = useState("admin");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [status, setStatus] = useState("");
  const [logoutDialogOpen, setLogoutDialogOpen] = useState(false);

  useEffect(() => {
    void api
      .get<{ username: string }>("/admin/profile")
      .then((response) => setUsername(response.data.username))
      .catch(() => undefined);
  }, []);

  const updateCredentials = async () => {
    const response = await api.put<{ username: string }>("/admin/credentials", {
      username,
      currentPassword,
      password: newPassword
    });

    const login = await api.post<{ token: string }>("/auth/login", {
      username: response.data.username,
      password: newPassword
    });

    setPanelToken(login.data.token);
    setCurrentPassword("");
    setNewPassword("");
    setStatus("Admin credentials updated.");
  };

  const logout = () => {
    clearPanelToken();
    navigate("/login", { replace: true });
  };

  return (
    <div className="grid gap-8 xl:grid-cols-2">
      <SectionCard title="Admin Session" description="This panel only routes to protected pages after a successful login.">
        <div className="space-y-4">
          <p className="rounded-2xl bg-slate-50 p-4 text-sm text-slate-600">
            Current JWT status: {token ? "Authenticated" : "Not signed in"}.
          </p>
          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-600">Username</span>
            <input
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm"
              placeholder="admin"
            />
          </label>
          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-600">Current Password</span>
            <input
              type="password"
              value={currentPassword}
              onChange={(event) => setCurrentPassword(event.target.value)}
              className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm"
              placeholder="Current password"
            />
          </label>
          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-600">New Password</span>
            <input
              type="password"
              value={newPassword}
              onChange={(event) => setNewPassword(event.target.value)}
              className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm"
              placeholder="New password"
            />
          </label>
          {status ? <p className="text-sm text-slate-500">{status}</p> : null}
          <div className="flex flex-wrap gap-3 pt-2">
            <button onClick={() => void updateCredentials()} className="rounded-2xl bg-ink px-4 py-3 text-sm font-semibold text-white">
              Update Credentials
            </button>
            <button onClick={() => setLogoutDialogOpen(true)} className="rounded-2xl bg-ink px-4 py-3 text-sm font-semibold text-white">
              Log Out
            </button>
          </div>
        </div>
      </SectionCard>

      <SectionCard title="Subscription Notes" description="The generated subscription is a base64-encoded bundle of all registered nodes and supported protocols.">
        <p className="rounded-2xl bg-slate-50 p-4 text-sm text-slate-600">
          Generate user subscriptions from the backend endpoint `/subscription/:userId`.
        </p>
      </SectionCard>
      <ConfirmDialog
        open={logoutDialogOpen}
        title="Log out?"
        description="Your current admin session will end and the panel will redirect back to the login page."
        confirmLabel="Log Out"
        onCancel={() => setLogoutDialogOpen(false)}
        onConfirm={logout}
      />
    </div>
  );
}

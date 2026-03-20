import { useEffect, useState } from "react";
import api from "../api/client";
import { clearPanelToken, getPanelToken, setPanelToken } from "../auth";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { SectionCard } from "../components/SectionCard";
import { useNavigate } from "react-router-dom";
import type { DistributionSettings } from "../types";

export function SettingsPage() {
  const navigate = useNavigate();
  const token = getPanelToken() ?? "";
  const [username, setUsername] = useState("admin");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [distributionSettings, setDistributionSettings] = useState<DistributionSettings>({
    adminPercent: 25,
    usagePoolPercent: 55,
    reservePoolPercent: 20
  });
  const [status, setStatus] = useState("");
  const [distributionError, setDistributionError] = useState("");
  const [logoutDialogOpen, setLogoutDialogOpen] = useState(false);

  useEffect(() => {
    void Promise.all([
      api.get<{ username: string }>("/admin/profile"),
      api.get<DistributionSettings>("/settings/distribution")
    ])
      .then(([profileResponse, distributionResponse]) => {
        setUsername(profileResponse.data.username);
        setDistributionSettings(distributionResponse.data);
      })
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

  const updateDistributionSettings = async () => {
    setDistributionError("");
    const total =
      distributionSettings.adminPercent +
      distributionSettings.usagePoolPercent +
      distributionSettings.reservePoolPercent;

    if (total !== 100) {
      setDistributionError(`Distribution percentages must total 100.00, got ${total.toFixed(2)}.`);
      return;
    }

    const response = await api.put<DistributionSettings>("/settings/distribution", distributionSettings);
    setDistributionSettings(response.data);
    setStatus("Distribution settings updated.");
  };

  const logout = () => {
    clearPanelToken();
    navigate("/login", { replace: true });
  };

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1.2fr),minmax(320px,0.8fr)]">
      <SectionCard eyebrow="Admin Access" title="Session and credentials" description="Rotate the admin login without leaving the compact control surface.">
        <div className="space-y-4">
          <p className="panel-subtle p-4 text-sm text-slate-300">
            Current JWT status: {token ? "Authenticated" : "Not signed in"}.
          </p>
          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-300">Username</span>
            <input
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              className="input-shell"
              placeholder="admin"
            />
          </label>
          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-300">Current Password</span>
            <input
              type="password"
              value={currentPassword}
              onChange={(event) => setCurrentPassword(event.target.value)}
              className="input-shell"
              placeholder="Current password"
            />
          </label>
          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-300">New Password</span>
            <input
              type="password"
              value={newPassword}
              onChange={(event) => setNewPassword(event.target.value)}
              className="input-shell"
              placeholder="New password"
            />
          </label>
          {status ? <p className="text-sm text-slate-400">{status}</p> : null}
          <div className="flex flex-wrap gap-3 pt-2">
            <button onClick={() => void updateCredentials()} className="btn-primary">
              Update Credentials
            </button>
            <button onClick={() => setLogoutDialogOpen(true)} className="btn-secondary">
              Log Out
            </button>
          </div>
        </div>
      </SectionCard>

      <div className="space-y-4">
        <SectionCard eyebrow="Distribution" title="Package split settings" description="New user packages snapshot these values at creation time. Existing packages keep the split they were created with.">
          <div className="grid gap-4">
            <div className="grid gap-4 md:grid-cols-3">
              <label className="block">
                <span className="mb-2 block text-sm font-medium text-slate-300">Admin %</span>
                <input
                  type="number"
                  min={0}
                  step="0.01"
                  value={distributionSettings.adminPercent}
                  onChange={(event) =>
                    setDistributionSettings((current) => ({ ...current, adminPercent: Number(event.target.value) || 0 }))
                  }
                  className="input-shell"
                />
              </label>
              <label className="block">
                <span className="mb-2 block text-sm font-medium text-slate-300">Usage Pool %</span>
                <input
                  type="number"
                  min={0}
                  step="0.01"
                  value={distributionSettings.usagePoolPercent}
                  onChange={(event) =>
                    setDistributionSettings((current) => ({ ...current, usagePoolPercent: Number(event.target.value) || 0 }))
                  }
                  className="input-shell"
                />
              </label>
              <label className="block">
                <span className="mb-2 block text-sm font-medium text-slate-300">Reserve Pool %</span>
                <input
                  type="number"
                  min={0}
                  step="0.01"
                  value={distributionSettings.reservePoolPercent}
                  onChange={(event) =>
                    setDistributionSettings((current) => ({ ...current, reservePoolPercent: Number(event.target.value) || 0 }))
                  }
                  className="input-shell"
                />
              </label>
            </div>
            <div className="rounded-2xl border border-white/10 bg-white/[0.03] px-4 py-3 text-sm text-slate-300">
              Total: {(distributionSettings.adminPercent + distributionSettings.usagePoolPercent + distributionSettings.reservePoolPercent).toFixed(2)}%
            </div>
            {distributionError ? <p className="text-sm text-rose-300">{distributionError}</p> : null}
            <div className="flex justify-end">
              <button onClick={() => void updateDistributionSettings()} className="btn-primary">
                Save Distribution
              </button>
            </div>
          </div>
        </SectionCard>
      </div>
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

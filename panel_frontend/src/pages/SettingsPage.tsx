import { Plus, Sparkles, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import api from "../api/client";
import { clearPanelToken, getPanelToken, setPanelToken } from "../auth";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { SectionCard } from "../components/SectionCard";
import { useNavigate } from "react-router-dom";
import type { DistributionSettings, ProtocolSettings, ProtocolSettingsUpdateResponse } from "../types";

function getRequestErrorMessage(error: unknown, fallback: string) {
  if (typeof error === "object" && error !== null) {
    const response = (error as { response?: { data?: { error?: unknown } } }).response;
    if (typeof response?.data?.error === "string" && response.data.error.trim()) {
      return response.data.error;
    }
  }

  if (error instanceof Error && error.message.trim()) {
    return error.message;
  }

  return fallback;
}

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
  const [protocolSettings, setProtocolSettings] = useState<ProtocolSettings>({
    realitySnis: ["www.cloudflare.com"],
    hysteria2Masquerades: []
  });
  const [status, setStatus] = useState("");
  const [protocolStatus, setProtocolStatus] = useState("");
  const [protocolError, setProtocolError] = useState("");
  const [distributionError, setDistributionError] = useState("");
  const [logoutDialogOpen, setLogoutDialogOpen] = useState(false);

  useEffect(() => {
    void Promise.all([
      api.get<{ username: string }>("/admin/profile"),
      api.get<DistributionSettings>("/settings/distribution"),
      api.get<ProtocolSettings>("/settings/protocols")
    ])
      .then(([profileResponse, distributionResponse, protocolResponse]) => {
        setUsername(profileResponse.data.username);
        setDistributionSettings(distributionResponse.data);
        setProtocolSettings({
          realitySnis: protocolResponse.data.realitySnis.length ? protocolResponse.data.realitySnis : [""],
          hysteria2Masquerades: protocolResponse.data.hysteria2Masquerades
        });
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

  const updateProtocolSettings = async () => {
    setProtocolError("");
    setProtocolStatus("");

    try {
      const payload: ProtocolSettings = {
        realitySnis: protocolSettings.realitySnis.map((value) => value.trim()).filter(Boolean),
        hysteria2Masquerades: protocolSettings.hysteria2Masquerades.map((value) => value.trim()).filter(Boolean)
      };

      const response = await api.put<ProtocolSettingsUpdateResponse>("/settings/protocols", payload);
      setProtocolSettings({
        realitySnis: response.data.realitySnis.length ? response.data.realitySnis : [""],
        hysteria2Masquerades: response.data.hysteria2Masquerades
      });
      setProtocolStatus(
        response.data.syncError
          ? `Protocol settings updated. ${response.data.syncError}`
          : `Protocol settings updated and synced to ${response.data.syncedNodes} node${response.data.syncedNodes === 1 ? "" : "s"}.`
      );
    } catch (error) {
      setProtocolError(getRequestErrorMessage(error, "Protocol settings update failed"));
    }
  };

  const updateListValue = (key: keyof ProtocolSettings, index: number, value: string) => {
    setProtocolSettings((current) => ({
      ...current,
      [key]: current[key].map((item, itemIndex) => (itemIndex === index ? value : item))
    }));
  };

  const addListValue = (key: keyof ProtocolSettings, value = "") => {
    setProtocolSettings((current) => ({
      ...current,
      [key]: [...current[key], value]
    }));
  };

  const removeListValue = (key: keyof ProtocolSettings, index: number) => {
    setProtocolSettings((current) => {
      const nextValues = current[key].filter((_, itemIndex) => itemIndex !== index);
      return {
        ...current,
        [key]: key === "realitySnis" && nextValues.length === 0 ? [""] : nextValues
      };
    });
  };

  const logout = () => {
    clearPanelToken();
    navigate("/login", { replace: true });
  };

  return (
    <div className="space-y-4">
      <SectionCard
        eyebrow="Transport Profiles"
        title="Reality and Hysteria2 routing"
        description="Manage the ordered SNI list for Reality and the reverse-proxy target URLs used by Hysteria2 masquerade. Saving here regenerates the extra inbounds on every node and pushes the update back automatically."
        action={
          <button onClick={() => void updateProtocolSettings()} className="btn-primary mt-1 gap-2 px-4 py-2">
            <Sparkles className="h-4 w-4" />
            Save and Sync
          </button>
        }
      >
        <div className="grid gap-4 xl:grid-cols-2">
          <div className="rounded-3xl border border-cyan-400/20 bg-cyan-400/5 p-4">
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="metric-kicker text-cyan-200">Reality</p>
                <h3 className="mt-2 font-display text-xl font-semibold text-white">SNI list</h3>
                <p className="mt-2 text-sm leading-6 text-slate-300">
                  Each SNI creates one `vless+reality` inbound per node on its own stable randomized port.
                </p>
              </div>
              <button type="button" onClick={() => addListValue("realitySnis")} className="btn-secondary shrink-0 gap-1.5 px-3 py-2 text-sm">
                <Plus className="h-3.5 w-3.5" />
                Add
              </button>
            </div>
            <div className="mt-4 space-y-3">
              {protocolSettings.realitySnis.map((value, index) => (
                <div key={`reality-${index}`} className="flex items-center gap-3 rounded-2xl border border-white/10 bg-slate-950/30 p-3">
                  <div className="flex h-9 w-9 items-center justify-center rounded-2xl bg-cyan-400/10 text-sm font-semibold text-cyan-200">
                    {index + 1}
                  </div>
                  <input
                    value={value}
                    onChange={(event) => updateListValue("realitySnis", index, event.target.value)}
                    className="input-shell flex-1"
                    placeholder="www.cloudflare.com"
                  />
                  <button
                    type="button"
                    onClick={() => removeListValue("realitySnis", index)}
                    className="rounded-2xl border border-rose-400/20 bg-rose-400/10 p-2.5 text-rose-200 transition hover:border-rose-300/40 hover:bg-rose-400/20"
                    aria-label={`Remove Reality SNI ${index + 1}`}
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              ))}
            </div>
          </div>

          <div className="rounded-3xl border border-emerald-400/20 bg-emerald-400/5 p-4">
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="metric-kicker text-emerald-200">Hysteria2</p>
                <h3 className="mt-2 font-display text-xl font-semibold text-white">Reverse proxy list</h3>
                <p className="mt-2 text-sm leading-6 text-slate-300">
                  Each target host or URL creates one Hysteria2 inbound with its own masquerade target and a generated Salamander obfuscation password.
                </p>
              </div>
              <button type="button" onClick={() => addListValue("hysteria2Masquerades")} className="btn-secondary shrink-0 gap-1.5 px-3 py-2 text-sm">
                <Plus className="h-3.5 w-3.5" />
                Add
              </button>
            </div>
            <div className="mt-4 space-y-3">
              {protocolSettings.hysteria2Masquerades.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-white/10 px-4 py-5 text-sm text-slate-400">
                  No reverse proxy targets yet. Add one or more hostnames like `www.cloudflare.com` or full `https://...` URLs to create Hysteria2 inbounds with backend-assigned randomized ports.
                </div>
              ) : null}
              {protocolSettings.hysteria2Masquerades.map((value, index) => (
                <div key={`hysteria2-${index}`} className="flex items-center gap-3 rounded-2xl border border-white/10 bg-slate-950/30 p-3">
                  <div className="flex h-9 w-9 items-center justify-center rounded-2xl bg-emerald-400/10 text-sm font-semibold text-emerald-200">
                    {index + 1}
                  </div>
                  <input
                    value={value}
                    onChange={(event) => updateListValue("hysteria2Masquerades", index, event.target.value)}
                    className="input-shell flex-1"
                    placeholder="www.cloudflare.com or https://news.ycombinator.com"
                  />
                  <button
                    type="button"
                    onClick={() => removeListValue("hysteria2Masquerades", index)}
                    className="rounded-2xl border border-rose-400/20 bg-rose-400/10 p-2.5 text-rose-200 transition hover:border-rose-300/40 hover:bg-rose-400/20"
                    aria-label={`Remove Hysteria2 reverse proxy ${index + 1}`}
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              ))}
            </div>
          </div>
        </div>
        <div className="mt-4 grid gap-3 md:grid-cols-3">
          <div className="rounded-2xl border border-white/10 bg-white/[0.03] px-4 py-3 text-sm text-slate-300">
            Reality inbounds per node: {protocolSettings.realitySnis.map((value) => value.trim()).filter(Boolean).length}
          </div>
          <div className="rounded-2xl border border-white/10 bg-white/[0.03] px-4 py-3 text-sm text-slate-300">
            Hysteria2 inbounds per node: {protocolSettings.hysteria2Masquerades.map((value) => value.trim()).filter(Boolean).length}
          </div>
          <div className="rounded-2xl border border-white/10 bg-white/[0.03] px-4 py-3 text-sm text-slate-300">
            Ports are backend-assigned, stable, and randomized per generated inbound.
          </div>
        </div>
        {protocolError ? <p className="mt-4 text-sm text-rose-300">{protocolError}</p> : null}
        {protocolStatus ? <p className="mt-4 text-sm text-slate-300">{protocolStatus}</p> : null}
      </SectionCard>

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

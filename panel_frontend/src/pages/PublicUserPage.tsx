import { Copy, Download, Link2, QrCode, Shield, CheckCircle, AlertCircle, Clock } from "lucide-react";
import { useEffect, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import QRCode from "qrcode";
import api from "../api/client";
import type { PublicUserResponse } from "../types";

const formatDate = (value?: string | null) => {
  if (!value) {
    return "No expiry";
  }
  return new Date(value).toLocaleString();
};

const formatBandwidthBytes = (bytes: number) => `${(bytes / (1024 ** 3)).toFixed(bytes >= 1024 ** 3 ? 1 : 2)} GB`;

const formatPercentage = (value: number) => `${value >= 10 || Number.isInteger(value) ? value.toFixed(0) : value.toFixed(1)}%`;

export function PublicUserPage() {
  const { uuid } = useParams<{ uuid: string }>();
  const [searchParams] = useSearchParams();
  const [user, setUser] = useState<PublicUserResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [qrCodeUrl, setQrCodeUrl] = useState("");
  const [qrLoading, setQrLoading] = useState(false);
  const [copiedKey, setCopiedKey] = useState("");
  const [showQrDialog, setShowQrDialog] = useState(false);
  const [qrTitle, setQrTitle] = useState("Sing-Box");

  const loadUser = async (userUuid: string) => {
    setLoading(true);
    setError("");
    try {
      const response = await api.get<PublicUserResponse>(`/api/public/users/${userUuid}`);
      setUser(response.data);
      
      // Generate QR code for the sing-box remote import URI
      const importUrl = response.data.singboxImportUrl || response.data.singboxProfileUrl;
      if (importUrl) {
        setQrLoading(true);
        const qr = await QRCode.toDataURL(importUrl, {
          width: 280,
          margin: 1
        });
        setQrCodeUrl(qr);
        setQrLoading(false);
      }
    } catch (err) {
      console.error("Failed to load user:", err);
      setError("We could not load this subscription. Please check the URL and try again.");
      setUser(null);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (uuid) {
      void loadUser(uuid);
    } else {
      // Try to get UUID from query param as fallback
      const queryUuid = searchParams.get("uuid");
      if (queryUuid) {
        void loadUser(queryUuid);
      } else {
        setError("No user ID provided. Please check your subscription link.");
        setLoading(false);
      }
    }
  }, [uuid, searchParams]);

  const copyText = async (value: string, key: string) => {
    if (!value) {
      return;
    }

    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(value);
      } else {
        const textarea = document.createElement("textarea");
        textarea.value = value;
        textarea.style.position = "fixed";
        textarea.style.left = "-9999px";
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand("copy");
        document.body.removeChild(textarea);
      }
      setCopiedKey(key);
      setTimeout(() => setCopiedKey(""), 2000);
    } catch {
      // Silent fail for copy
    }
  };

  const openQrDialog = async (value: string, title: string) => {
    if (!value) {
      return;
    }

    setQrTitle(title);
    setShowQrDialog(true);
    setQrLoading(true);
    try {
      const qr = await QRCode.toDataURL(value, {
        width: 280,
        margin: 1
      });
      setQrCodeUrl(qr);
    } finally {
      setQrLoading(false);
    }
  };

  const downloadProfile = async (format: "singbox" | "clash") => {
    const url = format === "clash" ? user?.clashProfileUrl : user?.singboxProfileUrl;
    if (!url) {
      return;
    }

    try {
      const response = await fetch(url);
      const content = await response.text();
      const blob = new Blob([content], { type: "text/plain" });
      const blobUrl = window.URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = blobUrl;
      link.download = `${user?.email || "profile"}.${format === "clash" ? "yaml" : "json"}`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(blobUrl);
    } catch {
      // Silent fail for download
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-slate-950 flex items-center justify-center p-4">
        <div className="text-center">
          <div className="mx-auto h-12 w-12 animate-spin rounded-full border-4 border-sky-400/20 border-t-sky-400" />
          <p className="mt-4 text-slate-400">Loading subscription...</p>
        </div>
      </div>
    );
  }

  if (error || !user) {
    return (
      <div className="min-h-screen bg-slate-950 flex items-center justify-center p-4">
        <div className="max-w-md w-full panel-surface rounded-3xl p-6 text-center">
          <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-rose-500/15 text-rose-400">
            <AlertCircle className="h-8 w-8" />
          </div>
          <h2 className="mt-4 font-display text-xl font-bold text-white">Subscription Not Found</h2>
          <p className="mt-2 text-slate-400">{error || "The requested subscription could not be found."}</p>
          <p className="mt-4 text-sm text-slate-500">
            Please check your subscription link or contact your administrator.
          </p>
        </div>
      </div>
    );
  }

  const usagePercentage = user.bandwidthLimitGb > 0 
    ? (user.bandwidthUsedBytes / (user.bandwidthLimitGb * 1024 ** 3)) * 100 
    : 0;

  const remainingBandwidthBytes = Math.max(
    (user.bandwidthLimitGb * 1024 ** 3) - user.bandwidthUsedBytes,
    0
  );

  return (
    <div className="min-h-screen bg-slate-950">
      {/* Header */}
      <header className="border-b border-white/10 bg-slate-900/50 backdrop-blur-sm">
        <div className="mx-auto max-w-5xl px-4 py-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-sky-400/15 text-sky-300">
                <Shield className="h-5 w-5" />
              </div>
              <div>
                <p className="text-xs font-semibold text-slate-400">MeiMei VPN</p>
                <h1 className="font-display text-lg font-bold text-white">Your Subscription</h1>
              </div>
            </div>
            <div className={`flex items-center gap-2 rounded-full px-3 py-1 text-xs font-semibold ${
              user.enabled 
                ? "bg-emerald-500/15 text-emerald-400" 
                : "bg-rose-500/15 text-rose-400"
            }`}>
              {user.enabled ? (
                <>
                  <CheckCircle className="h-3.5 w-3.5" />
                  <span>Active</span>
                </>
              ) : (
                <>
                  <AlertCircle className="h-3.5 w-3.5" />
                  <span>Inactive</span>
                </>
              )}
            </div>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-5xl px-4 py-6 sm:px-6 lg:px-8">
        <div className="space-y-4">
          {/* User Info Card */}
          <section className="panel-surface rounded-3xl p-4 sm:p-6">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <h2 className="font-display text-xl font-bold text-white">{user.email}</h2>
                  {user.isTesting && (
                    <span className="rounded-full bg-violet-500/15 px-2.5 py-0.5 text-xs font-semibold text-violet-400">
                      Test Account
                    </span>
                  )}
                </div>
                <p className="mt-1 text-sm text-slate-400">
                  UUID: <code className="rounded bg-white/5 px-1.5 py-0.5 font-mono text-xs text-slate-300">{user.uuid}</code>
                </p>
                <div className="mt-3 flex flex-wrap items-center gap-4 text-sm text-slate-400">
                  <div className="flex items-center gap-1.5">
                    <Clock className="h-4 w-4" />
                    <span>Expires: {formatDate(user.expiresAt)}</span>
                  </div>
                </div>
              </div>
            </div>
          </section>

          {/* Bandwidth Usage Card */}
          <section className="panel-surface rounded-3xl p-4 sm:p-6">
            <h3 className="font-display text-lg font-bold text-white">Bandwidth Usage</h3>
            <div className="mt-4 grid gap-4 sm:grid-cols-3">
              <div className="rounded-2xl border border-white/10 bg-slate-950/35 p-4">
                <p className="text-xs font-semibold text-slate-500">Total Allowance</p>
                <p className="mt-1 font-display text-2xl font-bold text-white">
                  {user.bandwidthLimitGb > 0 ? formatBandwidthBytes(user.bandwidthLimitGb * 1024 ** 3) : "Unlimited"}
                </p>
              </div>
              <div className="rounded-2xl border border-white/10 bg-slate-950/35 p-4">
                <p className="text-xs font-semibold text-slate-500">Used</p>
                <p className="mt-1 font-display text-2xl font-bold text-amber-300">
                  {formatBandwidthBytes(user.bandwidthUsedBytes)}
                </p>
              </div>
              <div className="rounded-2xl border border-white/10 bg-slate-950/35 p-4">
                <p className="text-xs font-semibold text-slate-500">Remaining</p>
                <p className="mt-1 font-display text-2xl font-bold text-emerald-300">
                  {user.bandwidthLimitGb > 0 ? formatBandwidthBytes(remainingBandwidthBytes) : "Unlimited"}
                </p>
              </div>
            </div>
            
            {user.bandwidthLimitGb > 0 && (
              <div className="mt-4">
                <div className="flex items-center justify-between text-sm">
                  <span className="text-slate-400">Usage Progress</span>
                  <span className="font-semibold text-slate-300">{formatPercentage(usagePercentage)}</span>
                </div>
                <div className="mt-2 h-3 overflow-hidden rounded-full bg-white/10">
                  <div 
                    className={`h-full rounded-full transition-all ${
                      usagePercentage >= 90 ? "bg-rose-400" : 
                      usagePercentage >= 70 ? "bg-amber-400" : 
                      "bg-emerald-400"
                    }`}
                    style={{ width: `${Math.min(usagePercentage, 100)}%` }}
                  />
                </div>
              </div>
            )}
          </section>

          {/* Subscription Links Card */}
          <section className="panel-surface rounded-3xl p-4 sm:p-6">
            <h3 className="font-display text-lg font-bold text-white">Subscription Links</h3>
            <p className="mt-1 text-sm text-slate-400">
              Use these links to import your subscription into your VPN client.
            </p>
            
            <div className="mt-4 space-y-3">
              {/* Sing-Box Import URL */}
              <div className="rounded-2xl border border-white/10 bg-slate-950/35 p-3">
                <div className="flex items-center justify-between">
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-semibold text-white">Sing-Box Import URL</p>
                    <p className="mt-0.5 truncate text-xs text-slate-400">{user.singboxImportUrl || user.singboxProfileUrl}</p>
                  </div>
                  <div className="ml-3 flex items-center gap-2">
                    <button
                      type="button"
                      onClick={() => copyText(user.singboxImportUrl || user.singboxProfileUrl || "", "singbox-import")}
                      className="rounded-lg bg-white/10 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-white/15"
                    >
                      {copiedKey === "singbox-import" ? "Copied!" : "Copy"}
                    </button>
                    <button
                      type="button"
                      onClick={() => void openQrDialog(user.singboxImportUrl || user.singboxProfileUrl || "", "Sing-Box")}
                      className="rounded-lg bg-sky-500/15 px-3 py-1.5 text-xs font-semibold text-sky-400 transition hover:bg-sky-500/25"
                    >
                      <QrCode className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              </div>

              <div className="rounded-2xl border border-white/10 bg-slate-950/35 p-3">
                <div className="flex items-center justify-between">
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-semibold text-white">Hiddify Import URL</p>
                    <p className="mt-0.5 truncate text-xs text-slate-400">{user.hiddifyImportUrl || user.singboxProfileUrl}</p>
                  </div>
                  <div className="ml-3 flex items-center gap-2">
                    <button
                      type="button"
                      onClick={() => copyText(user.hiddifyImportUrl || user.singboxProfileUrl || "", "hiddify-import")}
                      className="rounded-lg bg-white/10 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-white/15"
                    >
                      {copiedKey === "hiddify-import" ? "Copied!" : "Copy"}
                    </button>
                    <button
                      type="button"
                      onClick={() => void openQrDialog(user.hiddifyImportUrl || user.singboxProfileUrl || "", "Hiddify")}
                      className="rounded-lg bg-emerald-500/15 px-3 py-1.5 text-xs font-semibold text-emerald-400 transition hover:bg-emerald-500/25"
                    >
                      <QrCode className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              </div>

              {/* Clash Profile URL */}
              <div className="rounded-2xl border border-white/10 bg-slate-950/35 p-3">
                <div className="flex items-center justify-between">
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-semibold text-white">Clash Profile URL</p>
                    <p className="mt-0.5 truncate text-xs text-slate-400">{user.clashProfileUrl}</p>
                  </div>
                  <div className="ml-3 flex items-center gap-2">
                    <button
                      type="button"
                      onClick={() => copyText(user.clashProfileUrl || "", "clash-url")}
                      className="rounded-lg bg-white/10 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-white/15"
                    >
                      {copiedKey === "clash-url" ? "Copied!" : "Copy"}
                    </button>
                    <button
                      type="button"
                      onClick={() => downloadProfile("clash")}
                      className="rounded-lg bg-violet-500/15 px-3 py-1.5 text-xs font-semibold text-violet-400 transition hover:bg-violet-500/25"
                    >
                      <Download className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </section>

          {/* Quick Actions */}
          <section className="panel-surface rounded-3xl p-4 sm:p-6">
            <h3 className="font-display text-lg font-bold text-white">Quick Actions</h3>
            <div className="mt-4 grid gap-3 sm:grid-cols-3">
              <a
                href={user.singboxImportUrl || user.singboxProfileUrl || "#"}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center justify-center gap-2 rounded-2xl bg-sky-500/15 px-4 py-3 text-sm font-semibold text-sky-400 transition hover:bg-sky-500/25"
              >
                <Link2 className="h-4 w-4" />
                <span>Open Sing-Box Profile</span>
              </a>
              <a
                href={user.clashProfileUrl || "#"}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center justify-center gap-2 rounded-2xl bg-violet-500/15 px-4 py-3 text-sm font-semibold text-violet-400 transition hover:bg-violet-500/25"
              >
                <Link2 className="h-4 w-4" />
                <span>Open Clash Profile</span>
              </a>
              <a
                href={user.hiddifyImportUrl || user.singboxProfileUrl || "#"}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center justify-center gap-2 rounded-2xl bg-emerald-500/15 px-4 py-3 text-sm font-semibold text-emerald-400 transition hover:bg-emerald-500/25"
              >
                <Link2 className="h-4 w-4" />
                <span>Open Hiddify Import</span>
              </a>
            </div>
          </section>

          {/* Help Text */}
          <section className="rounded-3xl border border-white/10 bg-slate-900/50 p-4 sm:p-6">
            <h4 className="font-display text-sm font-bold text-white">How to use your subscription</h4>
            <div className="mt-3 space-y-2 text-sm text-slate-400">
              <p>
                <strong className="text-slate-300">For Sing-Box:</strong> Copy the Sing-Box Import URL and use the "Import Remote Profile" feature in your Sing-Box client.
              </p>
              <p>
                <strong className="text-slate-300">For Hiddify:</strong> Open the Hiddify Import URL to import the same remote JSON profile into Hiddify.
              </p>
              <p>
                <strong className="text-slate-300">For Clash:</strong> Copy the Clash Profile URL and import it into your Clash client, or download the YAML file directly.
              </p>
              <p>
                <strong className="text-slate-300">QR Code:</strong> Scan the QR code with your mobile device to quickly import the subscription into Sing-Box mobile apps.
              </p>
            </div>
          </section>
        </div>
      </main>

      {/* QR Code Dialog */}
      {showQrDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
          <div className="panel-surface max-w-sm w-full rounded-3xl p-6">
            <div className="flex items-center justify-between">
              <h3 className="font-display text-lg font-bold text-white">Scan to Import</h3>
              <button
                type="button"
                onClick={() => setShowQrDialog(false)}
                className="rounded-lg p-2 text-slate-400 transition hover:bg-white/10 hover:text-white"
              >
                <svg className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div className="mt-4 flex justify-center">
              {qrLoading ? (
                <div className="h-64 w-64 flex items-center justify-center">
                  <div className="h-12 w-12 animate-spin rounded-full border-4 border-sky-400/20 border-t-sky-400" />
                </div>
              ) : qrCodeUrl ? (
                <img src={qrCodeUrl} alt="Subscription QR Code" className="h-64 w-64 rounded-xl border border-white/10" />
              ) : null}
            </div>
            <p className="mt-4 text-center text-sm text-slate-400">
              Scan this QR code with your {qrTitle} mobile app to import your subscription.
            </p>
          </div>
        </div>
      )}
    </div>
  );
}

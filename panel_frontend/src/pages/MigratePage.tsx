import { ArrowRightLeft, CheckCircle2, ExternalLink, Link2, RefreshCw } from "lucide-react";
import { FormEvent, useEffect, useState } from "react";
import axios from "axios";
import api from "../api/client";
import { SectionCard } from "../components/SectionCard";
import type { User } from "../types";

interface MigrationImportResult {
  user: User;
  username: string;
  subscriptionUrl: string;
  uploadBytes: number;
  downloadBytes: number;
  usedBytes: number;
  totalBytes: number;
  remainingBytes: number;
  expiresAt?: string | null;
  bandwidthLimitGb: number;
  enabled: boolean;
}

const defaultSubscriptionUrl = "";
const bytesPerGB = 1024 ** 3;

const formatBytes = (bytes: number) => {
  if (bytes >= bytesPerGB) {
    return `${(bytes / bytesPerGB).toFixed(2)} GB`;
  }
  if (bytes >= 1024 ** 2) {
    return `${(bytes / (1024 ** 2)).toFixed(2)} MB`;
  }
  return `${bytes.toLocaleString()} B`;
};

const formatDate = (value?: string | null) => {
  if (!value) {
    return "No expiry";
  }
  return new Date(value).toLocaleString();
};

const extractApiError = (error: unknown, fallback: string) => {
  if (axios.isAxiosError(error)) {
    const backendMessage =
      typeof error.response?.data?.error === "string" ? error.response.data.error : "";
    return backendMessage || error.message || fallback;
  }
  return fallback;
};

export function MigratePage() {
  const [subscriptionUrl, setSubscriptionUrl] = useState(defaultSubscriptionUrl);
  const [loading, setLoading] = useState(false);
  const [mapLoading, setMapLoading] = useState(false);
  const [error, setError] = useState("");
  const [mapError, setMapError] = useState("");
  const [result, setResult] = useState<MigrationImportResult | null>(null);
  const [userMap, setUserMap] = useState<Record<string, number>>({});

  const loadMap = async () => {
    setMapLoading(true);
    setMapError("");
    try {
      const mapToken = import.meta.env.VITE_MAP_TOKEN as string | undefined;
      const headers: Record<string, string> = {};
      if (mapToken) {
        headers.Authorization = `Bearer ${mapToken}`;
      }
      const response = await api.get<Record<string, number>>("/public/migration-map", { headers });
      setUserMap(response.data);
    } catch (err) {
      setMapError(extractApiError(err, "Could not load migration map."));
    } finally {
      setMapLoading(false);
    }
  };

  useEffect(() => {
    void loadMap();
  }, []);

  const submitMigration = async (event: FormEvent) => {
    event.preventDefault();
    if (loading) {
      return;
    }

    setLoading(true);
    setError("");
    setResult(null);
    try {
      const response = await api.post<MigrationImportResult>("/migrate/subscription", {
        subscriptionUrl
      });
      setResult(response.data);
      await loadMap();
    } catch (err) {
      setError(extractApiError(err, "Could not import this subscription."));
    } finally {
      setLoading(false);
    }
  };

  const mapEntries = Object.entries(userMap).sort(([left], [right]) => left.localeCompare(right));
  const publicMapUrl = `${window.location.origin}/api/map`;

  return (
    <div className="space-y-4">
      <section className="panel-surface p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p className="metric-kicker">Subscription Migration</p>
            <h3 className="mt-1.5 font-display text-xl font-semibold text-white">Import existing usage</h3>
          </div>
          <a href={publicMapUrl} target="_blank" rel="noreferrer" className="btn-secondary gap-2">
            <ExternalLink className="h-4 w-4" />
            Map API
          </a>
        </div>
      </section>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr),minmax(360px,0.85fr)]">
        <SectionCard
          eyebrow="Import"
          title="Subscription URL"
          description="The importer reads Subscription-Userinfo usage headers and creates a matching local account."
        >
          <form className="space-y-4" onSubmit={(event) => void submitMigration(event)}>
            {error ? (
              <div className="rounded-2xl border border-rose-400/20 bg-rose-400/10 px-4 py-3 text-sm text-rose-200">
                {error}
              </div>
            ) : null}

            <label className="block">
              <span className="mb-2 block text-sm font-medium text-slate-300">Subscription URL</span>
              <div className="flex min-w-0 gap-2">
                <span className="hidden h-12 w-12 shrink-0 items-center justify-center rounded-2xl border border-white/10 bg-white/[0.04] text-slate-400 sm:flex">
                  <Link2 className="h-4 w-4" />
                </span>
                <input
                  type="url"
                  required
                  value={subscriptionUrl}
                  onChange={(event) => setSubscriptionUrl(event.target.value)}
                  className="input-shell min-w-0"
                  placeholder="https://example.com:2096/sub/username?format=json"
                />
              </div>
            </label>

            <div className="flex flex-wrap justify-end gap-3">
              <button type="submit" disabled={loading} className="btn-primary disabled:cursor-not-allowed disabled:opacity-70">
                {loading ? (
                  <span className="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
                ) : (
                  <ArrowRightLeft className="h-4 w-4" />
                )}
                {loading ? "Importing..." : "Import Subscription"}
              </button>
            </div>
          </form>

          {result ? (
            <div className="mt-5 rounded-[22px] border border-emerald-400/20 bg-emerald-400/10 p-4">
              <div className="flex flex-wrap items-start gap-3">
                <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-emerald-300" />
                <div className="min-w-0">
                  <p className="font-semibold text-emerald-100">Imported {result.username}</p>
                  <p className="mt-1 break-all font-mono text-xs text-emerald-100/70">{result.user.uuid}</p>
                </div>
              </div>

              <div className="mt-4 grid gap-3 sm:grid-cols-2">
                <div className="rounded-xl bg-slate-950/35 px-3 py-3">
                  <p className="metric-kicker">Used</p>
                  <p className="mt-1 text-lg font-semibold text-white">{formatBytes(result.usedBytes)}</p>
                  <p className="text-xs text-slate-500">
                    Up {formatBytes(result.uploadBytes)} / Down {formatBytes(result.downloadBytes)}
                  </p>
                </div>
                <div className="rounded-xl bg-slate-950/35 px-3 py-3">
                  <p className="metric-kicker">Total</p>
                  <p className="mt-1 text-lg font-semibold text-white">{formatBytes(result.totalBytes)}</p>
                  <p className="text-xs text-slate-500">{result.bandwidthLimitGb} GB panel limit</p>
                </div>
                <div className="rounded-xl bg-slate-950/35 px-3 py-3">
                  <p className="metric-kicker">Remaining</p>
                  <p className="mt-1 text-lg font-semibold text-white">{formatBytes(result.remainingBytes)}</p>
                  <p className="text-xs text-slate-500">{result.enabled ? "Enabled for sync" : "Disabled after import"}</p>
                </div>
                <div className="rounded-xl bg-slate-950/35 px-3 py-3">
                  <p className="metric-kicker">Expiry</p>
                  <p className="mt-1 break-words text-lg font-semibold text-white">{formatDate(result.expiresAt)}</p>
                </div>
              </div>
            </div>
          ) : null}
        </SectionCard>

        <SectionCard
          eyebrow="Public"
          title="Username map"
          description="This no-auth endpoint returns username-to-user-id pairs for migration clients."
          action={
            <button type="button" onClick={() => void loadMap()} disabled={mapLoading} className="btn-secondary gap-2 disabled:opacity-60">
              <RefreshCw className={`h-4 w-4 ${mapLoading ? "animate-spin" : ""}`} />
              Refresh
            </button>
          }
        >
          <div className="space-y-4">
            <div className="rounded-2xl border border-white/10 bg-white/[0.03] px-4 py-3">
              <p className="metric-kicker">Endpoint</p>
              <p className="mt-2 break-all font-mono text-xs text-slate-300">{publicMapUrl}</p>
            </div>

            {mapError ? (
              <div className="rounded-2xl border border-rose-400/20 bg-rose-400/10 px-4 py-3 text-sm text-rose-200">
                {mapError}
              </div>
            ) : null}

            <div className="max-h-[420px] overflow-y-auto rounded-2xl border border-white/10 bg-slate-950/35">
              {mapEntries.length ? (
                <div className="divide-y divide-white/10">
                  {mapEntries.map(([username, userId]) => (
                    <div key={username} className="grid grid-cols-[minmax(0,1fr),auto] gap-3 px-4 py-3 text-sm">
                      <span className="min-w-0 break-words text-slate-200">{username}</span>
                      <span className="font-mono text-sky-200">{userId}</span>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="px-4 py-8 text-center text-sm text-slate-500">
                  {mapLoading ? "Loading map..." : "No users mapped yet."}
                </div>
              )}
            </div>
          </div>
        </SectionCard>
      </div>
    </div>
  );
}

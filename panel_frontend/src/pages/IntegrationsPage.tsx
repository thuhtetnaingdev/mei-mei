import { ChevronDown, ChevronRight, Loader2, Plus, Trash2, Upload, CheckCircle2 } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import axios from "axios";
import api from "../api/client";

interface SubscriptionIntegration {
  id: number;
  subscriptionUrl: string;
  workingCount: number;
  totalCount: number;
  status: string;
  errorMessage?: string;
  lastTestStartedAt?: string;
  lastTestCompletedAt?: string;
  nextTestAt?: string;
  createdAt: string;
  updatedAt: string;
}

interface IntegrationDetail {
  id: number;
  subscriptionUrl: string;
  workingCount: number;
  totalCount: number;
  failCount: number;
  status: string;
  working: Array<Record<string, unknown>>;
  page: number;
  pageSize: number;
  totalPages: number;
  lastTestStartedAt?: string;
  lastTestCompletedAt?: string;
  nextTestAt?: string;
  createdAt: string;
  updatedAt: string;
}

interface IntegrationsResponse {
  integrations: SubscriptionIntegration[];
}

const extractApiError = (error: unknown, fallback: string) => {
  if (axios.isAxiosError(error)) {
    const msg = typeof error.response?.data?.error === "string" ? error.response.data.error : "";
    return msg || error.message || fallback;
  }
  return fallback;
};

const statusConfig: Record<string, { color: string; label: string }> = {
  pending: { color: "bg-amber-400", label: "Pending" },
  testing: { color: "bg-blue-400 animate-pulse", label: "Testing" },
  completed: { color: "bg-emerald-400", label: "Completed" },
  failed: { color: "bg-rose-400", label: "Failed" },
};

const fmtRelative = (iso: string | undefined): string => {
  if (!iso) return "";
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  return `${days}d ago`;
};

const fmtRelativeFuture = (iso: string | undefined): string => {
  if (!iso) return "";
  const diff = new Date(iso).getTime() - Date.now();
  if (diff < 0) return "soon";
  const mins = Math.floor(diff / 60000);
  if (mins < 60) return `in ${mins}m`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `in ${hrs}h`;
  const days = Math.floor(hrs / 24);
  return `in ${days}d`;
};

export function IntegrationsPage() {
  const [integrations, setIntegrations] = useState<SubscriptionIntegration[]>([]);
  const [loading, setLoading] = useState(true);
  const [subUrl, setSubUrl] = useState("");
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState("");
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [showInput, setShowInput] = useState(false);
  const [detail, setDetail] = useState<IntegrationDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailPage, setDetailPage] = useState(1);
  const pageSize = 20;

  const loadIntegrations = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.get<IntegrationsResponse>("/integration");
      setIntegrations(res.data.integrations);
    } catch {
      // silent
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadIntegrations();
  }, [loadIntegrations]);

  const loadDetail = async (id: number, page: number) => {
    setDetailLoading(true);
    try {
      const res = await api.get<IntegrationDetail>(`/integration/${id}?page=${page}&pageSize=${pageSize}`);
      setDetail(res.data);
    } catch {
      setDetail(null);
    } finally {
      setDetailLoading(false);
    }
  };

  const toggleExpand = async (id: number) => {
    if (expandedId === id) {
      setExpandedId(null);
      setDetail(null);
      return;
    }
    setExpandedId(id);
    setDetailPage(1);
    await loadDetail(id, 1);
  };

  const changePage = async (page: number) => {
    if (!expandedId) return;
    setDetailPage(page);
    await loadDetail(expandedId, page);
  };

  const handleImport = async () => {
    if (!subUrl.trim() || importing) return;
    setImporting(true);
    setError("");
    try {
      await api.post("/integration/import", { subscriptionUrl: subUrl.trim() });
      setSubUrl("");
      setShowInput(false);
      await loadIntegrations();
    } catch (err) {
      setError(extractApiError(err, "Import failed"));
    } finally {
      setImporting(false);
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await api.delete(`/integration/${id}`);
      setIntegrations(prev => prev.filter(i => i.id !== id));
      if (expandedId === id) {
        setExpandedId(null);
        setDetail(null);
      }
    } catch {
      // silent
    }
  };

  return (
    <div className="space-y-4">
      <section className="panel-surface p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p className="metric-kicker">Subscription Integration</p>
            <h3 className="mt-1.5 font-display text-xl font-semibold text-white">Import & Test</h3>
          </div>
          <button onClick={() => setShowInput(true)} className="btn-primary gap-2">
            <Plus className="h-4 w-4" />
            New Import
          </button>
        </div>
      </section>

      {showInput && (
        <section className="panel-surface p-4 sm:p-5">
          <p className="text-sm font-semibold text-white">Import Subscription URL</p>
          <p className="mt-1 text-xs text-slate-400">
            Paste a subscription URL to fetch and parse. Testing runs via automated cron.
          </p>
          <div className="mt-3 flex gap-2">
            <input
              type="text"
              value={subUrl}
              onChange={e => setSubUrl(e.target.value)}
              placeholder="https://example.com/subscription"
              className="flex-1 rounded-xl border border-white/10 bg-slate-950 px-4 py-2.5 text-sm text-white placeholder-slate-500 outline-none focus:border-sky-500/50"
              onKeyDown={e => e.key === "Enter" && handleImport()}
            />
            <button onClick={handleImport} disabled={importing || !subUrl.trim()} className="btn-primary gap-2">
              {importing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
              Import
            </button>
            <button onClick={() => setShowInput(false)} className="btn-secondary">
              Cancel
            </button>
          </div>
          {error && <p className="mt-2 text-xs text-rose-400">{error}</p>}
        </section>
      )}

      {loading ? (
        <div className="flex justify-center py-12">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-sky-400/20 border-t-sky-400" />
        </div>
      ) : integrations.length === 0 ? (
        <section className="panel-surface p-8 text-center">
          <p className="text-sm text-slate-400">No integrations yet. Click "New Import" to start.</p>
        </section>
      ) : (
        integrations.map(integ => {
          const cfg = statusConfig[integ.status] || { color: "bg-slate-400", label: integ.status };
          return (
            <section key={integ.id} className="panel-surface overflow-hidden p-0">
              <button
                onClick={() => toggleExpand(integ.id)}
                className="flex w-full items-center justify-between p-4 text-left sm:p-5"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className={`h-2 w-2 shrink-0 rounded-full ${cfg.color}`} />
                    <p className="truncate text-sm font-medium text-white">{integ.subscriptionUrl}</p>
                    <span className="shrink-0 rounded bg-white/5 px-2 py-0.5 text-[10px] uppercase tracking-wider text-slate-400">
                      {cfg.label}
                    </span>
                  </div>
                  <p className="mt-0.5 text-xs text-slate-500">
                    {new Date(integ.createdAt).toLocaleString()}
                    {integ.status !== "pending" && (
                      <> &middot; {integ.workingCount}/{integ.totalCount} working</>
                    )}
                  </p>
                  {(integ.lastTestCompletedAt || integ.nextTestAt) && (
                    <p className="mt-0.5 text-[11px] text-slate-600">
                      {integ.lastTestCompletedAt && <>Last: {fmtRelative(integ.lastTestCompletedAt)}</>}
                      {integ.lastTestCompletedAt && integ.nextTestAt && <> &middot; </>}
                      {integ.nextTestAt && <>Next: {fmtRelativeFuture(integ.nextTestAt)}</>}
                    </p>
                  )}
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <div
                    onClick={e => { e.stopPropagation(); handleDelete(integ.id); }}
                    className="cursor-pointer rounded-lg p-1.5 text-slate-500 hover:bg-rose-500/15 hover:text-rose-400"
                  >
                    <Trash2 className="h-4 w-4" />
                  </div>
                  {expandedId === integ.id ? <ChevronDown className="h-4 w-4 text-slate-400" /> : <ChevronRight className="h-4 w-4 text-slate-400" />}
                </div>
              </button>
              {expandedId === integ.id && (
                <div className="border-t border-white/10 p-4 sm:p-5">
                  {detailLoading ? (
                    <div className="flex justify-center py-4">
                      <div className="h-6 w-6 animate-spin rounded-full border-4 border-sky-400/20 border-t-sky-400" />
                    </div>
                  ) : detail ? (
                    <div className="space-y-3">
                      <div className="grid grid-cols-3 gap-2">
                        <div className="rounded-xl border border-white/10 bg-slate-950/35 p-2 text-center">
                          <p className="text-lg font-bold text-white">{detail.totalCount}</p>
                          <p className="text-xs text-slate-400">Total</p>
                        </div>
                        <div className="rounded-xl border border-white/10 bg-slate-950/35 p-2 text-center">
                          <p className="text-lg font-bold text-emerald-400">{detail.workingCount}</p>
                          <p className="text-xs text-slate-400">Working</p>
                        </div>
                        <div className="rounded-xl border border-white/10 bg-slate-950/35 p-2 text-center">
                          <p className="text-lg font-bold text-rose-400">{detail.failCount}</p>
                          <p className="text-xs text-slate-400">Failed</p>
                        </div>
                      </div>

                      {detail.working && detail.working.length > 0 && (
                        <div>
                          <p className="mb-1 text-xs font-semibold text-emerald-400">Working</p>
                          <div className="space-y-1">
                            {detail.working.map((ob, i) => (
                              <div key={i} className="flex items-center gap-2 rounded-lg bg-emerald-500/5 p-1.5">
                                <CheckCircle2 className="h-3 w-3 shrink-0 text-emerald-400" />
                                <span className="min-w-0 truncate text-xs text-slate-300">
                                  {(ob as Record<string, unknown>).tag as string || (ob as Record<string, unknown>).remark as string || "outbound"}
                                </span>
                                <span className="shrink-0 text-xs text-slate-500">
                                  {(ob as Record<string, unknown>).latencyMs != null ? `${(ob as Record<string, unknown>).latencyMs}ms` : ""}
                                </span>
                                {(ob as Record<string, unknown>).speedMbps != null && (ob as Record<string, unknown>).speedMbps as number > 0 && (
                                  <span className="shrink-0 text-xs text-sky-400">
                                    {(ob as Record<string, unknown>).speedMbps as number} Mbps
                                  </span>
                                )}
                                <button
                                  onClick={() => navigator.clipboard.writeText((ob as Record<string, unknown>).rawUri as string || "")}
                                  className="ml-auto shrink-0 rounded bg-white/5 px-2 py-1 text-xs text-slate-400 hover:text-white"
                                >
                                  Copy
                                </button>
                              </div>
                            ))}
                          </div>
                        </div>
                      )}

                      {detail.totalPages > 1 && (
                        <div className="flex items-center justify-center gap-2 pt-2">
                          <button
                            onClick={() => changePage(detail.page - 1)}
                            disabled={detail.page <= 1}
                            className="btn-secondary px-3 py-1 text-xs disabled:opacity-40"
                          >
                            Prev
                          </button>
                          <span className="text-xs text-slate-400">
                            Page {detail.page} of {detail.totalPages}
                          </span>
                          <button
                            onClick={() => changePage(detail.page + 1)}
                            disabled={detail.page >= detail.totalPages}
                            className="btn-secondary px-3 py-1 text-xs disabled:opacity-40"
                          >
                            Next
                          </button>
                        </div>
                      )}
                    </div>
                  ) : null}
                </div>
              )}
            </section>
          );
        })
      )}
    </div>
  );
}

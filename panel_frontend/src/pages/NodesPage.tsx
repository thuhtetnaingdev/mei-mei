import { FormEvent, useEffect, useState } from "react";
import { ExternalLink, Globe, PauseCircle, PlayCircle, Plus, RefreshCw, Trash2, Wrench } from "lucide-react";
import api from "../api/client";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { SectionCard } from "../components/SectionCard";
import type { Miner, Node, NodeDiagnosticResult } from "../types";

interface BootstrapResult {
  id: string;
  status: string;
  node: Node;
  steps: string[];
  logs: string[];
  error?: string;
}

interface NodeDiagnosticsResponse {
  results: NodeDiagnosticResult[];
}

interface SyncNodeResult {
  node: string;
  status: string;
  error?: string;
  verificationStatus?: string;
  verificationError?: string;
  expectedUserCount?: number;
  appliedUserCount?: number;
}

interface SyncNodesResponse {
  syncedUsers: number;
  results: SyncNodeResult[];
}

const emptyBootstrapForm = {
  name: "",
  minerId: "",
  ip: "",
  username: "",
  password: "",
  location: "",
  publicHost: "",
  isTestable: false,
  sshPort: "22",
  nodePort: "9090",
  singboxReloadCommand: "systemctl restart meimei-sing-box.service"
};

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

export function NodesPage() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [miners, setMiners] = useState<Miner[]>([]);
  const [bootstrapLog, setBootstrapLog] = useState<string[]>([]);
  const [bootstrapJobId, setBootstrapJobId] = useState("");
  const [isBootstrapping, setIsBootstrapping] = useState(false);
  const [createNodeDialogOpen, setCreateNodeDialogOpen] = useState(false);
  const [nodeActionStatus, setNodeActionStatus] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<Node | null>(null);
  const [uninstallTarget, setUninstallTarget] = useState<Node | null>(null);
  const [toggleTarget, setToggleTarget] = useState<Node | null>(null);
  const [reinstallTarget, setReinstallTarget] = useState<Node | null>(null);
  const [isReinstalling, setIsReinstalling] = useState(false);
  const [isTogglingNode, setIsTogglingNode] = useState(false);
  const [isRunningDiagnostics, setIsRunningDiagnostics] = useState(false);
  const [diagnostics, setDiagnostics] = useState<NodeDiagnosticResult[]>([]);
  const [editingNode, setEditingNode] = useState<Node | null>(null);
  const [nodeEditForm, setNodeEditForm] = useState({
    location: "",
    publicHost: "",
    isTestable: false,
    minerId: "",
    bandwidthLimitGb: "0",
    expiresAt: ""
  });
  const [form, setForm] = useState(emptyBootstrapForm);

  const loadNodes = () => api.get<Node[]>("/nodes").then((res) => setNodes(res.data));
  const loadMiners = () => api.get<Miner[]>("/miners").then((res) => setMiners(res.data));

  useEffect(() => {
    void Promise.all([loadNodes(), loadMiners()]).catch(() => undefined);
  }, []);

  useEffect(() => {
    if (!bootstrapJobId) {
      return;
    }

    const interval = window.setInterval(() => {
      void api
        .get<BootstrapResult>(`/nodes/bootstrap/${bootstrapJobId}`)
        .then(async (response) => {
          setBootstrapLog(response.data.steps);

          if (response.data.status === "completed") {
            setIsBootstrapping(false);
            setBootstrapJobId("");
            setNodeActionStatus(`Node ${response.data.node?.name ?? ""} bootstrapped successfully.`.trim());
            setForm(emptyBootstrapForm);
            await loadNodes();
          }

          if (response.data.status === "failed") {
            setIsBootstrapping(false);
            setBootstrapJobId("");
            setBootstrapLog((current) => [...current, response.data.error || "Bootstrap failed"]);
          }
        })
        .catch(() => undefined);
    }, 1500);

    return () => window.clearInterval(interval);
  }, [bootstrapJobId]);

  const bootstrapNode = async (event: FormEvent) => {
    event.preventDefault();
    setIsBootstrapping(true);
    setBootstrapLog(["Bootstrap job created", "Connecting to VPS and starting bootstrap..."]);
    setNodeActionStatus("");

    try {
      const response = await api.post<BootstrapResult>("/nodes/bootstrap", {
        ...form,
        minerId: Number(form.minerId),
        isTestable: form.isTestable,
        sshPort: Number(form.sshPort),
        nodePort: Number(form.nodePort)
      });
      setBootstrapJobId(response.data.id);
      setBootstrapLog(response.data.steps);
      setCreateNodeDialogOpen(false);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Bootstrap failed";
      setBootstrapLog((current) => [...current, message]);
      setIsBootstrapping(false);
    }
  };

  const syncNodes = async () => {
    const response = await api.post<SyncNodesResponse>("/nodes/sync");
    const failedNodes = (response.data.results ?? []).filter((result) => result.status !== "success");
    if (failedNodes.length > 0) {
      setNodeActionStatus(
        failedNodes
          .map((result) => `${result.node}: ${result.verificationError || result.error || "sync failed"}`)
          .join("; ")
      );
    } else {
      setNodeActionStatus(`Verified sync on ${response.data.results?.length ?? 0} nodes.`);
    }
    await loadNodes();
  };

  const runDiagnostics = async () => {
    setIsRunningDiagnostics(true);
    setNodeActionStatus("");

    try {
      const response = await api.post<NodeDiagnosticsResponse>("/nodes/diagnostics", {});
      setDiagnostics(response.data.results ?? []);
      setNodeActionStatus(`Tested ${response.data.results?.length ?? 0} nodes.`);
    } catch (error) {
      setNodeActionStatus(getRequestErrorMessage(error, "Node test failed"));
    } finally {
      setIsRunningDiagnostics(false);
    }
  };

  const openEditNode = (node: Node) => {
    setEditingNode(node);
    setNodeEditForm({
      location: node.location ?? "",
      publicHost: node.publicHost ?? "",
      isTestable: node.isTestable ?? false,
      minerId: node.minerId ? String(node.minerId) : "",
      bandwidthLimitGb: String(node.bandwidthLimitGb ?? 0),
      expiresAt: node.expiresAt ? node.expiresAt.slice(0, 16) : ""
    });
  };

  const closeEditNode = () => {
    setEditingNode(null);
    setNodeEditForm({
      location: "",
      publicHost: "",
      isTestable: false,
      minerId: "",
      bandwidthLimitGb: "0",
      expiresAt: ""
    });
  };

  const saveNodeDetails = async (event: FormEvent) => {
    event.preventDefault();
    if (!editingNode) {
      return;
    }

    await api.patch(`/nodes/${editingNode.id}`, {
      location: nodeEditForm.location,
      publicHost: nodeEditForm.publicHost,
      isTestable: nodeEditForm.isTestable,
      minerId: Number(nodeEditForm.minerId),
      bandwidthLimitGb: Number(nodeEditForm.bandwidthLimitGb) || 0,
      expiresAt: nodeEditForm.expiresAt ? new Date(nodeEditForm.expiresAt).toISOString() : null
    });

    setNodeActionStatus(`Updated node ${editingNode.name}.`);
    closeEditNode();
    await loadNodes();
  };

  const deleteNode = async () => {
    if (!deleteTarget) {
      return;
    }

    try {
      await api.delete(`/nodes/${deleteTarget.id}`);
      setNodeActionStatus(`Removed node ${deleteTarget.name} from the panel database.`);
      setDeleteTarget(null);
      await loadNodes();
    } catch (error) {
      setNodeActionStatus(getRequestErrorMessage(error, "Delete failed"));
    }
  };

  const uninstallNode = async () => {
    if (!uninstallTarget) {
      return;
    }

    try {
      await api.post(`/nodes/${uninstallTarget.id}/uninstall`, {});
      setNodeActionStatus(`Uninstalled node ${uninstallTarget.name} and removed it from the panel.`);
      setUninstallTarget(null);
      await loadNodes();
    } catch (error) {
      setNodeActionStatus(getRequestErrorMessage(error, "Uninstall failed"));
    }
  };

  const reinstallNode = async () => {
    if (!reinstallTarget) {
      return;
    }

    setIsReinstalling(true);
    setNodeActionStatus("");

    try {
      await api.post(`/nodes/${reinstallTarget.id}/reinstall`, {});
      setNodeActionStatus(`Reinstalled node ${reinstallTarget.name}.`);
      setReinstallTarget(null);
      await loadNodes();
    } catch (error) {
      setNodeActionStatus(getRequestErrorMessage(error, "Reinstall failed"));
    } finally {
      setIsReinstalling(false);
    }
  };

  const toggleNodeEnabled = async () => {
    if (!toggleTarget) {
      return;
    }

    const nextEnabled = !toggleTarget.enabled;
    setIsTogglingNode(true);
    setNodeActionStatus("");

    try {
      await api.patch(`/nodes/${toggleTarget.id}`, {
        enabled: nextEnabled
      });
      setNodeActionStatus(
        nextEnabled
          ? `Enabled node ${toggleTarget.name}. Active users were synced back to this node.`
          : `Disabled node ${toggleTarget.name}. Active users can no longer use this node.`
      );
      setToggleTarget(null);
      await loadNodes();
    } catch (error) {
      setNodeActionStatus(getRequestErrorMessage(error, nextEnabled ? "Enable failed" : "Disable failed"));
    } finally {
      setIsTogglingNode(false);
    }
  };

  const bootstrapFields = [
    { key: "name", label: "Node Name", placeholder: "sg-1", type: "text" },
    { key: "ip", label: "VPS IP", placeholder: "203.0.113.10", type: "text" },
    { key: "username", label: "SSH Username", placeholder: "root", type: "text" },
    { key: "password", label: "SSH Password", placeholder: "VPS password", type: "password" },
    { key: "location", label: "Location", placeholder: "Singapore", type: "text" },
    { key: "publicHost", label: "Public Host", placeholder: "Optional public host, defaults to IP", type: "text" },
    { key: "sshPort", label: "SSH Port", placeholder: "22", type: "number" },
    { key: "nodePort", label: "Node API Port", placeholder: "9090", type: "number" },
    { key: "singboxReloadCommand", label: "Reload Command", placeholder: "systemctl restart meimei-sing-box.service", type: "text" }
  ] as const;

  const formatBytes = (bytes: number) => {
    if (!bytes) {
      return "0 B";
    }

    const units = ["B", "KB", "MB", "GB", "TB"];
    let value = bytes;
    let unitIndex = 0;
    while (value >= 1024 && unitIndex < units.length - 1) {
      value /= 1024;
      unitIndex += 1;
    }
    return `${value.toFixed(value >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
  };

  const formatDateTime = (value?: string | null) => {
    if (!value) {
      return "Never";
    }

    return new Intl.DateTimeFormat(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit"
    }).format(new Date(value));
  };

  const formatCompactDateTime = (value?: string | null) => {
    if (!value) {
      return "No signal yet";
    }

    return new Intl.DateTimeFormat(undefined, {
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit"
    }).format(new Date(value));
  };

  const formatStatusLabel = (value?: string | null) => {
    if (!value) {
      return "Unknown";
    }

    return value
      .replace(/[_-]+/g, " ")
      .replace(/\b\w/g, (character) => character.toUpperCase());
  };

  const onlineNodes = nodes.filter((node) => node.healthStatus === "online").length;
  const offlineNodes = nodes.filter((node) => node.healthStatus === "offline").length;
  const enabledNodes = nodes.filter((node) => node.enabled).length;
  const verifiedNodes = nodes.filter((node) => node.syncVerificationStatus === "verified").length;
  const healthyDiagnostics = diagnostics.filter((entry) => entry.qualityStatus === "healthy").length;
  const degradedDiagnostics = diagnostics.filter((entry) => entry.qualityStatus === "degraded").length;
  const offlineDiagnostics = diagnostics.filter((entry) => entry.qualityStatus === "offline").length;

  const formatLatency = (value: number) => {
    if (!value) {
      return "n/a";
    }
    return `${value} ms`;
  };

  const formatSpeed = (value: number) => {
    if (!value) {
      return "n/a";
    }
    return `${value >= 100 ? value.toFixed(0) : value >= 10 ? value.toFixed(1) : value.toFixed(2)} Mbps`;
  };

  const inventoryActionClass =
    "inline-flex items-center justify-center gap-1.5 rounded-xl border border-white/10 bg-white/[0.03] px-3 py-2 text-xs font-semibold text-slate-100 transition hover:bg-white/[0.08] disabled:cursor-not-allowed disabled:opacity-60";

  const renderBandwidthRing = (usedBytes: number, limitGb: number) => {
    const unlimited = limitGb === 0;
    const limitBytes = limitGb * 1024 * 1024 * 1024;
    const rawPercent = unlimited || limitBytes <= 0 ? 0 : (usedBytes / limitBytes) * 100;
    const percent = Math.min(Math.max(rawPercent, 0), 100);
    const tone = unlimited
      ? "bg-slate-400"
      : rawPercent >= 90
        ? "bg-rose-400"
        : rawPercent >= 70
          ? "bg-amber-300"
          : "bg-emerald-400";
    const summary = unlimited ? "Unlimited" : `${limitGb} GB`;
    const progressWidth = unlimited ? 18 : percent;

    return (
      <div className="panel-subtle rounded-[18px] px-3.5 py-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="metric-kicker">Bandwidth</p>
            <p className="mt-1 text-sm font-semibold text-slate-50 sm:text-[15px]">
              {formatBytes(usedBytes)}
              <span className="mx-1.5 text-slate-500">/</span>
              {summary}
            </p>
          </div>
          <span className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-[11px] font-semibold text-slate-300">
            {unlimited ? "No cap" : `${Math.round(percent)}%`}
          </span>
        </div>

        <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-white/8">
          <div className={`h-full rounded-full transition-all duration-300 ${tone}`} style={{ width: `${progressWidth}%` }} />
        </div>

        <p className="mt-2 text-xs text-slate-500">{unlimited ? "This node has no bandwidth cap." : `${rawPercent.toFixed(1)}% used this cycle.`}</p>
      </div>
    );
  };

  const compactStats = [
    { label: "Total", value: nodes.length, tone: "text-white" },
    { label: "Online", value: onlineNodes, tone: "text-emerald-300" },
    { label: "Enabled", value: enabledNodes, tone: "text-sky-300" },
    { label: "Verified", value: verifiedNodes, tone: "text-emerald-300" },
    { label: "Alerts", value: offlineNodes, tone: "text-amber-200" }
  ];

  return (
    <div className="space-y-3">
      <section className="panel-surface px-4 py-4 sm:px-5 sm:py-4">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
          <div className="min-w-0">
            <p className="metric-kicker">Node Fleet</p>
            <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-2">
              <h2 className="font-display text-2xl font-semibold text-white">Nodes</h2>
              <span className="text-sm text-slate-500">Provision, verify, and maintain the active fleet.</span>
            </div>
          </div>

          <div className="flex flex-wrap gap-2">
            <button type="button" onClick={() => setCreateNodeDialogOpen(true)} className="btn-primary gap-1.5 px-3 py-2 text-sm">
              <Plus className="h-3.5 w-3.5" />
              Create
            </button>
            <button type="button" onClick={() => void syncNodes()} className="btn-secondary gap-1.5 px-3 py-2 text-sm">
              <RefreshCw className="h-3.5 w-3.5" />
              Sync
            </button>
            <button
              type="button"
              onClick={() => void runDiagnostics()}
              disabled={isRunningDiagnostics}
              className="btn-secondary gap-1.5 px-3 py-2 text-sm"
            >
              <Wrench className="h-3.5 w-3.5" />
              {isRunningDiagnostics ? "Testing..." : "Test"}
            </button>
          </div>
        </div>

        <div className="mt-4 grid gap-2 sm:grid-cols-2 xl:grid-cols-[minmax(0,1fr),340px]">
          <div className="grid gap-2 grid-cols-2 md:grid-cols-5">
            {compactStats.map((item) => (
              <div key={item.label} className="panel-subtle rounded-[20px] px-3 py-2.5">
                <p className="metric-kicker">{item.label}</p>
                <p className={`mt-1 text-lg font-semibold ${item.tone}`}>{item.value}</p>
              </div>
            ))}
          </div>

          <div className="panel-subtle rounded-[20px] px-3 py-2.5">
            <div className="flex items-start gap-3">
              <div className="rounded-xl border border-white/10 bg-sky-400/10 p-2 text-sky-300">
                <Globe className="h-4 w-4" />
              </div>
              <div className="min-w-0">
                <p className="metric-kicker">Recent Action</p>
                <p className="mt-1 break-words text-sm text-slate-200">{nodeActionStatus || "No recent changes"}</p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {bootstrapLog.length > 0 ? (
        <SectionCard
          eyebrow="Provision Activity"
          title={isBootstrapping ? "Node provisioning in progress" : "Latest provisioning run"}
          description="Bootstrap progress stays visible here after the modal is submitted."
          className="!p-4 sm:!p-5"
        >
          <div className="panel-subtle rounded-[20px] p-3">
            <div className="space-y-1.5 text-sm text-slate-300">
              {bootstrapLog.map((step, index) => (
                <p key={`${step}-${index}`}>{step}</p>
              ))}
            </div>
          </div>
        </SectionCard>
      ) : null}

      {diagnostics.length > 0 ? (
        <SectionCard
          eyebrow="Node Diagnostics"
          title="Manual speed-quality snapshot"
          description="These are manual panel-to-node checks focused on what matters for the MVP: API latency plus a small upload and download speed sample."
          className="!p-4 sm:!p-5"
        >
          <div className="space-y-3">
            <div className="grid gap-2 sm:grid-cols-3">
              <div className="panel-subtle rounded-[20px] p-3">
                <p className="metric-kicker">Healthy</p>
                <p className="mt-1 text-lg font-semibold text-emerald-300">{healthyDiagnostics}</p>
              </div>
              <div className="panel-subtle rounded-[20px] p-3">
                <p className="metric-kicker">Degraded</p>
                <p className="mt-1 text-lg font-semibold text-amber-200">{degradedDiagnostics}</p>
              </div>
              <div className="panel-subtle rounded-[20px] p-3">
                <p className="metric-kicker">Offline</p>
                <p className="mt-1 text-lg font-semibold text-rose-300">{offlineDiagnostics}</p>
              </div>
            </div>

            <div className="grid gap-3 xl:grid-cols-2">
              {diagnostics.map((entry) => (
                <div key={entry.nodeId} className="rounded-[20px] border border-white/10 bg-white/[0.03] p-3">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="text-sm font-semibold text-white">{entry.nodeName}</h3>
                        <span className={`status-pill ${
                          entry.qualityStatus === "healthy"
                            ? "text-emerald-300"
                            : entry.qualityStatus === "degraded"
                              ? "text-amber-200"
                              : "text-rose-300"
                        }`}>
                          <span className={`h-2 w-2 rounded-full ${
                            entry.qualityStatus === "healthy"
                              ? "bg-emerald-400"
                              : entry.qualityStatus === "degraded"
                                ? "bg-amber-300"
                                : "bg-rose-400"
                          }`} />
                          {entry.qualityStatus}
                        </span>
                      </div>
                      <p className="mt-1 text-xs text-slate-500">{entry.publicHost || entry.baseUrl}</p>
                    </div>
                    <p className="text-xs text-slate-500">{new Date(entry.testedAt).toLocaleString()}</p>
                  </div>

                  <div className="mt-3 grid gap-2 sm:grid-cols-2">
                    <div className="rounded-[16px] border border-white/10 bg-slate-950/28 px-3 py-2.5">
                      <p className="metric-kicker">API Latency</p>
                      <p className="mt-1 text-sm font-semibold text-white">
                        {entry.apiReachable ? formatLatency(entry.apiLatencyMs) : "Offline"}
                      </p>
                      <p className="mt-1 text-xs text-slate-500">{entry.apiErrorMessage || "Node API /status probe"}</p>
                    </div>
                    <div className="rounded-[16px] border border-white/10 bg-slate-950/28 px-3 py-2.5">
                      <p className="metric-kicker">Download</p>
                      <p className="mt-1 text-sm font-semibold text-white">
                        {entry.downloadError ? "Failed" : formatSpeed(entry.downloadMbps)}
                      </p>
                      <p className="mt-1 text-xs text-slate-500">
                        {entry.downloadError || `${formatBytes(entry.downloadBytes)} sample from node agent`}
                      </p>
                    </div>
                  </div>

                  <div className="mt-2 grid gap-2 sm:grid-cols-2">
                    <div className="rounded-[16px] border border-white/10 bg-slate-950/28 px-3 py-2.5">
                      <p className="metric-kicker">Upload</p>
                      <p className="mt-1 text-sm font-semibold text-white">
                        {entry.uploadError ? "Failed" : formatSpeed(entry.uploadMbps)}
                      </p>
                      <p className="mt-1 text-xs text-slate-500">
                        {entry.uploadError || `${formatBytes(entry.uploadBytes)} sample pushed to node agent`}
                      </p>
                    </div>
                    <div className="rounded-[16px] border border-white/10 bg-slate-950/28 px-3 py-2.5">
                      <p className="metric-kicker">Snapshot</p>
                      <p className="mt-1 text-sm font-semibold text-white">
                        {entry.qualityStatus === "healthy" ? "Looks solid" : entry.qualityStatus === "degraded" ? "Needs review" : "Offline"}
                      </p>
                      <p className="mt-1 text-xs text-slate-500">
                        Based on API latency plus small upload and download probes from the panel host.
                      </p>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </SectionCard>
      ) : null}

      <SectionCard eyebrow="Node Inventory" title="Registered nodes" description="A denser inventory view with the key facts and actions kept on one screen." className="!p-4 sm:!p-5">
        <div className="space-y-3">
          {nodeActionStatus ? (
            <div className="rounded-[18px] border border-sky-400/15 bg-sky-400/10 px-3 py-2.5 text-sm text-sky-100">
              {nodeActionStatus}
            </div>
          ) : null}

          {nodes.length === 0 ? (
            <div className="rounded-[20px] border border-dashed border-white/12 bg-white/[0.02] px-4 py-5">
              <p className="metric-kicker">Fleet Offline</p>
              <h3 className="mt-2 font-display text-xl font-semibold text-white">No nodes registered yet</h3>
              <p className="mt-2 max-w-2xl text-sm text-slate-400">
                Add a VPS node to start syncing users and generating routes.
              </p>
              <button type="button" onClick={() => setCreateNodeDialogOpen(true)} className="btn-primary mt-4 gap-1.5">
                <Plus className="h-4 w-4" />
                Create first node
              </button>
            </div>
          ) : (
            <div className="space-y-3">
              {nodes.map((node) => {
                const miner = miners.find((item) => item.id === node.minerId);
                const healthTone =
                  node.healthStatus === "online"
                    ? "border-emerald-400/15 bg-emerald-400/10 text-emerald-200"
                    : node.healthStatus === "offline"
                      ? "border-rose-400/15 bg-rose-400/10 text-rose-200"
                      : "border-white/10 bg-white/[0.04] text-slate-300";
                const healthDotTone =
                  node.healthStatus === "online" ? "bg-emerald-400" : node.healthStatus === "offline" ? "bg-rose-400" : "bg-slate-500";
                const verificationTone =
                  node.syncVerificationStatus === "verified"
                    ? "border-emerald-400/15 bg-emerald-400/10 text-emerald-200"
                    : node.syncVerificationStatus === "mismatch"
                      ? "border-amber-300/15 bg-amber-300/10 text-amber-100"
                      : node.syncVerificationStatus === "error"
                        ? "border-rose-400/15 bg-rose-400/10 text-rose-200"
                        : "border-white/10 bg-white/[0.04] text-slate-300";
                const verificationDotTone =
                  node.syncVerificationStatus === "verified"
                    ? "bg-emerald-400"
                    : node.syncVerificationStatus === "mismatch"
                      ? "bg-amber-300"
                      : node.syncVerificationStatus === "error"
                        ? "bg-rose-400"
                        : "bg-slate-500";

                return (
                  <article
                    key={node.id}
                    className={`overflow-hidden rounded-[24px] border shadow-panel ${
                      node.enabled ? "border-white/10 bg-white/[0.04]" : "border-amber-300/15 bg-amber-300/[0.05]"
                    }`}
                  >
                    <div className="px-3.5 py-3.5 sm:px-4 sm:py-4">
                      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr),320px] xl:items-start">
                        <div className="min-w-0 space-y-4">
                          <div>
                            <div className="flex flex-wrap items-center gap-2">
                              <h3 className="font-display text-base font-semibold text-white sm:text-lg">{node.name}</h3>
                              <span className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-400">
                                {node.location || "Unknown region"}
                              </span>
                            </div>

                            <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1">
                              <p className="text-sm font-medium text-slate-100 sm:text-[15px]">{node.publicHost || "No public host"}</p>
                              {node.baseUrl ? (
                                <a
                                  href={node.baseUrl}
                                  target="_blank"
                                  rel="noreferrer"
                                  className="inline-flex items-center gap-1 text-xs font-medium text-sky-300 transition hover:text-sky-200"
                                >
                                  Open endpoint
                                  <ExternalLink className="h-3.5 w-3.5" />
                                </a>
                              ) : null}
                            </div>
                            <p className="mt-1 text-xs text-slate-500">{node.baseUrl}</p>

                            <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-xs text-slate-400">
                              <span>{node.enabled ? "Serving traffic" : "Traffic paused"}</span>
                              <span>{miner?.name || "Unassigned"} miner</span>
                              <span>{node.appliedUserCount ?? 0} applied users</span>
                            </div>

                            <div className="mt-3 flex flex-wrap gap-2">
                              <span className={`inline-flex items-center gap-2 rounded-full border px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em] ${healthTone}`}>
                                <span className={`h-1.5 w-1.5 rounded-full ${healthDotTone}`} />
                                {formatStatusLabel(node.healthStatus)}
                              </span>
                              <span
                                className={`inline-flex items-center gap-2 rounded-full border px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em] ${
                                  node.enabled
                                    ? "border-sky-400/15 bg-sky-400/10 text-sky-200"
                                    : "border-amber-300/15 bg-amber-300/10 text-amber-100"
                                }`}
                              >
                                <span className={`h-1.5 w-1.5 rounded-full ${node.enabled ? "bg-sky-400" : "bg-amber-300"}`} />
                                {node.enabled ? "Enabled" : "Disabled"}
                              </span>
                              {node.isTestable ? (
                                <span className="inline-flex items-center gap-2 rounded-full border border-amber-300/15 bg-amber-300/10 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-amber-100">
                                  <span className="h-1.5 w-1.5 rounded-full bg-amber-300" />
                                  Testable
                                </span>
                              ) : null}
                              <span className={`inline-flex items-center gap-2 rounded-full border px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em] ${verificationTone}`}>
                                <span className={`h-1.5 w-1.5 rounded-full ${verificationDotTone}`} />
                                {formatStatusLabel(node.syncVerificationStatus)}
                              </span>
                            </div>
                          </div>

                          <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
                            <div className="panel-subtle rounded-[16px] px-3 py-2.5">
                              <p className="metric-kicker">Miner</p>
                              <p className="mt-1 text-sm font-medium text-slate-200">{miner?.name || "Unassigned"}</p>
                            </div>
                            <div className="panel-subtle rounded-[16px] px-3 py-2.5">
                              <p className="metric-kicker">Traffic</p>
                              <p className="mt-1 text-sm font-medium text-slate-200">
                                {node.enabled ? "Serving traffic" : "Disabled"}
                                {node.isTestable ? " • Testable" : ""}
                              </p>
                            </div>
                            <div className="panel-subtle rounded-[16px] px-3 py-2.5">
                              <p className="metric-kicker">Last Sync</p>
                              <p className="mt-1 text-sm font-medium text-slate-200">{formatCompactDateTime(node.lastSyncAt)}</p>
                              <p className="mt-0.5 text-xs text-slate-500">Verified {formatCompactDateTime(node.syncVerifiedAt)}</p>
                            </div>
                            <div className="panel-subtle rounded-[16px] px-3 py-2.5">
                              <p className="metric-kicker">Expiry</p>
                              <p className="mt-1 text-sm font-medium text-slate-200">{formatDateTime(node.expiresAt)}</p>
                              <p className="mt-0.5 text-xs text-slate-500">{node.appliedUserCount ?? 0} applied users</p>
                            </div>
                          </div>

                          {node.syncVerificationError ? (
                            <p className="rounded-[16px] border border-amber-300/15 bg-amber-300/[0.06] px-3 py-2 text-xs text-amber-200">
                              {node.syncVerificationError}
                            </p>
                          ) : null}
                        </div>

                        <div className="flex flex-col gap-2">
                          {renderBandwidthRing(node.bandwidthUsedBytes, node.bandwidthLimitGb)}

                          <div className="panel-subtle rounded-[18px] p-2.5">
                            <p className="metric-kicker px-1">Actions</p>
                            <div className="mt-2 flex flex-wrap gap-2">
                              <button onClick={() => openEditNode(node)} className={inventoryActionClass}>
                                Edit
                              </button>
                              <button
                                onClick={() => setToggleTarget(node)}
                                disabled={isTogglingNode}
                                className={
                                  node.enabled
                                    ? inventoryActionClass
                                    : "inline-flex items-center justify-center gap-1.5 rounded-xl bg-white px-3 py-2 text-xs font-semibold text-slate-950 transition hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-60"
                                }
                              >
                                {node.enabled ? <PauseCircle className="h-3.5 w-3.5" /> : <PlayCircle className="h-3.5 w-3.5" />}
                                {node.enabled ? "Disable" : "Enable"}
                              </button>
                              <button onClick={() => setReinstallTarget(node)} disabled={isReinstalling} className={inventoryActionClass}>
                                <RefreshCw className="h-3.5 w-3.5" />
                                Reinstall
                              </button>
                              <button
                                onClick={() => setDeleteTarget(node)}
                                className={inventoryActionClass}
                                title="Delete node from panel only"
                              >
                                <Trash2 className="h-3.5 w-3.5" />
                                Delete
                              </button>
                            </div>
                            <div className="mt-2 border-t border-white/8 pt-2">
                              <button
                                onClick={() => setUninstallTarget(node)}
                                className="inline-flex w-full items-center justify-center gap-1.5 rounded-xl border border-rose-400/20 bg-rose-500/10 px-3 py-2 text-xs font-semibold text-rose-100 transition hover:bg-rose-500/20"
                                title="Uninstall node"
                              >
                                <Trash2 className="h-3.5 w-3.5" />
                                Uninstall from server
                              </button>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  </article>
                );
              })}
            </div>
          )}
        </div>
      </SectionCard>

      <ConfirmDialog
        open={createNodeDialogOpen}
        title="Provision New Node"
        description="Enter VPS access and node API details. Transport ports are assigned automatically by the backend whenever the node regenerates its inbounds."
        hideActions
        panelClassName="max-w-6xl"
        onCancel={() => setCreateNodeDialogOpen(false)}
        onConfirm={() => undefined}
      >
        <form className="space-y-5" onSubmit={(event) => void bootstrapNode(event)}>
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            <label className="block">
              <span className="mb-2 block text-sm font-semibold text-slate-300">Miner</span>
              <select
                value={form.minerId}
                onChange={(event) => setForm((current) => ({ ...current, minerId: event.target.value }))}
                className="input-shell"
                required
              >
                <option value="">Select miner</option>
                {miners.map((miner) => (
                  <option key={miner.id} value={miner.id}>
                    {miner.name}
                  </option>
                ))}
              </select>
            </label>
            {bootstrapFields.map((field) => (
              <label key={field.key} className="block">
                <span className="mb-2 block text-sm font-semibold text-slate-300">{field.label}</span>
                <input
                  type={field.type}
                  value={form[field.key]}
                  onChange={(event) => setForm((current) => ({ ...current, [field.key]: event.target.value }))}
                  placeholder={field.placeholder}
                  className="input-shell"
                />
              </label>
            ))}
          </div>

          <label className="flex items-center gap-3 rounded-2xl border border-amber-300/15 bg-amber-300/[0.06] px-4 py-3 text-sm text-amber-100">
            <input
              type="checkbox"
              checked={form.isTestable}
              onChange={(event) => setForm((current) => ({ ...current, isTestable: event.target.checked }))}
              className="h-4 w-4 rounded border-white/20 bg-transparent"
            />
            Testable node
          </label>

          <div className="flex flex-wrap justify-end gap-3">
            <button type="button" onClick={() => setCreateNodeDialogOpen(false)} className="btn-secondary">
              Cancel
            </button>
            <button disabled={isBootstrapping} className="btn-primary">
              {isBootstrapping ? "Bootstrapping..." : "Create New Node"}
            </button>
          </div>
        </form>
      </ConfirmDialog>

      <ConfirmDialog
        open={Boolean(editingNode)}
        title="Update Node"
        description="Update node metadata such as public host, bandwidth limit, and expiry."
        hideActions
        onCancel={closeEditNode}
        onConfirm={() => undefined}
      >
        <form className="space-y-4" onSubmit={(event) => void saveNodeDetails(event)}>
          <label className="block">
            <span className="mb-2 block text-sm font-semibold text-slate-300">Miner</span>
            <select
              value={nodeEditForm.minerId}
              onChange={(event) => setNodeEditForm((current) => ({ ...current, minerId: event.target.value }))}
              className="input-shell"
              required
            >
              <option value="">Select miner</option>
              {miners.map((miner) => (
                <option key={miner.id} value={miner.id}>
                  {miner.name}
                </option>
              ))}
            </select>
          </label>
          <label className="block">
            <span className="mb-2 block text-sm font-semibold text-slate-300">Location</span>
            <input
              value={nodeEditForm.location}
              onChange={(event) => setNodeEditForm((current) => ({ ...current, location: event.target.value }))}
              className="input-shell"
            />
          </label>
          <label className="block">
            <span className="mb-2 block text-sm font-semibold text-slate-300">Public Host</span>
            <input
              value={nodeEditForm.publicHost}
              onChange={(event) => setNodeEditForm((current) => ({ ...current, publicHost: event.target.value }))}
              className="input-shell"
            />
          </label>
          <label className="flex items-center gap-3 rounded-2xl border border-amber-300/15 bg-amber-300/[0.06] px-4 py-3 text-sm text-amber-100">
            <input
              type="checkbox"
              checked={nodeEditForm.isTestable}
              onChange={(event) => setNodeEditForm((current) => ({ ...current, isTestable: event.target.checked }))}
              className="h-4 w-4 rounded border-white/20 bg-transparent"
            />
            Testable node
          </label>
          <label className="block">
            <span className="mb-2 block text-sm font-semibold text-slate-300">Bandwidth Limit (GB)</span>
            <input
              type="number"
              min={0}
              value={nodeEditForm.bandwidthLimitGb}
              onChange={(event) => setNodeEditForm((current) => ({ ...current, bandwidthLimitGb: event.target.value }))}
              className="input-shell"
            />
          </label>
          <label className="block">
            <span className="mb-2 block text-sm font-semibold text-slate-300">Expiry Date</span>
            <input
              type="datetime-local"
              value={nodeEditForm.expiresAt}
              onChange={(event) => setNodeEditForm((current) => ({ ...current, expiresAt: event.target.value }))}
              className="input-shell"
            />
          </label>
          <div className="flex flex-wrap justify-end gap-3">
            <button type="button" onClick={closeEditNode} className="btn-secondary">
              Cancel
            </button>
            <button type="submit" className="btn-primary">
              Save Node
            </button>
          </div>
        </form>
      </ConfirmDialog>

      <ConfirmDialog
        open={Boolean(toggleTarget)}
        title={toggleTarget?.enabled ? "Disable node?" : "Enable node?"}
        description={
          toggleTarget
            ? toggleTarget.enabled
              ? `This will disable ${toggleTarget.name}, remove active users from that node, and keep it out of subscriptions and sing-box profiles.`
              : `This will enable ${toggleTarget.name}, sync active users back to it, and include it in subscriptions again.`
            : ""
        }
        confirmLabel={
          isTogglingNode
            ? toggleTarget?.enabled
              ? "Disabling..."
              : "Enabling..."
            : toggleTarget?.enabled
              ? "Disable Node"
              : "Enable Node"
        }
        tone={toggleTarget?.enabled ? "danger" : "neutral"}
        onCancel={() => setToggleTarget(null)}
        onConfirm={() => void toggleNodeEnabled()}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title="Delete node?"
        description={deleteTarget ? `This will only remove ${deleteTarget.name} from the panel database. Nothing will be changed on the VPS.` : ""}
        confirmLabel="Delete Node"
        tone="neutral"
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => void deleteNode()}
      />

      <ConfirmDialog
        open={Boolean(uninstallTarget)}
        title="Uninstall node?"
        description={uninstallTarget ? `This will uninstall Meimei services and files from ${uninstallTarget.name}'s VPS, then remove the node from the panel. If the VPS is unreachable, uninstall will be blocked.` : ""}
        confirmLabel="Uninstall Node"
        tone="danger"
        onCancel={() => setUninstallTarget(null)}
        onConfirm={() => void uninstallNode()}
      />

      <ConfirmDialog
        open={Boolean(reinstallTarget)}
        title="Reinstall node?"
        description={reinstallTarget ? `This will reinstall and re-bootstrap ${reinstallTarget.name}. The node backend will be updated and the service will be restarted.` : ""}
        confirmLabel={isReinstalling ? "Reinstalling..." : "Reinstall Node"}
        tone="neutral"
        onCancel={() => setReinstallTarget(null)}
        onConfirm={() => void reinstallNode()}
      />
    </div>
  );
}

import { FormEvent, useEffect, useState } from "react";
import { Globe, PauseCircle, PlayCircle, Plus, RefreshCw, ServerCog, ShieldCheck, Trash2, Wrench } from "lucide-react";
import api from "../api/client";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { SectionCard } from "../components/SectionCard";
import type { Node, NodeDiagnosticResult } from "../types";

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

const emptyBootstrapForm = {
  name: "",
  ip: "",
  username: "",
  password: "",
  location: "",
  publicHost: "",
  sshPort: "22",
  nodePort: "9090",
  vlessPort: "443",
  tuicPort: "8443",
  hysteria2Port: "9443",
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
    bandwidthLimitGb: "0",
    expiresAt: ""
  });
  const [form, setForm] = useState(emptyBootstrapForm);

  const loadNodes = () => api.get<Node[]>("/nodes").then((res) => setNodes(res.data));

  useEffect(() => {
    void loadNodes().catch(() => undefined);
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
        sshPort: Number(form.sshPort),
        nodePort: Number(form.nodePort),
        vlessPort: Number(form.vlessPort),
        tuicPort: Number(form.tuicPort),
        hysteria2Port: Number(form.hysteria2Port)
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
    await api.post("/nodes/sync");
    setNodeActionStatus("Triggered full node sync.");
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
      bandwidthLimitGb: String(node.bandwidthLimitGb ?? 0),
      expiresAt: node.expiresAt ? node.expiresAt.slice(0, 16) : ""
    });
  };

  const closeEditNode = () => {
    setEditingNode(null);
    setNodeEditForm({
      location: "",
      publicHost: "",
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
    { key: "vlessPort", label: "VLESS Port", placeholder: "443", type: "number" },
    { key: "tuicPort", label: "TUIC Port", placeholder: "8443", type: "number" },
    { key: "hysteria2Port", label: "Hysteria2 Port", placeholder: "9443", type: "number" },
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
    return new Date(value).toLocaleString();
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

  const onlineNodes = nodes.filter((node) => node.healthStatus === "online").length;
  const offlineNodes = nodes.filter((node) => node.healthStatus === "offline").length;
  const enabledNodes = nodes.filter((node) => node.enabled).length;
  const disabledNodes = nodes.filter((node) => !node.enabled).length;
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

  return (
    <div className="space-y-4">
      <section className="grid gap-3 xl:grid-cols-[minmax(0,1.1fr),minmax(320px,0.9fr)]">
        <SectionCard
          eyebrow="Node Fleet"
          title="Operate VPS nodes"
          description="Provision new VPN nodes from a modal flow, keep metadata tidy, and expose health status from a denser infrastructure workspace."
          className="!p-4 sm:!p-5"
          action={
            <div className="mt-1 flex flex-col items-stretch gap-2.5 sm:min-w-[160px]">
              <button type="button" onClick={() => setCreateNodeDialogOpen(true)} className="btn-primary justify-center gap-1.5 px-3 py-2 text-sm">
                <Plus className="h-3.5 w-3.5" />
                Create
              </button>
              <button type="button" onClick={() => void syncNodes()} className="btn-secondary justify-center gap-1.5 px-3 py-2 text-sm">
                <RefreshCw className="h-3.5 w-3.5" />
                Sync
              </button>
              <button
                type="button"
                onClick={() => void runDiagnostics()}
                disabled={isRunningDiagnostics}
                className="btn-secondary justify-center gap-1.5 px-3 py-2 text-sm"
              >
                <Wrench className="h-3.5 w-3.5" />
                {isRunningDiagnostics ? "Testing..." : "Test All"}
              </button>
            </div>
          }
        >
          <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-4">
            <div className="panel-subtle p-3">
              <p className="metric-kicker">Registered</p>
              <p className="mt-2 font-display text-2xl font-bold text-white">{nodes.length}</p>
              <p className="mt-1 text-xs text-slate-500">All known VPS nodes</p>
            </div>
            <div className="panel-subtle p-3">
              <p className="metric-kicker">Online</p>
              <p className="mt-2 font-display text-2xl font-bold text-emerald-300">{onlineNodes}</p>
              <p className="mt-1 text-xs text-slate-500">Responding to health checks</p>
            </div>
            <div className="panel-subtle p-3">
              <p className="metric-kicker">Traffic Live</p>
              <p className="mt-2 font-display text-2xl font-bold text-sky-300">{enabledNodes}</p>
              <p className="mt-1 text-xs text-slate-500">Included in user access</p>
            </div>
            <div className="panel-subtle p-3">
              <p className="metric-kicker">Attention</p>
              <p className="mt-2 font-display text-2xl font-bold text-amber-200">{offlineNodes + disabledNodes}</p>
              <p className="mt-1 text-xs text-slate-500">Offline or intentionally paused</p>
            </div>
          </div>
        </SectionCard>

        <div className="panel-surface p-4 sm:p-5">
          <div className="flex items-center justify-between">
            <div>
              <p className="metric-kicker">Ops Signals</p>
              <h3 className="mt-1.5 font-display text-xl font-semibold text-white">Fleet summary</h3>
            </div>
            <div className="rounded-2xl border border-white/10 bg-sky-400/10 p-2.5 text-sky-300">
              <Globe className="h-4.5 w-4.5" />
            </div>
          </div>
          <div className="mt-4 grid gap-2.5">
            {[
              { label: "Online nodes", value: onlineNodes, icon: ShieldCheck, tone: "text-emerald-300 bg-emerald-500/10" },
              { label: "Needs attention", value: offlineNodes, icon: Wrench, tone: "text-amber-200 bg-amber-500/10" },
              { label: "Recent action", value: nodeActionStatus || "No recent changes", icon: ServerCog, tone: "text-sky-300 bg-sky-500/10" }
            ].map((item) => (
              <div key={item.label} className="panel-subtle p-3">
                <div className="flex items-start gap-3">
                  <div className={`rounded-xl p-2 ${item.tone}`}>
                    <item.icon className="h-4 w-4" />
                  </div>
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-slate-300">{item.label}</p>
                    <p className="mt-0.5 break-words text-sm text-white">{item.value}</p>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {bootstrapLog.length > 0 ? (
        <SectionCard
          eyebrow="Provision Activity"
          title={isBootstrapping ? "Node provisioning in progress" : "Latest provisioning run"}
          description="Bootstrap progress stays visible here after the modal is submitted."
        >
          <div className="panel-subtle p-4">
            <p className="metric-kicker">Bootstrap Log</p>
            <div className="mt-4 space-y-2 text-sm text-slate-300">
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
        >
          <div className="space-y-4">
            <div className="grid gap-2.5 sm:grid-cols-3">
              <div className="panel-subtle p-3">
                <p className="metric-kicker">Healthy</p>
                <p className="mt-2 font-display text-2xl font-bold text-emerald-300">{healthyDiagnostics}</p>
              </div>
              <div className="panel-subtle p-3">
                <p className="metric-kicker">Degraded</p>
                <p className="mt-2 font-display text-2xl font-bold text-amber-200">{degradedDiagnostics}</p>
              </div>
              <div className="panel-subtle p-3">
                <p className="metric-kicker">Offline</p>
                <p className="mt-2 font-display text-2xl font-bold text-rose-300">{offlineDiagnostics}</p>
              </div>
            </div>

            <div className="grid gap-3 xl:grid-cols-2">
              {diagnostics.map((entry) => (
                <div key={entry.nodeId} className="rounded-[22px] border border-white/10 bg-white/[0.03] p-4">
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

                  <div className="mt-4 grid gap-2.5 sm:grid-cols-2">
                    <div className="rounded-[18px] border border-white/10 bg-slate-950/28 px-3 py-3">
                      <p className="metric-kicker">API Latency</p>
                      <p className="mt-2 text-sm font-semibold text-white">
                        {entry.apiReachable ? formatLatency(entry.apiLatencyMs) : "Offline"}
                      </p>
                      <p className="mt-1 text-xs text-slate-500">{entry.apiErrorMessage || "Node API /status probe"}</p>
                    </div>
                    <div className="rounded-[18px] border border-white/10 bg-slate-950/28 px-3 py-3">
                      <p className="metric-kicker">Download</p>
                      <p className="mt-2 text-sm font-semibold text-white">
                        {entry.downloadError ? "Failed" : formatSpeed(entry.downloadMbps)}
                      </p>
                      <p className="mt-1 text-xs text-slate-500">
                        {entry.downloadError || `${formatBytes(entry.downloadBytes)} sample from node agent`}
                      </p>
                    </div>
                  </div>

                  <div className="mt-3 grid gap-2 sm:grid-cols-2">
                    <div className="rounded-[18px] border border-white/10 bg-slate-950/28 px-3 py-3">
                      <p className="metric-kicker">Upload</p>
                      <p className="mt-2 text-sm font-semibold text-white">
                        {entry.uploadError ? "Failed" : formatSpeed(entry.uploadMbps)}
                      </p>
                      <p className="mt-1 text-xs text-slate-500">
                        {entry.uploadError || `${formatBytes(entry.uploadBytes)} sample pushed to node agent`}
                      </p>
                    </div>
                    <div className="rounded-[18px] border border-white/10 bg-slate-950/28 px-3 py-3">
                      <p className="metric-kicker">Snapshot</p>
                      <p className="mt-2 text-sm font-semibold text-white">
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

      <SectionCard eyebrow="Node Inventory" title="Registered nodes" description="Compact rows designed for larger fleets, with the controls and node facts kept close together.">
        <div className="space-y-4">
          {nodeActionStatus ? (
            <div className="rounded-2xl border border-sky-400/15 bg-sky-400/10 px-4 py-3 text-sm text-sky-100">
              {nodeActionStatus}
            </div>
          ) : null}

          {nodes.length === 0 ? (
            <div className="rounded-[24px] border border-dashed border-white/12 bg-white/[0.02] px-5 py-6">
              <p className="metric-kicker">Fleet Offline</p>
              <h3 className="mt-3 font-display text-2xl font-semibold text-white">No nodes registered yet</h3>
              <p className="mt-2 max-w-2xl text-sm leading-7 text-slate-400">
                Add a VPS node to start syncing users and generating routes.
              </p>
              <button type="button" onClick={() => setCreateNodeDialogOpen(true)} className="btn-primary mt-5 gap-1.5">
                <Plus className="h-4 w-4" />
                Create first node
              </button>
            </div>
          ) : (
            <div className="space-y-3">
              {nodes.map((node) => {
                return (
                  <article
                    key={node.id}
                    className={`rounded-[24px] border px-4 py-4 sm:px-5 ${
                      node.enabled ? "border-white/10 bg-white/[0.04] shadow-panel" : "border-amber-400/16 bg-amber-300/[0.05] shadow-panel"
                    }`}
                  >
                    <div className="flex flex-col gap-3">
                      <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <h3 className="font-display text-xl font-semibold text-white">{node.name}</h3>
                            <span className={`status-pill ${
                              node.healthStatus === "online" ? "text-emerald-300" : node.healthStatus === "offline" ? "text-rose-300" : "text-slate-400"
                            }`}>
                              <span className={`h-2 w-2 rounded-full ${
                                node.healthStatus === "online" ? "bg-emerald-400" : node.healthStatus === "offline" ? "bg-rose-400" : "bg-slate-500"
                              }`} />
                              {node.healthStatus}
                            </span>
                            <span className={`status-pill ${node.enabled ? "text-sky-300" : "text-amber-200"}`}>
                              <span className={`h-2 w-2 rounded-full ${node.enabled ? "bg-sky-400" : "bg-amber-300"}`} />
                              {node.enabled ? "enabled" : "disabled"}
                            </span>
                          </div>
                          <p className="mt-1 text-sm text-slate-400">
                            {node.location || "Unknown region"} • {node.publicHost || "No public host"}
                          </p>
                        </div>

                        <div className="flex flex-wrap gap-2">
                          <button onClick={() => openEditNode(node)} className="btn-secondary px-3 py-2 text-xs">
                            Edit
                          </button>
                          <button
                            onClick={() => setToggleTarget(node)}
                            disabled={isTogglingNode}
                            className={`${node.enabled ? "btn-secondary" : "btn-primary"} gap-1 px-3 py-2 text-xs`}
                          >
                            {node.enabled ? <PauseCircle className="h-3.5 w-3.5" /> : <PlayCircle className="h-3.5 w-3.5" />}
                            {node.enabled ? "Disable" : "Enable"}
                          </button>
                          <button onClick={() => setReinstallTarget(node)} disabled={isReinstalling} className="btn-secondary gap-1 px-3 py-2 text-xs">
                            <RefreshCw className="h-3.5 w-3.5" />
                            Reinstall
                          </button>
                          <button
                            onClick={() => setDeleteTarget(node)}
                            className="btn-secondary gap-1 px-3 py-2 text-xs"
                            title="Delete node from panel only"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                            Delete
                          </button>
                          <button
                            onClick={() => setUninstallTarget(node)}
                            className="inline-flex items-center justify-center gap-1 rounded-2xl border border-rose-400/20 bg-rose-500/10 px-3 py-2 text-xs font-semibold text-rose-100 transition hover:bg-rose-500/20"
                            title="Uninstall node"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                            Uninstall
                          </button>
                        </div>
                      </div>

                      <dl className="grid gap-2 text-sm sm:grid-cols-2 xl:grid-cols-6">
                        <div className="rounded-[20px] border border-white/10 bg-slate-950/28 px-3 py-3">
                          <dt className="metric-kicker">API</dt>
                          <dd className="mt-2 break-all text-slate-200">{node.baseUrl}</dd>
                        </div>
                        <div className="rounded-[20px] border border-white/10 bg-slate-950/28 px-3 py-3">
                          <dt className="metric-kicker">Ports</dt>
                          <dd className="mt-2 text-slate-200">V {node.vlessPort} · T {node.tuicPort} · H {node.hysteria2Port}</dd>
                        </div>
                        <div className="rounded-[20px] border border-white/10 bg-slate-950/28 px-3 py-3">
                          <dt className="metric-kicker">Usage</dt>
                          <dd className="mt-2 text-slate-200">{formatBytes(node.bandwidthUsedBytes)}</dd>
                        </div>
                        <div className="rounded-[20px] border border-white/10 bg-slate-950/28 px-3 py-3">
                          <dt className="metric-kicker">Limit</dt>
                          <dd className="mt-2 text-slate-200">{node.bandwidthLimitGb > 0 ? `${node.bandwidthLimitGb} GB` : "Unlimited"}</dd>
                        </div>
                        <div className="rounded-[20px] border border-white/10 bg-slate-950/28 px-3 py-3">
                          <dt className="metric-kicker">Last Sync</dt>
                          <dd className="mt-2 text-slate-200">{formatCompactDateTime(node.lastSyncAt)}</dd>
                        </div>
                        <div className="rounded-[20px] border border-white/10 bg-slate-950/28 px-3 py-3">
                          <dt className="metric-kicker">Expires</dt>
                          <dd className="mt-2 text-slate-200">{formatDateTime(node.expiresAt)}</dd>
                        </div>
                      </dl>
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
        description="Enter VPS credentials and transport ports. The panel will upload the node backend, install services, and register the node automatically."
        hideActions
        panelClassName="max-w-6xl"
        onCancel={() => setCreateNodeDialogOpen(false)}
        onConfirm={() => undefined}
      >
        <form className="space-y-5" onSubmit={(event) => void bootstrapNode(event)}>
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
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

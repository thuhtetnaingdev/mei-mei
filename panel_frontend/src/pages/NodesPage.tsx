import { FormEvent, useEffect, useState } from "react";
import { Trash2 } from "lucide-react";
import api from "../api/client";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { SectionCard } from "../components/SectionCard";
import type { Node } from "../types";

interface BootstrapResult {
  id: string;
  status: string;
  node: Node;
  steps: string[];
  logs: string[];
  error?: string;
}

export function NodesPage() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [bootstrapLog, setBootstrapLog] = useState<string[]>([]);
  const [bootstrapJobId, setBootstrapJobId] = useState("");
  const [isBootstrapping, setIsBootstrapping] = useState(false);
  const [nodeActionStatus, setNodeActionStatus] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<Node | null>(null);
  const [reinstallTarget, setReinstallTarget] = useState<Node | null>(null);
  const [isReinstalling, setIsReinstalling] = useState(false);
  const [editingNode, setEditingNode] = useState<Node | null>(null);
  const [nodeEditForm, setNodeEditForm] = useState({
    location: "",
    publicHost: "",
    bandwidthLimitGb: "0",
    expiresAt: ""
  });
  const [form, setForm] = useState({
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
  });

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
            setForm({
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
            });
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
    } catch (error) {
      const message = error instanceof Error ? error.message : "Bootstrap failed";
      setBootstrapLog((current) => [...current, message]);
      setIsBootstrapping(false);
    } finally {
    }
  };

  const syncNodes = async () => {
    await api.post("/nodes/sync");
    await loadNodes();
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
      setNodeActionStatus(`Deleted node ${deleteTarget.name}.`);
      setDeleteTarget(null);
      await loadNodes();
    } catch (error) {
      const message = error instanceof Error ? error.message : "Delete failed";
      setNodeActionStatus(message);
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
      const message = error instanceof Error ? error.message : "Reinstall failed";
      setNodeActionStatus(message);
    } finally {
      setIsReinstalling(false);
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

  return (
    <div className="space-y-8">
      <SectionCard
        title="Bootstrap VPS Node"
        description="Enter the VPS IP, username, and password. The panel will update the host, upload the node backend, install a systemd service, and register the node automatically."
        action={
          <button onClick={() => void syncNodes()} className="rounded-2xl bg-tide px-4 py-2 text-sm font-semibold text-white">
            Sync All Nodes
          </button>
        }
      >
        <form className="grid gap-4 md:grid-cols-2 xl:grid-cols-3" onSubmit={(event) => void bootstrapNode(event)}>
          {bootstrapFields.map((field) => (
            <label key={field.key} className="block">
              <span className="mb-2 block text-sm font-semibold text-slate-700">{field.label}</span>
              <input
                type={field.type}
                value={form[field.key]}
                onChange={(event) => setForm((current) => ({ ...current, [field.key]: event.target.value }))}
                placeholder={field.placeholder}
                className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm outline-none transition focus:border-tide"
              />
            </label>
          ))}
          <button disabled={isBootstrapping} className="rounded-2xl bg-ink px-4 py-3 text-sm font-semibold text-white disabled:opacity-60 md:col-span-2 xl:col-span-3">
            {isBootstrapping ? "Bootstrapping..." : "Bootstrap Node"}
          </button>
        </form>

        {bootstrapLog.length > 0 ? (
          <div className="mt-5 rounded-2xl bg-slate-50 p-4">
            <p className="mb-3 text-sm font-semibold text-ink">Bootstrap Log</p>
            <div className="space-y-2 text-sm text-slate-600">
              {bootstrapLog.map((step) => (
                <p key={step}>{step}</p>
              ))}
            </div>
          </div>
        ) : null}
      </SectionCard>

      <SectionCard title="Nodes" description="The control plane stores metadata, health, and last sync times for each data-plane agent.">
        {nodeActionStatus ? <p className="mb-4 text-sm text-slate-500">{nodeActionStatus}</p> : null}
        <div className="grid gap-4 lg:grid-cols-2">
          {nodes.map((node) => (
            <div key={node.id} className="rounded-2xl border border-slate-100 bg-slate-50 p-5">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="font-display text-xl text-ink">{node.name}</h3>
                  <p className="text-sm text-slate-500">{node.location || "Unknown region"}</p>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => openEditNode(node)}
                    className="rounded-full border border-slate-200 bg-white px-3 py-1 text-xs font-semibold text-slate-700"
                  >
                    Edit
                  </button>
                  <button
                    onClick={() => setReinstallTarget(node)}
                    disabled={isReinstalling}
                    className="rounded-full border border-slate-200 bg-white px-3 py-1 text-xs font-semibold text-slate-700 disabled:opacity-60"
                  >
                    Reinstall
                  </button>
                  <button
                    onClick={() => setDeleteTarget(node)}
                    className="rounded-full border border-rose-200 bg-transparent p-2 text-rose-600 hover:bg-rose-50"
                    title="Delete node"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              </div>
              <dl className="mt-4 space-y-2 text-sm text-slate-600">
                <div className="flex justify-between gap-4"><dt>Status</dt><dd><span className={`inline-flex items-center gap-1.5 ${
                  node.healthStatus === "online" 
                    ? "text-emerald-700" 
                    : node.healthStatus === "offline"
                    ? "text-rose-700"
                    : "text-slate-600"
                }`}><span className={`inline-block h-2 w-2 rounded-full ${
                  node.healthStatus === "online" 
                    ? "bg-emerald-500" 
                    : node.healthStatus === "offline"
                    ? "bg-rose-500"
                    : "bg-slate-400"
                }`}></span>{node.healthStatus}</span></dd></div>
                <div className="flex justify-between gap-4"><dt>Base URL</dt><dd>{node.baseUrl}</dd></div>
                <div className="flex justify-between gap-4"><dt>Public Host</dt><dd>{node.publicHost}</dd></div>
                <div className="flex justify-between gap-4"><dt>VLESS</dt><dd>{node.vlessPort}</dd></div>
                <div className="flex justify-between gap-4"><dt>TUIC</dt><dd>{node.tuicPort}</dd></div>
                <div className="flex justify-between gap-4"><dt>Hysteria2</dt><dd>{node.hysteria2Port}</dd></div>
                <div className="flex justify-between gap-4"><dt>Bandwidth Limit</dt><dd>{node.bandwidthLimitGb > 0 ? `${node.bandwidthLimitGb} GB` : "Unlimited"}</dd></div>
                <div className="flex justify-between gap-4"><dt>Bandwidth Usage</dt><dd>{formatBytes(node.bandwidthUsedBytes)}</dd></div>
                <div className="flex justify-between gap-4"><dt>Expires At</dt><dd>{formatDateTime(node.expiresAt)}</dd></div>
              </dl>
            </div>
          ))}
        </div>
      </SectionCard>
      <ConfirmDialog
        open={Boolean(editingNode)}
        title="Update Node"
        description="Update node metadata such as bandwidth limit and expiry date."
        hideActions
        onCancel={closeEditNode}
        onConfirm={() => undefined}
      >
        <form className="space-y-4" onSubmit={(event) => void saveNodeDetails(event)}>
          <label className="block">
            <span className="mb-2 block text-sm font-semibold text-slate-700">Location</span>
            <input
              value={nodeEditForm.location}
              onChange={(event) => setNodeEditForm((current) => ({ ...current, location: event.target.value }))}
              className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm outline-none transition focus:border-tide"
            />
          </label>
          <label className="block">
            <span className="mb-2 block text-sm font-semibold text-slate-700">Public Host</span>
            <input
              value={nodeEditForm.publicHost}
              onChange={(event) => setNodeEditForm((current) => ({ ...current, publicHost: event.target.value }))}
              className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm outline-none transition focus:border-tide"
            />
          </label>
          <label className="block">
            <span className="mb-2 block text-sm font-semibold text-slate-700">Bandwidth Limit (GB)</span>
            <input
              type="number"
              min={0}
              value={nodeEditForm.bandwidthLimitGb}
              onChange={(event) => setNodeEditForm((current) => ({ ...current, bandwidthLimitGb: event.target.value }))}
              className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm outline-none transition focus:border-tide"
            />
          </label>
          <label className="block">
            <span className="mb-2 block text-sm font-semibold text-slate-700">Expiry Date</span>
            <input
              type="datetime-local"
              value={nodeEditForm.expiresAt}
              onChange={(event) => setNodeEditForm((current) => ({ ...current, expiresAt: event.target.value }))}
              className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm outline-none transition focus:border-tide"
            />
          </label>
          <div className="flex justify-end gap-3">
            <button
              type="button"
              onClick={closeEditNode}
              className="rounded-2xl border border-slate-200 px-4 py-2.5 text-sm font-semibold text-slate-700 transition hover:bg-slate-50"
            >
              Cancel
            </button>
            <button type="submit" className="rounded-2xl bg-ink px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-slate-800">
              Save Node
            </button>
          </div>
        </form>
      </ConfirmDialog>
      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title="Delete node?"
        description={deleteTarget ? `This will remove ${deleteTarget.name} from the panel metadata. It will not uninstall software from the VPS.` : ""}
        confirmLabel="Delete Node"
        tone="danger"
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => void deleteNode()}
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

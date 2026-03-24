import { FormEvent, useEffect, useState } from "react";
import { Cpu, Plus, Trash2, Wallet } from "lucide-react";
import api from "../api/client";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { SectionCard } from "../components/SectionCard";
import type { Miner, Node } from "../types";

type MinerFormState = {
  name: string;
  walletAddress: string;
  notes: string;
  nodeIds: number[];
};

const defaultFormState: MinerFormState = {
  name: "",
  walletAddress: "",
  notes: "",
  nodeIds: []
};

const formatTokenAmount = (value: number | undefined) => {
  const safeValue = value ?? 0;
  if (safeValue === 0) {
    return "0.00";
  }
  if (safeValue < 0.01) {
    return safeValue.toFixed(6);
  }
  return safeValue.toFixed(2);
};

export function MinersPage() {
  const [miners, setMiners] = useState<Miner[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [form, setForm] = useState<MinerFormState>(defaultFormState);
  const [editingMinerId, setEditingMinerId] = useState<number | null>(null);
  const [minerDialogOpen, setMinerDialogOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Miner | null>(null);
  const [status, setStatus] = useState("");

  const loadData = () =>
    Promise.all([api.get<Miner[]>("/miners"), api.get<Node[]>("/nodes")]).then(([minersRes, nodesRes]) => {
      setMiners(minersRes.data);
      setNodes(nodesRes.data);
    });

  useEffect(() => {
    void loadData().catch(() => undefined);
  }, []);

  useEffect(() => {
    const interval = window.setInterval(() => {
      void loadData().catch(() => undefined);
    }, 5000);

    return () => window.clearInterval(interval);
  }, []);

  const closeMinerDialog = () => {
    setForm(defaultFormState);
    setEditingMinerId(null);
    setMinerDialogOpen(false);
  };

  const openCreateDialog = () => {
    setForm(defaultFormState);
    setEditingMinerId(null);
    setMinerDialogOpen(true);
  };

  const startEdit = (miner: Miner) => {
    setEditingMinerId(miner.id);
    setForm({
      name: miner.name,
      walletAddress: miner.walletAddress,
      notes: miner.notes ?? "",
      nodeIds: miner.nodes.map((node) => node.id)
    });
    setMinerDialogOpen(true);
  };

  const toggleNodeSelection = (nodeId: number) => {
    setForm((current) => ({
      ...current,
      nodeIds: current.nodeIds.includes(nodeId)
        ? current.nodeIds.filter((id) => id !== nodeId)
        : [...current.nodeIds, nodeId]
    }));
  };

  const submitMiner = async (event: FormEvent) => {
    event.preventDefault();
    const payload = {
      name: form.name,
      walletAddress: form.walletAddress,
      notes: form.notes,
      nodeIds: form.nodeIds
    };

    if (editingMinerId) {
      await api.patch(`/miners/${editingMinerId}`, payload);
      setStatus("Miner updated.");
    } else {
      await api.post("/miners", payload);
      setStatus("Miner created.");
    }

    await loadData();
    closeMinerDialog();
  };

  const deleteMiner = async () => {
    if (!deleteTarget) {
      return;
    }

    await api.delete(`/miners/${deleteTarget.id}`);
    setStatus(`Deleted miner ${deleteTarget.name}.`);
    setDeleteTarget(null);
    await loadData();
  };

  const availableNodes = nodes.map((node) => {
    const owner = miners.find((miner) => miner.nodes.some((minerNode) => minerNode.id === node.id));
    const assignedToDifferentMiner = owner && owner.id !== editingMinerId;
    return { node, owner, assignedToDifferentMiner };
  });

  const totalRewardedTokens = miners.reduce((sum, miner) => sum + (miner.rewardedTokens ?? 0), 0);

  return (
    <div className="space-y-4">
      <section className="grid gap-3 xl:grid-cols-[minmax(0,1.1fr),minmax(320px,0.9fr)]">
        <SectionCard
          eyebrow="Mining Operations"
          title="Miners, wallets, and nodes"
          description="Track miner operators, store their wallet addresses, and attach the nodes each miner is responsible for."
          className="!p-4 sm:!p-5"
          action={
            <button type="button" onClick={openCreateDialog} className="btn-primary gap-1.5 px-3 py-2 text-sm">
              <Plus className="h-3.5 w-3.5" />
              Add
            </button>
          }
        >
          <div className="grid gap-2.5 sm:grid-cols-3">
            <div className="panel-subtle p-3">
              <p className="metric-kicker">Miners</p>
              <p className="mt-2 font-display text-2xl font-bold text-white">{miners.length}</p>
            </div>
            <div className="panel-subtle p-3">
              <p className="metric-kicker">Linked Nodes</p>
              <p className="mt-2 font-display text-2xl font-bold text-emerald-300">
                {miners.reduce((sum, miner) => sum + miner.nodes.length, 0)}
              </p>
            </div>
            <div className="panel-subtle p-3">
              <p className="metric-kicker">Rewarded Credits</p>
              <p className="mt-2 font-display text-2xl font-bold text-sky-300">
                {formatTokenAmount(totalRewardedTokens)}
              </p>
            </div>
          </div>
        </SectionCard>

        <div className="panel-surface p-4 sm:p-5">
          <div className="flex items-center justify-between">
            <div>
              <p className="metric-kicker">Ownership View</p>
              <h3 className="mt-1.5 font-display text-xl font-semibold text-white">Miner coverage</h3>
            </div>
            <div className="rounded-2xl border border-white/10 bg-sky-400/10 p-2.5 text-sky-300">
              <Cpu className="h-4.5 w-4.5" />
            </div>
          </div>
          <div className="mt-4 space-y-2.5">
            {miners.length ? (
              miners.slice(0, 4).map((miner) => (
                <div key={miner.id} className="panel-subtle flex items-center justify-between p-3">
                  <div>
                    <p className="text-sm font-semibold text-white">{miner.name}</p>
                    <p className="mt-0.5 text-xs text-slate-500">{miner.walletAddress}</p>
                  </div>
                  <span className="status-pill">
                    <span className="h-2 w-2 rounded-full bg-emerald-400" />
                    {formatTokenAmount(miner.rewardedTokens)} credits
                  </span>
                </div>
              ))
            ) : (
              <div className="panel-subtle p-4 text-sm text-slate-400">No miners added yet.</div>
            )}
          </div>
        </div>
      </section>

      {status ? <div className="rounded-2xl border border-emerald-400/20 bg-emerald-400/10 px-4 py-3 text-sm text-emerald-200">{status}</div> : null}

      <SectionCard
        eyebrow="Miner Registry"
        title="Current miners"
        description="Each miner includes a wallet address and a list of linked nodes."
        className="!p-4 sm:!p-5"
      >
        <div className="grid gap-4 xl:grid-cols-2">
          {miners.length ? (
            miners.map((miner) => (
              <div key={miner.id} className="panel-subtle p-4">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <p className="text-base font-semibold text-white">{miner.name}</p>
                    <div className="mt-1.5 inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.03] px-3 py-1 text-xs text-slate-300">
                      <Wallet className="h-3.5 w-3.5" />
                      <span className="font-mono">{miner.walletAddress}</span>
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <button type="button" className="btn-secondary px-3 py-2 text-sm" onClick={() => startEdit(miner)}>
                      Edit
                    </button>
                    <button type="button" className="btn-danger gap-1.5 px-3 py-2 text-sm" onClick={() => setDeleteTarget(miner)}>
                      <Trash2 className="h-3.5 w-3.5" />
                      Delete
                    </button>
                  </div>
                </div>
                <p className="mt-3 text-sm text-slate-400">{miner.notes || "No notes for this miner."}</p>
                <div className="mt-3">
                  <p className="metric-kicker">Rewarded Credits</p>
                  <p className="mt-1.5 text-xl font-semibold text-emerald-300">{formatTokenAmount(miner.rewardedTokens)}</p>
                </div>
                <div className="mt-3">
                  <p className="metric-kicker">Linked Nodes</p>
                  <div className="mt-2 flex flex-wrap gap-2">
                    {miner.nodes.length ? (
                      miner.nodes.map((node) => (
                        <span key={node.id} className="status-pill">
                          <span className={`h-2 w-2 rounded-full ${node.healthStatus === "online" ? "bg-emerald-400" : "bg-slate-500"}`} />
                          {node.name}
                        </span>
                      ))
                    ) : (
                      <span className="text-sm text-slate-500">No nodes assigned.</span>
                    )}
                  </div>
                </div>
              </div>
            ))
          ) : (
            <div className="panel-subtle p-4 text-sm text-slate-400">No miners found yet.</div>
          )}
        </div>
      </SectionCard>

      <ConfirmDialog
        open={minerDialogOpen}
        title={editingMinerId ? "Edit miner" : "Create miner"}
        description="Save the miner identity, wallet address, and the nodes they operate."
        confirmLabel={editingMinerId ? "Save Miner" : "Create Miner"}
        panelClassName="max-w-3xl"
        onCancel={closeMinerDialog}
        onConfirm={() => {
          const formElement = document.getElementById("miner-form");
          if (formElement instanceof HTMLFormElement) {
            formElement.requestSubmit();
          }
        }}
      >
        <form id="miner-form" className="grid gap-4" onSubmit={submitMiner}>
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <label className="metric-kicker" htmlFor="minerName">
                Miner Name
              </label>
              <input
                id="minerName"
                className="input-shell mt-2"
                value={form.name}
                onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
                placeholder="Miner A"
              />
            </div>
            <div>
              <label className="metric-kicker" htmlFor="walletAddress">
                Wallet Address
              </label>
              <input
                id="walletAddress"
                className="input-shell mt-2"
                value={form.walletAddress}
                onChange={(event) => setForm((current) => ({ ...current, walletAddress: event.target.value }))}
                placeholder="0x..., KBZ wallet id, or payout wallet"
              />
            </div>
          </div>

          <div>
            <label className="metric-kicker" htmlFor="minerNotes">
              Notes
            </label>
            <textarea
              id="minerNotes"
              className="input-shell mt-2 min-h-[120px] resize-y"
              value={form.notes}
              onChange={(event) => setForm((current) => ({ ...current, notes: event.target.value }))}
              placeholder="Payout rules, region, or operator notes."
            />
          </div>

          <div>
            <p className="metric-kicker">Assign Nodes</p>
            <div className="mt-3 grid gap-3 md:grid-cols-2">
              {availableNodes.length ? (
                availableNodes.map(({ node, owner, assignedToDifferentMiner }) => (
                  <label
                    key={node.id}
                    className={`panel-subtle flex items-start gap-3 p-4 text-sm ${
                      assignedToDifferentMiner ? "opacity-60" : ""
                    }`}
                  >
                    <input
                      type="checkbox"
                      className="mt-1 h-4 w-4"
                      checked={form.nodeIds.includes(node.id)}
                      onChange={() => toggleNodeSelection(node.id)}
                      disabled={assignedToDifferentMiner}
                    />
                    <span>
                      <span className="block font-medium text-white">{node.name}</span>
                      <span className="mt-1 block text-slate-400">
                        {node.location || "Unknown location"} · {node.healthStatus}
                        {owner ? ` · owned by ${owner.name}` : ""}
                      </span>
                    </span>
                  </label>
                ))
              ) : (
                <div className="panel-subtle p-4 text-sm text-slate-400">No nodes available yet.</div>
              )}
            </div>
          </div>
        </form>
      </ConfirmDialog>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title="Delete miner?"
        description="This removes the miner record and detaches any linked nodes, but it does not delete the nodes themselves."
        confirmLabel="Delete Miner"
        tone="danger"
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => void deleteMiner()}
      />
    </div>
  );
}

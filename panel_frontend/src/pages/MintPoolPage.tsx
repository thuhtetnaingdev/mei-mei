import { Coins, Landmark, PlusCircle } from "lucide-react";
import type { ReactNode } from "react";
import { FormEvent, useEffect, useState } from "react";
import axios from "axios";
import api from "../api/client";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { SectionCard } from "../components/SectionCard";
import type { MintPoolSnapshot } from "../types";

const defaultSnapshot: MintPoolSnapshot = {
  pool: {
    id: 0,
    totalMmkReserve: 0,
    totalMeiMinted: 0,
    mainWalletBalance: 0,
    adminWalletBalance: 0,
    totalTransferredToUsers: 0,
    totalRewardedToMiners: 0,
    totalAdminCollected: 0,
    lastMintAt: null,
    createdAt: "",
    updatedAt: ""
  },
  history: [],
  transfers: []
};

function normalizeSnapshot(snapshot?: Partial<MintPoolSnapshot> | null): MintPoolSnapshot {
  return {
    pool: snapshot?.pool ?? defaultSnapshot.pool,
    history: Array.isArray(snapshot?.history) ? snapshot.history : [],
    transfers: Array.isArray(snapshot?.transfers) ? snapshot.transfers : []
  };
}

const formatNumber = (value: number) => new Intl.NumberFormat().format(value);
const formatTokenAmount = (value: number) =>
  new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(value);

const parseMMKAmount = (value: string) => {
  const digitsOnly = value.replace(/[^\d]/g, "");
  if (!digitsOnly) {
    return NaN;
  }

  return Number.parseInt(digitsOnly, 10);
};

const formatDate = (value?: string | null) => {
  if (!value) {
    return "No mint yet";
  }

  return new Date(value).toLocaleString();
};

function CompactStat({
  label,
  value,
  tone = "text-white",
  caption
}: {
  label: string;
  value: string;
  tone?: string;
  caption?: string;
}) {
  return (
    <div className="panel-subtle min-w-0 px-3 py-3">
      <p className="metric-kicker">{label}</p>
      <p className={`mt-2 font-display text-2xl font-bold leading-none tracking-tight sm:text-[2rem] ${tone}`}>{value}</p>
      {caption ? <p className="mt-1.5 text-xs text-slate-500">{caption}</p> : null}
    </div>
  );
}

function CompactMeta({
  icon,
  label,
  value
}: {
  icon: ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className="panel-subtle flex items-center gap-3 px-3 py-3">
      <div className="rounded-2xl bg-white/5 p-2.5 text-slate-300">{icon}</div>
      <div className="min-w-0">
        <p className="metric-kicker">{label}</p>
        <p className="mt-1 text-sm text-slate-300">{value}</p>
      </div>
    </div>
  );
}

export function MintPoolPage() {
  const [snapshot, setSnapshot] = useState<MintPoolSnapshot>(defaultSnapshot);
  const [mmkAmount, setMmkAmount] = useState("8000");
  const [note, setNote] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [approvalDialogOpen, setApprovalDialogOpen] = useState(false);
  const [pendingAmount, setPendingAmount] = useState<number | null>(null);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const visibleTransfers = snapshot.transfers.filter((event) => !event.transferType.includes("miner"));

  const loadSnapshot = async () => {
    const response = await api.get<MintPoolSnapshot>("/mint-pool");
    setSnapshot(normalizeSnapshot(response.data));
  };

  useEffect(() => {
    void loadSnapshot().catch(() => {
      setError("We could not load the mint pool right now.");
    });
  }, []);

  const performMint = async (parsedAmount: number) => {
    setSubmitting(true);
    setError("");
    setStatus("");

    try {
      const response = await api.post<MintPoolSnapshot>("/mint-pool/mint", {
        mmkAmount: parsedAmount,
        note,
        approved: true
      });
      setSnapshot(normalizeSnapshot(response.data));
      setStatus(`Minted ${formatNumber(parsedAmount)} Mei from ${formatNumber(parsedAmount)} MMK at a fixed 1:1 rate.`);
      setNote("");
      setApprovalDialogOpen(false);
      setPendingAmount(null);
    } catch (requestError) {
      if (axios.isAxiosError(requestError)) {
        const backendMessage =
          typeof requestError.response?.data?.error === "string" ? requestError.response.data.error : "";
        if (backendMessage) {
          setError(`Mint failed: ${backendMessage}`);
        } else {
          setError(`Mint failed: ${requestError.message}`);
        }
      } else {
        setError("Mint failed. Please try again.");
      }
    } finally {
      setSubmitting(false);
    }
  };

  const submitMint = (event: FormEvent) => {
    event.preventDefault();
    const parsedAmount = parseMMKAmount(mmkAmount);

    if (!Number.isFinite(parsedAmount) || parsedAmount <= 0) {
      setError("Enter a whole MMK amount greater than zero.");
      return;
    }

    setError("");
    setStatus("");
    setPendingAmount(parsedAmount);
    setApprovalDialogOpen(true);
  };

  return (
    <div className="space-y-4">
      <section className="grid gap-3 xl:grid-cols-[minmax(0,1.2fr),minmax(320px,0.8fr)]">
        <SectionCard
          eyebrow="Liquidity Mint"
          title="Internal MMK-backed minting"
          description="Minted Mei lands in the main wallet first. User packages then draw from that wallet, and miner rewards are tracked as downstream transfers."
          className="!p-4 sm:!p-5"
        >
          <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
            <CompactStat label="MMK Reserve" value={formatNumber(snapshot.pool.totalMmkReserve)} />
            <CompactStat label="Mei Minted" value={formatNumber(snapshot.pool.totalMeiMinted)} tone="text-emerald-300" />
            <CompactStat
              label="Main Wallet"
              value={formatTokenAmount(snapshot.pool.mainWalletBalance)}
              tone={snapshot.pool.mainWalletBalance < 0 ? "text-rose-300" : "text-sky-300"}
            />
            <CompactStat label="Admin Wallet" value={formatTokenAmount(snapshot.pool.adminWalletBalance)} tone="text-fuchsia-200" />
            <CompactStat label="Sent To Users" value={formatTokenAmount(snapshot.pool.totalTransferredToUsers)} tone="text-amber-200" />
            <CompactStat label="Events" value={formatNumber(snapshot.history.length)} caption={formatDate(snapshot.pool.lastMintAt)} />
          </div>
          <div className="mt-3 grid gap-2.5 md:grid-cols-2 xl:grid-cols-3">
            <CompactMeta icon={<Coins className="h-4 w-4" />} label="Rewarded To Miners" value={`${formatTokenAmount(snapshot.pool.totalRewardedToMiners)} Mei`} />
            <CompactMeta icon={<Coins className="h-4 w-4" />} label="Admin Collected" value={`${formatTokenAmount(snapshot.pool.totalAdminCollected)} Mei`} />
            <CompactMeta icon={<Landmark className="h-4 w-4" />} label="Last Mint" value={formatDate(snapshot.pool.lastMintAt)} />
          </div>
          {snapshot.pool.mainWalletBalance < 0 ? (
            <div className="mt-3 rounded-2xl border border-rose-400/20 bg-rose-400/10 px-3 py-3 text-sm text-rose-200">
              Main wallet is below zero because older user packages were created before treasury enforcement. Mint more Mei to fund the deficit before assigning new user credits.
            </div>
          ) : null}
        </SectionCard>

        <SectionCard
          eyebrow="Mint Action"
          title="Add MMK and mint Mei"
          description="Example: when the admin confirms 8000 MMK received from KBZ Pay or another mobile payment, the system mints exactly 8000 Mei."
          className="!p-4 sm:!p-5"
        >
          <form className="space-y-3" onSubmit={submitMint}>
            <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr),180px] sm:items-end">
              <div>
              <label className="metric-kicker" htmlFor="mmkAmount">
                MMK Amount
              </label>
              <input
                id="mmkAmount"
                className="input-shell mt-2"
                inputMode="numeric"
                min={1}
                step={1}
                value={mmkAmount}
                onChange={(event) => setMmkAmount(event.target.value)}
                placeholder="8000"
              />
                <p className="mt-2 text-xs text-slate-500">You can enter `8000`, `8,000`, or `8000 MMK`.</p>
              </div>
              <div className="panel-subtle px-3 py-3 text-sm text-slate-300">
                <p className="metric-kicker">Output</p>
                <p className="mt-2 font-medium text-white">{`${formatNumber(parseMMKAmount(mmkAmount || "0") || 0)} Mei`}</p>
              </div>
            </div>

            <div>
              <label className="metric-kicker" htmlFor="mintNote">
                Payment Note
              </label>
              <textarea
                id="mintNote"
                className="input-shell mt-2 min-h-[88px] resize-y"
                value={note}
                onChange={(event) => setNote(event.target.value)}
                placeholder="Optional KBZ Pay transaction ID, phone number, or audit note."
              />
            </div>

            <div className="panel-subtle px-3 py-3 text-sm text-slate-300">
              {`${formatNumber(parseMMKAmount(mmkAmount || "0") || 0)} MMK -> ${formatNumber(parseMMKAmount(mmkAmount || "0") || 0)} Mei`}
            </div>

            {status ? <div className="rounded-2xl border border-emerald-400/20 bg-emerald-400/10 px-4 py-3 text-sm text-emerald-200">{status}</div> : null}
            {error ? <div className="rounded-2xl border border-rose-400/20 bg-rose-400/10 px-4 py-3 text-sm text-rose-200">{error}</div> : null}

            <button type="submit" className="btn-primary w-full justify-center gap-2 sm:w-auto" disabled={submitting}>
              <PlusCircle className="h-4 w-4" />
              {submitting ? "Minting..." : "Review And Mint"}
            </button>
          </form>
        </SectionCard>
      </section>

      <SectionCard
        eyebrow="Mint Ledger"
        title="Recent internal mint history"
        description="Every admin-confirmed MMK payment is stored as a mint event so the reserve and main wallet can be audited."
        className="!p-4 sm:!p-5"
      >
        <div className="hidden overflow-x-auto rounded-[22px] border border-white/10 lg:block">
          <table className="data-table">
            <thead>
              <tr>
                <th>Date</th>
                <th>MMK</th>
                <th>Mei</th>
                <th>Rate</th>
                <th>Note</th>
              </tr>
            </thead>
            <tbody>
              {snapshot.history.length ? (
                snapshot.history.map((event) => (
                  <tr key={event.id}>
                    <td>{formatDate(event.createdAt)}</td>
                    <td>{formatNumber(event.mmkAmount)}</td>
                    <td>{formatNumber(event.meiAmount)}</td>
                    <td>{event.exchangeRate}</td>
                    <td className="max-w-[280px] text-slate-400">{event.note || "No note"}</td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={5} className="text-slate-400">
                    No mint events yet.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        <div className="grid gap-2.5 lg:hidden">
          {snapshot.history.length ? snapshot.history.map((event) => (
            <div key={event.id} className="panel-subtle px-3 py-3">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="text-sm font-semibold text-white">{formatNumber(event.mmkAmount)} MMK</p>
                  <p className="mt-1 text-xs text-slate-500">{formatDate(event.createdAt)}</p>
                </div>
                <span className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-[11px] text-slate-300">{event.exchangeRate}</span>
              </div>
              <div className="mt-3 grid grid-cols-2 gap-2 text-sm">
                <div>
                  <p className="metric-kicker">Mei</p>
                  <p className="mt-1 text-slate-200">{formatNumber(event.meiAmount)}</p>
                </div>
                <div>
                  <p className="metric-kicker">Note</p>
                  <p className="mt-1 break-words text-slate-400">{event.note || "No note"}</p>
                </div>
              </div>
            </div>
          )) : (
            <div className="rounded-2xl border border-dashed border-white/10 px-4 py-4 text-sm text-slate-500">
              No mint events yet.
            </div>
          )}
        </div>
      </SectionCard>
      <SectionCard
        eyebrow="Wallet Transfers"
        title="Recent treasury movements"
        description="These entries show treasury-side wallet movements such as mint funding and user allocation funding."
        className="!p-4 sm:!p-5"
      >
        <div className="hidden overflow-x-auto rounded-[22px] border border-white/10 lg:block">
          <table className="data-table">
            <thead>
              <tr>
                <th>Date</th>
                <th>Type</th>
                <th>From</th>
                <th>To</th>
                <th>Amount</th>
                <th>Note</th>
              </tr>
            </thead>
            <tbody>
              {visibleTransfers.length ? (
                visibleTransfers.map((event) => (
                  <tr key={event.id}>
                    <td>{formatDate(event.createdAt)}</td>
                    <td className="text-slate-300">{event.transferType}</td>
                    <td className="max-w-[180px] break-all font-mono text-xs text-slate-400">{event.fromWallet}</td>
                    <td className="max-w-[180px] break-all font-mono text-xs text-slate-400">{event.toWallet}</td>
                    <td>{formatTokenAmount(event.amount)}</td>
                    <td className="max-w-[260px] text-slate-400">{event.note || "No note"}</td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={6} className="text-slate-400">
                    No treasury wallet transfers yet.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        <div className="grid gap-2.5 lg:hidden">
          {visibleTransfers.length ? visibleTransfers.map((event) => (
            <div key={event.id} className="panel-subtle px-3 py-3">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="text-sm font-semibold capitalize text-white">{event.transferType}</p>
                  <p className="mt-1 text-xs text-slate-500">{formatDate(event.createdAt)}</p>
                </div>
                <p className="text-sm font-semibold text-sky-300">{formatTokenAmount(event.amount)}</p>
              </div>
              <div className="mt-3 grid gap-2 text-xs text-slate-400">
                <div>
                  <p className="metric-kicker">From</p>
                  <p className="mt-1 break-all font-mono">{event.fromWallet}</p>
                </div>
                <div>
                  <p className="metric-kicker">To</p>
                  <p className="mt-1 break-all font-mono">{event.toWallet}</p>
                </div>
                <div>
                  <p className="metric-kicker">Note</p>
                  <p className="mt-1 break-words">{event.note || "No note"}</p>
                </div>
              </div>
            </div>
          )) : (
            <div className="rounded-2xl border border-dashed border-white/10 px-4 py-4 text-sm text-slate-500">
              No treasury wallet transfers yet.
            </div>
          )}
        </div>
      </SectionCard>
      <ConfirmDialog
        open={approvalDialogOpen}
        title="Approve mint?"
        description="Confirm that the MMK payment has been checked before minting Mei into the internal ledger."
        confirmLabel={submitting ? "Minting..." : "Approve Mint"}
        cancelLabel="Cancel"
        onCancel={() => {
          if (!submitting) {
            setApprovalDialogOpen(false);
            setPendingAmount(null);
          }
        }}
        onConfirm={() => {
          if (!submitting && pendingAmount) {
            void performMint(pendingAmount);
          }
        }}
      >
        <div className="space-y-3">
          <div className="panel-subtle p-4 text-sm text-slate-300">
            {`${formatNumber(pendingAmount || 0)} MMK -> ${formatNumber(pendingAmount || 0)} Mei`}
          </div>
          <div className="panel-subtle p-4 text-sm text-slate-300">
            <p className="metric-kicker">Payment Note</p>
            <p className="mt-2 whitespace-pre-wrap break-words text-slate-300">{note || "No note provided"}</p>
          </div>
        </div>
      </ConfirmDialog>
    </div>
  );
}

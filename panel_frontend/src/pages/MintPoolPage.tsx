import { Coins, Landmark, PlusCircle, Scale } from "lucide-react";
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
    setSnapshot(response.data);
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
      setSnapshot(response.data);
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
      <section className="grid gap-4 xl:grid-cols-[minmax(0,1.05fr),minmax(320px,0.95fr)]">
        <SectionCard
          eyebrow="Liquidity Mint"
          title="Internal MMK-backed minting"
          description="Minted Mei lands in the main wallet first. User packages then draw from that wallet, and miner rewards are tracked as downstream transfers."
        >
          <div className="grid gap-3 md:grid-cols-2">
            <div className="panel-subtle min-w-0 p-4">
              <p className="metric-kicker">MMK Reserve</p>
              <p className="mt-3 font-display text-[clamp(2rem,5vw,3.2rem)] font-bold leading-none text-white">{formatNumber(snapshot.pool.totalMmkReserve)}</p>
            </div>
            <div className="panel-subtle min-w-0 p-4">
              <p className="metric-kicker">Mei Minted</p>
              <p className="mt-3 font-display text-[clamp(2rem,5vw,3.2rem)] font-bold leading-none text-emerald-300">{formatNumber(snapshot.pool.totalMeiMinted)}</p>
            </div>
            <div className="panel-subtle min-w-0 p-4">
              <p className="metric-kicker">Main Wallet</p>
              <p className={`mt-3 font-display text-[clamp(1.9rem,4vw,3rem)] font-bold leading-none tracking-tight ${snapshot.pool.mainWalletBalance < 0 ? "text-rose-300" : "text-sky-300"}`}>
                {formatTokenAmount(snapshot.pool.mainWalletBalance)}
              </p>
            </div>
            <div className="panel-subtle min-w-0 p-4">
              <p className="metric-kicker">Admin Wallet</p>
              <p className="mt-3 font-display text-[clamp(1.9rem,4vw,3rem)] font-bold leading-none tracking-tight text-fuchsia-200">
                {formatTokenAmount(snapshot.pool.adminWalletBalance)}
              </p>
            </div>
            <div className="panel-subtle min-w-0 p-4">
              <p className="metric-kicker">Sent To Users</p>
              <p className="mt-3 font-display text-[clamp(1.9rem,4vw,3rem)] font-bold leading-none tracking-tight text-amber-200">
                {formatTokenAmount(snapshot.pool.totalTransferredToUsers)}
              </p>
            </div>
          </div>
          <div className="mt-5 grid gap-3 md:grid-cols-2">
            <div className="panel-subtle flex items-start gap-3 p-4">
              <div className="rounded-2xl bg-emerald-500/10 p-3 text-emerald-300">
                <Coins className="h-5 w-5" />
              </div>
              <div className="min-w-0">
                <p className="metric-kicker">Rewarded To Miners</p>
                <p className="mt-1 text-sm text-slate-300">{formatTokenAmount(snapshot.pool.totalRewardedToMiners)} Mei</p>
              </div>
            </div>
            <div className="panel-subtle flex items-start gap-3 p-4">
              <div className="rounded-2xl bg-fuchsia-400/10 p-3 text-fuchsia-200">
                <Coins className="h-5 w-5" />
              </div>
              <div className="min-w-0">
                <p className="metric-kicker">Admin Collected</p>
                <p className="mt-1 text-sm text-slate-300">{formatTokenAmount(snapshot.pool.totalAdminCollected)} Mei</p>
              </div>
            </div>
            <div className="panel-subtle flex items-start gap-3 p-4">
              <div className="rounded-2xl bg-amber-300/10 p-3 text-amber-200">
                <Landmark className="h-5 w-5" />
              </div>
              <div className="min-w-0">
                <p className="metric-kicker">Last Mint</p>
                <p className="mt-1 text-sm text-slate-300">{formatDate(snapshot.pool.lastMintAt)}</p>
              </div>
            </div>
            <div className="panel-subtle flex items-start gap-3 p-4">
              <div className="rounded-2xl bg-emerald-400/10 p-3 text-emerald-300">
                <Coins className="h-5 w-5" />
              </div>
              <div className="min-w-0">
                <p className="metric-kicker">Mint Events</p>
                <p className="mt-1 text-sm text-slate-300">{formatNumber(snapshot.history.length)}</p>
              </div>
            </div>
            <div className="panel-subtle flex items-start gap-3 p-4">
              <div className="rounded-2xl bg-sky-400/10 p-3 text-sky-300">
                <Scale className="h-5 w-5" />
              </div>
              <div className="min-w-0">
                <p className="metric-kicker">Rule</p>
                <p className="mt-1 text-sm leading-6 text-slate-300">Wallet funds users, users fund miners.</p>
              </div>
            </div>
          </div>
          {snapshot.pool.mainWalletBalance < 0 ? (
            <div className="mt-5 rounded-2xl border border-rose-400/20 bg-rose-400/10 px-4 py-3 text-sm text-rose-200">
              Main wallet is below zero because older user packages were created before treasury enforcement. Mint more Mei to fund the deficit before assigning new user tokens.
            </div>
          ) : null}
        </SectionCard>

        <SectionCard
          eyebrow="Mint Action"
          title="Add MMK and mint Mei"
          description="Example: when the admin confirms 8000 MMK received from KBZ Pay or another mobile payment, the system mints exactly 8000 Mei."
        >
          <form className="space-y-4" onSubmit={submitMint}>
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

            <div>
              <label className="metric-kicker" htmlFor="mintNote">
                Payment Note
              </label>
              <textarea
                id="mintNote"
                className="input-shell mt-2 min-h-[120px] resize-y"
                value={note}
                onChange={(event) => setNote(event.target.value)}
                placeholder="Optional KBZ Pay transaction ID, phone number, or audit note."
              />
            </div>

            <div className="panel-subtle p-4 text-sm text-slate-300">
              {`${formatNumber(parseMMKAmount(mmkAmount || "0") || 0)} MMK -> ${formatNumber(parseMMKAmount(mmkAmount || "0") || 0)} Mei`}
            </div>

            {status ? <div className="rounded-2xl border border-emerald-400/20 bg-emerald-400/10 px-4 py-3 text-sm text-emerald-200">{status}</div> : null}
            {error ? <div className="rounded-2xl border border-rose-400/20 bg-rose-400/10 px-4 py-3 text-sm text-rose-200">{error}</div> : null}

            <button type="submit" className="btn-primary gap-2" disabled={submitting}>
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
      >
        <div className="overflow-x-auto">
          <table className="data-table">
            <thead>
              <tr>
                <th>Date</th>
                <th>MMK Added</th>
                <th>Mei Minted</th>
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
                    <td className="max-w-[320px] text-slate-400">{event.note || "No note"}</td>
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
      </SectionCard>
      <SectionCard
        eyebrow="Wallet Transfers"
        title="Recent treasury movements"
        description="These entries show treasury-side wallet movements such as mint funding and user allocation funding."
      >
        <div className="overflow-x-auto">
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
                    <td className="font-mono text-xs text-slate-400">{event.fromWallet}</td>
                    <td className="font-mono text-xs text-slate-400">{event.toWallet}</td>
                    <td>{formatTokenAmount(event.amount)}</td>
                    <td className="max-w-[320px] text-slate-400">{event.note || "No note"}</td>
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

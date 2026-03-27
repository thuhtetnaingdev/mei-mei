import type { KeyVerificationResult } from "../types";
import { createPortal } from "react-dom";
import {
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Clock,
  Shield,
  Key,
  RefreshCw,
  RotateCw,
} from "lucide-react";

interface KeyStatusModalProps {
  open: boolean;
  result: KeyVerificationResult | null;
  onClose: () => void;
  onVerify?: () => void;
  onFix?: () => void;
  onForceRotate?: () => void;
  isVerifying?: boolean;
  isFixing?: boolean;
  isRotating?: boolean;
}

export function KeyStatusModal({
  open,
  result,
  onClose,
  onVerify,
  onFix,
  onForceRotate,
  isVerifying = false,
  isFixing = false,
  isRotating = false,
}: KeyStatusModalProps) {
  if (!open || !result) {
    return null;
  }

  const allMatch = result.publicKeyMatch && result.shortIDMatch;
  const isVerified = result.status === "verified";
  const statusTone = allMatch ? "text-emerald-300" : "text-amber-200";
  const statusBg = allMatch
    ? "bg-emerald-400/10 border-emerald-400/15"
    : "bg-amber-300/10 border-amber-300/15";
  const StatusIcon = allMatch ? CheckCircle2 : AlertTriangle;
  const statusLabel = isVerified
    ? "Verified"
    : result.status === "mismatch"
      ? "Mismatch Detected"
      : result.status;

  const formatKey = (key: string) => {
    if (!key || key.length <= 8) {
      return key || "N/A";
    }
    return `${key.slice(0, 4)}...${key.slice(-4)}`;
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
      minute: "2-digit",
    }).format(new Date(value));
  };

  const MatchIndicator = ({ match }: { match: boolean }) => (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-semibold ${
        match
          ? "bg-emerald-400/10 text-emerald-300"
          : "bg-rose-400/10 text-rose-300"
      }`}
    >
      {match ? (
        <CheckCircle2 className="h-3.5 w-3.5" />
      ) : (
        <XCircle className="h-3.5 w-3.5" />
      )}
      {match ? "Match" : "Mismatch"}
    </span>
  );

  return createPortal(
    <div
      className="fixed inset-0 z-[100] flex items-start justify-center bg-slate-950/75 px-3 pb-4 pt-4 backdrop-blur-md sm:px-4 sm:pb-6 sm:pt-10 md:items-center md:py-6"
      onClick={onClose}
    >
      <div
        className="relative max-h-[calc(100vh-2rem)] w-full max-w-3xl overflow-hidden rounded-[28px] border border-white/10 bg-[#0d172b] p-4 shadow-panel sm:max-h-[calc(100vh-3rem)] sm:p-6"
        onClick={(event) => event.stopPropagation()}
      >
        <button
          type="button"
          onClick={onClose}
          className="absolute right-4 top-4 rounded-full p-2 text-slate-500 transition hover:bg-white/5 hover:text-slate-200"
          aria-label="Close"
        >
          <svg
            className="h-5 w-5"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M6 18L18 6M6 6l12 12"
            />
          </svg>
        </button>

        <div className="flex items-start gap-3">
          <div className={`rounded-xl border p-2 ${statusBg}`}>
            <StatusIcon className={`h-5 w-5 ${statusTone}`} />
          </div>
          <div className="min-w-0 flex-1">
            <h3 className="font-display text-xl font-semibold text-white sm:text-2xl">
              {result.nodeName} - Key Status
            </h3>
            <p className={`mt-1 text-sm font-medium ${statusTone}`}>
              {statusLabel}
            </p>
          </div>
        </div>

        <div className="mt-4 grid gap-3 sm:grid-cols-2">
          <div className="rounded-[18px] border border-white/10 bg-white/[0.03] p-3">
            <div className="flex items-center gap-2">
              <Shield className="h-4 w-4 text-sky-300" />
              <p className="metric-kicker">Public Key</p>
            </div>
            <div className="mt-2 space-y-2">
              <div>
                <p className="text-xs text-slate-500">Panel Public Key</p>
                <p className="mt-0.5 font-mono text-sm text-slate-200">
                  {formatKey(result.panelPublicKey)}
                </p>
                <MatchIndicator match={result.publicKeyMatch} />
              </div>
              <div className="border-t border-white/8 pt-2">
                <p className="text-xs text-slate-500">Node Public Key</p>
                <p className="mt-0.5 font-mono text-sm text-slate-200">
                  {formatKey(result.nodePublicKey)}
                </p>
              </div>
            </div>
          </div>

          <div className="rounded-[18px] border border-white/10 bg-white/[0.03] p-3">
            <div className="flex items-center gap-2">
              <Key className="h-4 w-4 text-violet-300" />
              <p className="metric-kicker">Short ID</p>
            </div>
            <div className="mt-2 space-y-2">
              <div>
                <p className="text-xs text-slate-500">Panel Short ID</p>
                <p className="mt-0.5 font-mono text-sm text-slate-200">
                  {result.panelShortId || "N/A"}
                </p>
                <MatchIndicator match={result.shortIDMatch} />
              </div>
              <div className="border-t border-white/8 pt-2">
                <p className="text-xs text-slate-500">Node Short ID</p>
                <p className="mt-0.5 font-mono text-sm text-slate-200">
                  {result.nodeShortId || "N/A"}
                </p>
              </div>
            </div>
          </div>
        </div>

        <div className="mt-4 rounded-[18px] border border-white/10 bg-white/[0.03] p-3">
          <div className="flex items-center gap-2">
            <Clock className="h-4 w-4 text-slate-400" />
            <p className="metric-kicker">Verification Timeline</p>
          </div>
          <div className="mt-3 grid gap-3 sm:grid-cols-3">
            <div className="rounded-[16px] border border-white/10 bg-slate-950/28 px-3 py-2.5">
              <p className="text-xs text-slate-500">Last Verification</p>
              <p className="mt-1 text-sm font-semibold text-slate-200">
                {formatDateTime(result.verifiedAt)}
              </p>
            </div>
            <div className="rounded-[16px] border border-white/10 bg-slate-950/28 px-3 py-2.5">
              <p className="text-xs text-slate-500">Mismatch Detected</p>
              <p className="mt-1 text-sm font-semibold text-slate-200">
                {result.autoFixTriggered
                  ? "Auto-fix in progress"
                  : "No mismatch"}
              </p>
            </div>
            <div className="rounded-[16px] border border-white/10 bg-slate-950/28 px-3 py-2.5">
              <p className="text-xs text-slate-500">Auto-Fix Status</p>
              <p className="mt-1 text-sm font-semibold text-slate-200">
                {result.autoFixSuccess
                  ? "Success"
                  : result.autoFixTriggered
                    ? "In progress"
                    : "Not attempted"}
              </p>
            </div>
          </div>
        </div>

        {result.error ? (
          <div className="mt-4 rounded-[18px] border border-rose-400/15 bg-rose-400/10 px-3 py-2.5 text-sm text-rose-200">
            <p className="font-semibold">Verification Error</p>
            <p className="mt-1 text-xs text-rose-300">{result.error}</p>
          </div>
        ) : null}

        <div className="mt-6 flex flex-wrap justify-end gap-3">
          {!allMatch && onFix ? (
            <button
              type="button"
              onClick={onFix}
              disabled={isFixing}
              className="btn-primary gap-1.5"
            >
              <RefreshCw
                className={`h-4 w-4 ${isFixing ? "animate-spin" : ""}`}
              />
              {isFixing ? "Fixing Keys..." : "Auto-Fix Keys"}
            </button>
          ) : null}
          {onForceRotate ? (
            <button
              type="button"
              onClick={onForceRotate}
              disabled={isRotating}
              className="btn-secondary gap-1.5 border-violet-400/30 text-violet-200 hover:bg-violet-400/10 hover:text-violet-100"
            >
              <RotateCw className={`h-4 w-4 ${isRotating ? "animate-spin" : ""}`} />
              {isRotating ? "Rotating Keys..." : "Force Rotate Keys"}
            </button>
          ) : null}
          {onVerify ? (
            <button
              type="button"
              onClick={onVerify}
              disabled={isVerifying}
              className="btn-secondary gap-1.5"
            >
              <CheckCircle2 className="h-4 w-4" />
              {isVerifying ? "Verifying..." : "Re-verify"}
            </button>
          ) : null}
          <button type="button" onClick={onClose} className="btn-secondary">
            Close
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

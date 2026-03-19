import type { ReactNode } from "react";
import { createPortal } from "react-dom";

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  description: string;
  confirmLabel?: string;
  cancelLabel?: string;
  tone?: "neutral" | "danger";
  hideActions?: boolean;
  panelClassName?: string;
  onConfirm: () => void;
  onCancel: () => void;
  children?: ReactNode;
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  tone = "neutral",
  hideActions = false,
  panelClassName = "",
  onConfirm,
  onCancel,
  children
}: ConfirmDialogProps) {
  if (!open) {
    return null;
  }

  const panelWidthClass = panelClassName || "max-w-md";

  const confirmClasses =
    tone === "danger"
      ? "bg-rose-500 text-white hover:bg-rose-600"
      : "bg-ink text-white hover:bg-slate-800";

  return createPortal(
    <div
      className="fixed inset-0 z-[100] flex items-start justify-center bg-slate-950/55 px-4 pb-6 pt-16 backdrop-blur-md md:items-center md:py-6"
      onClick={onCancel}
    >
      <div
        className={`relative w-full ${panelWidthClass} rounded-[2rem] border border-white/70 bg-white p-6 shadow-panel`.trim()}
        onClick={(event) => event.stopPropagation()}
      >
        <button
          type="button"
          onClick={onCancel}
          className="absolute right-4 top-4 rounded-full p-2 text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          aria-label="Close"
        >
          <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
        <h3 className="pr-10 font-display text-2xl font-semibold text-ink">{title}</h3>
        <p className="mt-3 text-sm leading-6 text-slate-600">{description}</p>
        {children ? <div className="mt-4">{children}</div> : null}
        {hideActions ? null : (
          <div className="mt-6 flex justify-end gap-3">
            <button
              type="button"
              onClick={onCancel}
              className="rounded-2xl border border-slate-200 px-4 py-2.5 text-sm font-semibold text-slate-700 transition hover:bg-slate-50"
            >
              {cancelLabel}
            </button>
            <button
              type="button"
              onClick={onConfirm}
              className={`rounded-2xl px-4 py-2.5 text-sm font-semibold transition ${confirmClasses}`}
            >
              {confirmLabel}
            </button>
          </div>
        )}
      </div>
    </div>,
    document.body
  );
}

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
      ? "btn-danger"
      : "btn-primary";

  return createPortal(
    <div
      className="fixed inset-0 z-[100] flex items-start justify-center bg-slate-950/75 px-4 pb-6 pt-12 backdrop-blur-md md:items-center md:py-6"
      onClick={onCancel}
    >
      <div
        className={`relative w-full ${panelWidthClass} rounded-[28px] border border-white/10 bg-[#0d172b] p-6 shadow-panel`.trim()}
        onClick={(event) => event.stopPropagation()}
      >
        <button
          type="button"
          onClick={onCancel}
          className="absolute right-4 top-4 rounded-full p-2 text-slate-500 transition hover:bg-white/5 hover:text-slate-200"
          aria-label="Close"
        >
          <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
        <h3 className="pr-10 font-display text-2xl font-semibold text-white">{title}</h3>
        <p className="mt-3 text-sm leading-6 text-slate-400">{description}</p>
        {children ? <div className="mt-4">{children}</div> : null}
        {hideActions ? null : (
          <div className="mt-6 flex flex-wrap justify-end gap-3">
            <button
              type="button"
              onClick={onCancel}
              className="btn-secondary"
            >
              {cancelLabel}
            </button>
            <button
              type="button"
              onClick={onConfirm}
              className={`transition ${confirmClasses}`}
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

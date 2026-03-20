import type { ReactNode } from "react";

interface Props {
  label: string;
  value: string | number;
  icon: ReactNode;
  accent: string;
  detail?: string;
}

export function StatCard({ label, value, icon, accent, detail }: Props) {
  return (
    <div className="panel-surface p-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-slate-400">{label}</p>
          <p className="mt-3 font-display text-3xl font-bold text-white">{value}</p>
          {detail ? <p className="mt-2 text-xs uppercase tracking-[0.18em] text-slate-500">{detail}</p> : null}
        </div>
        <div className={`rounded-2xl border border-white/10 p-3 shadow-glow ${accent}`}>{icon}</div>
      </div>
    </div>
  );
}

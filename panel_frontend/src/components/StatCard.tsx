import type { ReactNode } from "react";

interface Props {
  label: string;
  value: string | number;
  icon: ReactNode;
  accent: string;
}

export function StatCard({ label, value, icon, accent }: Props) {
  return (
    <div className="rounded-3xl border border-white/70 bg-white/80 p-6 shadow-panel backdrop-blur">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm text-slate-500">{label}</p>
          <p className="mt-3 font-display text-3xl font-bold text-ink">{value}</p>
        </div>
        <div className={`rounded-2xl p-3 ${accent}`}>{icon}</div>
      </div>
    </div>
  );
}

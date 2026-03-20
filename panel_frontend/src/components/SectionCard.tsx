import type { ReactNode } from "react";

interface Props {
  title: string;
  description?: string;
  children: ReactNode;
  action?: ReactNode;
  eyebrow?: string;
  className?: string;
}

export function SectionCard({ title, description, children, action, eyebrow, className = "" }: Props) {
  return (
    <section className={`panel-surface p-5 sm:p-6 ${className}`.trim()}>
      <div className="mb-5 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          {eyebrow ? <p className="metric-kicker">{eyebrow}</p> : null}
          <h2 className="mt-2 font-display text-2xl font-semibold text-white">{title}</h2>
          {description ? <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-400">{description}</p> : null}
        </div>
        {action}
      </div>
      {children}
    </section>
  );
}

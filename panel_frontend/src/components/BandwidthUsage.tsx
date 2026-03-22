interface BandwidthUsageProps {
  usedBytes: number;
  limitGb: number;
  showDetails?: boolean;
}

export function BandwidthUsage({ usedBytes, limitGb, showDetails = true }: BandwidthUsageProps) {
  const limitBytes = limitGb * 1024 * 1024 * 1024;
  const percentage = limitBytes > 0 ? (usedBytes / limitBytes) * 100 : 0;
  const isExceeded = usedBytes > limitBytes;
  const isUnlimited = limitGb === 0;

  // Format bytes to human-readable format
  const formatBytes = (bytes: number) => {
    if (bytes === 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${units[i]}`;
  };

  // Determine progress bar color based on usage
  const getProgressColor = () => {
    if (isUnlimited) return "bg-slate-400";
    if (isExceeded) return "bg-rose-500";
    if (percentage >= 90) return "bg-rose-500";
    if (percentage >= 70) return "bg-amber-400";
    return "bg-sky-400";
  };

  if (isUnlimited) {
    return (
      <div className="min-w-0 space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
          <span className="font-medium text-slate-200">{formatBytes(usedBytes)} used</span>
          <span className="text-slate-500">Unlimited</span>
        </div>
        <div className="h-2 w-full rounded-full bg-white/10">
          <div className="h-2 rounded-full bg-slate-400" style={{ width: "100%" }} />
        </div>
        {showDetails && (
          <p className="text-xs text-slate-500">No bandwidth limit set for this user</p>
        )}
      </div>
    );
  }

  return (
    <div className="min-w-0 space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
        <span className={`font-medium ${isExceeded ? "text-rose-400" : "text-slate-200"}`}>
          {formatBytes(usedBytes)} used
        </span>
        <span className={`text-right ${isExceeded ? "font-semibold text-rose-400" : "text-slate-400"}`}>
          {limitGb} GB limit
          {isExceeded && " (Exceeded!)"}
        </span>
      </div>
      <div className="h-2 w-full rounded-full bg-white/10">
        <div
          className={`h-2 rounded-full transition-all duration-300 ${getProgressColor()}`}
          style={{ width: `${Math.min(percentage, 100)}%` }}
        />
      </div>
      {showDetails && (
        <p className={`text-xs ${isExceeded ? "font-medium text-rose-400" : "text-slate-500"}`}>
          {percentage.toFixed(1)}% used
          {isExceeded && ` • ${formatBytes(usedBytes - limitBytes)} over limit`}
        </p>
      )}
    </div>
  );
}

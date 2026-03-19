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
    if (isUnlimited) return "bg-slate-300";
    if (isExceeded) return "bg-rose-500";
    if (percentage >= 90) return "bg-rose-500";
    if (percentage >= 70) return "bg-amber-500";
    return "bg-tide";
  };

  if (isUnlimited) {
    return (
      <div className="space-y-1">
        <div className="flex items-center justify-between text-xs">
          <span className="font-medium text-slate-600">{formatBytes(usedBytes)} used</span>
          <span className="text-slate-400">Unlimited</span>
        </div>
        <div className="h-2 w-full rounded-full bg-slate-200">
          <div className="h-2 rounded-full bg-slate-300" style={{ width: "100%" }} />
        </div>
        {showDetails && (
          <p className="text-xs text-slate-400">No bandwidth limit set for this user</p>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-xs">
        <span className={`font-medium ${isExceeded ? "text-rose-600" : "text-slate-600"}`}>
          {formatBytes(usedBytes)} used
        </span>
        <span className={`${isExceeded ? "text-rose-600 font-semibold" : "text-slate-500"}`}>
          {limitGb} GB limit
          {isExceeded && " (Exceeded!)"}
        </span>
      </div>
      <div className="h-2 w-full rounded-full bg-slate-200">
        <div
          className={`h-2 rounded-full transition-all duration-300 ${getProgressColor()}`}
          style={{ width: `${Math.min(percentage, 100)}%` }}
        />
      </div>
      {showDetails && (
        <p className={`text-xs ${isExceeded ? "text-rose-500 font-medium" : "text-slate-400"}`}>
          {percentage.toFixed(1)}% used
          {isExceeded && ` • ${formatBytes(usedBytes - limitBytes)} over limit`}
        </p>
      )}
    </div>
  );
}

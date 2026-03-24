import { Search, X, Filter, ChevronDown, ChevronUp } from "lucide-react";
import { useEffect, useState } from "react";
import type { UserListOptions, UserType } from "../types";

interface UserFiltersProps {
  filters: UserListOptions;
  onFiltersChange: (filters: UserListOptions) => void;
  onReset: () => void;
  totalResults: number;
  onSearchChange?: (search: string | undefined) => void;
}

export function UserFilters({ filters, onFiltersChange, onReset, totalResults, onSearchChange }: UserFiltersProps) {
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [localSearch, setLocalSearch] = useState(filters.search || "");
  const hasActiveFilters = checkHasActiveFilters(filters);

  // Sync local search with filters when filters.search changes from outside (e.g., reset, other filters)
  useEffect(() => {
    setLocalSearch(filters.search || "");
  }, [filters.search]);

  const updateFilter = <K extends keyof UserListOptions>(key: K, value: UserListOptions[K]) => {
    onFiltersChange({ ...filters, [key]: value });
  };

  const clearFilter = <K extends keyof UserListOptions>(key: K) => {
    onFiltersChange({ ...filters, [key]: undefined });
  };

  const handleSearchChange = (value: string | undefined) => {
    setLocalSearch(value || "");
    if (onSearchChange) {
      onSearchChange(value);
    } else {
      updateFilter("search", value);
    }
  };

  const handleClearSearch = () => {
    if (onSearchChange) {
      onSearchChange(undefined);
    } else {
      clearFilter("search");
    }
  };

  return (
    <div className="space-y-3">
      {/* Search and main filters */}
      <div className="flex flex-col gap-3 md:flex-row md:items-center">
        <div className="relative w-full md:flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
          <input
            type="text"
            value={localSearch}
            onChange={(e) => handleSearchChange(e.target.value || undefined)}
            placeholder="Search by email, UUID, or notes..."
            className="input-shell pl-10"
          />
          {localSearch && (
            <button
              onClick={handleClearSearch}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300"
              aria-label="Clear search"
            >
              <X className="h-4 w-4" />
            </button>
          )}
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <select
            value={filters.enabled === undefined ? "" : filters.enabled ? "true" : "false"}
            onChange={(e) => {
              const value = e.target.value;
              updateFilter("enabled", value === "" ? undefined : value === "true");
            }}
            className="input-shell w-full min-w-[140px] flex-1 sm:w-auto"
          >
            <option value="">All Status</option>
            <option value="true">Enabled</option>
            <option value="false">Disabled</option>
          </select>

          <select
            value={filters.userType || ""}
            onChange={(e) => updateFilter("userType", (e.target.value as UserType) || undefined)}
            className="input-shell w-full min-w-[140px] flex-1 sm:w-auto"
          >
            <option value="">All Types</option>
            <option value="light">Light</option>
            <option value="medium">Medium</option>
            <option value="moderate">Moderate</option>
            <option value="unknown">Unknown</option>
          </select>

          <label className="flex items-center gap-2 rounded-2xl border border-white/10 bg-white/[0.03] px-3 py-2 text-sm whitespace-nowrap">
            <input
              type="checkbox"
              checked={filters.isTesting || false}
              onChange={(e) => updateFilter("isTesting", e.target.checked || undefined)}
              className="h-4 w-4 rounded border-white/20 bg-transparent"
            />
            <span className="text-slate-300">Testing</span>
          </label>

          <button
            onClick={() => setAdvancedOpen(!advancedOpen)}
            className="flex items-center gap-2 rounded-2xl border border-white/10 bg-white/[0.03] px-3 py-2 text-sm text-slate-300 transition-colors hover:bg-white/[0.08] whitespace-nowrap"
          >
            <Filter className="h-4 w-4" />
            <span>Advanced</span>
            {advancedOpen ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
          </button>

          {hasActiveFilters && (
            <button
              onClick={onReset}
              className="flex items-center gap-2 rounded-2xl border border-rose-400/20 bg-rose-400/10 px-3 py-2 text-sm text-rose-300 transition-colors hover:bg-rose-400/20 whitespace-nowrap"
            >
              <X className="h-4 w-4" />
              <span>Reset</span>
            </button>
          )}
        </div>
      </div>

      {/* Results count */}
      <div className="flex items-center gap-2 text-sm text-slate-400">
        <span>Found</span>
        <span className="font-semibold text-white">{totalResults}</span>
        <span>user(s)</span>
        {hasActiveFilters && (
          <span className="rounded-full bg-sky-500/10 px-2 py-0.5 text-xs text-sky-300">
            filtered
          </span>
        )}
      </div>

      {/* Advanced filters */}
      {advancedOpen && (
        <div className="panel-subtle rounded-2xl p-4">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">
                Min Bandwidth (GB)
              </label>
              <input
                type="number"
                min={0}
                value={filters.minBandwidthGb ?? ""}
                onChange={(e) => updateFilter("minBandwidthGb", e.target.value ? Number(e.target.value) : null)}
                placeholder="Any"
                className="input-shell"
              />
            </div>

            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">
                Max Bandwidth (GB)
              </label>
              <input
                type="number"
                min={0}
                value={filters.maxBandwidthGb ?? ""}
                onChange={(e) => updateFilter("maxBandwidthGb", e.target.value ? Number(e.target.value) : null)}
                placeholder="Any"
                className="input-shell"
              />
            </div>

            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">
                Min Usage (GB)
              </label>
              <input
                type="number"
                min={0}
                value={filters.minUsageBytes !== null && filters.minUsageBytes !== undefined 
                  ? Math.round(filters.minUsageBytes / (1024 ** 3)) 
                  : ""}
                onChange={(e) => updateFilter("minUsageBytes", e.target.value 
                  ? Number(e.target.value) * (1024 ** 3) 
                  : null)}
                placeholder="Any"
                className="input-shell"
              />
            </div>

            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">
                Max Usage (GB)
              </label>
              <input
                type="number"
                min={0}
                value={filters.maxUsageBytes !== null && filters.maxUsageBytes !== undefined 
                  ? Math.round(filters.maxUsageBytes / (1024 ** 3)) 
                  : ""}
                onChange={(e) => updateFilter("maxUsageBytes", e.target.value 
                  ? Number(e.target.value) * (1024 ** 3) 
                  : null)}
                placeholder="Any"
                className="input-shell"
              />
            </div>

            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">
                Min Token Balance
              </label>
              <input
                type="number"
                min={0}
                step="0.01"
                value={filters.minTokenBalance ?? ""}
                onChange={(e) => updateFilter("minTokenBalance", e.target.value ? Number(e.target.value) : null)}
                placeholder="Any"
                className="input-shell"
              />
            </div>

            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">
                Max Token Balance
              </label>
              <input
                type="number"
                min={0}
                step="0.01"
                value={filters.maxTokenBalance ?? ""}
                onChange={(e) => updateFilter("maxTokenBalance", e.target.value ? Number(e.target.value) : null)}
                placeholder="Any"
                className="input-shell"
              />
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function checkHasActiveFilters(filters: UserListOptions): boolean {
  return (
    filters.search !== undefined ||
    filters.enabled !== undefined ||
    filters.isTesting !== undefined ||
    filters.userType !== undefined ||
    filters.minBandwidthGb !== undefined ||
    filters.maxBandwidthGb !== undefined ||
    filters.minUsageBytes !== undefined ||
    filters.maxUsageBytes !== undefined ||
    filters.minTokenBalance !== undefined ||
    filters.maxTokenBalance !== undefined ||
    filters.createdAfter !== undefined ||
    filters.createdBefore !== undefined ||
    filters.sortBy !== undefined ||
    filters.sortOrder !== undefined
  );
}

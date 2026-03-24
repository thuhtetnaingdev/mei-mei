import { ChevronLeft, ChevronRight } from "lucide-react";
import type { PaginationMeta } from "../types";

interface PaginationProps {
  meta: PaginationMeta;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
}

const PAGE_SIZE_OPTIONS = [20, 50, 100];

export function Pagination({ meta, onPageChange, onPageSizeChange }: PaginationProps) {
  const { total, page, pageSize, totalPages } = meta;

  const startItem = (page - 1) * pageSize + 1;
  const endItem = Math.min(page * pageSize, total);

  // Generate page numbers to display
  const getPageNumbers = () => {
    const pages: (number | string)[] = [];
    const maxVisible = 5;

    if (totalPages <= maxVisible) {
      // Show all pages
      for (let i = 1; i <= totalPages; i++) {
        pages.push(i);
      }
    } else {
      // Show first page
      pages.push(1);

      if (page > 3) {
        pages.push("...");
      }

      // Show pages around current page
      for (let i = Math.max(2, page - 1); i <= Math.min(totalPages - 1, page + 1); i++) {
        if (!pages.includes(i)) {
          pages.push(i);
        }
      }

      if (page < totalPages - 2) {
        pages.push("...");
      }

      // Show last page
      if (!pages.includes(totalPages)) {
        pages.push(totalPages);
      }
    }

    return pages;
  };

  if (totalPages <= 1) {
    return null;
  }

  return (
    <div className="flex flex-col items-center justify-between gap-4 border-t border-white/10 pt-4 sm:flex-row">
      <div className="flex items-center gap-2 text-sm text-slate-400">
        <span>Showing</span>
        <span className="font-medium text-white">{startItem}</span>
        <span>-</span>
        <span className="font-medium text-white">{endItem}</span>
        <span>of</span>
        <span className="font-medium text-white">{total}</span>
        <span>results</span>
      </div>

      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2">
          <label className="text-sm text-slate-400">Rows per page:</label>
          <select
            value={pageSize}
            onChange={(e) => onPageSizeChange(Number(e.target.value))}
            className="input-shell px-2 py-1.5 text-sm"
          >
            {PAGE_SIZE_OPTIONS.map((size) => (
              <option key={size} value={size}>
                {size}
              </option>
            ))}
          </select>
        </div>

        <div className="flex items-center gap-1">
          <button
            onClick={() => onPageChange(1)}
            disabled={page === 1}
            className="flex h-8 w-8 items-center justify-center rounded-lg border border-white/10 bg-white/[0.03] text-slate-400 transition-colors hover:bg-white/[0.08] disabled:cursor-not-allowed disabled:opacity-50"
            aria-label="First page"
          >
            <ChevronLeft className="h-4 w-4" />
            <ChevronLeft className="h-4 w-4 -ml-1.5" />
          </button>

          <button
            onClick={() => onPageChange(page - 1)}
            disabled={page === 1}
            className="flex h-8 w-8 items-center justify-center rounded-lg border border-white/10 bg-white/[0.03] text-slate-400 transition-colors hover:bg-white/[0.08] disabled:cursor-not-allowed disabled:opacity-50"
            aria-label="Previous page"
          >
            <ChevronLeft className="h-4 w-4" />
          </button>

          <div className="flex items-center gap-1">
            {getPageNumbers().map((pageNum, index) =>
              typeof pageNum === "number" ? (
                <button
                  key={index}
                  onClick={() => onPageChange(pageNum)}
                  className={`flex h-8 min-w-[32px] items-center justify-center rounded-lg border px-2 text-sm font-medium transition-colors ${
                    pageNum === page
                      ? "border-sky-500/30 bg-sky-500/10 text-sky-300"
                      : "border-white/10 bg-white/[0.03] text-slate-400 hover:bg-white/[0.08]"
                  }`}
                >
                  {pageNum}
                </button>
              ) : (
                <span
                  key={index}
                  className="flex h-8 w-8 items-center justify-center text-slate-500"
                >
                  {pageNum}
                </span>
              )
            )}
          </div>

          <button
            onClick={() => onPageChange(page + 1)}
            disabled={page === totalPages}
            className="flex h-8 w-8 items-center justify-center rounded-lg border border-white/10 bg-white/[0.03] text-slate-400 transition-colors hover:bg-white/[0.08] disabled:cursor-not-allowed disabled:opacity-50"
            aria-label="Next page"
          >
            <ChevronRight className="h-4 w-4" />
          </button>

          <button
            onClick={() => onPageChange(totalPages)}
            disabled={page === totalPages}
            className="flex h-8 w-8 items-center justify-center rounded-lg border border-white/10 bg-white/[0.03] text-slate-400 transition-colors hover:bg-white/[0.08] disabled:cursor-not-allowed disabled:opacity-50"
            aria-label="Last page"
          >
            <ChevronRight className="h-4 w-4" />
            <ChevronRight className="h-4 w-4 -ml-1.5" />
          </button>
        </div>
      </div>
    </div>
  );
}

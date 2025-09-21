import React from "react";
import { ChevronRight } from "lucide-react";

export type Crumb = { id?: string; name: string };

type Props = {
  crumbs: Crumb[];
  onClick?: (id?: string) => void;
  className?: string;
};

export default function Breadcrumbs({ crumbs, onClick, className }: Props) {
  if (!crumbs || crumbs.length === 0) return null;

  return (
    <nav aria-label="Breadcrumb" className={className}>
      <ol className="flex items-center gap-2 text-sm text-muted-foreground">
        {crumbs.map((c, idx) => (
          <li key={`${c.id || "root"}-${idx}`}>
            <button
              onClick={() => onClick && onClick(c.id)}
              className={`flex items-center gap-1 hover:text-foreground focus:outline-none ${
                idx === crumbs.length - 1 ? "font-medium text-foreground" : ""
              }`}
              aria-current={idx === crumbs.length - 1 ? "page" : undefined}
            >
              <span className="truncate max-w-xs">{c.name}</span>
              {idx < crumbs.length - 1 && (
                <ChevronRight className="w-3 h-3 text-muted-foreground" />
              )}
            </button>
          </li>
        ))}
      </ol>
    </nav>
  );
}

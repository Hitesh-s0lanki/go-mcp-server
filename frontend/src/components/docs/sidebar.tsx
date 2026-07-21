"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { nav } from "@/lib/docs";
import { cn } from "@/lib/utils";

export function SidebarNav() {
  const pathname = usePathname();

  return (
    <nav className="space-y-7">
      {nav.map((group) => (
        <div key={group.title}>
          <div className="mb-2 px-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {group.title}
          </div>
          <ul className="space-y-0.5">
            {group.items.map((item) => {
              const href = `/doc/${item.slug}`;
              const active = pathname === href;
              return (
                <li key={item.slug}>
                  <Link
                    href={href}
                    className={cn(
                      "flex items-center justify-between gap-2 rounded-md px-3 py-1.5 text-sm transition-colors",
                      active
                        ? "bg-accent font-medium text-accent-foreground"
                        : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
                    )}
                  >
                    {item.label}
                    {item.badge && (
                      <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium text-amber-600 dark:text-amber-400">
                        {item.badge}
                      </span>
                    )}
                  </Link>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </nav>
  );
}

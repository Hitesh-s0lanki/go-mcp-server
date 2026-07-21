"use client";

import { useEffect, useState } from "react";
import { List } from "lucide-react";
import { cn } from "@/lib/utils";

export function Toc({ items }: { items: { id: string; text: string }[] }) {
  const [active, setActive] = useState<string>(items[0]?.id ?? "");

  useEffect(() => {
    if (items.length === 0) return;
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) setActive(entry.target.id);
        }
      },
      { rootMargin: "-80px 0px -70% 0px", threshold: 0 },
    );
    for (const item of items) {
      const el = document.getElementById(item.id);
      if (el) observer.observe(el);
    }
    return () => observer.disconnect();
  }, [items]);

  if (items.length === 0) return null;

  return (
    <nav className="sticky top-24 hidden w-56 shrink-0 xl:block">
      <div className="mb-3 flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <List className="size-3.5" />
        On this page
      </div>
      <ul className="space-y-1 border-l">
        {items.map((item) => (
          <li key={item.id}>
            <a
              href={`#${item.id}`}
              className={cn(
                "-ml-px block border-l py-1 pl-4 text-sm transition-colors",
                active === item.id
                  ? "border-foreground font-medium text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              {item.text}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}

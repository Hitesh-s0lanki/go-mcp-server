"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { FileText, Search, Wrench } from "lucide-react";
import { nav, pages, serverList } from "@/lib/docs";
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";

type Entry = {
  slug: string;
  label: string;
  group: string;
  keywords: string[];
  tool?: string;
};

// Flatten pages + every tool into a single searchable index.
function buildIndex(): Entry[] {
  const entries: Entry[] = [];
  for (const group of nav) {
    for (const item of group.items) {
      const page = pages[item.slug];
      entries.push({
        slug: item.slug,
        label: item.label,
        group: "Pages",
        keywords: [item.label, group.title, page?.title ?? ""],
      });
    }
  }
  for (const s of serverList) {
    for (const t of s.tools) {
      entries.push({
        slug: s.key,
        label: t.name,
        group: "Tools",
        tool: t.name,
        keywords: [t.name, s.name, t.summary],
      });
    }
  }
  return entries;
}

export function SearchButton() {
  const [open, setOpen] = useState(false);
  const router = useRouter();
  const index = buildIndex();

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((v) => !v);
      }
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  function go(entry: Entry) {
    setOpen(false);
    const anchor = entry.tool ? "#tools" : "";
    router.push(`/doc/${entry.slug}${anchor}`);
  }

  const pageEntries = index.filter((e) => e.group === "Pages");
  const toolEntries = index.filter((e) => e.group === "Tools");

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="hidden items-center gap-2 rounded-lg border bg-muted/40 px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted sm:flex"
      >
        <Search className="size-3.5" />
        <span>Search...</span>
        <kbd className="ml-4 rounded border bg-background px-1.5 font-mono text-[10px]">
          ⌘K
        </kbd>
      </button>

      <CommandDialog
        open={open}
        onOpenChange={setOpen}
        title="Search docs"
        description="Search pages and tools"
      >
        <Command>
          <CommandInput placeholder="Search pages and tools..." />
          <CommandList>
            <CommandEmpty>No results found.</CommandEmpty>
            <CommandGroup heading="Pages">
              {pageEntries.map((e) => (
                <CommandItem
                  key={`page-${e.slug}`}
                  value={`${e.label} ${e.keywords.join(" ")}`}
                  onSelect={() => go(e)}
                >
                  <FileText className="size-4 text-muted-foreground" />
                  {e.label}
                </CommandItem>
              ))}
            </CommandGroup>
            <CommandGroup heading="Tools">
              {toolEntries.map((e) => (
                <CommandItem
                  key={`tool-${e.label}`}
                  value={`${e.label} ${e.keywords.join(" ")}`}
                  onSelect={() => go(e)}
                >
                  <Wrench className="size-4 text-muted-foreground" />
                  <span className="font-mono text-[13px]">{e.label}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </CommandDialog>
    </>
  );
}

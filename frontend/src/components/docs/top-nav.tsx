"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Menu } from "lucide-react";
import { topNav } from "@/lib/docs";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Logo } from "@/components/brand/logo";
import { GitHubIcon } from "@/components/marketing/github-icon";
import { site } from "@/lib/site";
import { SidebarNav } from "./sidebar";
import { SearchButton } from "./search-command";
import { ThemeToggle } from "./theme-toggle";

export function TopNav() {
  const pathname = usePathname();
  const current = pathname.split("/")[2] ?? "overview";

  return (
    <header className="sticky top-0 z-40 border-b bg-background/80 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="mx-auto flex h-14 max-w-[1400px] items-center gap-4 px-4 sm:px-6">
        {/* Mobile menu */}
        <Sheet>
          <SheetTrigger asChild>
            <Button variant="ghost" size="icon" className="lg:hidden">
              <Menu className="size-5" />
              <span className="sr-only">Open navigation</span>
            </Button>
          </SheetTrigger>
          <SheetContent side="left" className="w-72 overflow-y-auto p-6">
            <SheetTitle className="mb-6 flex items-center gap-2 text-base">
              <Logo className="size-5" /> Relivo
            </SheetTitle>
            <SidebarNav />
          </SheetContent>
        </Sheet>

        <Link href="/doc/overview" className="flex items-center gap-2 font-semibold">
          <Logo className="size-7" />
          <span className="hidden sm:inline">Relivo</span>
          <span className="rounded-md bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
            Docs
          </span>
        </Link>

        <nav className="ml-4 hidden items-center gap-1 md:flex">
          {topNav.map((t) => (
            <Link
              key={t.slug}
              href={`/doc/${t.slug}`}
              className={cn(
                "rounded-md px-3 py-1.5 text-sm transition-colors",
                current === t.slug
                  ? "font-medium text-foreground"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {t.label}
            </Link>
          ))}
        </nav>

        <div className="ml-auto flex items-center gap-2">
          <SearchButton />
          <ThemeToggle />
          <Button variant="ghost" size="icon" asChild>
            <a href={site.github} target="_blank" rel="noopener noreferrer" aria-label="GitHub">
              <GitHubIcon className="size-4" />
            </a>
          </Button>
          <Button variant="outline" size="sm" asChild className="hidden sm:inline-flex">
            <a href={site.app} target="_blank" rel="noopener noreferrer">
              Open app
              <span
                className="size-1.5 rounded-full bg-amber-500"
                title={site.appStatus}
              />
            </a>
          </Button>
        </div>
      </div>
    </header>
  );
}

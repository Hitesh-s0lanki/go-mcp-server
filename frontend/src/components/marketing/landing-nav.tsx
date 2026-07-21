"use client";

import Link from "next/link";
import { site } from "@/lib/site";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/docs/theme-toggle";
import { Logo } from "@/components/brand/logo";
import { GitHubIcon } from "./github-icon";

const links = [
  { href: "#namespaces", label: "Namespaces" },
  { href: "#features", label: "Features" },
  { href: "/doc/overview", label: "Docs" },
];

export function LandingNav() {
  return (
    <header className="sticky top-0 z-40 border-b bg-background/80 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="mx-auto flex h-16 max-w-6xl items-center gap-6 px-4 sm:px-6">
        <Link href="/" className="flex items-center gap-2 font-semibold">
          <Logo className="size-8" />
          {site.name}
        </Link>

        <nav className="ml-2 hidden items-center gap-1 md:flex">
          {links.map((l) => (
            <Link
              key={l.href}
              href={l.href}
              className="rounded-md px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              {l.label}
            </Link>
          ))}
        </nav>

        <div className="ml-auto flex items-center gap-2">
          <ThemeToggle />
          <Button variant="ghost" size="icon" asChild>
            <a href={site.github} target="_blank" rel="noopener noreferrer" aria-label="GitHub">
              <GitHubIcon className="size-4.5" />
            </a>
          </Button>
          <Button variant="outline" size="sm" asChild className="hidden sm:inline-flex">
            <a href={site.app} target="_blank" rel="noopener noreferrer">
              Open app
              <span className="ml-0.5 flex items-center gap-1 rounded-full bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium text-amber-600 dark:text-amber-400">
                <span className="size-1.5 rounded-full bg-amber-500" />
                Beta
              </span>
            </a>
          </Button>
          <Button size="sm" asChild>
            <Link href="/doc/overview">Get started</Link>
          </Button>
        </div>
      </div>
    </header>
  );
}

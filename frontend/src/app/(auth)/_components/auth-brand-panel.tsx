import Link from "next/link";
import { ArrowLeft } from "lucide-react";

import { site } from "@/lib/site";
import { Logo } from "@/components/brand/logo";

/**
 * Left-hand brand panel for the auth pages. Uses theme tokens so it reads well
 * in both light and dark mode, and mirrors the landing hero's radial glow.
 */
export function AuthBrandPanel() {
  return (
    <div className="relative hidden h-full flex-col justify-between overflow-hidden bg-muted/30 p-12 lg:flex">
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(60%_50%_at_30%_0%,color-mix(in_oklch,var(--primary)_14%,transparent),transparent)]"
      />

      <Link href="/" className="flex items-center gap-2 text-lg font-semibold">
        <Logo className="size-8" />
        {site.name}
      </Link>

      <div className="max-w-md">
        <h2 className="text-3xl font-semibold tracking-tight">
          {site.tagline}
        </h2>
        <p className="mt-4 text-muted-foreground">{site.description}</p>
        <Link
          href="/"
          className="mt-8 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-3.5" />
          Back to home
        </Link>
      </div>

      <p className="text-sm text-muted-foreground">
        {site.license} licensed · Built with Go
      </p>
    </div>
  );
}

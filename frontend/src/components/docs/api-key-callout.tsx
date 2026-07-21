"use client";

import Link from "next/link";
import { ArrowRight, KeyRound } from "lucide-react";
import { useUser } from "@clerk/nextjs";
import { Button } from "@/components/ui/button";

/**
 * Auth-aware prompt shown alongside the MCP connection URLs. Every namespace
 * requires an `X-API-Key` header, so this is where a reader realises they need
 * to sign in and mint a key. Signed-out users get a sign-in CTA; signed-in
 * users get a link to manage their keys. Rendered on the (statically
 * generated) docs pages, so the branch resolves on the client — we default to
 * the signed-out CTA until Clerk has loaded to avoid a flash for the common
 * (logged-out) reader.
 */
export function ApiKeyCallout() {
  const { isSignedIn } = useUser();

  return (
    <div className="not-prose flex min-w-0 flex-col gap-3 rounded-xl border bg-card p-4 sm:flex-row sm:items-center">
      <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted">
        <KeyRound className="size-4.5 text-foreground/70" />
      </div>

      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium">
          {isSignedIn ? "Use your API key to connect" : "You need an API key to connect"}
        </div>
        <p className="text-sm text-muted-foreground">
          {isSignedIn ? "Pass it as an " : "Every request must carry an "}
          <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">X-API-Key</code>
          {isSignedIn
            ? " header on every request. Create and manage keys in your dashboard."
            : " header. Sign in to create one."}
        </p>
      </div>

      {isSignedIn ? (
        <Button size="sm" variant="outline" asChild className="shrink-0 sm:ml-auto">
          <Link href="/doc/keys">
            Manage API keys
            <ArrowRight className="ml-0.5 size-4" />
          </Link>
        </Button>
      ) : (
        <Button size="sm" asChild className="shrink-0 sm:ml-auto">
          <Link href="/sign-in">
            Sign in to get your key
            <ArrowRight className="ml-0.5 size-4" />
          </Link>
        </Button>
      )}
    </div>
  );
}

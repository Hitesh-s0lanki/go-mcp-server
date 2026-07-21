import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { auth } from "@clerk/nextjs/server";

import { listKeys } from "@/lib/api-keys";
import { site } from "@/lib/site";
import { ApiKeysManager } from "./_components/api-keys-manager";

export const metadata: Metadata = {
  title: `API keys · ${site.name}`,
  description: "Create and manage the API keys you use to connect to the MCP server.",
};

// This page reads the Clerk session, so it must render per request.
export const dynamic = "force-dynamic";

export default async function KeysPage() {
  const { userId } = await auth();
  // proxy.ts already gates this route; this is the in-handler backstop. The Go
  // server scopes the query to the caller's Clerk id from the forwarded token.
  if (!userId) redirect("/sign-in");

  const { keys, max } = await listKeys();

  // Rendered inside the docs layout (TopNav + sidebar), so the page owns only
  // its content column — the chrome is shared with the rest of /doc.
  return (
    <div className="max-w-3xl py-10">
      <ApiKeysManager initialKeys={keys} max={max} />
    </div>
  );
}

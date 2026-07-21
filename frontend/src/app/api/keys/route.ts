import { auth } from "@clerk/nextjs/server";

import { keysProxy } from "@/lib/api-keys";

// Thin proxy to the Go MCP server's key API. The Go server is the authority:
// it re-verifies the Clerk token, scopes every key to the caller's user id, and
// enforces the per-user cap. proxy.ts also gates /api/keys; the userId check
// here is the in-handler backstop and gives a clean 401 without a network hop.

export const runtime = "nodejs";

/** Relay the Go server's response (status + JSON body) back to the browser. */
async function relay(res: Response): Promise<Response> {
  return new Response(await res.text(), {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}

/** GET /api/keys — list the caller's keys (masked; secrets never returned). */
export async function GET() {
  const { userId } = await auth();
  if (!userId) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  return relay(await keysProxy("/api/keys"));
}

/**
 * POST /api/keys — mint a new key for the caller.
 * Body: { label?: string }. The full secret comes back exactly once, from Go.
 */
export async function POST(request: Request) {
  const { userId } = await auth();
  if (!userId) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const body = await request.text();
  return relay(await keysProxy("/api/keys", { method: "POST", body }));
}

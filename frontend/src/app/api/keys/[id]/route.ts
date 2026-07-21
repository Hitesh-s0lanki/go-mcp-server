import { auth } from "@clerk/nextjs/server";

import { keysProxy } from "@/lib/api-keys";

export const runtime = "nodejs";

/**
 * DELETE /api/keys/[id] — revoke one of the caller's keys, proxied to the Go
 * server. The delete there is scoped to the caller's userId, so guessing another
 * user's key id gets a 404, not their data. Revoking cascades to that key's
 * memories.
 */
export async function DELETE(
  _request: Request,
  ctx: { params: Promise<{ id: string }> },
) {
  const { userId } = await auth();
  if (!userId) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await ctx.params;
  const res = await keysProxy(`/api/keys/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
  return new Response(await res.text(), {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}

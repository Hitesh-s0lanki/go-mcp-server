"use client";

import { useState } from "react";
import {
  Check,
  Copy,
  KeyRound,
  Loader2,
  Plus,
  TriangleAlert,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

import { MCP_BASE_URL } from "@/lib/mcp";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";

export type ApiKeySummary = {
  id: string;
  label: string;
  masked: string;
  createdAt: string;
};

const dateFmt = new Intl.DateTimeFormat("en", {
  year: "numeric",
  month: "short",
  day: "numeric",
});

/** Small copy-to-clipboard button with a transient check state. */
function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      onClick={async () => {
        await navigator.clipboard.writeText(value);
        setCopied(true);
        toast.success(`${label} copied to clipboard`);
        setTimeout(() => setCopied(false), 1500);
      }}
    >
      {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
      {copied ? "Copied" : "Copy"}
    </Button>
  );
}

export function ApiKeysManager({
  initialKeys,
  max,
}: {
  initialKeys: ApiKeySummary[];
  max: number;
}) {
  const [keys, setKeys] = useState<ApiKeySummary[]>(initialKeys);

  // Create dialog state.
  const [createOpen, setCreateOpen] = useState(false);
  const [label, setLabel] = useState("");
  const [creating, setCreating] = useState(false);

  // View-once reveal: holds the full secret ONLY in memory, only until dismissed.
  const [revealed, setRevealed] = useState<string | null>(null);

  // Delete confirmation state.
  const [pendingDelete, setPendingDelete] = useState<ApiKeySummary | null>(null);
  const [deleting, setDeleting] = useState(false);

  const atLimit = keys.length >= max;

  async function handleCreate() {
    setCreating(true);
    try {
      const res = await fetch("/api/keys", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ label: label.trim() }),
      });
      const data = await res.json();
      if (!res.ok) {
        toast.error(data.error ?? "Failed to create key");
        return;
      }
      const created = data.key as ApiKeySummary & { key: string };
      setKeys((prev) => [
        {
          id: created.id,
          label: created.label,
          masked: created.masked,
          createdAt: created.createdAt,
        },
        ...prev,
      ]);
      setCreateOpen(false);
      setLabel("");
      setRevealed(created.key); // open the one-time reveal
    } catch {
      toast.error("Network error — could not create key");
    } finally {
      setCreating(false);
    }
  }

  async function handleDelete() {
    if (!pendingDelete) return;
    setDeleting(true);
    try {
      const res = await fetch(`/api/keys/${pendingDelete.id}`, {
        method: "DELETE",
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        toast.error(data.error ?? "Failed to revoke key");
        return;
      }
      setKeys((prev) => prev.filter((k) => k.id !== pendingDelete.id));
      toast.success("API key revoked");
      setPendingDelete(null);
    } catch {
      toast.error("Network error — could not revoke key");
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">API keys</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Use a key as the{" "}
            <code className="rounded bg-muted px-1 py-0.5 text-xs">
              X-API-Key
            </code>{" "}
            header when connecting to the MCP server. You can hold up to {max}{" "}
            keys.
          </p>
        </div>
        <Button
          onClick={() => setCreateOpen(true)}
          disabled={atLimit}
          title={atLimit ? `Limit of ${max} keys reached` : undefined}
        >
          <Plus className="size-4" />
          Create key
        </Button>
      </div>

      {atLimit && (
        <div className="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-400">
          <TriangleAlert className="size-4 shrink-0" />
          You&apos;ve reached the limit of {max} keys. Revoke one to create
          another.
        </div>
      )}

      {keys.length === 0 ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <KeyRound className="size-6" />
            </EmptyMedia>
            <EmptyTitle>No API keys yet</EmptyTitle>
            <EmptyDescription>
              Create your first key to start connecting agents to the MCP
              server.
            </EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" />
              Create key
            </Button>
          </EmptyContent>
        </Empty>
      ) : (
        <div className="grid gap-3">
          {keys.map((k) => (
            <Card key={k.id}>
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-base">
                  <KeyRound className="size-4 text-muted-foreground" />
                  {k.label || (
                    <span className="text-muted-foreground">Untitled key</span>
                  )}
                </CardTitle>
                <CardDescription className="font-mono text-xs">
                  {k.masked}
                </CardDescription>
              </CardHeader>
              <CardContent className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">
                  Created {dateFmt.format(new Date(k.createdAt))}
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-destructive hover:text-destructive"
                  onClick={() => setPendingDelete(k)}
                >
                  <Trash2 className="size-4" />
                  Revoke
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* Create dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Create API key</DialogTitle>
            <DialogDescription>
              Give the key a name so you can recognise it later. The secret is
              shown only once, right after you create it.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="key-label">Name (optional)</Label>
            <Input
              id="key-label"
              placeholder="e.g. Laptop, CI, Claude Desktop"
              value={label}
              maxLength={60}
              onChange={(e) => setLabel(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !creating) handleCreate();
              }}
              autoFocus
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setCreateOpen(false)}
              disabled={creating}
            >
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={creating}>
              {creating && <Loader2 className="size-4 animate-spin" />}
              Create key
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* One-time reveal dialog — the only place the full secret appears */}
      <Dialog
        open={revealed !== null}
        onOpenChange={(open) => {
          if (!open) setRevealed(null);
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Check className="size-5 text-emerald-500" />
              API key created
            </DialogTitle>
            <DialogDescription>
              Copy your key now — for your security it won&apos;t be shown again.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3">
            <div className="flex items-center gap-2 rounded-lg border bg-muted/40 p-3">
              <code className="flex-1 overflow-x-auto whitespace-nowrap font-mono text-sm">
                {revealed}
              </code>
              {revealed && <CopyButton value={revealed} label="API key" />}
            </div>
            <div className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-400">
              <TriangleAlert className="mt-0.5 size-3.5 shrink-0" />
              <span>
                Store it in your secrets manager now. If you lose it, revoke this
                key and create a new one.
              </span>
            </div>
            <div className="text-xs text-muted-foreground">
              Send it as the{" "}
              <code className="rounded bg-muted px-1 py-0.5">X-API-Key</code>{" "}
              header to{" "}
              <code className="rounded bg-muted px-1 py-0.5">
                {MCP_BASE_URL}
              </code>
              .
            </div>
          </div>

          <DialogFooter>
            <Button onClick={() => setRevealed(null)}>Done</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete confirmation */}
      <AlertDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Revoke this API key?</AlertDialogTitle>
            <AlertDialogDescription>
              Anything using{" "}
              <span className="font-medium text-foreground">
                {pendingDelete?.label || "this key"}
              </span>{" "}
              will immediately lose access, and its stored memories are deleted.
              This can&apos;t be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault();
                handleDelete();
              }}
              disabled={deleting}
              className="bg-destructive text-white hover:bg-destructive/90"
            >
              {deleting && <Loader2 className="size-4 animate-spin" />}
              Revoke key
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

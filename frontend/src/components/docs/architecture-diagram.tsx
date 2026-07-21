import { ArrowDown } from "lucide-react";
import { cn } from "@/lib/utils";

/* Presentational architecture diagrams for the Architecture doc page.
   Pure server-rendered JSX + Tailwind theme tokens — no mermaid, no client
   runtime — so they match the cards/callouts and render in light and dark. */

type Tone = "default" | "auth" | "target" | "muted";

const toneClass: Record<Tone, string> = {
  default: "border bg-card",
  auth: "border-sky-500/30 bg-sky-500/[0.06]",
  target: "border-emerald-500/25 bg-emerald-500/[0.05]",
  muted: "border-dashed bg-muted/40",
};

function Node({
  title,
  sub,
  tone = "default",
  mono = true,
}: {
  title: string;
  sub?: string;
  tone?: Tone;
  mono?: boolean;
}) {
  return (
    <div className={cn("rounded-xl px-4 py-3 text-center", toneClass[tone])}>
      <div
        className={cn(
          "text-sm font-semibold text-foreground",
          mono && "font-mono",
        )}
      >
        {title}
      </div>
      {sub && <div className="mt-0.5 text-xs leading-snug text-muted-foreground">{sub}</div>}
    </div>
  );
}

function Down({ label }: { label?: string }) {
  return (
    <div className="flex flex-col items-center py-1.5">
      <ArrowDown className="size-4 text-muted-foreground/50" />
      {label && (
        <span className="mt-0.5 font-mono text-[11px] text-muted-foreground/70">{label}</span>
      )}
    </div>
  );
}

function Pill({ children }: { children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center justify-center rounded-full border bg-muted px-3 py-1 font-mono text-xs font-medium text-foreground">
      {children}
    </span>
  );
}

/** The runtime path of a single request: global middleware, then per-route auth. */
function RequestFlow() {
  return (
    <div className="not-prose my-6 rounded-2xl border bg-background/40 p-5 sm:p-6">
      {/* Linear middleware stack, applied outermost-first. */}
      <div className="mx-auto max-w-xs">
        <Node title="MCP client" sub="sends X-API-Key header" mono={false} />
        <Down />
        <Node title="Recover" sub="panic → 500" />
        <Down />
        <Node title="LogRequests" sub="logs arrival + completion" />
        <Down />
        <Node title="net/http mux" sub="match route" tone="muted" />
      </div>

      {/* Per-route auth: each route carries its own admission. */}
      <div className="mt-5 grid gap-4 sm:grid-cols-3">
        <div className="flex flex-col items-center">
          <Pill>GET /healthz</Pill>
          <Down />
          <div className="w-full">
            <Node title="200 ok" sub="no auth — liveness only" tone="target" />
          </div>
        </div>

        <div className="flex flex-col items-center">
          <Pill>{"/*/mcp"}</Pill>
          <Down />
          <div className="w-full">
            <Node title="RequireAPIKey" sub="key in api_keys? else 401" tone="auth" />
          </div>
          <Down />
          <div className="w-full">
            <Node
              title="5 namespaces"
              sub="memory · skills · gsc · producthunt · event"
              tone="target"
              mono={false}
            />
          </div>
        </div>

        <div className="flex flex-col items-center">
          <Pill>/api/keys</Pill>
          <Down />
          <div className="w-full">
            <Node title="Clerk RequireUser" sub="signed-in human? else 401" tone="auth" />
          </div>
          <Down />
          <div className="w-full">
            <Node title="keysapi" sub="list · create · delete (owner-scoped)" tone="target" />
          </div>
        </div>
      </div>

      <p className="mt-5 text-center text-xs leading-relaxed text-muted-foreground">
        Admission is enforced <strong className="font-medium text-foreground">per route</strong>, not
        globally — so a namespace can&rsquo;t forget it, and{" "}
        <code className="rounded bg-muted px-1 py-0.5 font-mono text-[11px]">/healthz</code> never
        needs a credential. <strong className="font-medium text-foreground">memory</strong> also
        resolves the same key to an <code className="rounded bg-muted px-1 py-0.5 font-mono text-[11px]">api_key_id</code>{" "}
        to scope every row — identity, not admission.
      </p>
    </div>
  );
}

/** How namespaces enroll themselves at startup and get mounted. */
function RegisterFlow() {
  const packages = ["memory", "skills", "gsc", "producthunt", "event"];
  return (
    <div className="not-prose my-6 rounded-2xl border bg-background/40 p-5 sm:p-6">
      <div className="mb-1 text-center text-xs font-medium uppercase tracking-wide text-muted-foreground">
        package init()
      </div>
      <div className="flex flex-wrap justify-center gap-2">
        {packages.map((p) => (
          <Pill key={p}>{p}</Pill>
        ))}
      </div>

      <div className="mx-auto mt-1 max-w-sm">
        <Down label="mcpx.Register(ns)" />
        <Node title="mcpx registry" sub="collects every self-registered namespace" />
        <Down />
        <Node title="mcpx.Handler(opts)" sub="cmd/server/main.go calls this once" />
        <Down />
        <Node title="ns.Server(deps)" sub="build each server — fail fast on bad config" />
        <Down />
        <Node title="mux.Handle(path, RequireAPIKey(server))" sub="mounted over Streamable HTTP" />
      </div>

      <p className="mt-5 text-center text-xs leading-relaxed text-muted-foreground">
        Adding a namespace is a new package plus a blank import —{" "}
        <code className="rounded bg-muted px-1 py-0.5 font-mono text-[11px]">main.go</code> never
        enumerates them.
      </p>
    </div>
  );
}

export function ArchitectureDiagram({ variant }: { variant: "request" | "register" }) {
  return variant === "request" ? <RequestFlow /> : <RegisterFlow />;
}

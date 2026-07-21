import Link from "next/link";
import {
  ArrowRight,
  Blocks,
  BookOpen,
  Database,
  Plug,
  ShieldCheck,
  Sparkles,
  Star,
  Zap,
} from "lucide-react";
import { site } from "@/lib/site";
import { serverList, connectionConfig } from "@/lib/docs";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/docs/code-block";
import { LandingNav } from "@/components/marketing/landing-nav";
import { GitHubIcon } from "@/components/marketing/github-icon";

const toolCount = serverList.reduce((n, s) => n + s.tools.length, 0);

const features = [
  {
    icon: Blocks,
    title: "Multi-namespace mux",
    body: "Five independent MCP servers on one endpoint. A domain never reaches across another except through internal/mcpx.",
  },
  {
    icon: Plug,
    title: "Self-registering",
    body: "Add a namespace as a single package with an init(). cmd/server/main.go never changes; the mux picks it up on the next start.",
  },
  {
    icon: Zap,
    title: "Streamable HTTP",
    body: "Built on the official MCP Go SDK's Streamable HTTP transport, over stdlib net/http with structured slog logging.",
  },
  {
    icon: Database,
    title: "Hybrid RAG memory",
    body: "Durable, per-user recall by meaning and keyword over Postgres + pgvector. Store a fact once, retrieve it across sessions.",
  },
  {
    icon: Sparkles,
    title: "Live Agent Skills",
    body: "Find and download Agent Skills straight from GitHub in real time. Nothing is cached; every call reflects live GitHub.",
  },
  {
    icon: ShieldCheck,
    title: "Keyed by default",
    body: "Every namespace requires an X-API-Key before dispatch, and memory scopes each caller to their own rows — one caller can never read another's memories.",
  },
];

const addNamespaceSnippet = `// internal/todo/register.go
package todo

func init() { mcpx.Register(namespace{}) }

// That's it. Blank-import the package in main.go
// and the mux mounts /todo/mcp on the next start.`;

export default function Home() {
  return (
    <div className="flex min-h-screen flex-col">
      <LandingNav />

      <main className="flex-1">
        {/* Hero */}
        <section className="relative overflow-hidden">
          <div
            aria-hidden
            className="pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(60%_50%_at_50%_0%,color-mix(in_oklch,var(--primary)_14%,transparent),transparent)]"
          />
          <div className="mx-auto max-w-6xl px-4 pb-20 pt-20 text-center sm:px-6 sm:pt-28">
            <Link
              href={site.github}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 rounded-full border bg-card px-3 py-1 text-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              <Star className="size-3.5" />
              Open source · {site.license} licensed
              <ArrowRight className="size-3.5" />
            </Link>

            <h1 className="mx-auto mt-6 max-w-3xl bg-gradient-to-b from-foreground to-foreground/60 bg-clip-text text-4xl font-semibold tracking-tight text-transparent sm:text-6xl">
              {site.tagline}
            </h1>

            <p className="mx-auto mt-6 max-w-2xl text-lg leading-relaxed text-muted-foreground">
              <strong className="font-semibold text-foreground">{site.fullName}</strong>{" "}
              is a multi-namespace Model Context Protocol server in Go. Point any
              MCP client at a route and its tools appear: memory, skills, Search
              Console, Product Hunt, and events, all behind one HTTP mux.
            </p>

            <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
              <Button size="lg" asChild>
                <Link href="/doc/overview">
                  Get started <ArrowRight className="size-4" />
                </Link>
              </Button>
              <Button size="lg" variant="outline" asChild>
                <a href={site.github} target="_blank" rel="noopener noreferrer">
                  <GitHubIcon className="size-4" /> Star on GitHub
                </a>
              </Button>
            </div>

            <div className="mt-5">
              <a
                href={site.app}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
              >
                <span className="relative flex size-2">
                  <span className="absolute inline-flex size-full animate-ping rounded-full bg-amber-500/60" />
                  <span className="relative inline-flex size-2 rounded-full bg-amber-500" />
                </span>
                Preview the Relivo app ({site.appStatus.toLowerCase()})
                <ArrowRight className="size-3.5" />
              </a>
            </div>

            <div className="mt-6 flex flex-wrap items-center justify-center gap-x-6 gap-y-2 text-sm text-muted-foreground">
              <span>{serverList.length} namespaces</span>
              <span className="text-border">·</span>
              <span>{toolCount} tools</span>
              <span className="text-border">·</span>
              <span>1 endpoint</span>
              <span className="text-border">·</span>
              <span>{site.goVersion}</span>
            </div>

            {/* Hero code card */}
            <div className="mx-auto mt-14 max-w-2xl text-left">
              <CodeBlock code={connectionConfig} lang="json" title="mcp.json" />
            </div>
          </div>
        </section>

        {/* Namespaces */}
        <section id="namespaces" className="border-t bg-muted/20 py-20">
          <div className="mx-auto max-w-6xl px-4 sm:px-6">
            <div className="max-w-2xl">
              <h2 className="text-3xl font-semibold tracking-tight">
                {serverList.length} servers, one endpoint
              </h2>
              <p className="mt-3 text-muted-foreground">
                Each namespace is a fully independent MCP server. Mount the ones
                you need and ignore the rest.
              </p>
            </div>

            <div className="mt-10 grid gap-4 md:grid-cols-3">
              {serverList.map((s) => {
                const Icon = s.icon;
                return (
                  <Link
                    key={s.key}
                    href={`/doc/${s.key}`}
                    className="group flex flex-col rounded-2xl border bg-card p-6 transition-colors hover:border-foreground/20 hover:bg-accent/30"
                  >
                    <div className="flex items-center justify-between">
                      <div className="flex size-11 items-center justify-center rounded-xl bg-muted">
                        <Icon className="size-5.5 text-foreground/80" />
                      </div>
                      <Badge variant="secondary" className="font-mono text-[11px]">
                        {s.tools.length} tools
                      </Badge>
                    </div>
                    <div className="mt-4 flex items-center gap-1.5 text-lg font-semibold">
                      {s.name}
                      <code className="font-mono text-xs font-normal text-muted-foreground">
                        {s.path}
                      </code>
                    </div>
                    <p className="mt-2 flex-1 text-sm leading-relaxed text-muted-foreground">
                      {s.tagline}
                    </p>
                    <span className="mt-4 inline-flex items-center gap-1 text-sm font-medium text-foreground">
                      Explore
                      <ArrowRight className="size-3.5 transition-transform group-hover:translate-x-0.5" />
                    </span>
                  </Link>
                );
              })}
            </div>
          </div>
        </section>

        {/* Features */}
        <section id="features" className="py-20">
          <div className="mx-auto max-w-6xl px-4 sm:px-6">
            <div className="max-w-2xl">
              <h2 className="text-3xl font-semibold tracking-tight">
                Built to extend
              </h2>
              <p className="mt-3 text-muted-foreground">
                Opinionated where it counts, out of your way everywhere else.
              </p>
            </div>

            <div className="mt-10 grid gap-px overflow-hidden rounded-2xl border bg-border sm:grid-cols-2 lg:grid-cols-3">
              {features.map((f) => {
                const Icon = f.icon;
                return (
                  <div key={f.title} className="bg-card p-6">
                    <Icon className="size-5 text-foreground/70" />
                    <h3 className="mt-4 font-semibold">{f.title}</h3>
                    <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                      {f.body}
                    </p>
                  </div>
                );
              })}
            </div>
          </div>
        </section>

        {/* Add a namespace */}
        <section className="border-t bg-muted/20 py-20">
          <div className="mx-auto grid max-w-6xl items-center gap-10 px-4 sm:px-6 lg:grid-cols-2">
            <div>
              <h2 className="text-3xl font-semibold tracking-tight">
                Adding a namespace is one package
              </h2>
              <p className="mt-4 text-muted-foreground">
                Namespaces self-register through <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm">internal/mcpx</code>.
                Create a package that calls <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm">mcpx.Register</code> from
                its <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm">init()</code>, blank-import it, and the mux mounts
                the new route on the next start. No wiring, no central registry to edit.
              </p>
              <Button variant="outline" className="mt-6" asChild>
                <Link href="/doc/architecture">
                  <BookOpen className="size-4" /> Read the architecture
                </Link>
              </Button>
            </div>
            <CodeBlock code={addNamespaceSnippet} lang="go" title="register.go" />
          </div>
        </section>

        {/* CTA */}
        <section className="py-24">
          <div className="mx-auto max-w-3xl px-4 text-center sm:px-6">
            <h2 className="text-3xl font-semibold tracking-tight sm:text-4xl">
              Point a client at it and go
            </h2>
            <p className="mx-auto mt-4 max-w-xl text-muted-foreground">
              Free and open source. Clone the repo, run{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm">make run</code>,
              and connect your first MCP client in under a minute.
            </p>
            <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
              <Button size="lg" asChild>
                <Link href="/doc/quickstart">
                  Read the quickstart <ArrowRight className="size-4" />
                </Link>
              </Button>
              <Button size="lg" variant="outline" asChild>
                <a href={site.github} target="_blank" rel="noopener noreferrer">
                  <GitHubIcon className="size-4" /> View source
                </a>
              </Button>
            </div>
          </div>
        </section>
      </main>

      {/* Footer */}
      <footer className="border-t py-10">
        <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 px-4 sm:flex-row sm:px-6">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <span className="font-semibold text-foreground">{site.name}</span>
            <span className="text-border">·</span>
            {site.license} licensed · Built with Go
          </div>
          <div className="flex items-center gap-5 text-sm text-muted-foreground">
            <Link href="/doc/overview" className="hover:text-foreground">
              Docs
            </Link>
            <a
              href={site.app}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1.5 hover:text-foreground"
            >
              App
              <span className="size-1.5 rounded-full bg-amber-500" title={site.appStatus} />
            </a>
            <a
              href={site.github}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1.5 hover:text-foreground"
            >
              <GitHubIcon className="size-4" /> GitHub
            </a>
          </div>
        </div>
      </footer>
    </div>
  );
}

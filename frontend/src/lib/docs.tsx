import type { ReactNode } from "react";
import Link from "next/link";
import {
  Bot,
  Brain,
  Compass,
  KeyRound,
  Layers,
  LineChart,
  Radio,
  Rocket,
  Sparkles,
  Waypoints,
  Workflow,
  Wrench,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { MCP_BASE_URL } from "./mcp";

/* ------------------------------------------------------------------ */
/* Servers: the live namespaces exposed by the Relivo MCP Server.     */
/* Connection URLs are derived from NEXT_PUBLIC_MCP_BASE_URL at        */
/* render time (see components/docs/connect.tsx), never hard-coded.    */
/* ------------------------------------------------------------------ */

export type ToolParam = {
  name: string;
  type: string;
  required?: boolean;
  default?: string;
  description: string;
};

export type ToolDoc = {
  name: string;
  summary: string;
  params: ToolParam[];
};

/**
 * Two very different kinds of "requirement" a namespace has, kept apart on
 * purpose. `caller` is the only thing the person calling the API supplies on
 * every request. `server` lists the operator-side environment the hosted
 * instance already has configured -- callers never set these, they matter only
 * when self-hosting. Flattening the two into one line reads as "you must supply
 * an OpenAI key", which is false for a hosted user.
 */
export type ServerRequires = {
  /** Sent by the caller on every request (e.g. the X-API-Key header). */
  caller: string;
  /**
   * Extra credentials the user connects themselves, scoped to their own
   * account -- e.g. GSC, where each caller reads their own Google properties.
   * Unlike `server`, this IS shown to the reader: it's their job to supply.
   */
  userProvided?: string;
  /** Configured on the server; already set on the hosted instance. Not shown. */
  server?: string;
};

export type ServerDoc = {
  key: string;
  name: string;
  /** MCP route mounted on the HTTP mux. */
  path: string;
  icon: LucideIcon;
  tagline: string;
  requires?: ServerRequires;
  /** Optional operational note rendered as a tip on the server page. */
  note?: string;
  /** A concrete "how it helps" walkthrough, rendered as an example callout. */
  example?: string;
  tools: ToolDoc[];
};

export const servers: Record<string, ServerDoc> = {
  memory: {
    key: "memory",
    name: "Memory",
    path: "/memory/mcp",
    icon: Brain,
    tagline:
      "Per-user long-term memory. Hybrid RAG (semantic + keyword) over Postgres + pgvector.",
    requires: {
      caller: "X-API-Key header",
      server: "DATABASE_URL (pgvector) · OPENAI_API_KEY",
    },
    example:
      "Give an agent memory that outlives the session. Call memory_write to store a durable fact once (a user's stack, a decision, a summary), then memory_search before you act so the agent recalls it next time instead of asking again.",
    tools: [
      {
        name: "memory_search",
        summary:
          "Search stored memories by meaning and keyword. Returns ranked snippets with a similarity score and an id; call memory_get with that id for full content. An empty result means nothing relevant is stored.",
        params: [
          { name: "query", type: "string", required: true, description: "What to search for, in natural language." },
          { name: "limit", type: "int", default: "5", description: "Max results to return." },
          { name: "min_score", type: "float", default: "0.35", description: "Cosine-similarity floor (0 to 1). Raise for stricter matches." },
          { name: "tags", type: "string[]", description: "Only search memories carrying any of these tags." },
        ],
      },
      {
        name: "memory_write",
        summary: "Store a new memory. Returns the created memory's id.",
        params: [
          { name: "content", type: "string", required: true, description: "The memory to store." },
          { name: "tags", type: "string[]", description: "Labels for filtering later." },
          { name: "metadata", type: "object", description: "Arbitrary key-value context." },
        ],
      },
      {
        name: "memory_get",
        summary: "Fetch one memory in full by id.",
        params: [{ name: "id", type: "string", required: true, description: "The memory id, as returned by memory_search." }],
      },
      {
        name: "memory_update",
        summary: "Replace a memory's content by id. The memory is re-embedded.",
        params: [
          { name: "id", type: "string", required: true, description: "The memory id to update." },
          { name: "content", type: "string", required: true, description: "Replacement content; the memory is re-embedded." },
        ],
      },
      {
        name: "memory_delete",
        summary: "Permanently delete a memory by id.",
        params: [{ name: "id", type: "string", required: true, description: "The memory id to delete." }],
      },
      {
        name: "memory_list",
        summary: "List recent memories newest-first, optionally filtered by tag. Use this to browse when you have no search query.",
        params: [
          { name: "tags", type: "string[]", description: "Only list memories carrying any of these tags." },
          { name: "limit", type: "int", default: "20", description: "Max results." },
        ],
      },
    ],
  },
  skills: {
    key: "skills",
    name: "Skills",
    path: "/skills/mcp",
    icon: Sparkles,
    tagline:
      "Find and download Agent Skills live from GitHub. Nothing is cached; every call reflects live GitHub.",
    requires: {
      caller: "X-API-Key header",
    },
    example:
      "Need a capability the agent lacks, like editing a PDF form or scaffolding a route? Call skills_find with the requirement in plain language, get the matching SKILL.md back with its source links, then skills_download to pull the whole skill folder from GitHub and use it.",
    tools: [
      {
        name: "skills_find",
        summary:
          "THE tool for obtaining Agent Skills. Runs a live OpenAI + web agent that finds the most relevant SKILL.md on GitHub and returns its complete, ready-to-use content plus source links. Pass multiple needs to resolve them in parallel.",
        params: [
          { name: "requirement", type: "string", description: "A single need, in natural language, e.g. 'edit a PDF form'." },
          { name: "requirements", type: "string[]", description: "Several needs at once; each is searched in parallel." },
        ],
      },
      {
        name: "skills_download",
        summary:
          "Download a COMPLETE Agent Skill from GitHub: every file in the skill folder (SKILL.md plus scripts and reference files, recursively), fetched concurrently. Use after skills_find to install a skill you located.",
        params: [{ name: "source", type: "string", required: true, description: "GitHub URL (repo/tree/blob/raw) or 'owner/repo/path'." }],
      },
    ],
  },
  gsc: {
    key: "gsc",
    name: "Search Console",
    path: "/gsc/mcp",
    icon: LineChart,
    tagline:
      "Google Search Console over MCP: properties, search-analytics reports, URL inspection, and sitemaps.",
    requires: {
      caller: "X-API-Key header",
      userProvided: "your own Google Search Console credentials — a service-account JSON (or OAuth via Application Default Credentials) for the Google account whose properties you want to read",
    },
    note: "Mounts even without credentials: every tool reports the config problem instead of failing the server. Call gsc_capabilities first to check auth status. Mutating tools (add/delete site, submit/delete sitemap) require GSC_ALLOW_DESTRUCTIVE=true.",
    example:
      "Ask which pages are losing search traffic. Call gsc_compare_periods across two date ranges to surface the movers, then gsc_inspect_url on a page that dropped to check whether Google still has it indexed and why.",
    tools: [
      {
        name: "gsc_capabilities",
        summary:
          "Report this server's configuration and readiness: whether credentials resolved, the auth source, the default data-state, whether mutations are enabled, and the tool catalogue. Call this first if other tools error.",
        params: [],
      },
      {
        name: "gsc_list_properties",
        summary: "List every Search Console property the authenticated principal can access, with permission level.",
        params: [],
      },
      {
        name: "gsc_get_site_details",
        summary: "Get details (permission level) for a single property identified by site_url.",
        params: [{ name: "site_url", type: "string", required: true, description: "The Search Console property." }],
      },
      {
        name: "gsc_add_site",
        summary: "Add a property to the account (Sites.add). Mutating; requires GSC_ALLOW_DESTRUCTIVE=true.",
        params: [{ name: "site_url", type: "string", required: true, description: "The property to add." }],
      },
      {
        name: "gsc_delete_site",
        summary: "Remove a property from the account (Sites.delete). Mutating; requires GSC_ALLOW_DESTRUCTIVE=true.",
        params: [{ name: "site_url", type: "string", required: true, description: "The property to remove." }],
      },
      {
        name: "gsc_search_analytics",
        summary: "Search-traffic report for a property over the last N days: top rows by dimension.",
        params: [
          { name: "site_url", type: "string", required: true, description: "The Search Console property." },
          { name: "days", type: "int", default: "28", description: "Look-back window in days ending today." },
          { name: "dimensions", type: "string", default: "query", description: "Comma-separated: query, page, country, device, date, searchAppearance." },
          { name: "row_limit", type: "int", default: "20", description: "Max rows to return." },
        ],
      },
      {
        name: "gsc_advanced_search_analytics",
        summary: "Full-featured Search Analytics query: explicit dates, dimensions, filters, sorting, pagination, and data-state.",
        params: [
          { name: "site_url", type: "string", required: true, description: "The Search Console property." },
          { name: "start_date", type: "string", description: "YYYY-MM-DD; defaults to 28 days before end_date." },
          { name: "end_date", type: "string", description: "YYYY-MM-DD; defaults to today." },
          { name: "search_type", type: "string", default: "WEB", description: "WEB, IMAGE, VIDEO, NEWS, DISCOVER or GOOGLE_NEWS." },
          { name: "row_limit", type: "int", default: "1000", description: "Max rows, up to 25000." },
          { name: "data_state", type: "string", default: "ALL", description: "ALL (fresh data) or FINAL (finalized only)." },
        ],
      },
      {
        name: "gsc_performance_overview",
        summary: "High-level performance summary for a property over the last N days: total clicks, impressions, CTR, position.",
        params: [
          { name: "site_url", type: "string", required: true, description: "The Search Console property." },
          { name: "days", type: "int", default: "28", description: "Look-back window in days." },
        ],
      },
      {
        name: "gsc_compare_periods",
        summary: "Compare Search Analytics between two explicit date ranges (period1 vs period2), grouped by dimension.",
        params: [
          { name: "site_url", type: "string", required: true, description: "The Search Console property." },
          { name: "period1_start", type: "string", required: true, description: "YYYY-MM-DD start of the baseline period." },
          { name: "period1_end", type: "string", required: true, description: "YYYY-MM-DD end of the baseline period." },
          { name: "period2_start", type: "string", required: true, description: "YYYY-MM-DD start of the comparison period." },
          { name: "period2_end", type: "string", required: true, description: "YYYY-MM-DD end of the comparison period." },
          { name: "limit", type: "int", default: "10", description: "Max keys to compare (ranked by period-2 clicks)." },
        ],
      },
      {
        name: "gsc_search_by_page_query",
        summary: "For a single page URL, the search queries that drove impressions/clicks to it over the last N days.",
        params: [
          { name: "site_url", type: "string", required: true, description: "The Search Console property." },
          { name: "page_url", type: "string", required: true, description: "The exact page URL to break down by query." },
          { name: "days", type: "int", default: "28", description: "Look-back window in days." },
          { name: "row_limit", type: "int", default: "20", description: "Max queries to return." },
        ],
      },
      {
        name: "gsc_inspect_url",
        summary: "Inspect a single URL's Google index status (URL Inspection API): verdict, coverage state, and more.",
        params: [
          { name: "site_url", type: "string", required: true, description: "The property that owns the URL." },
          { name: "page_url", type: "string", required: true, description: "The fully-qualified URL to inspect." },
        ],
      },
      {
        name: "gsc_batch_inspect_urls",
        summary: "Inspect up to 10 URLs concurrently and return each one's index status.",
        params: [
          { name: "site_url", type: "string", required: true, description: "The Search Console property." },
          { name: "urls", type: "string", required: true, description: "URLs to inspect, one per line or comma-separated (max 10)." },
        ],
      },
      {
        name: "gsc_check_indexing_issues",
        summary: "Inspect up to 10 URLs and summarize indexing problems: which are indexed vs not, and why.",
        params: [
          { name: "site_url", type: "string", required: true, description: "The Search Console property." },
          { name: "urls", type: "string", required: true, description: "URLs to inspect (max 10)." },
        ],
      },
      {
        name: "gsc_list_sitemaps",
        summary: "List sitemaps for a property with status detail: submitted/downloaded dates, type, pending status.",
        params: [
          { name: "site_url", type: "string", required: true, description: "The Search Console property." },
          { name: "sitemap_index", type: "string", description: "Optional sitemap-index URL to list child sitemaps of." },
        ],
      },
      {
        name: "gsc_get_sitemap",
        summary: "Get full detail for a single sitemap by its feedpath URL, including content breakdown.",
        params: [
          { name: "site_url", type: "string", required: true, description: "The Search Console property." },
          { name: "sitemap_url", type: "string", required: true, description: "The full sitemap URL (feedpath)." },
        ],
      },
      {
        name: "gsc_submit_sitemap",
        summary: "Submit (or resubmit) a sitemap to Search Console. Mutating; requires GSC_ALLOW_DESTRUCTIVE=true.",
        params: [
          { name: "site_url", type: "string", required: true, description: "The Search Console property." },
          { name: "sitemap_url", type: "string", required: true, description: "The full sitemap URL to submit." },
        ],
      },
      {
        name: "gsc_delete_sitemap",
        summary: "Delete/unsubmit a sitemap from Search Console. Mutating; requires GSC_ALLOW_DESTRUCTIVE=true.",
        params: [
          { name: "site_url", type: "string", required: true, description: "The Search Console property." },
          { name: "sitemap_url", type: "string", required: true, description: "The full sitemap URL to delete." },
        ],
      },
    ],
  },
  producthunt: {
    key: "producthunt",
    name: "Product Hunt",
    path: "/producthunt/mcp",
    icon: Rocket,
    tagline:
      "Product Hunt's v2 GraphQL API over MCP: posts, topics, collections, users, and a raw GraphQL escape hatch.",
    requires: {
      caller: "X-API-Key header",
      server: "PRODUCTHUNT_TOKEN (developer token) or PRODUCTHUNT_CLIENT_ID + PRODUCTHUNT_CLIENT_SECRET",
    },
    note: "Read-only: only public queries are exposed, no mutations. Mounts even without a token: call producthunt_capabilities first to check auth. producthunt_viewer needs a user-scoped token.",
    example:
      "Track what launched today. Call producthunt_list_posts ordered by RANKING for a topic like artificial-intelligence, read the discussion with producthunt_get_post_comments, or drop to producthunt_graphql for any field the typed tools do not expose.",
    tools: [
      {
        name: "producthunt_capabilities",
        summary:
          "Report this server's configuration and readiness: whether credentials resolved, the auth source, and the tool catalogue. Call this first if other tools error.",
        params: [],
      },
      {
        name: "producthunt_list_posts",
        summary: "List Product Hunt posts (product launches) with their votes, comments and topics.",
        params: [
          { name: "order", type: "string", default: "RANKING", description: "RANKING, NEWEST, VOTES or FEATURED_AT." },
          { name: "topic", type: "string", description: "Filter to a topic slug, e.g. artificial-intelligence." },
          { name: "posted_after", type: "string", description: "Only posts created after this ISO-8601 timestamp." },
          { name: "first", type: "int", default: "10", description: "Page size, max 20." },
          { name: "after", type: "string", description: "Pagination cursor from a previous call." },
        ],
      },
      {
        name: "producthunt_get_post",
        summary: "Get one Product Hunt post in full by id or slug (the slug from its URL).",
        params: [
          { name: "id", type: "string", description: "The post ID; provide this or slug." },
          { name: "slug", type: "string", description: "The post slug from its URL; provide this or id." },
        ],
      },
      {
        name: "producthunt_get_post_comments",
        summary: "List the comments on a Product Hunt post identified by id or slug.",
        params: [
          { name: "id", type: "string", description: "The post ID; provide this or slug." },
          { name: "slug", type: "string", description: "The post slug; provide this or id." },
          { name: "order", type: "string", default: "VOTES", description: "VOTES or NEWEST." },
          { name: "first", type: "int", default: "20", description: "Page size, max 20." },
        ],
      },
      {
        name: "producthunt_list_topics",
        summary: "List or search Product Hunt topics (categories) with their follower and post counts.",
        params: [
          { name: "query", type: "string", description: "Search topics by name." },
          { name: "order", type: "string", default: "FOLLOWERS_COUNT", description: "FOLLOWERS_COUNT or NEWEST." },
          { name: "first", type: "int", default: "20", description: "Page size, max 20." },
        ],
      },
      {
        name: "producthunt_get_topic",
        summary: "Get one Product Hunt topic by id or slug: description, follower and post counts, and URL.",
        params: [
          { name: "id", type: "string", description: "The topic ID; provide this or slug." },
          { name: "slug", type: "string", description: "The topic slug, e.g. artificial-intelligence; provide this or id." },
        ],
      },
      {
        name: "producthunt_list_collections",
        summary: "List Product Hunt collections (curated lists of posts).",
        params: [
          { name: "featured", type: "bool", description: "Only featured (true) or only non-featured (false); omit for all." },
          { name: "user_id", type: "string", description: "Only collections curated by this user ID." },
          { name: "post_id", type: "string", description: "Only collections that contain this post ID." },
          { name: "first", type: "int", default: "10", description: "Page size, max 20." },
        ],
      },
      {
        name: "producthunt_get_collection",
        summary: "Get one Product Hunt collection by id or slug, including the first posts it contains.",
        params: [
          { name: "id", type: "string", description: "The collection ID; provide this or slug." },
          { name: "slug", type: "string", description: "The collection slug; provide this or id." },
          { name: "first", type: "int", default: "10", description: "How many contained posts to include, max 20." },
        ],
      },
      {
        name: "producthunt_get_user",
        summary: "Get a Product Hunt user's profile by id or username, with a sample of the posts they made.",
        params: [
          { name: "id", type: "string", description: "The user ID; provide this or username." },
          { name: "username", type: "string", description: "The user's username; provide this or id." },
          { name: "posts", type: "int", default: "5", description: "How many of the user's posts to include, max 20." },
        ],
      },
      {
        name: "producthunt_viewer",
        summary: "Return the profile of the user the current token belongs to (the GraphQL viewer). Needs a user-scoped token.",
        params: [],
      },
      {
        name: "producthunt_graphql",
        summary:
          "Run an arbitrary GraphQL query against the Product Hunt v2 API and return the raw data. Escape hatch for anything the typed tools don't expose.",
        params: [
          { name: "query", type: "string", required: true, description: "The GraphQL query or document to execute." },
          { name: "variables", type: "object", description: "Optional variables object referenced by the query." },
        ],
      },
    ],
  },
  event: {
    key: "event",
    name: "Event",
    path: "/event/mcp",
    icon: Radio,
    tagline:
      "Publish and consume Kafka messages over MCP. Produce to a topic, poll it back through a durable consumer group, and manage topics on Confluent Cloud.",
    requires: {
      caller: "X-API-Key header",
      server: "KAFKA_BOOTSTRAP_SERVERS, KAFKA_API_KEY and KAFKA_API_SECRET (Confluent Cloud) · KAFKA_ALLOW_TOPIC_ADMIN=true to create or delete topics",
    },
    note: "Mounts even without credentials: every tool reports the config problem instead of failing the server. Call event_capabilities first to check status. event_publish and event_consume fall back to KAFKA_DEFAULT_TOPIC when no topic is given. Topic create and delete stay disabled until KAFKA_ALLOW_TOPIC_ADMIN=true.",
    example:
      "Wire an agent into your event stream. Call event_publish to emit a record to a Kafka topic, then event_consume from a durable group so a later call picks up only the new messages. Confluent Cloud credentials are all it needs.",
    tools: [
      {
        name: "event_capabilities",
        summary:
          "Report this event server's configuration and readiness: whether a broker and Confluent Cloud credentials are set, the default consumer group and topic, whether topic admin is enabled, and the tool catalogue. Call this first if other tools error.",
        params: [],
      },
      {
        name: "event_publish",
        summary:
          "Publish one message to a Kafka topic and return the partition and offset the record was written to.",
        params: [
          { name: "value", type: "string", required: true, description: "The message payload as a string. JSON is fine." },
          { name: "topic", type: "string", default: "KAFKA_DEFAULT_TOPIC", description: "Topic to publish to." },
          { name: "key", type: "string", description: "Optional partition key. The same key always routes to the same partition." },
          { name: "headers", type: "object", description: "Optional string headers attached to the record." },
        ],
      },
      {
        name: "event_consume",
        summary:
          "Read up to max messages from a Kafka topic, returning within timeout_ms. This is a bounded poll, not a live stream, so call it again to read more.",
        params: [
          { name: "topic", type: "string", default: "KAFKA_DEFAULT_TOPIC", description: "Topic to read." },
          { name: "from", type: "string", default: "group", description: "Where to read from: group (durable offsets that advance across calls), earliest, or latest." },
          { name: "group", type: "string", default: "KAFKA_CONSUMER_GROUP", description: "Consumer group. Only used when from is group." },
          { name: "max", type: "int", default: "10", description: "Max messages to return, capped at 100." },
          { name: "timeout_ms", type: "int", default: "5000", description: "Overall poll budget in milliseconds." },
        ],
      },
      {
        name: "event_topics",
        summary: "List the topics on the Kafka cluster with their partition counts. Read-only.",
        params: [],
      },
      {
        name: "event_create_topic",
        summary: "Create a Kafka topic. Mutating; requires KAFKA_ALLOW_TOPIC_ADMIN=true. Confluent Cloud requires a replication factor of 3.",
        params: [
          { name: "topic", type: "string", required: true, description: "The topic name to create." },
          { name: "partitions", type: "int", default: "1", description: "Partition count." },
          { name: "replication_factor", type: "int", default: "3", description: "Replication factor. Confluent Cloud requires 3." },
        ],
      },
      {
        name: "event_delete_topic",
        summary: "Delete a Kafka topic. Mutating; requires KAFKA_ALLOW_TOPIC_ADMIN=true.",
        params: [{ name: "topic", type: "string", required: true, description: "The topic name to delete." }],
      },
    ],
  },
};

export const serverList = Object.values(servers);

/* ------------------------------------------------------------------ */
/* Navigation                                                          */
/* ------------------------------------------------------------------ */

export type NavItem = { slug: string; label: string; badge?: string };
export type NavGroup = { title: string; items: NavItem[] };

export const nav: NavGroup[] = [
  {
    title: "Get started",
    items: [
      { slug: "overview", label: "Overview" },
      { slug: "quickstart", label: "Quickstart" },
      { slug: "authentication", label: "Authentication" },
      { slug: "keys", label: "API keys" },
    ],
  },
  {
    title: "MCP Servers",
    items: [
      { slug: "memory", label: "Memory" },
      { slug: "skills", label: "Skills" },
      { slug: "gsc", label: "Search Console" },
      { slug: "producthunt", label: "Product Hunt" },
      { slug: "event", label: "Event" },
    ],
  },
  {
    title: "Relivo Platform",
    items: [
      { slug: "platform", label: "Orchestration", badge: "Soon" },
      { slug: "agents", label: "Agents", badge: "Soon" },
      { slug: "workflows", label: "Workflows", badge: "Soon" },
      { slug: "deployments", label: "Deployments", badge: "Soon" },
    ],
  },
  {
    title: "Reference",
    items: [{ slug: "architecture", label: "Architecture" }],
  },
];

export const topNav: { slug: string; label: string }[] = [
  { slug: "overview", label: "Overview" },
  { slug: "memory", label: "Memory" },
  { slug: "skills", label: "Skills" },
  { slug: "architecture", label: "Reference" },
];

/* ------------------------------------------------------------------ */
/* Content blocks                                                      */
/* ------------------------------------------------------------------ */

export type Block =
  | { kind: "lead"; content: ReactNode }
  | { kind: "heading"; id: string; text: string; icon?: LucideIcon }
  | { kind: "text"; content: ReactNode }
  | { kind: "callout"; variant?: "tip" | "info" | "warning"; title?: string; content: ReactNode }
  | { kind: "code"; lang?: string; title?: string; code: string }
  | { kind: "connect"; serverKeys: string[] }
  | { kind: "tools"; serverKey: string }
  | { kind: "skill" }
  | { kind: "cards"; items: { title: string; description: string; href: string; icon: LucideIcon }[] }
  | { kind: "steps"; items: { title: string; content: ReactNode }[] }
  | { kind: "diagram"; variant: "request" | "register" };

export type DocPage = {
  slug: string;
  group: string;
  eyebrow: string;
  title: string;
  description: ReactNode;
  blocks: Block[];
  /** Under-development page: renders a "coming soon" state instead of blocks. */
  wip?: boolean;
  /** For wip pages: an icon and the capabilities this surface will ship. */
  wipIcon?: LucideIcon;
  wipFeatures?: { title: string; body: string }[];
};

/**
 * Best-effort Markdown serialization of a page for the "Copy page" button.
 * Covers the machine-readable blocks (headings, code, connection URLs, tools);
 * rich prose is represented by its heading structure.
 */
export function pageMarkdown(page: DocPage, baseUrl: string): string {
  const lines: string[] = [`# ${page.title}`, ""];
  if (typeof page.description === "string") lines.push(page.description, "");

  for (const b of page.blocks) {
    switch (b.kind) {
      case "heading":
        lines.push(`## ${b.text}`, "");
        break;
      case "code":
        lines.push("```" + (b.lang ?? ""), b.code, "```", "");
        break;
      case "connect":
        for (const key of b.serverKeys) {
          const s = servers[key];
          if (s) lines.push(`- **${s.name}**: \`${baseUrl}${s.path}\``);
        }
        lines.push("");
        break;
      case "tools": {
        const s = servers[b.serverKey];
        if (s)
          for (const t of s.tools) {
            lines.push(`### \`${t.name}\``, "", t.summary, "");
            for (const p of t.params)
              lines.push(`- \`${p.name}\` (${p.type}${p.required ? ", required" : ""}): ${p.description}`);
            lines.push("");
          }
        break;
      }
      case "skill":
        lines.push(
          "Download the stateful-memory Agent Skill (drop into `.claude/skills/stateful-memory/SKILL.md`): `/api/skills/stateful-memory`",
          "",
        );
        break;
    }
  }
  return lines.join("\n").trim() + "\n";
}

/** Table-of-contents entries derive from the heading blocks of a page. */
export function tocOf(page: DocPage) {
  return page.blocks
    .filter((b): b is Extract<Block, { kind: "heading" }> => b.kind === "heading")
    .map((b) => ({ id: b.id, text: b.text }));
}

// A complete, copy-paste-ready MCP client config. URLs come from the env base
// (NEXT_PUBLIC_MCP_BASE_URL). Every namespace sits behind the RequireAPIKey
// middleware, so all five carry the same X-API-Key header -- one key admits the
// whole server. Built with JSON.stringify so it's always valid.
const apiKeyHeaders = { "X-API-Key": "<your-api-key>" };

const clientExample = JSON.stringify(
  {
    mcpServers: {
      memory: {
        type: "http",
        url: `${MCP_BASE_URL}/memory/mcp`,
        headers: apiKeyHeaders,
      },
      skills: {
        type: "http",
        url: `${MCP_BASE_URL}/skills/mcp`,
        headers: apiKeyHeaders,
      },
      gsc: {
        type: "http",
        url: `${MCP_BASE_URL}/gsc/mcp`,
        headers: apiKeyHeaders,
      },
      producthunt: {
        type: "http",
        url: `${MCP_BASE_URL}/producthunt/mcp`,
        headers: apiKeyHeaders,
      },
      event: {
        type: "http",
        url: `${MCP_BASE_URL}/event/mcp`,
        headers: apiKeyHeaders,
      },
    },
  },
  null,
  2,
);

/** The copy-paste MCP client config, reused by the landing page. */
export { clientExample as connectionConfig };

// A copy-paste config scoped to a single namespace, for that server's own doc
// page. Same shape as clientExample -- one X-API-Key header admits the whole
// server -- but trimmed to the one route the reader is looking at.
function serverConfigJson(key: string): string {
  const s = servers[key];
  return JSON.stringify(
    { mcpServers: { [key]: { type: "http", url: `${MCP_BASE_URL}${s.path}`, headers: apiKeyHeaders } } },
    null,
    2,
  );
}

/* ------------------------------------------------------------------ */
/* Worked example for the event page: using the topic as an agent's    */
/* task queue. This documents a pattern that was actually run against  */
/* this server, not a hypothetical -- the envelope below is the real   */
/* one that drove the X-API-Key auth change.                           */
/* ------------------------------------------------------------------ */

const taskEnvelope = JSON.stringify(
  {
    type: "task",
    id: "auth-x-api-key-all-namespaces",
    title: "Require X-API-Key auth verification on every MCP namespace",
    priority: "high",
    source: "go-mcp-server audit",
    summary:
      "Only the memory namespace verifies X-API-Key, and it does so per-tool inside handlers. skills, event, gsc and producthunt accept unauthenticated calls. Enforce the key centrally so every server verifies auth before executing any action.",
    findings: [
      { namespace: "memory", key_required: true },
      { namespace: "event", key_required: false, detail: "Kafka publish reachable unauthenticated." },
    ],
    plan: [
      { step: 1, action: "Add RequireAPIKey middleware validating X-API-Key against the api_keys table." },
      { step: 2, action: "Add it to the Chain in internal/mcpx/registry.go so it covers every namespace." },
    ],
    open_questions: ["Should /healthz remain unauthenticated?"],
  },
  null,
  2,
);

const publishCall = `event_publish({
  topic: "go-mcp-events",
  key: "auth-x-api-key",          // same key -> same partition -> ordered
  headers: {
    "event-type": "task.created",
    "domain": "security",
    "producer": "ci-audit"
  },
  value: JSON.stringify(task)     // value is a string; stringify your JSON
})

// -> { "topic": "go-mcp-events", "partition": 3, "offset": 1 }`;

const consumeCall = `event_consume({
  topic: "go-mcp-events",
  from: "group",                  // durable: offsets advance across calls
  max: 20,
  timeout_ms: 20000
})

// -> { "count": 1, "group": "go-mcp-server", "messages": [ ... ] }
// Call it again and count is 0 — the offset was committed.`;

const eventTaskQueueWalkthrough: Block[] = [
  { kind: "heading", id: "task-queue", text: "Worked example: a task queue for agents", icon: Workflow },
  {
    kind: "text",
    content: (
      <>
        The two tools above compose into something more useful than message
        plumbing: a <strong>durable inbox for an agent</strong>. A publisher
        &mdash; CI, a cron job, another agent, or you &mdash; drops a task on the
        topic. An agent polls the topic and carries the task out. Neither side has
        to be running at the same time, which is the whole point of putting a log
        between them.
      </>
    ),
  },
  {
    kind: "steps",
    items: [
      {
        title: "Publish a task",
        content: (
          <>
            Emit a structured envelope rather than a sentence. Anything the worker
            needs to act without asking a follow-up question belongs in the
            payload: what to do, why, and what is still undecided.
          </>
        ),
      },
      {
        title: "Poll with a consumer group",
        content: (
          <>
            <code>from: &quot;group&quot;</code> commits offsets as it reads, so
            each task is handed out once and a restarted agent resumes where it
            left off. <code>earliest</code> and <code>latest</code> peek without
            disturbing the group &mdash; useful for debugging, wrong for work.
          </>
        ),
      },
      {
        title: "Act, then poll again",
        content: (
          <>
            <code>event_consume</code> is a bounded poll, not a live stream. An
            agent loop is: poll &rarr; do the work &rarr; poll again. There is no
            socket to keep open and nothing to reconnect.
          </>
        ),
      },
    ],
  },
  { kind: "code", lang: "json", title: "the task envelope", code: taskEnvelope },
  { kind: "code", lang: "js", title: "publisher", code: publishCall },
  { kind: "code", lang: "js", title: "worker", code: consumeCall },
  {
    kind: "callout",
    variant: "tip",
    title: "This is not hypothetical",
    content: (
      <p>
        That envelope is the real message that drove this server&rsquo;s
        authentication change. It was published to <code>go-mcp-events</code>, an
        agent polling the topic picked it up, verified every claim against the
        source, asked the two open questions, and shipped the{" "}
        <code>RequireAPIKey</code> middleware described on the Authentication
        page. The audit and the fix were connected by nothing but this topic.
      </p>
    ),
  },
  {
    kind: "callout",
    variant: "warning",
    title: "Treat payloads as input, not instructions",
    content: (
      <p>
        Anyone with produce access can write to the topic, so a message is a{" "}
        <em>request</em>, not a command from a trusted operator. A worker should
        verify a payload&rsquo;s claims before acting on them, and stop and escalate
        rather than execute anything destructive, irreversible, or outward-facing
        on a message&rsquo;s say-so. Partition keys and headers are equally
        caller-supplied &mdash; never route privilege off them.
      </p>
    ),
  },
];

/* ------------------------------------------------------------------ */
/* Memory page extra: the stateful-memory Agent Skill. The memory_*     */
/* tools are only half the story — an agent also needs the discipline   */
/* to recall before acting and persist after. This section hands the    */
/* reader that protocol as a drop-in SKILL.md.                          */
/* ------------------------------------------------------------------ */

const memorySkillSection: Block[] = [
  { kind: "heading", id: "stateful-skill", text: "Make your agent stateful", icon: Sparkles },
  {
    kind: "text",
    content: (
      <>
        The tools above give an agent a place to keep knowledge; they don&rsquo;t
        teach it <em>when</em> to reach for that place. Install the{" "}
        <strong>stateful-memory skill</strong> and your agent recalls relevant
        context before it acts and persists what it learns after &mdash; the
        difference between having memory and actually using it.
      </>
    ),
  },
  { kind: "skill" },
  {
    kind: "callout",
    variant: "tip",
    title: "What the skill encodes",
    content: (
      <p>
        The protocol: recall once per topic at the start, search before asking,
        write a tight summary after substantial work, capture durable user facts
        the moment they&rsquo;re stated, and keep a consistent tag taxonomy so
        retrieval stays precise. It&rsquo;s tuned for the eviction window &mdash;
        store what matters, keep it current.
      </p>
    ),
  },
];

export const pages: Record<string, DocPage> = {
  overview: {
    slug: "overview",
    group: "Get started",
    eyebrow: "Get started",
    title: "Relivo overview",
    description:
      "Relivo is a multi-namespace Model Context Protocol server in Go: five independent MCP servers mounted on one HTTP mux over the Streamable HTTP transport.",
    blocks: [
      {
        kind: "lead",
        content: (
          <>
            <strong>One server, five namespaces.</strong> Relivo exposes{" "}
            <code>memory</code>, <code>skills</code>, <code>gsc</code>,{" "}
            <code>producthunt</code>, and <code>event</code> as independent MCP
            servers on a single <code>net/http</code> mux. Each namespace
            self-registers at startup via <code>init()</code>, so adding one is a
            single package and <code>cmd/server/main.go</code> never changes. Point
            any MCP client at a namespace route and its tools appear.
          </>
        ),
      },
      {
        kind: "callout",
        variant: "tip",
        title: "Pick a namespace",
        content: (
          <>
            <p>
              <strong>Memory</strong> holds durable, per-user knowledge in a hybrid
              RAG store (semantic and keyword) over Postgres and pgvector that
              survives across sessions.
            </p>
            <p>
              <strong>Skills</strong> finds and downloads Agent Skills live from
              GitHub. Nothing is cached.
            </p>
            <p>
              <strong>Search Console</strong> exposes Google Search Console:
              property management, search-analytics reports, URL inspection, and
              sitemaps.
            </p>
            <p>
              <strong>Product Hunt</strong> wraps the Product Hunt v2 GraphQL API:
              posts, topics, collections, users, plus a raw GraphQL escape hatch.
            </p>
            <p>
              <strong>Event</strong> is a Kafka bridge over MCP: publish messages
              to a topic, consume them back through a durable consumer group, and
              manage topics on Confluent Cloud.
            </p>
          </>
        ),
      },
      { kind: "heading", id: "the-namespaces", text: "The namespaces", icon: Layers },
      {
        kind: "text",
        content: (
          <>
            Each route below is a fully independent MCP server. A domain never
            reaches across another domain except through <code>internal/mcpx</code>.
          </>
        ),
      },
      {
        kind: "cards",
        items: [
          { title: "Memory", description: "Store & recall per-user memories with hybrid RAG.", href: "/doc/memory", icon: Brain },
          { title: "Skills", description: "Find & download Agent Skills live from GitHub.", href: "/doc/skills", icon: Sparkles },
          { title: "Search Console", description: "GSC analytics, URL inspection & sitemaps.", href: "/doc/gsc", icon: LineChart },
          { title: "Product Hunt", description: "Posts, topics, collections & raw GraphQL.", href: "/doc/producthunt", icon: Rocket },
          { title: "Event", description: "Publish & consume Kafka messages, and manage topics.", href: "/doc/event", icon: Radio },
        ],
      },
      { kind: "heading", id: "connect", text: "Connect", icon: Compass },
      {
        kind: "text",
        content: (
          <>
            Every URL below is derived from{" "}
            <code>NEXT_PUBLIC_MCP_BASE_URL</code>. Change it once and it updates
            everywhere in these docs.
          </>
        ),
      },
      { kind: "connect", serverKeys: ["memory", "skills", "gsc", "producthunt", "event"] },
      {
        kind: "text",
        content: <>Drop the routes into any MCP client config:</>,
      },
      { kind: "code", lang: "json", title: "mcp.json", code: clientExample },
      {
        kind: "callout",
        variant: "info",
        title: "Headers",
        content: (
          <>
            <p>
              Every namespace requires an <code>X-API-Key</code> header carrying a
              registered key &mdash; add it to each server&rsquo;s{" "}
              <code>headers</code>. Requests without one are rejected with{" "}
              <code>401</code> before they reach a tool.
            </p>
            <p>
              The <strong>memory</strong> namespace additionally uses the key as an
              identity: your memories are scoped to it and stay isolated from other
              callers&rsquo;.
            </p>
          </>
        ),
      },
    ],
  },

  quickstart: {
    slug: "quickstart",
    group: "Get started",
    eyebrow: "Get started",
    title: "Quickstart",
    description: "Run the server and connect your first MCP client in under a minute.",
    blocks: [
      {
        kind: "lead",
        content: (
          <>
            You need <strong>Go 1.26+</strong>, <strong>Postgres + pgvector</strong>{" "}
            (for the memory namespace), and an <strong>OpenAI API key</strong> for
            embeddings.
          </>
        ),
      },
      { kind: "heading", id: "run-the-server", text: "Run the server", icon: Rocket },
      {
        kind: "steps",
        items: [
          { title: "Configure your environment", content: <>Copy the example env and fill in <code>DATABASE_URL</code> and <code>OPENAI_API_KEY</code>.</> },
          { title: "Apply the migration", content: <>Run <code>migrations/0001_memories.sql</code> against your pgvector database.</> },
          { title: "Mint an API key", content: <><code>make apikey</code> inserts a key into <code>api_keys</code> and prints it. Every namespace requires it, so do this before connecting a client.</> },
          { title: "Start it", content: <>The server listens on <code>:8080</code> (override with <code>PORT</code>).</> },
        ],
      },
      { kind: "code", lang: "bash", title: "terminal", code: "cp .env.example .env\n# fill in DATABASE_URL + OPENAI_API_KEY\nmake migrate\nmake apikey          # prints mcp_<32 hex> — use it below\nmake run" },
      {
        kind: "callout",
        variant: "info",
        title: "DB-backed only for memory",
        content: (
          <>
            The server refuses to start without <code>DATABASE_URL</code> because
            the memory namespace requires it. The skills and event namespaces have
            no hard startup dependency.
          </>
        ),
      },
      { kind: "heading", id: "point-a-client", text: "Point a client at it", icon: Compass },
      { kind: "text", content: <>Give your MCP client a namespace URL, and the key from <code>make apikey</code> as the <code>X-API-Key</code> header on every entry:</> },
      { kind: "connect", serverKeys: ["memory", "skills", "gsc", "producthunt", "event"] },
      { kind: "code", lang: "json", title: "mcp.json", code: clientExample },
      {
        kind: "callout",
        variant: "warning",
        title: "Behind a tunnel?",
        content: (
          <>
            Set <code>MCP_ALLOW_EXTERNAL_HOST=true</code> when serving through
            ngrok or a reverse proxy. The transport&rsquo;s DNS-rebinding protection
            rejects loopback requests carrying a non-loopback <code>Host</code>{" "}
            header otherwise.
          </>
        ),
      },
    ],
  },

  authentication: {
    slug: "authentication",
    group: "Get started",
    eyebrow: "Get started",
    title: "Authentication",
    description: "How requests are admitted and scoped across the namespaces.",
    blocks: [
      {
        kind: "lead",
        content: (
          <>
            One credential secures the whole server: every namespace requires an{" "}
            <code>X-API-Key</code> header carrying a registered key. There is no
            second mechanism and no per-namespace exception.
          </>
        ),
      },
      { kind: "heading", id: "api-key", text: "Admission (X-API-Key)", icon: KeyRound },
      {
        kind: "text",
        content: (
          <>
            The <code>RequireAPIKey</code> middleware guards the whole mux, so every
            namespace verifies the key <em>before</em> dispatch &mdash; a namespace
            cannot forget the check, and a new one inherits it for free. A missing,
            malformed, or unregistered key gets a <code>401</code> and no tool ever
            runs. <code>GET /healthz</code> is the one exempt route, so an uptime
            probe needs no credential; it reveals only liveness.
          </>
        ),
      },
      { kind: "code", lang: "bash", title: "request header", code: "X-API-Key: <your-key>" },
      { kind: "heading", id: "scope", text: "Per-caller scope (memory)", icon: KeyRound },
      {
        kind: "text",
        content: (
          <>
            The memory namespace resolves the same key a second time, to an{" "}
            <code>api_key_id</code> that scopes every store call &mdash; so one caller
            can never read another&rsquo;s memories. That is the key used as an{" "}
            <strong>identity</strong>, not merely as admission, which is why the check
            lives in both places.
          </>
        ),
      },
      {
        kind: "callout",
        variant: "info",
        title: "Admission vs. scoping",
        content: (
          <>
            Every namespace requires a key to be reached. Only memory uses it to
            partition data &mdash; the other tools hold no per-caller state, so any
            valid key sees the same thing.
          </>
        ),
      },
    ],
  },

  memory: serverPage(
    "memory",
    "Durable, per-user memory over hybrid RAG. Store facts once and recall them by meaning across sessions.",
    memorySkillSection,
  ),
  skills: serverPage("skills", "Locate and pull Agent Skills live from GitHub."),
  gsc: serverPage("gsc", "Google Search Console over MCP: property management, search-analytics reporting, URL inspection, and sitemaps."),
  producthunt: serverPage("producthunt", "Product Hunt's v2 GraphQL API over MCP: browse posts, topics, collections, and users, or run raw GraphQL."),
  event: serverPage(
    "event",
    "A Kafka bridge over MCP: publish messages to a topic, consume them back through a durable consumer group, and manage topics on Confluent Cloud.",
    eventTaskQueueWalkthrough,
  ),

  platform: platformPage("platform", {
    icon: Waypoints,
    title: "Agent orchestration",
    description:
      "The Relivo app is where you compose agents, tools, and workflows into agentic pipelines visually, on top of the MCP servers documented here.",
    features: [
      { title: "Visual canvas", body: "Wire agents, tools, and control flow together on a node graph, with no glue code." },
      { title: "MCP-native", body: "Drop in Relivo's memory, skills, Search Console, and Product Hunt servers as first-class tools." },
      { title: "Run & observe", body: "Execute pipelines and trace every step, input, and output in one place." },
    ],
  }),
  agents: platformPage("agents", {
    icon: Bot,
    title: "Agents",
    description:
      "Define agents (model, system prompt, tool access, and guardrails) and compose them into higher-order agents.",
    features: [
      { title: "Configurable", body: "Pick the model, write the system prompt, and set limits per agent." },
      { title: "Scoped tools", body: "Grant each agent exactly the MCP tools its job needs, nothing more." },
      { title: "Sub-agents", body: "Let an agent delegate to other agents for multi-step reasoning." },
    ],
  }),
  workflows: platformPage("workflows", {
    icon: Workflow,
    title: "Workflows",
    description:
      "Chain agents and tools into multi-step workflows with branching, loops, and triggers.",
    features: [
      { title: "Multi-step flows", body: "Sequence agents and tools with conditional branches and loops." },
      { title: "Triggers", body: "Kick off runs on a schedule, a webhook, or on demand." },
      { title: "Observability", body: "Inspect each run end to end, including every step, retry, and payload." },
    ],
  }),
  deployments: platformPage("deployments", {
    icon: Rocket,
    title: "Deployments",
    description:
      "Promote a workflow to a hosted endpoint, version it, and roll changes across environments.",
    features: [
      { title: "Ship it", body: "Turn a workflow into a callable, hosted endpoint in a click." },
      { title: "Versioning", body: "Roll revisions forward and back with confidence." },
      { title: "Environments", body: "Keep dev and prod configuration cleanly separated." },
    ],
  }),

  architecture: {
    slug: "architecture",
    group: "Reference",
    eyebrow: "Reference",
    title: "Architecture",
    description: "How the namespaces are wired onto one mux, and why adding one never touches main.go.",
    blocks: [
      {
        kind: "lead",
        content: (
          <>
            Namespaces <strong>self-register</strong> through{" "}
            <code>internal/mcpx</code>. Each package calls <code>mcpx.Register</code>{" "}
            from its <code>init()</code>; the mux mounts every registered namespace
            on the next start.
          </>
        ),
      },
      { kind: "heading", id: "request-lifecycle", text: "Request lifecycle", icon: Waypoints },
      {
        kind: "text",
        content: (
          <>
            Every request passes global middleware &mdash; panic recovery, then
            logging &mdash; hits the stdlib mux, and is admitted by the auth that
            belongs to <em>that route</em>: <code>X-API-Key</code> for the MCP
            namespaces, Clerk for the dashboard&rsquo;s key API, nothing for the
            liveness probe.
          </>
        ),
      },
      { kind: "diagram", variant: "request" },
      { kind: "heading", id: "self-registration", text: "Self-registration", icon: Workflow },
      {
        kind: "text",
        content: (
          <>
            Each package calls <code>mcpx.Register</code> from its{" "}
            <code>init()</code>, so importing it is enough to enroll the namespace.{" "}
            <code>mcpx.Handler</code> then builds every registered server,{" "}
            <strong>failing fast</strong> if any can&rsquo;t &mdash; rather than
            mounting a half-working server that only breaks on the first tool call.
          </>
        ),
      },
      { kind: "diagram", variant: "register" },
      { kind: "heading", id: "layout", text: "Layout", icon: Layers },
      {
        kind: "code",
        lang: "text",
        title: "internal/",
        code:
          "cmd/server/main.go     # config, graceful shutdown\ninternal/\n  mcpx/                # registry, Handler(), Chain()\n  memory/              # per-user memories, hybrid RAG (pg + pgvector)\n  skills/              # skills_find agent + skills_download + web tools\n  gsc/                 # Google Search Console (analytics, inspection, sitemaps)\n  producthunt/         # Product Hunt v2 GraphQL API\n  event/               # Kafka publish/consume + topic admin (Confluent Cloud)",
      },
      { kind: "heading", id: "adding", text: "Adding a namespace", icon: Wrench },
      {
        kind: "steps",
        items: [
          { title: "Create the package", content: <>Add <code>internal/&lt;name&gt;/register.go</code> that calls <code>mcpx.Register</code> from <code>init()</code>.</> },
          { title: "Blank-import it", content: <>Add the package to <code>cmd/server/main.go</code> imports. The mux picks it up on the next start.</> },
        ],
      },
      {
        kind: "callout",
        variant: "tip",
        title: "One boundary",
        content: <>Router is stdlib <code>net/http</code>; a domain never reaches across another domain except through <code>internal/mcpx</code>.</>,
      },
    ],
  },
};

/** Build a standard server reference page from the server data. */
function serverPage(key: string, description: string, extra: Block[] = []): DocPage {
  const s = servers[key];
  return {
    slug: key,
    group: "MCP Servers",
    eyebrow: "MCP Server",
    title: `${s.name} server`,
    description,
    blocks: [
      { kind: "lead", content: <>{s.tagline}</> },
      ...(s.example
        ? ([{ kind: "callout", variant: "tip", title: "How it helps", content: <p>{s.example}</p> }] as Block[])
        : []),
      { kind: "heading", id: "connect", text: "Connect", icon: Compass },
      { kind: "text", content: <>Point your MCP client at the {s.name.toLowerCase()} route:</> },
      { kind: "connect", serverKeys: [key] },
      { kind: "text", content: <>Drop this into your client&rsquo;s config and swap in the key you mint on the <Link href="/doc/keys" className="font-medium text-foreground underline underline-offset-2">Keys</Link> page:</> },
      { kind: "code", lang: "json", title: "mcp.json", code: serverConfigJson(key) },
      ...(s.requires
        ? ([
            {
              kind: "callout",
              variant: "info",
              title: "What you provide",
              content: (
                <>
                  <p>
                    Every request carries your <code>{s.requires.caller}</code> &mdash; mint one on the{" "}
                    <Link href="/doc/keys" className="font-medium text-foreground underline underline-offset-2">Keys</Link>{" "}
                    page. That single key admits the whole server.
                  </p>
                  {s.requires.userProvided && (
                    <p>
                      This namespace also runs against <strong>your own</strong>{" "}
                      {s.requires.userProvided}, connected to your account.
                    </p>
                  )}
                </>
              ),
            },
          ] as Block[])
        : []),
      ...(s.note
        ? ([{ kind: "callout", variant: "tip", title: "Good to know", content: <p>{s.note}</p> }] as Block[])
        : []),
      { kind: "heading", id: "tools", text: "Tools", icon: Wrench },
      { kind: "tools", serverKey: key },
      ...extra,
    ],
  };
}

/** Build an under-development page for a Relivo Platform surface. */
function platformPage(
  slug: string,
  opts: {
    icon: LucideIcon;
    title: string;
    description: string;
    features: { title: string; body: string }[];
  },
): DocPage {
  return {
    slug,
    group: "Relivo Platform",
    eyebrow: "Relivo Platform",
    title: opts.title,
    description: opts.description,
    blocks: [],
    wip: true,
    wipIcon: opts.icon,
    wipFeatures: opts.features,
  };
}

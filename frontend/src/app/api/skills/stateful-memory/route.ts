import { statefulSkillMarkdown } from "@/lib/stateful-skill";

// Serves the stateful-memory Agent Skill as a downloadable SKILL.md. Kept as a
// GET with an attachment disposition so it works both from the docs download
// button and straight from a terminal:
//
//   curl -sL <origin>/api/skills/stateful-memory -o SKILL.md
//
// The body is a static string (no auth, no per-user state), so it can be cached
// hard at the edge.

export const runtime = "nodejs";
export const dynamic = "force-static";

export function GET() {
  return new Response(statefulSkillMarkdown, {
    status: 200,
    headers: {
      "Content-Type": "text/markdown; charset=utf-8",
      "Content-Disposition": 'attachment; filename="SKILL.md"',
      "Cache-Control": "public, max-age=3600, s-maxage=86400",
    },
  });
}

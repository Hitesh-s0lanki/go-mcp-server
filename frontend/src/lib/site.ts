/** Brand + project constants, in one place. */
export const site = {
  name: "Relivo",
  fullName: "Relivo MCP Server",
  tagline: "Memory, skills, and live data sources behind one MCP endpoint.",
  description:
    "An open-source, multi-namespace Model Context Protocol server in Go. Five independent MCP servers on one HTTP mux over the Streamable HTTP transport.",
  github: "https://github.com/Hitesh-s0lanki/go-mcp-server",
  /** The main Relivo app, currently under active development. */
  app: "https://relivo-fe.vercel.app/",
  appStatus: "In development",
  /** The Relivo orchestration console (agents, workflows, deployments); WIP preview. */
  console: "https://master.d2p4p6tfmpfvri.amplifyapp.com/projects/relivo",
  license: "MIT",
  goVersion: "Go 1.26+",
} as const;

# ResultVPC — Codex Instructions

## Skills

This project uses skills defined in `.agents/skills/`. Key skills are imported below and apply automatically. Additional skills are available via the `Skill` tool.

---

## Always Active

### Skill Discovery & Brainstorming
@.agents/skills/brainstorming-ideas-into-designs/SKILL.md

### Verification Before Completion
@.agents/skills/verification-before-completion/SKILL.md

### Systematic Debugging
@.agents/skills/systematic-debugging/SKILL.md

### Frontend Design
@.agents/skills/frontend-design/SKILL.md

### Web Design Guidelines
@.agents/skills/web-design-guidelines/SKILL.md

### Karpathy Guidelines
@.agents/skills/karpathy-guidelines/SKILL.md

---

## Available Skills (invoke via Skill tool when needed)

| Skill | When to use |
|---|---|
| `brainstorming-ideas-into-designs` | Before any new feature, component, or behavior change — explores intent before implementation |
| `writing-plans` | When given a spec or multi-step requirements, before touching code |
| `executing-plans` | When executing a written implementation plan with review checkpoints |
| `test-driven-development-tdd` | When implementing any feature or bugfix, before writing implementation code |
| `dispatching-parallel-agents` | When facing 2+ independent tasks with no shared state |
| `subagent-driven-development` | When executing implementation plans with independent tasks in the current session |
| `using-git-worktrees` | When starting feature work that needs isolation from current workspace |
| `requesting-code-review` | When completing tasks or before merging |
| `code-review-reception` | When receiving code review feedback before implementing suggestions |
| `finishing-a-development-branch` | When implementation is complete and ready to integrate |
| `writing-skills` | When creating or editing skills in `.agents/skills/` |
| `shadcn` | When working with shadcn/ui components or any project with `components.json` |
| `vercel-react-best-practices` | When writing or reviewing React / Next.js code for performance and correctness |
| `supabase-postgres-best-practices` | When writing, reviewing, or optimizing Postgres queries, schema, or RLS |
| `canvas-design` | When generating posters, visual art, or design assets as PNG/PDF via code |
| `mcp-builder` | When building MCP servers in Python (FastMCP) or TypeScript |
| `frontend-design` | When building web components, pages, or UI artifacts requiring high design quality |
| `web-design-guidelines` | When auditing UI for accessibility, UX, or web interface best practices |
| `systematic-debugging` | When diagnosing any bug — find root cause before attempting fixes |
| `verification-before-completion` | Before claiming any task is done — run and read verification output |

---
name: review-docs
description: Review Agentgram documentation (CLAUDE.md, READMEs, SECURITY.md, runbooks). Improve clarity, consistency, and accuracy.
argument-hint: "[paths|files|folders] (optional)"
disable-model-invocation: true
---

Act as a Technical Writer + Senior Engineer + QA. Your goal is to review and clean Agentgram documentation specified in $ARGUMENTS (or current context if no arguments) to make it clear, correct, consistent, and maintainable.

## Project context

- **Doc structure**: `CLAUDE.md` (root), `api/CLAUDE.md`, `web/CLAUDE.md`, `api/docs/SECURITY.md`
- **API**: Go multiplexer, AG-UI SSE protocol, Keycloak JWT auth, Redis + PostgreSQL sessions
- **Web**: Next.js 16, SSE streaming, parallel multi-agent support. UI text in Spanish (es).
- **Commands**: `make api`, `make web`, `make docker-up`, `make image-all`, `make test`
- **Config**: YAML with `${ENV:VAR}` syntax for secrets
- **Deployment**: Kubernetes, images to `ghcr.io/dfradehubs/`

## Deliver in this format

A) SUMMARY
- Status: ✅ Ready / ⚠️ Requires adjustments / ❌ Inconsistent or dangerous
- Top 5 issues (prioritized)
- Minimum actions to reach "✅ Ready"

B) FINDINGS (prioritized)
For each finding include:
- Severity: P0 (blocking) / P1 / P2 / P3
- Evidence: file:section (or exact heading)
- Problem: what's confusing or wrong
- Proposed fix: suggested text or concrete restructuring (in Markdown)

C) REWRITE PROPOSAL (if applicable)
- Proposed index (TOC) or recommended structure
- Sections to merge/delete/move
- List of normalized "names/terminology"

## Review criteria

1) Accuracy and currency
- Detect obsolete content: old paths (`backend/`, `frontend/`), deprecated commands, wrong port numbers.
- Flag contradictions between `CLAUDE.md` (root), `api/CLAUDE.md`, `web/CLAUDE.md`, and `api/docs/SECURITY.md`.
- Verify AG-UI event types and SSE format match actual implementation.
- Check Makefile targets match documented commands.

2) Clarity and readability
- Long sentences, ambiguities, logical jumps.
- Rewrite so a new developer understands the "what", "why", and "how".
- Add minimum context: prerequisites, limits, gotchas.

3) Editorial and technical consistency
- Unify terminology: "API" vs "backend", "Web" vs "frontend", "agent" vs "service".
- Normalize command examples (shell fenced, consistent prompt).
- Maintain convention: "imperative" for steps ("Execute...", "Verify...").
- Consistent naming: `agentgram-api`, `agentgram-web` for images/K8s resources.

4) Security and compliance
- Find and remove/anonymize secrets, tokens, credentials, internal URLs, or PII.
- Verify docs recommend `${ENV:VAR}` for secrets, never literal values.
- Flag any docs suggesting `auth.enabled: false` or `POSTGRES_SSLMODE: disable` without dev-only context.

5) Operations
- Verify deployment docs cover: build → push → update values → rollout.
- Check `make image-all` / `make image-api` / `make image-web` are documented.
- Flag non-deterministic steps or tribal knowledge dependencies.

6) Actionability
- Each section must allow executing the task without guessing:
  - prerequisites
  - concrete commands (`make`, `go`, `npx`)
  - examples of expected inputs/outputs (AG-UI events, API requests)
  - relevant internal links (no broken links)

## Rules
- Don't invent tools/processes: if data is missing, mark "NEEDS CONFIRMATION" and propose where to verify in the repo.
- Minimize meaning changes: prioritize clarity and correctness, not style for style's sake.
- When proposing text, deliver it ready to paste into Markdown.
# Verification — round 2

All sources accessed 2026-10-12. "Primary" = the GitHub artifact itself, read through the `gh` API, not through anyone's report of it.

## Results

### C1 — Anthropic subscription-login policy; how T3 Code authenticates Claude
**Status: verified for the policy core. Corrected on billing. The part about Anthropic commenting on T3 Code or other GUIs is unverified.**
- Independent source 1: GIGAZINE, "Anthropic officially bans third-party subscription authentication", https://gigazine.net/gsc_news/en/20260220-anthropic-third-party-block, published 2026-02-20. It quotes the Anthropic docs: Anthropic "does not permit third parties to provide Claude.ai logins" or to "use Free, Pro, or Max plan credentials to route requests on your behalf"; products built on the Agent SDK must use API keys. It also quotes Thariq Shihipar (Anthropic) on X, 2026-02-18: "encourage local development and experimentation with the Agent SDK and claude -p … If you're building a business on top of the Agent SDK, you should use an API key instead."
- Independent source 2: Danilo Falcão da Silva, falcao.org, https://falcao.org/posts/anthropic-claude-access-crackdown-ecosystem-fallout/, published 2026-05-24. This is the enforcement timeline:
  - 2026-01-09: server-side block on subscription OAuth outside the genuine Claude Code binary.
  - 2026-02-17 to 02-20: terms updated.
  - 2026-03-19: legal demands to OpenCode, which then removed Anthropic OAuth.
  - 2026-04-04: Pro and Max no longer cover third-party tools.
  - 2026-05-13: "Agent SDK credits" announced, effective 2026-06-15.
  - The block targeted clients that spoofed the Claude Code identity. API keys were never blocked.
- Same vendor, different artifact: Anthropic Help Center, "Use the Claude Agent SDK with your Claude plan", https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-claude-plan, last dated note 2026-10-07. **Correction:** the change to Agent SDK billing planned for June 2026 was paused. As of 2026-10-07, "Agent SDK, `claude -p`, and third-party app usage still draw from your subscription limits". Max and Team plans now also include monthly API credits, which apply only when you use a Console API key. The article does not restate the rule against third parties offering claude.ai login. It neither confirms nor withdraws it.
- How T3 Code authenticates (primary): `docs/user/providers-claude.md` says "T3 Code uses Claude Code's login and configuration". Users sign in with `claude auth login`, with an optional `CLAUDE_CONFIG_DIR` for each account. `apps/server/package.json` depends on `@anthropic-ai/claude-agent-sdk` ^0.3.276. So T3 Code drives the user's locally logged-in Claude Code through the Agent SDK. It does not run its own OAuth flow.
- Anthropic on T3 Code, Conductor, Crystal or opcode: no Anthropic statement found. pingdotgg/t3code issue #991 (March 2026) contains only community speculation. One user says they have used Conductor for a long time without a ban. Another separates "subscription OAuth token for your own requests" (risky) from "CC CLI supported programmatic use via headless mode". Treat any policy position on these tools as unverified.

### C2 — No multi-repo support; Discussion #7041 open; PR #15752 open and unreviewed
**Status: verified (primary, rechecked live).**
- Discussion #7041, "[Question] Is there a way to work with multi-repo projects?": created 2026-03-27, still open, not answered, 62 upvotes, 29 comments. No comment or reply from a MEMBER, OWNER or COLLABORATOR.
- PR #15752, "feat: one project can hold several Git repositories", by Tr1Fecta-7: opened 2026-10-04, OPEN, updated 2026-10-12. There is no review decision. The only reviews come from bots (coderabbitai, macroscopeapp) and the author. Labels are `vouch:unvouched` and `size:XXL`.
- Maintainer statements found (primary):
  - Julius Marminge closed the earlier PR #11251 on 2026-09-19: "We're not taking changes to the orchestration and provider layers right now … being rewritten for V2 … open a fresh PR against the new code and we'll take a proper look." That invites a resubmission. It does not commit to the feature.
  - His triage of #12089 (2026-09-16) calls cross-repo changed files "a real enhancement".
- Nothing found from Theo Browne on X or YouTube. Web search for those was not done (budget).
- Workaround seen in the community: LoganRupe's fork branch `multi-repo-workspaces` carries #15752 plus support for `.code-workspace` files.

### C3 — v2 rewrite (PR #2829) ignored backward compatibility; aaditagrawal fork unmaintained; TrogonStack PR #71
**Status: partly disputed. The facts hold, but the framing needs correcting.**
- PR #2829, "feat(orchestrator): introduce new orchestrator", merged 2026-10-02T19:22Z (primary).
- **"Intentionally ignored backward compatibility" is not supported as worded.** The PR body describes a `LegacyV1ThreadImporter`, "prove v1 to v2 cutover on a copied database", and migrations that include "legacy import state". User data was migrated. Compatibility did break in one place: the client/server protocol.
  - Primary: the android-nightly release note (2026-10-12) says "The Play Store release can't connect to a nightly server yet."
  - Independent: SmartScope, https://smartscope.blog/en/blog/t3-code-client-not-supported-mobile-v2-2026/, about 2026-10-03, reports that V2 servers reject V1 mobile clients with "Client not supported".
  - Internal APIs did change in ways that broke forks. Maintainers closed outside PRs ahead of V2 (#11251).
- aaditagrawal/t3code (primary): archived. Last commit 2026-10-09, "docs: point users to upstream t3code". The README says "no longer maintained" **because upstream "now ships everything this fork added, through its ACP integration"**, not because of v2 breakage. **"Large fork" is wrong:** it has 27 stars and 4 forks.
- TrogonStack/t3code PR #71, "chore: sync with upstream", merged 2026-10-02T20:59Z (primary). It dropped fork patches 0003, 0006, 0011, 0009 and 0019, the ACP watchdog, trace joining, and the dev marker. It kept or adapted 0016 (1Password secrets), 0008 (folder drop) and the CI pins. "Most divergences" is roughly fair. Several drops were supersessions by upstream rather than forced losses. The fork has 0 stars.

### C4 — Windows tests not gating; data-loss bug #14990
**Status: verified (primary, rechecked live).**
- `.github/workflows/windows-tests.yml` runs on `workflow_dispatch` only. Its header comment reads: "Manual only: nothing in the suite passes on Windows yet … Once the suite is green here, fold it into ci.yml."
- #14990, "Removing a worktree on Windows follows directory junctions and deletes the files they point to": opened 2026-10-03, still OPEN.
- Fix PRs are both still OPEN and unmerged:
  - #15833: no review decision.
  - #17076: CHANGES_REQUESTED.
- Recent nightlies do merge individual Windows test fixes, for example #18450 "terminal kill test passes on Windows hosts" (2026-10-12). Windows work is happening. It is not gating.

### C5 — Vibe Kanban: Bloop shut down 2026-04-10; community-maintained; last release v0.1.44 2026-04-24
**Status: verified, with an addition.**
- Primary: https://www.vibekanban.com/blog/shutdown, dated 2026-04-10. It says the company is shutting down, the project stays "open source and community maintained", remote services last 30 days, and a community roadmap would come "over the next few weeks".
- Third-party coverage of the same facts: Nimbalyst, https://www.nimbalyst.com/blog/best-vibe-kanban-alternatives-2026/. Nimbalyst is a competitor. I did not fetch this page; only the search result was seen.
- GitHub (primary):
  - Latest release is still v0.1.44, 2026-04-24.
  - **Addition:** there have been 10 commits since 2026-04-25. The last was 2026-09-19 ("chore: bump version to 0.1.45", #3462), and 0.1.45 has not been released.
  - The README shows "Vibe Kanban is sunsetting." The repo is not archived and has 28.3k stars.
  - No published community roadmap. The closest is pinned Discussion #3424, "What would a community edition of Vibe Kanban look like?" (2026-06-02).

### C6 — Repo stats
**Status: verified (primary, `gh api repos/pingdotgg/t3code`).**
- 27,180 stars and 7,162 forks.
- Created 2026-02-08T23:46:22Z.
- MIT license, "Copyright (c) 2026 T3 Tools Inc."
- Not archived. Last push 2026-10-12.

## New findings
- **Billing/ToS nuance that matters for the decision:** a GUI built on the Agent SDK that runs the user's own logged-in Claude Code is the T3 Code pattern. Anthropic's support page currently counts that kind of "third-party app" usage against the subscription, and the June 2026 billing change is paused. The developer-docs rule still tells anyone "building a business on top of the Agent SDK" to use API keys. A personal tool that one solo developer runs locally fits the "local development and experimentation" carve-out better than a distributed product does. There is no explicit Anthropic ruling on wrapper GUIs.
- PR #15752 carries the label `vouch:unvouched`, which points to a contributor-vouching gate in T3 Code's PR triage. Outside PRs from unvouched authors may wait indefinitely.
- #12089, which asked for changed files across sibling repos, was closed as COMPLETED on 2026-10-11. No closing PR or commit is linked, so the feature may not actually have shipped.
- T3 Code ships nightlies several times a day (four releases on 2026-10-11 to 10-12). That level of upstream churn is relevant to the cost of keeping a fork up to date.

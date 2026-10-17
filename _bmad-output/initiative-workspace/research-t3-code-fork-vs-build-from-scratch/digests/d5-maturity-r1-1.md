# D5 maturity & risks — round 1

**Summary.** T3 Code (MIT, T3 Tools Inc) is a very high-velocity alpha. About 4,150 commits landed in the last 6 months. Releases ship as several nightlies a day, plus a stable roughly every 1–2 weeks; the latest stable is v0.0.45, released 2026-10-02.

**Engineering discipline is strong on paper.** About 1,790 first-party test files, a sharded Linux CI (lint, knip, typecheck, build, tests, Rust, release smoke), custom oxlint rules, and 86 docs files including a 3.3k-line Orchestration V2 design set.

**But Windows is not covered by CI.** The manual-only Windows workflow says "nothing in the suite passes on Windows yet".

**Much of the code is AI-written.** About half of the commits carry Co-authored-by trailers, mostly AI agents (Codex, Claude Opus/Fable, Cursor). AGENTS.md says most contributions come from T3 Code itself. Issue triage is also AI-driven ("Grok responding on behalf of Julius").

**The issue tracker is very large and churning.** 1,327 open issues (804 labeled bug) against 3,216 closed. 1,383 PRs are open. 1,711 issues were filed in the last 30 days.

**Contributions are concentrated.** One maintainer, Julius Marminge, made about 38% of commits. The top 3 made about 61%.

**Outside contributions are explicitly not sought.** The business model is unclear: Theo reportedly said there is no paid tier, yet T3 Connect runs on hosted infrastructure (Clerk plus a relay with paid tunnels).

**A major rewrite is in flight.** Orchestration V2 reached nightly around 2026-10-03, with confirmed state/migration regressions. There are open data-loss bugs on Windows (worktree removal follows junctions) and on macOS (SQLite corruption), both with fix PRs still open.

**Outside opinion is mostly positive but thin.** Blog and HN comments call it pleasant and the multi-project/remote features strong. They also note it is alpha with frequent changes, and that "OpenChamber has fewer bugs".

## Claims

1. 4,149 commits on HEAD since 2026-04-12 (5,424 total; first commit 2026-02-07). | `git rev-list --count` on local clone 0e7346c | local git | 2026-10-12 | accessed 2026-10-12 | high | governance
2. Top committers since 2026-04-12: Julius Marminge 1,583 (two emails merged; 38.2%), Theo Browne 580 (14.0%), maria / maria-rcks 363 (8.7%). Top 3 = about 2,526 (60.9%). Bots: t3-code[bot] 109, cursor[bot] 15. Next human contributor: Wout Stiens, 92 commits. | `git shortlog -sne HEAD --since=2026-04-12` | local git | 2026-10-12 | accessed 2026-10-12 | high | governance
3. 2,087 of 4,149 commits (50.3%) since 2026-04-12 carry a Co-authored-by trailer. The largest trailer counts are AI agents: codex 412, Claude Opus 5.5 (1M context) 359, Claude Fable 5 316, Claude Opus 5.5 161, Claude Fable 5.1 91, Claude Opus 5 (1M) 60, Claude Code 55, Cursor Agent 48 (about 1,500 AI trailer lines). These are trailer lines, not distinct commits, so the result is a lower bound on AI involvement. | `git log --format=%(trailers)` | local git | 2026-10-12 | accessed 2026-10-12 | high | quality
4. AGENTS.md: "Most T3 Code contributions will come from T3 Code itself, often controlled remotely". It also tells PR bodies to "End with the model and harness that did the work". AI-authored development is the house norm. | AGENTS.md | T3 Tools Inc (repo) | 2026-10-12 HEAD | accessed 2026-10-12 | high | quality
5. AGENTS.md claims "over 500,000 users" and that "A large number of our users run forks". It lists non-negotiables: open core, performance, remote-ready, multi-surface (web/desktop/mobile). | AGENTS.md | T3 Tools Inc | 2026-10-12 HEAD | accessed 2026-10-12 | medium (self-reported user count) | governance
6. Test files: 5,546 tracked `*.test|spec.*`. 3,758 of them sit in vendored `.repos/` (alchemy-effect, effect-smol), leaving about 1,788 first-party. Largest: apps/web 487, apps/server 426, apps/mobile 271, packages/client-runtime 134, apps/desktop 116, packages/shared 95, packages/contracts 39, infra/relay 34. Per-provider package tests are thin (opencode 17, cursor 15, acp 8). | `git ls-files` | local git | 2026-10-12 | accessed 2026-10-12 | high | quality
7. CI (`ci.yml`), all on Ubuntu (Blacksmith runners): Lint (rejects repo PR assets, `knip:check` unused code, `vp check`); Typecheck; Build desktop pipeline plus preload-bundle verification; Test (all packages except web/server); Test Web; Test Server in 6 shards with headless Chromium plus a thread-transfer performance budget report; Rust fmt and tests; mobile native-change detection; release smoke (`scripts/release-smoke.ts`). | .github/workflows/ci.yml | T3 Tools Inc | HEAD | accessed 2026-10-12 | high | quality
8. Windows is not in the CI gate. `windows-tests.yml` is `workflow_dispatch` only, and its header reads: "Manual only: nothing in the suite passes on Windows yet … Once the suite is green here, fold it into ci.yml." | .github/workflows/windows-tests.yml | T3 Tools Inc | HEAD | accessed 2026-10-12 | high | quality
9. Lint and format run through Vite+ (`vp lint` = oxlint with eslint/oxc/react/unicorn/typescript plugins). The correctness, suspicious and perf categories are set to "warn". A repo-local `oxlint-plugin-t3code` adds custom rules (e.g. no-rpc-permission-bypass, no-raw-mcp-registration, no-manual-effect-runtime-in-tests), each with tests. `vp fmt` is the pre-commit hook ("Formatter only for now — no lint or typecheck on commit"). | vite.config.ts; oxlint-plugin-t3code/rules | T3 Tools Inc | HEAD | accessed 2026-10-12 | high | quality
10. AGENTS.md sets these engineering standards:
    - "Hit every surface" checklist (entry points, clients, providers, MCP, contracts, reverse states, connection modes, docs).
    - Event-sourced server tests must drain the effect worker; "A test that needs a timeout to pass is wrong".
    - Do not run repo-wide checks locally; "CI owns the full suite".
    - Conventional-commit PR titles.

    Theo's note: "simple systems … fight scope creep". | AGENTS.md | T3 Tools Inc | HEAD | accessed 2026-10-12 | high | quality
11. CONTRIBUTING.md says: "We are not actively seeking outside contributions and have limited review capacity." Features need prior explicit maintainer approval. Rules are one problem per PR, and drafts are triaged the same as ready PRs. | CONTRIBUTING.md | T3 Tools Inc | HEAD | accessed 2026-10-12 | high | governance
12. Docs: 86 files in four groups:
    - user: about 40 guides, one per provider/feature.
    - internals: 31 files (overview, providers, adding-a-provider, remote, environment-auth, T3 Connect, terminal runtime, effect services, legacy-orchestration-migration, …).
    - operations: 9 runbooks.
    - orchestration-v2: 9 docs, 3,294 lines.

    The V2 README calls itself "the target architecture … intentionally ignores migration/backward compatibility". It specifies invariants (app ids primary, explicit capability flags, handoff artifacts). Docs are actively maintained (orchestration-v2 last touched 2026-10-10). The docs index links resolve. | docs/ | T3 Tools Inc | HEAD | accessed 2026-10-12 | high | quality
13. Repo stats: 27,180 stars, 7,161 forks, 1,417 discussions. Issues: 1,327 open / 3,216 closed. PRs: 1,383 open / 4,941 merged / 6,295 closed unmerged. 804 open issues carry `bug`; 113 carry `needs-triage`. | GitHub GraphQL via gh | GitHub | 2026-10-12 | accessed 2026-10-12 | high | issues
14. Last-30-day velocity: 1,711 issues created since 2026-09-12 (718 already closed); 1,825 PRs merged, about 716 of them not authored by juliusmarminge/t3dotgg/maria-rcks/t3-code app (search approximation). | GitHub search API via gh | GitHub | 2026-10-12 | accessed 2026-10-12 | medium (search-API exclusion approximate) | issues
15. Releases: stable v0.0.39 (09-07) through v0.0.45 (10-02), roughly weekly. Nightlies (0.0.46-nightly.*) ship several per day; 100+ releases since 2026-04-12. Still 0.0.x / "Alpha". | `gh release list`, releases API | GitHub | 2026-10-12 | accessed 2026-10-12 | high | governance
16. Issue triage is largely AI-driven. Maintainer comments open with "Grok responding on behalf of Julius" (#14990, #15139, #15017). maria's account runs "an automated response from a bot acting on maria's behalf" for staleness pings (#4261, stale since 2026-07-22). Triage answers are detailed and confirm bugs quickly. | gh issue view 14990/15139/15017/4261 | GitHub | 2026-10-03..10-09 | accessed 2026-10-12 | high | issues
17. Open data-loss bug on Windows: worktree removal follows directory junctions and deletes their targets (#14990). Maintainers triaged it as "a real data-loss bug". Fix PRs #15833 and #17076 are still OPEN. No fix commit on HEAD references it. | gh issue/pr | GitHub | 2026-10-03 | accessed 2026-10-12 | high | issues
18. Open macOS state-corruption bug: hard shutdown corrupts state.sqlite (no F_FULLFSYNC, #13544), accepted as "Valid bug". PRs #14457 and #14411 are still OPEN (#13597 was closed). | gh | GitHub | 2026-09-25 | accessed 2026-10-12 | high | issues
19. The Orchestration V2 nightly rollout (about 2026-10-03) caused confirmed state regressions:
    - #15017: silently reuses a preview DB and misses newer V1 threads; confirmed on 0.0.46-nightly.20261003. A Windows user rolled back after losing the sidebar.
    - #15001: V2 profile makes connection-catalog.json undecryptable; still open.
    - #15139: crash loop if startup migrations fail; duplicate of open #10517 from 2026-09-07.
    | gh | GitHub | 2026-10-03 | accessed 2026-10-12 | high | issues
20. MIT license, "Copyright (c) 2026 T3 Tools Inc." LICENSE was last changed 2026-03-05; no licensing change since. | LICENSE, git log | local git | 2026-03-05 | accessed 2026-10-12 | high | governance
21. T3 Connect, the monetization-adjacent hosted piece, uses Clerk for cloud identity plus a relay Worker (Cloudflare Durable Objects) that allocates managed tunnels. The docs mention a "paid tunnel" cost. No pricing or entitlement logic was found in docs. | docs/internals/t3-connect.md | T3 Tools Inc | HEAD | accessed 2026-10-12 | medium | governance
22. Theo Browne reportedly restated on X that T3 Code "has no paid tier and will not monetize usage". Secondary source only; primary X post x.com/theo/status/2085621311909642621 returned 402 and could not be verified. The article notes no explanation of how infrastructure is funded. | enterprisedna.co AI Pulse | Enterprise DNA | 2026-08-08 | accessed 2026-10-12 | low-medium | governance
23. Hands-on review after a few weeks of freelance use: "genuinely pleasant to use day to day". Multi-project work, Linux support, and T3 Connect phone control are strengths. "Not perfect"; usage visibility is missing; the author recommends nightly over stable on Linux. It reports no stability or data-loss problems. | bmateas.com/blog/t3-code-review | Branislav Mateas (blog) | 2026-07-28 (upd. 08-18) | accessed 2026-10-12 | medium | reception
24. Architecture deep dive: "explicitly marked as Alpha. It is moving quickly"; "expect bugs, unfinished edges, and frequent changes"; "Pinning an old client while updating the server is likely to create more trouble than stability". The hard part is "keeping those provider adapters correct as every provider changes". On forking: MIT allows it, but maintainers are unlikely to merge large features. | flaviocopes.com/t3-code | Flavio Copes | 2026-08-28 | accessed 2026-10-12 | medium | reception
25. HN comments (Apr–Oct 2026) are mostly short, positive mentions:
    - "T3 Code has been amazing… desktop app and the mobile app experience" (rob, 08-14, on nightly).
    - "[OpenChamber] has less bugs than T3 Code" (polski-g, 08-10).
    - "sadly no official Docker image" (fratellobigio, 10-03).
    - Called one of many near-identical harnesses with "no moat" (impulser_, 10-06).
    - A fork-based derivative exists: Synara "started as a T3 Code fork" (07-30).

    No HN story with real traction was found. | HN Algolia API | Hacker News | 2026-04..10 | accessed 2026-10-12 | medium | reception
26. A daily.dev post called it "good but not usable" (tilde paths, missing diff view, insecure web mode). This dates from the Codex-only era (about Mar 2026), so it is history only and likely superseded. | daily.dev (via search snippet) | daily.dev | ~2026-03 | accessed 2026-10-12 | low | reception

## Relevant open issues

| # | Title | State | Date | Relevance |
|---|---|---|---|---|
| 14990 | Removing a worktree on Windows follows directory junctions and deletes the files they point to | open (fix PRs #15833/#17076 open) | 2026-10-03 | Windows, worktrees, data loss |
| 13544 | Hard shutdown on macOS corrupts state.sqlite: SQLite never uses F_FULLFSYNC | open, accepted | 2026-09-25 | data loss/persistence |
| 15017 | Orchestrator V2 nightly silently reuses a preview-created database and misses newer V1 threads | open | 2026-10-03 | migration, data loss |
| 15001 | V2 desktop seeds Local State from stale profile → connection-catalog.json undecryptable | open | 2026-10-03 | migration |
| 10517 / 15139 | Backend startup crashes leave desktop blank / crash loop on failed startup migrations | open (15139 dup) | 2026-09-07 / 10-03 | migrations, reliability |
| 4261 | Projects and Chat Threads Disappear After Updating Nightly Build | open, untriaged since July | 2026-07-22 | data loss on update |
| 4374 | Cannot downgrade nightly to stable; manual downgrade hides existing chats | open | 2026-07-23 | migrations/rollback |
| 14940 | Check out here forces PR branch reset and can discard local commits | open | 2026-10-02 | data loss, git |
| 18485 | Nightly reaches ~11.5 GB while concurrent agent progress stalls | open | 2026-10-12 | performance |
| 18165 | Windows: chrome-headless-shell GPU process busy-spins ~5 cores while idle | open | 2026-10-11 | Windows, performance |
| 9661 | Track performance audit fixes (2026-09-04 audit; most items unchecked) | open | 2026-09-04 | performance |
| 4403 | Unconditional PowerShell profile discovery delays Windows desktop startup | open | 2026-07-23 | Windows |
| 5174 | Windows Start Menu shortcut disappears after Nightly updates | open | 2026-08-01 | Windows |
| 17927 | Windows repository cloning fails while creating an existing drive root | open | 2026-10-10 | Windows |
| 9065 | Bundled `opencode serve` holds shared OpenCode SQLite DB — concurrent `opencode run` hangs | open | 2026-09-01 | OpenCode |
| 17806 | Creating a worktree thread intermittently fails — second opencode serve loses shared-DB race | open | 2026-10-10 | OpenCode, worktrees |
| 15155 | OpenCode 2 custom providers/models omitted during boot probe race (11 comments) | open | 2026-10-03 | OpenCode |
| 11458 | OpenCode Auto mode ignores user permission patterns | open | 2026-09-12 | OpenCode |
| 17182 | OpenCode not available as provider on ssh remote connection | open | 2026-10-08 | OpenCode, remote |
| 18658 | Claude Code mods are dead in T3: no panes, status line, toasts, commands | open | 2026-10-12 | Claude Code |
| 15243 | Imported Claude Code threads fail on first follow-up ("Session ID already in use") | open | 2026-10-03 | Claude Code |
| 18635 | Claude threads lack T3 orchestration tools under enterprise-managed MCP policy | open | 2026-10-12 | Claude Code, MCP |
| 16067 | Codex 0.160 agents get no T3 Code MCP tools: /mcp server/discover HTTP 400 | open | 2026-10-05 | MCP, provider drift |
| 15541 | Parallel worktree creation hits fixed 5-min git timeout, leaves orphan worktrees | open | 2026-10-04 | worktrees |
| 17813 | Worktree cleanup never removes worktrees whose branch was renamed/switched/detached | open | 2026-10-10 | worktrees |
| 13390 | `#` PR suggestions only search the project's repo, not other repos linked to the thread | open | 2026-09-24 | multi-repo |
| 9853 | New worktree creation reclones already-initialized submodule objects | open | 2026-09-05 | monorepo/submodules |
| 16503 | Remote environment can't connect while unknown-remote probes saturate VcsProcess | open | 2026-10-06 | remote |
| 18644 | Android app hangs opening subagent thread over T3 Connect relay | open | 2026-10-12 | mobile/relay |
| 5742 | Mobile thread list projection goes stale over churning remote connection | open | 2026-08-08 | mobile/remote |

## Leads

- Check whether `vp check` fails on lint warnings. All lint categories are "warn", so lint may be advisory. Inspect the Vite+ docs and ci.yml lines 21–56.
- The fork-maintenance burden is likely extreme: about 140 commits/day on main, and a V2 orchestrator rewrite that ignores backward compatibility. A fork would diverge within days. Quantify churn per directory (e.g. `git log --since=2026-09-12 --numstat -- apps/server/src/orchestration-v2`).
- Known forks/derivatives to examine as fork-precedent case studies: Synara (HN 49106342), maria-rcks/t1code, OpenChamber (compared favorably on bugs).
- Verify Theo's "no paid tier" statement from a primary source (X post 2085621311909642621, or t3.codes). Check whether T3 Connect is free, and whether relay/Clerk costs would fall on a forker who wants remote access.
- `docs/internals/legacy-orchestration-migration.md` and `docs/orchestration-v2/testing-strategy.md` are worth a deep read for D-architecture.
- Issue #18485 (11.5 GB memory) and the perf-audit tracker #9661 are signals for performance claims in AGENTS.md.

## Looked for, not found

- No HN story with meaningful discussion of T3 Code since 2026-04-12 (only comments); no Reddit threads surfaced via search.
- No YouTube or X retrospective accounts retrieved (X returns 402; not attempted further within budget).
- No published roadmap document, deprecation notice, or license-change signal in repo or search results.
- No pricing page or paid-tier code (entitlements/billing) in docs; T3 Connect pricing not found.
- No Windows job in the gating CI; no evidence the Windows test suite passes.
- No merged fix yet for #14990 (Windows junction deletion) or #13544 (macOS fullfsync).

# D3/D4 license & velocity — round 1

**Summary.** T3 Code is MIT-licensed (copyright "T3 Tools Inc.", 2026). The repo has no trademark file, no NOTICE file, and no written name or logo policy. Third-party notices are generated at build time from `third-party-licenses.config.json`. Bundled native or vendored pieces such as libghostty-vt, GhosttyKit and the fonts are MIT or Apache-2.0, and the sounds are CC0. The authors explicitly endorse forking ("we want you to have everything you need to fork", "A large number of our users run forks"). They are "not actively seeking outside contributions", although about a third of recently merged PRs come from non-maintainers.

Upstream velocity is extreme and still rising: about 1,600 commits in Sep 2026, about 870 in the first 12 days of Oct, more than 125 distinct authors per month, heavy AI co-authorship, and nightly builds several times a day. The version is still 0.0.x.

Internal APIs are unstable. On 2026-10-02 a single squash commit landed "orchestration v2", a 4-month PR with +380K/-204K lines. It replaced `apps/server/src/orchestration`, deleted the v1 contracts and moved to a new SQLite database and wire protocol 2. The next week brought a wave of package extractions. Core-directory churn since 2026-04-12 is about 2.5x the current size of those directories.

Forks exist in quantity (7,161) and several sync actively. The best-documented one, TrogonStack, had to **drop** most of its divergences when v2 landed. One notable fork (aaditagrawal, 27 stars, 2,923 commits ahead) was abandoned on 2026-10-09 because upstream absorbed its features. Downstream DB schema changes collide with upstream's migration-id ledger, and upstream docs say so explicitly.

## Metrics

All local commands ran read-only in `C:/Users/fpiec/recon-src/t3code` (HEAD `0e7346ccd9`, 2026-10-12, branch `main`). Total commits: 5,424. The first commit is dated 2026-02-07 (Theo Browne), so a 12-month window covers only about 8 months of history.

### Monthly commits and authors (author date)
Commands:
`git log --since=$m-01 --until=<next>-01 --format=%h | wc -l` (all commits), the same with `--first-parent` and with `--merges`, and `--format=%an | sort -u | wc -l` for authors. The "human" column filters out names containing `[bot]`, `Cursor Agent`, `codex` or `claude`.

| Month | Commits | First-parent | Merge commits | Distinct author names | Excl. bot/agent names |
|---|---|---|---|---|---|
| 2026-02 | 545 | 471 | 15 | 4 | 2 |
| 2026-03 | 637 | 637 | 0 | 84 | 81 |
| 2026-04 | 213 | 213 | 0 | 65 | 64 |
| 2026-05 | 86 | 86 | 0 | 20 | 18 |
| 2026-06 | 411 | 411 | 0 | 29 | 27 |
| 2026-07 | 367 | 367 | 0 | 72 | 71 |
| 2026-08 | 704 | 704 | 0 | 146 | 145 |
| 2026-09 | 1,595 | 1,595 | 0 | 128 | 126 |
| 2026-10 (1–12 only) | 866 | 866 | 0 | 135 | 132 |

- `main` has been squash-merge only since March, so one commit is roughly one PR.
- Author concentration since 2026-02 (`--format=%an | sort | uniq -c`): Julius Marminge 2,539, Theo Browne 653, maria 355, t3-code[bot] 114, then a long tail. github-actions[bot] has 25, cursor[bot] 15 and Cursor Agent 14.
- AI co-author trailers (`git log --format=%B | grep -i '^co-authored-by:'`): codex 578, Claude Opus 5.5 (1M) 354, Claude Fable 5 305, Claude Opus 5.5 158, Claude Fable 5.1 86, Cursor Agent 69, Claude Opus 5 (1M) 60, Claude Code 55, Claude Opus 4.6 50. Upstream is heavily agent-assisted.

### Releases and tags
Commands: `git for-each-ref --sort=creatordate --format='%(creatordate:short) %(refname:short)' refs/tags` and `gh release list -R pingdotgg/t3code --limit 50`.

| Month | Tags created |
|---|---|
| 2026-03 | 36 |
| 2026-04 | 29 |
| 2026-05 | 40 |
| 2026-06 | 73 |
| 2026-07 | 105 |
| 2026-08 | 178 |
| 2026-09 | 156 |
| 2026-10 (1–12) | 59 |

- There are 676 tags in total: 563 `-nightly.YYYYMMDD.N`, 42 `-preview.` and 41 stable `v0.0.N`.
- Stable releases run from v0.0.2 (2026-03-06) to v0.0.45 (2026-10-02), about 6 per month. Recent ones: v0.0.42 on 09-15, v0.0.43 and v0.0.44 on 09-29, v0.0.45 on 10-02.
- The current line is `0.0.46-nightly`, which ships 2 to 4 nightlies per day. "Preview" builds are labelled "maintainer test build, do not install".
- `apps/desktop/package.json` and `apps/server/package.json` are at `0.0.45`. Version scheme: 0.0.x, with no semver stability promise.

### Core churn (apps/server/src + packages/contracts)
Size at HEAD (`git ls-files <dir> | xargs cat | wc -l`):

| Directory | Files | Lines |
|---|---|---|
| apps/server/src | 1,302 | 478,892 |
| packages/contracts | 109 | 35,689 |
| apps/web/src | 1,490 | 440,617 |
| apps/desktop/src | 268 | 83,717 |

Total since 2026-04-12, using `git log --since=2026-04-12 --format=%h --shortstat -- apps/server/src packages/contracts`: 1,493 commits, 13,311 file-changes, +850,211 / -423,148 lines. That is 1.27M lines changed, about 2.5x the current ~515K lines of these two directories.

Monthly figures use the same command bounded by `--since/--until`:

| Month | Core commits | Insertions | Deletions | Total | % of current core size |
|---|---|---|---|---|---|
| 2026-02 | 248 | 67,636 | 29,478 | 97,114 | 19% |
| 2026-03 | 261 | 70,680 | 39,493 | 110,173 | 21% |
| 2026-04 | 91 | 63,707 | 19,145 | 82,852 | 16% |
| 2026-05 | 45 | 33,544 | 15,228 | 48,772 | 9% |
| 2026-06 | 150 | 58,442 | 23,394 | 81,836 | 16% |
| 2026-07 | 102 | 35,490 | 6,060 | 41,550 | 8% |
| 2026-08 | 219 | 81,475 | 4,906 | 86,381 | 17% |
| 2026-09 | 529 | 178,056 | 23,992 | 202,048 | 39% |
| 2026-10 (1–12) | 396 | 422,151 | 335,131 | 757,282 | 147% |

The October total is dominated by `de34391427` "feat(orchestrator): introduce new orchestrator (#2829)", which alone accounts for 427,120 core lines. Some of those lines are test fixtures under `orchestration-v2/testkit/fixtures` (16% of the dirstat).

Largest core commits since 2026-09-01:
- #2829, the orchestrator rewrite: 427K lines.
- #17607, GitHub moved into `@t3tools/source-control-github`: 20.9K.
- #17345, OpenCode moved into its own package: 20.2K.
- #17354, the ACP adapter moved into its own package: 18.7K.
- #16282, layer variable renames: 11.8K.
- #17302, Pi moved into its own package: 11.5K.
- #17405, the ACP registry package: 9.2K.
- #17349, the Cursor package: 9.1K.
- #17299, the provider-core and provider-testing packages: 8.4K.

Keyword counts over all history, then since 2026-04-12 (`git log --format=%s | grep -cE ...`):
- `breaking|BREAKING|!:`: 3 / 2
- conventional `type!:`: 0
- `^refactor`: 253 / 243
- `rename`: 20 / 9
- `migrat`: 26 / 7
- `move .* into its own`: 7 / 7

Upstream does not mark breaking changes in commit messages, so these keyword counts understate real breakage.

### Repo, forks and PR flow
`gh api repos/pingdotgg/t3code` returned 27,180 stars, 7,161 forks, 2,710 open issues+PRs and 74 watchers. The repo was created 2026-02-08, the license is MIT and the last push was 2026-10-12.

PR flow from `gh api search/issues`, window 2026-09-12..2026-10-12:
- 1,825 PRs merged.
- 1,173 of those were by maintainers listed in TRIAGE_EXEMPTIONS or by `app/t3-code`.
- That leaves about 652 merged PRs (about 36%) by others.
- 429 PRs were closed unmerged.
- 1,383 PRs are open now.

Fork divergence, from `gh api repos/pingdotgg/t3code/compare/main...<owner>:<repo>:<branch>`. The list comes from `gh api "repos/pingdotgg/t3code/forks?sort=stargazers&per_page=30"`, plus forks found on the web.

| Fork | Stars | Ahead | Behind | Merge base | Notes |
|---|---|---|---|---|---|
| i2cjak/Backplane | 420 | — | — | none | No common ancestor; history was rewritten (KiCad tool) |
| opencoredev/akeru-bot | 97 | 0 | 4,054 | 2026-04-23 | "Independent fork of T3 Code"; work is not on `main` |
| maria-rcks/t3libre | 40 | 1 | 2,696 | 2026-08-21 | Satirical fork by a maintainer |
| what1f/t3kanban | 28 | 17 | 1,650 | 2026-09-09 | |
| aaditagrawal/t3code | 27 | 2,923 | 1,070 | 2026-09-24 | **Declared unmaintained 2026-10-09** |
| UsefulSoftwareCo/openbot | 13 | 436 | 2,053 | 2026-09-05 | Product built on T3; stale since 09-16 |
| ZhiweiXiao98/t3code (i18n-zh-cn) | 8 | 66 | 849 | 2026-10-02 | Claims to "continuously sync" |
| astraly-labs/campfire | 4 | 81 | 3,219 | 2026-07-30 | "A thin fork of T3 Code" |
| patroza/t3code (fork/dev) | 4 | 88 | 6 | 2026-10-12 | Syncs through `sync/upstream-*` PRs |
| munimtechnologies/mtcode | 8 | 1,492 | 6 | 2026-10-12 | Syncs through "merge upstream/main: 97 upstream commits" |
| bdsqqq/t3code | 8 | 150 | 1,005 | 2026-09-25 | |
| camie-ace/KamiCode | 7 | 223 | 534 | 2026-10-06 | Rebranded; own release CI |
| travisjneuman/t3code | 0 | 237 | 0 | 2026-10-12 | Automated 30-min upstream merge; Claude Code resolves conflicts |
| jarmen423/t3code | 1 | 52 | 371 | 2026-10-08 | |
| SergeSerb2/t3-pretty | 0 | 17 | 631 | 2026-10-05 | |
| TrogonStack/t3code | 0 | — | — | — | About 20 "sync with upstream" PRs between 09-17 and 10-12 |

## Claims

1. The license is MIT, "Copyright (c) 2026 T3 Tools Inc." Source: `LICENSE` at HEAD 0e7346c. Publisher: pingdotgg. pub_date: 2026-10-12 (HEAD). Accessed 2026-10-12. Confidence: high. Class: license. The GitHub API also reports `MIT`.
2. The repo has no TRADEMARK file and no NOTICE file at the top three levels, and the docs contain no written trademark, name or logo restriction. Grep of `*.md` for trademark/brand/logo found only `assets/README.md` "# Brand icons", which covers how to export icons, not usage rights. Source: local grep and find at HEAD. Publisher: pingdotgg. pub_date: 2026-10-12. Accessed 2026-10-12. Confidence: medium-high, since this is absence of evidence and the project may still hold unregistered or registered marks. Class: license.
3. Brand assets (the "T3 mark" in `assets/{dev,nightly,prod}/app-icon.icon`) sit in the MIT repo without separate terms. App identity strings are spread across files: `com.t3tools.t3code` appears in 33 ts/json/yml files. Branding modules exist at `apps/web/src/branding.ts` and `apps/mobile/src/lib/mobileBranding.ts`. Forks such as aaditagrawal, travisjneuman and KamiCode rebrand app id, profile, URL scheme and name so they can install side by side. Source: local repo and fork READMEs. Publishers: pingdotgg and fork owners. pub_date: 2026-10. Accessed 2026-10-12. Confidence: high (facts). Whether rebranding is legally *required* is unverified. Class: license.
4. Third-party obligations are handled by a build-time generator. The build fails when a collected package lacks a license identifier or license text. Custom notices cover:
   - libghostty-vt: MIT, `native/libghostty-vt/LICENSE`, from ghostty-org/ghostty.
   - GhosttyKit for iOS: MIT, "Copyright (c) 2024 Mitchell Hashimoto, Ghostty contributors", vendored from the `Yash-Singh1/ghostty` branch.
   - MesloLGS NF: Apache-2.0.
   - Symbols Nerd Font Mono: MIT.
   - expo-device-hub: "MIT AND Apache-2.0 AND BSD-3-Clause".
   - Four CC0 sounds.

   Source: `third-party-licenses.config.json` and `docs/internals/open-source-licenses.md`. Publisher: pingdotgg. pub_date: HEAD. Accessed 2026-10-12. Confidence: high. Class: license.
5. The authors explicitly endorse forking. The README says "If we ever go the wrong direction, we want you to have everything you need to fork". AGENTS.md says "A large number of our users run forks." Both lines entered in commit b64ae880e0 (Theo Browne, 2026-07-28). Source: README.md and AGENTS.md. Publisher: pingdotgg / Theo Browne. pub_date: 2026-07-28. Accessed 2026-10-12. Confidence: high. Class: forks.
6. Outside PRs are formally discouraged but in practice often merged. CONTRIBUTING says "We are not actively seeking outside contributions". Features need prior maintainer approval, and an automated triage policy was added in 0d9468fea9 on 2026-09-30. Even so, about 36% of PRs merged in the last 30 days came from logins outside the maintainer list. Source: CONTRIBUTING.md, README.md and the gh search API. Publisher: pingdotgg. pub_date: 2026-09-30 and 2026-10-12. Accessed 2026-10-12. Confidence: high for the policy, medium for the 36% (it depends on how GitHub search ORs `author:`). Class: velocity.
7. Upstream velocity is very high and accelerating: 1,595 commits in Sep 2026 and 866 in Oct 1–12. That is about 50 to 70 squash-merged PRs per day from more than 125 distinct authors per month, concentrated in one maintainer (Julius Marminge, 47% of all commits). Source: local git log. Publisher: pingdotgg. pub_date: 2026-10-12. Accessed 2026-10-12. Confidence: high. Class: velocity.
8. The release cadence has 2 to 4 nightlies per day, frequent maintainer previews, and about 6 stable releases per month. The version is still 0.0.x (v0.0.45 on 2026-10-02, 0.0.46-nightly in progress). The README says "We are very very early in this project. Expect bugs." Source: git tags, `gh release list` and README. Publisher: pingdotgg. pub_date: 2026-10-12. Accessed 2026-10-12. Confidence: high. Class: velocity.
9. Orchestration v2 is a large rewrite. PR #2829 was opened 2026-05-27 and merged 2026-10-02 as one squash commit: +380K/-204K across 1,907 files. It deleted 75 files in `apps/server/src/orchestration`, 43 in `provider`, 22 in `persistence`, and `packages/contracts/src/orchestration.ts`. It moved state into a new `statev2.sqlite` and requires `ORCHESTRATION_PROTOCOL_VERSION` 2: mismatched clients and servers are rejected with HTTP 426. Source: `git show de34391427`, `gh pr view 2829`, `docs/orchestration-v2/README.md` and `docs/internals/legacy-orchestration-migration.md`. Publisher: pingdotgg. pub_date: 2026-10-02. Accessed 2026-10-12. Confidence: high. Class: stability.
10. Upstream's design philosophy deprioritizes internal compatibility. The v2 docs say they "intentionally ignore migration/backward compatibility". Theo's note in AGENTS.md says "Do not preserve complexity just because it already exists." A follow-up wave from 2026-10-05 to 10-09 extracted provider and source-control code into new packages (provider-core, provider-acp, provider-opencode, provider-pi, provider-cursor, source-control-github). Source: docs/orchestration-v2/README.md, AGENTS.md and git log. Publisher: pingdotgg. pub_date: 2026-07-28 to 2026-10-09. Accessed 2026-10-12. Confidence: high. Class: stability.
11. Core churn (server/src + contracts) since 2026-04-12 totals about 1.27M changed lines, about 2.5x the current core size. Even excluding October, monthly churn ran 8% to 39% of current size. Source: git shortstat. Publisher: pingdotgg. pub_date: 2026-10-12. Accessed 2026-10-12. Confidence: high (the metric is crude; it includes fixtures and moves). Class: stability.
12. Upstream documents that forks cannot safely add DB migrations in its migration ledger: "There is no safe id range for a fork inside this ledger … fork schema changes belong in a separate migration table or outside the migrator entirely." Source: `docs/internals/legacy-orchestration-migration.md`, added in de34391427. Publisher: pingdotgg. pub_date: 2026-10-02. Accessed 2026-10-12. Confidence: high. Class: forks.
13. GitHub reports 7,161 forks and 27,180 stars. Most top forks are hundreds to thousands of commits behind. Only a few (patroza, munimtechnologies, travisjneuman, TrogonStack) are kept within about one day of upstream, all through frequent merge PRs that are often agent-assisted. Source: the gh API compare and forks endpoints. Publisher: GitHub. pub_date: 2026-10-12. Accessed 2026-10-12. Confidence: high. Class: forks.
14. TrogonStack's sync after the v2 rewrite (#71: +408K/-216K, 2,243 files) dropped most of the fork's tracked divergences: native subagent threads, the fork migration chain, background-work details, an ACP stall watchdog, RPC trace joining and others. Some were superseded by upstream and others "conflicted with upstream's rewritten runtime". Source: github.com/TrogonStack/t3code/pull/71. Publisher: TrogonStack. pub_date: about 2026-10-02. Accessed 2026-10-12. Confidence: high. Class: forks.
15. aaditagrawal/t3code was a large divergent fork (2,923 commits ahead, its own branded releases, separate app id, state dir and port). It was declared unmaintained on 2026-10-09 because "Upstream T3 Code now ships everything this fork added". Source: the aaditagrawal/t3code README. Publisher: aaditagrawal. pub_date: 2026-10-09. Accessed 2026-10-12. Confidence: high. Class: forks.
16. A documented low-conflict fork pattern exists. travisjneuman keeps fork code in fork-owned files, adds one-line hooks marked `Fork add-on` in upstream files, keeps identity in two files, leaves upstream identity tests failing on purpose, and runs a marker-count check that catches hooks silently lost in clean merges. It auto-merges upstream nightlies every 30 minutes and has "Claude Code fix merge conflicts and build breaks". Source: travisjneuman/t3code README and `docs/internals/fork-add-ons.md`. Publisher: travisjneuman. pub_date: 2026-10. Accessed 2026-10-12. Confidence: high (that it exists). Its long-term success is unverified, and the fork has 0 stars. Class: forks.
17. Upstream also maintains its own forks of dependencies with a documented sync procedure (react-native-screens: "sync the fork's `main` with upstream, merge it and the required proposal branches into `t3/main`"). Source: `docs/operations/mobile-screens-fork.md`. Publisher: pingdotgg. Accessed 2026-10-12. Confidence: high. Class: forks (context only).

## Quotes

- LICENSE: "MIT License / Copyright (c) 2026 T3 Tools Inc." (repo `LICENSE`, HEAD 0e7346c).
- README: "We wanted something performant, remote-ready, and truly open. If we ever go the wrong direction, we want you to have everything you need to fork and build the editor that you want." (pingdotgg/t3code README.md)
- README: "We are very very early in this project. Expect bugs." and "We are (mostly) not accepting contributions yet. Small fixes may be considered. Big features will not be." (README.md)
- AGENTS.md: "T3 Code is truly open. We share our roadmap, we share how we think about things, and of course we share all our code. A large number of our users run forks. We work in the open, and should strive to stay that way." (AGENTS.md, b64ae880e0, 2026-07-28)
- AGENTS.md, "A note from Theo": "Do not preserve complexity just because it already exists. Do not introduce machinery because it looks architecturally impressive."
- CONTRIBUTING.md: "We are not actively seeking outside contributions and have limited review capacity. Opening a PR does not create an obligation to review or merge it." and "Focused bug fixes, reliability fixes, performance improvements, and maintenance work are the most likely to be accepted. Unsolicited features, opinionated rewrites, and unrelated cleanup are not."
- docs/orchestration-v2/README.md: "It is not a patch plan for the current implementation and it intentionally ignores migration/backward compatibility." and "V2 is an orchestrator rewrite, not a rewrite of the whole app domain platform."
- docs/internals/legacy-orchestration-migration.md: "There is no safe id range for a fork inside this ledger: any id at or below a future upstream id masks it forever, so fork schema changes belong in a separate migration table or outside the migrator entirely."
- aaditagrawal/t3code README: "This fork is no longer maintained. Upstream T3 Code now ships everything this fork added, through its ACP (Agent Client Protocol) integration, so please use upstream instead."
- TrogonStack/t3code PR #71: "Upstream's new orchestrator ships its own delegation and subagent child threads, so the fork's native subagent threads (0003) are dropped instead of being ported onto a design that replaces them." and "The ACP prompt-stall watchdog, RPC client trace joining, and dev browser marker are dropped because they conflicted with upstream's rewritten runtime, tracing, and startup code."
- travisjneuman/t3code docs/internals/fork-add-ons.md: "Every upstream merge is cheaper the less the fork touches upstream files, so the fork is shaped like a plug-in rather than a patch set." and "This catches the quiet failure of a clean merge: upstream rewrites the code around a hook, git takes upstream's version, and the hook disappears without a conflict."
- Theo on X (search snippet only, not fetched): "T3 Code is now available for everyone to use. Fully open source. Built on top of the Codex CLI…" (x.com/theo/status/2030071716530245800, about 2026-03-07)

## Leads

- **Measure the merge cost of a "plug-in shaped" fork directly.** Count conflicts in travisjneuman's `chore(sync)` merges and in TrogonStack's sync PRs #47 to #100: PR sizes, how often they land, and which files conflict. This gives an empirical cost per sync, about 1 per day.
- **Find out whether upstream offers extension points** (plugin, MCP or provider-package APIs) that would let the user's app sit *beside* T3 Code instead of forking it. Candidates:
  - The new `provider-core` package.
  - `docs/internals/adding-a-provider.md`.
  - The orchestrator MCP server.
  - "outside agents over MCP".

  This is D-architecture territory, but it decides fork vs. extend vs. scratch.
- **Check whether "T3 Code" or "T3" is a registered trademark** of T3 Tools Inc. (USPTO search). Not checked.
- **Track whether upstream ships a hosted or paid layer.** T3 Connect is a tunnel, and the satirical t3libre fork hints at community friction over monetization. Relicensing risk is low with MIT, since existing code stays MIT, but this is worth watching.
- **Collect a few more data points on long-divergent forks.** akeru-bot (97 stars, "Independent fork") has its work off `main`; check its branches. UsefulSoftwareCo/openbot went stale 3.5 weeks after its last sync.

## Looked for, not found

- A TRADEMARK, NOTICE, or brand/name/logo usage policy in the repo or docs. No trademark statement on the web either; web search was limited.
- Any maintainer post (X, YouTube or blog) specifically about forks or how to maintain one. Two web searches returned only aggregators, the launch tweet snippet and fork repos. The X post itself was not fetched.
- Any upstream promise of API, plugin or contract stability, or of a deprecation policy. None found. The v2 docs explicitly disclaim backward compatibility.
- Conventional-commit breaking markers (`!:`): 0. Breakage is not signposted in commit messages.
- Write-ups (blogs or articles) by fork maintainers on rebasing T3 Code. Only in-repo docs and PR bodies were found (travisjneuman, TrogonStack, aaditagrawal).
- Upstream history before 2026-02-07, so there is no full 12-month window.

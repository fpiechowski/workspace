---
title: 'Research techniczny: T3 Code — fork czy budowa od zera (Workspace)'
type: 'technical'
shape: 'select'
topic: 'T3 Code: fork vs budowa od zera (Workspace)'
decision: 'Budować Workspace jako fork T3 Code, od zera, czy trzecią drogą'
source: 'native-run'
status: complete
preset: 'deep'
validation: 'high'
red_team: 'on (2 passes)'
claims_verified: 23
claims_unverified: 11
claims_overturned: 1
recommendation: 'B2 — own app, domain core + MCP as service, SessionHost port, structured host default'
runner_up: 'C — sidecar on unmodified T3 Code (conditional)'
recheck_by: '2026-11-06'
upstream_snapshot: 'pingdotgg/t3code@0e7346c (2026-10-12)'
created: '2026-10-12'
updated: '2026-10-12'
---

# Research techniczny: T3 Code — fork czy budowa od zera (Workspace)

**Decyzja, której służy ten research:** czy produkt Workspace budować jako fork T3 Code (A), od zera (B), czy trzecią drogą (C).

## Streszczenie

**Werdykt: nie forkuj T3 Code. Buduj własną aplikację (B2).** Rdzeń domeny (Workspace, Task z zależnościami, ChangeRequest, Artifact, Handoff) z własnym serwerem MCP zrób jako niezależny serwis. Sesje agentów schowaj za portem `SessionHost`, z domyślnym hostem strukturalnym: Claude Agent SDK z Twoją lokalną subskrypcją oraz `opencode serve` z SSE.

**Drugi wybór: C**, czyli Twoja aplikacja steruje niezmodyfikowanym T3 Code z zewnątrz. Wybierz ją dopiero wtedy, gdy interfejs T3 dla zewnętrznych agentów trafi do stabilnego wydania, a Twoje spike'i pokażą, że własny host sesji na Windows jest za drogi.

**Wynik ważony** (Twoje wagi): B2 3,63 · B1 3,50 · C 3,13 · A 2,50 (rozdz. 11).

**Trzy ustalenia, które o tym decydują:**

1. **Twojego wyróżnika nie ma w żadnym kandydacie na bazę.** Model T3 to „wątek = jeden projekt, najwyżej jeden worktree” [3][4]. Nie ma w nim Workspace obejmującego kilka repo, zależności zadań, kolejności merge'a, artefaktów ani handoffów [8][29]. Ten rdzeń piszesz w każdej opcji. Fork oszczędza tylko warstwę wykonawczą.
2. **Fork wiąże Cię z najszybciej zmieniającą się częścią upstreamu.**
   - Upstream: ok. 1 500–1 600 commitów miesięcznie, przepisanie orkiestratora 2026-10-02 (+380K/−204K linii) [22][23].
   - Pliki, w które fork wpina hooki, zmieniają się co tydzień albo codziennie [22].
   - Realne forki synchronizują się kilka razy w tygodniu [26][28], najlepiej utrzymany z pomocą agenta [28]. Przy przepisaniach porzucają funkcje [26].
   - Do tego Effect-TS importowany w 67–99% plików, a upstream zakłada, że każda funkcja działa na wszystkich platformach [1][22]. To kłóci się z kryterium kontroli nad architekturą.
3. **Na Windows o wyborze hosta decydują dowody z issues, nie z dokumentacji.**
   - Wariant PTY ma zmierzone awarie wejścia i detekcji stanu [77][79][80].
   - Interfejs T3 dla zewnętrznych agentów ma 6 dni i istnieje tylko w wersjach nightly, a korzystający z niego klienci już trafiali na błędy protokołu [88][89]. GUI T3 potrafi też skasować dołączony z zewnątrz worktree [34][90].
   - Host strukturalny omija oba problemy, ale ma własne otwarte błędy na Windows [45]. Dlatego przed wyborem potrzebne są spike'i.

**Największe zastrzeżenie.** Wysiłek każdej opcji to ocena pośrednia (historia repo, liczba plików), bez danych o godzinach. Najmocniejszy argument przeciw rekomendacji: odtwarzasz samotnie warstwę, którą T3 już ma i utrzymuje, a fork t3kanban pokazał, że tablicę zadań da się wpiąć w T3 i wydać na trzy systemy [82] (rozdz. 14). Werdykt B2 kontra C jest bliski i odwracalny dzięki portowi `SessionHost`. Odrzucenie A ma wysoką pewność.

**Następny krok.** Przeprowadź spike'i S1, S7 i S2 (rozdz. 15) przed uruchomieniem `bmad-architecture`. Terminy ponownego sprawdzenia podaje mapa nieaktualności na końcu raportu.

## 1. Ramy wyboru

Kryteria zatwierdzone przy akceptacji planu 2026-10-12. Źródłem wymagań jest brief i addendum inicjatywy, a nie research z sieci.

**Warunki konieczne (gates):**

- **G1.** Licencja pozwala na prywatny fork i modyfikacje.
- **G2.** Działa na Windows.
- **G3.** Integruje się z Claude Code i z OpenCode.

Wszystkie trzy opcje przechodzą bramki. Przy A i C zastrzeżenie do G2 brzmi tak: Windows nie jest objęty CI T3 Code, które blokuje scalenie, a na Windows jest otwarty błąd z utratą danych przy usuwaniu worktree [33][34].

| Kryterium | Waga |
|---|---|
| Dopasowanie modelu domenowego i wymagań v1 | 30 |
| Kontrola nad architekturą | 20 |
| Wysiłek do działającego v1 (jedna osoba, po godzinach) | 20 |
| Koszt utrzymania i synchronizacji z upstreamem | 15 |
| Droga do trybu serwera i dostępu z telefonu | 10 |
| Ryzyko organizacyjne | 5 |

**Opcje:**

- **A** — fork T3 Code, w najlepszym znanym wariancie: zmiany w osobnych plikach, podpięte jednolinijkowymi hookami (tzw. fork w kształcie pluginu).
- **B** — własna aplikacja od zera. Plan zakładał wariant terminal-first, ale po red teamie rozdzieliłem ją na dwa warianty hosta sesji (rozdz. 11 i 13):
  - **B1** — terminal-first: niezmodyfikowane CLI `claude` i `opencode` w osadzonym terminalu (PTY);
  - **B2** — host strukturalny: Claude Agent SDK oraz `opencode serve` z SSE; terminal tylko jako wyjście awaryjne.
- **C** — „sidecar”: własna aplikacja domenowa z własnym MCP, która steruje niezmodyfikowanym T3 Code jako hostem sesji, przez jego interfejs MCP dla zewnętrznych agentów.

Opcję C wyłoniła runda 1. Nie była założona z góry.

## 2. Czym jest T3 Code i jak jest zbudowany (pytanie 1)

**Stack**
- TypeScript w całości, monorepo na pnpm, budowane narzędziem Vite+ [1].
- Effect-TS v4 (`effect` 4.0.2 z lokalnym patchem) we wszystkich pakietach. Importuje go 67% plików serwera i 99% plików `contracts` [22].
- Desktop: Electron 44. Web: React 19 i TanStack Router. Mobile: Expo / React Native. Strona marketingowa: Astro [1].
- Skala: ok. 840 tys. linii kodu bez testów [22] (pomiar zgrubny, z fiksturami).

**Procesy**
- Jeden serwer Node (`t3`, także jako `npx t3`) zarządza procesami agentów, gitem, terminalami i SQLite [1][2].
- Web, desktop i mobile to cienkie klienty. Komunikują się z serwerem przez typowane Effect RPC po WebSocket (`/ws`) [2].
- Desktop uruchamia serwer jako proces potomny i ładuje UI przez schemat `t3code://` [2].
- Serwer wystawia też uwierzytelniony endpoint HTTP MCP pod `/mcp` [8][9].

**Integracja z agentami**

Wszystkie integracje łączy wspólny interfejs `ProviderAdapterV2` w `packages/provider-core` oraz warstwa driverów (ustawienia, konta, status) [7]. Transport zależy od dostawcy:

| Agent | Sposób integracji |
|---|---|
| Claude Code | Claude Agent SDK w procesie, korzysta z lokalnego logowania Claude Code użytkownika [5][74] |
| Codex | `codex app-server`, JSON-RPC po stdio [7] |
| OpenCode | uruchomiony `opencode serve` + SDK; v1 i v2, osobne adaptery [7][75] |
| Grok, Antigravity, rejestr ACP | wspólny adapter ACP [7] |

**Persystencja**
- SQLite (`node:sqlite`, plik `statev2.sqlite`) z 61+ migracjami w jednym rejestrze [10].
- Orkiestracja jest event-sourcingowa: czysty `Orchestrator` (11,6 tys. linii) zamienia komendy na zdarzenia. Zdarzenia, projekcje i efekty trafiają do jednej transakcji, a efekty uboczne wykonuje osobny worker [2][6].
- Każda tura kończy się checkpointem zapisanym jako ukryty ref gita [2].

**Dokumentacja architektury** jest obszerna i aktualna: 86 plików, w tym 9 dokumentów o Orchestration V2 (ok. 3,3 tys. linii) [6].

## 3. Model pojęć T3 Code a model Workspace (pytanie 2)

Najważniejsze założenie T3 Code: **wątek należy do dokładnie jednego projektu i ma najwyżej jeden worktree i jedną gałąź**.

- Katalog roboczy agenta to `thread.worktreePath ?? project.workspaceRoot` [3][4].
- Projekt to jeden katalog (`workspaceRoot`) bez listy repozytoriów [3].
- Adapter Claude przekazuje jako `additionalDirectories` tylko `[cwd, attachmentsDir]` [5].

Potwierdzają to trzy niezależne odczyty kodu oraz otwarta prośba społeczności o obsługę wielu repo. Dyskusja #7041 ma 62 głosy i nie ma odpowiedzi maintainera [29]. PR #15752, który to częściowo dodaje, wisi bez recenzji [30].

| Pojęcie Workspace | Odpowiednik w T3 Code | Dopasowanie |
|---|---|---|
| Project (zbiór repo) | Project = jeden katalog. Grupowanie po `repositoryIdentity` jest tylko w UI [3] | **Kolizja.** Zbiór repo trzeba zbudować jako nowy byt albo jako N projektów T3 |
| Repository / Branch | Project + `RepositoryIdentity`; `thread.branch` [3] | **~1:1** (repo ≈ projekt T3) |
| Workspace (katalog z `intent`, status, ponowne otwarcie) | Brak. Najbliżej jest projekt z niegitowym katalogiem głównym albo wątek „scratch” [16] | **Nowy byt** |
| Worktree (kilka na repo w jednym Workspace) | Atrybut wątku (`worktreePath`); strategie `worktree` / `existing_worktree` / `root`; współdzielone worktree są możliwe [11][16] | **Rozszerzenie.** W T3 worktree należy do wątku, nie do Workspace |
| Task (zależności bez cykli, kryteria akceptacji) | `delegate_task`, zadania zaplanowane, `todo_list` agenta. Nie ma żadnego modelu zależności ani kolejności merge'a [8] | **Nowy byt** |
| ChangeRequest | `thread.pullRequests[]` (linki, także między repo na tym samym hoście), `watch_pull_request`, stacki GitHuba w obrębie jednego repo; pakiet `source-control-gitlab` [3][8] | **Rozszerzenie.** CR jest linkiem wątku, nie bytem; brak kolejności cross-repo |
| Artifact (wersjonowany) | Brak publicznego bytu | **Nowy byt** |
| Handoff (raport agenta) | `ContextHandoff` to przeniesienie kontekstu rozmowy przy zmianie dostawcy lub forku wątku [13] | **Kolizja nazw.** Inne znaczenie; Twój Handoff to nowy byt |
| Agent (profil: prompt, dostawca, model) | Instancja dostawcy (driver + konto) + `modelSelection`; brak promptu profilu [7] | **Rozszerzenie** |
| Session (zakres Workspace / Worktree) | `AppThread` + `ProviderSession` + `Run` [3][6] | **~1:1 dla zakresu Worktree.** Zakres Workspace = wątek w projekcie z niegitowym katalogiem; działa, ale bez funkcji gita dla zagnieżdżonych repo [31] |

**Wniosek.** Mechanika sesji T3 Code (wątek, sesja dostawcy, checkpointy, MCP) pasuje do Twojej warstwy *Session/Agent*. Rdzeń domeny Workspace nie ma odpowiedników i trzeba go napisać w każdej opcji: Workspace, Task z zależnościami, ChangeRequest jako byt, Artifact i Handoff. Dowody: wysoka pewność, kod upstreamu plus issues.

## 4. Wykonalność wymagań v1 w forku (pytanie 3)

| # | Wymaganie | Ocena w forku | Uzasadnienie i punkt rozszerzenia | Źródła |
|---|---|---|---|---|
| 1 | Aplikacja desktopowa | **Łatwo** (z zastrzeżeniem) | Electron, podpisane instalatory Windows, `winget`. Windows nie jest objęty CI blokującym scalenie („nothing in the suite passes on Windows yet”), a błąd #14990 (usunięcie worktree podąża za junction i kasuje pliki) jest otwarty | [33][34] |
| 2 | Sesje w zakresie Workspace i Worktree | **Worktree: łatwo. Workspace: średnio-trudno** | Wątek w niegitowym katalogu działa, ale funkcje gita nie widzą zagnieżdżonych repo (#12902, #10553). Pełna obsługa wielu repo wymaga zmian w skali PR #15752 (ok. 2,2 tys. linii, 54 pliki, tylko repo wewnątrz katalogu projektu). Zmiana założenia „wątek = jeden cwd” dotyka 86 plików serwera i pakietów oraz 68 plików web. Tańszy hook: `additionalDirectories` w adapterach | [5][22][30][31] |
| 3 | Agenci czytają i zmieniają stan przez CLI/MCP, GUI go pokazuje | **Łatwo (najlepsze dopasowanie)** | T3 wstrzykuje własny `/mcp` (ok. 80 narzędzi w 11 toolkitach) do sesji każdego dostawcy, z tokenem ograniczonym do sesji. Własny toolkit to ok. 3 nowe pliki + 1 linia rejestracji. CLI `t3` nie ma komend wątków ani zadań. Nie ma systemu pluginów: toolkity są wkompilowane w źródło | [8][9][16] |
| 4 | Taski z zależnościami i kolejność merge'a CR w wielu repo | **Średnio-trudno** | Brak modelu zależności (grep `dependsOn`/`mergeOrder` = 0 trafień). Wzorzec nowej funkcji domenowej (commit „guided reviews”) to ok. 50 plików: migracja, persystencja, kontrakt, RPC, client-runtime, web, toolkit MCP. Migracje forka muszą iść osobnym rejestrem | [10][21][69] |
| 5 | Kilka repo i kilka worktree tego samego repo w jednym Workspace | **Łatwo-średnio** | Wiele worktree jednego repo działa (strategie `worktree` / `existing_worktree`, współdzielenie). Brakuje bytu grupującego, który wraca do wymagania 2 | [11][16] |
| 6 | Integracja z Claude Code i OpenCode | **Łatwo, OpenCode z otwartymi błędami** | Adapter Claude jest dojrzały (9,3 tys. linii). OpenCode działa, ale ma otwarte błędy utraty narzędzi MCP T3 (#16141, #16768, #15736, #10485) i wyścigów o współdzieloną bazę (#9065, #17806) | [5][32][75] |
| 7 | Stan przeżywa zmianę sesji i dostawcy | **Łatwo dla rozmowy; Twój stan domenowy i tak jest nowy** | Przełączanie dostawcy w wątku jest wbudowane (wznowienie z deltą albo przekazanie kontekstu (ContextHandoff) z budżetem). Uwaga: wymaganie 7 dotyczy stanu *pracy* (Taski, Artefakty, Handoffy), a ten trzeba napisać | [13] |
| 8 | Tryb serwera, telefon, przeglądarka | **Łatwo** | Gotowe: aplikacja mobilna, web, parowanie w sieci LAN, Tailscale, SSH, T3 Connect. T3 Connect zależy od hostowanego relaya (Cloudflare + Clerk), więc fork musi go postawić u siebie albo używać Tailscale | [12] |

**Ogólnie.** Fork daje za darmo wymagania 1, 3, 6, 7 i 8, czyli warstwę wykonawczą. Wymagania 2, 4 i 5 to Twój wyróżnik i w forku są średnio-trudne. Dochodzi koszt utrzymania (rozdz. 6) i norma upstreamu, że każda funkcja ma działać na webie, desktopie i mobile [1].

## 5. Licencja i zasady (pytanie 4)

- **Licencja:** MIT, „Copyright (c) 2026 T3 Tools Inc.” [18][24]. Bez dodatkowych ograniczeń forkowania i dystrybucji poza zachowaniem informacji o licencji. Zależności natywne (libghostty-vt, fonty, dźwięki) są na MIT, Apache-2.0 lub CC0. Build generuje noty licencyjne i przerywa się, jeśli którejś brakuje [21].
- **Marka:** w repo nie ma pliku TRADEMARK ani NOTICE i nie ma polityki nazwy czy logo. Rejestru znaków towarowych nie sprawdzono, więc to dowód z braku dowodu. Forki publiczne zmieniają nazwę, app id i schemat URL, żeby instalować się obok oryginału [25].
- **Stanowisko autorów wobec forków** (cytaty):
  - README: „If we ever go the wrong direction, we want you to have everything you need to fork and build the editor that you want.” [19]
  - AGENTS.md: „A large number of our users run forks.” [1]
- **Kontrybucje z zewnątrz:** „We are not actively seeking outside contributions and have limited review capacity.” [20] Mimo to ok. 36% PR-ów scalonych w ostatnich 30 dniach pochodziło spoza listy maintainerów [24]. To przybliżenie z API wyszukiwania, pewność średnia.
- **Dla forka z bazą danych:** „There is no safe id range for a fork inside this ledger … fork schema changes belong in a separate migration table or outside the migrator entirely.” [21] Mechanizm istnieje: `Migrator.make({ table })` w Effect [76]. Precedens: TrogonStack prowadził osobny rejestr migracji przez 2 miesiące [70].

**Ryzyko licencyjne: niskie.** Ryzyka organizacyjne opisuje rozdz. 7.

## 6. Koszt utrzymania forka (pytanie 5)

**Tempo upstreamu** (git na klonie, pomiar z 2026-10-12) [22]:

| Okres | Wartość |
|---|---|
| Wrzesień 2026 | 1 595 commitów |
| 1–12 października | 866 commitów |
| Po przepisaniu orkiestratora | ok. 80 commitów dziennie |
| Autorzy | ponad 125 różnych autorów miesięcznie |

- Ok. połowa commitów ma współautora-agenta AI.
- Jedna osoba (Julius Marminge) zrobiła ok. 38–47% commitów, zależnie od okna czasowego.
- Wydania: 2–4 nightly dziennie i ok. 6 wersji stabilnych miesięcznie. Wciąż 0.0.x, bez obietnicy stabilności API [19][22].

**Stabilność wewnętrznych API: niska.**
- 2026-10-02 wszedł PR #2829 (+380K/−204K linii, 1 907 plików) z nowym orkiestratorem, nowym plikiem bazy i protokołem v2 [23].
- **Korekta po weryfikacji:** przepisanie *zachowało dane użytkowników* (importer V1→V2). Zgodność zerwał protokół klient–serwer (aplikacja mobilna ze sklepu nie łączy się z nowym serwerem [72]) oraz wewnętrzne API.
- Dokumentacja V2, jako opis architektury docelowej, wprost deklaruje: „intentionally ignores migration/backward compatibility” [6].
- W tygodniu po przepisaniu wydzielono providery i source-control do nowych pakietów [22].

**Częstotliwość zmian w plikach, w które fork wpina hooki** (30 dni) [22]:

| Plik | Commity / zmiany |
|---|---|
| `ws.ts` | 81 commitów, 2 prawie całkowite przepisania |
| `Sidebar.tsx` | 60 |
| `ClaudeAdapterV2.ts` | 41 w 10 dni istnienia |
| `rpc.ts` | 29 |
| `RpcAuthorization.ts` | 35 |

Spokojne pozostały tylko `orchestrationProject.ts`, `RuntimePolicy.ts` i `Sqlite.ts`.

**Jak radzą sobie realne forki:**

- **7 162 forki**; większość jest setki lub tysiące commitów z tyłu [24][25].
- **travisjneuman.** Kod forka w 182 własnych plikach, hooki w 86 plikach upstreamu (ok. 186 linii). 48 synchronizacji w 30 dni, z czego ok. 6–8 commitów naprawczych i 2 zapisane konflikty. Synchronizację automatyzuje agent co 30 minut. Fork ma też kontrolę liczby markerów, bo hook potrafi „zniknąć” w czystym merge'u [28].
- **TrogonStack.** 20 PR-ów synchronizacji w 30 dni. Przy przepisaniu porzucił ok. 6 funkcji forka zamiast je portować; część z nich upstream zastąpił własnymi [26].
- **aaditagrawal.** Mały fork (27 gwiazdek) zarchiwizowany 2026-10-09, bo „Upstream T3 Code now ships everything this fork added” [27]. **Korekta po weryfikacji:** to nie jest przykład porażki z powodu częstych zmian upstreamu, tylko wchłonięcia funkcji przez upstream.

**Luka w dowodach.** Żaden fork nie publikuje godzin poświęcanych na synchronizację. Koszt widać tylko przez częstotliwość i naprawy. Oba dobrze utrzymane forki opierają się na automatyzacji agentami.

**Wniosek.** Utrzymanie forka z własną domeną oznacza w praktyce synchronizację kilka razy w tygodniu, z ryzykiem utraty funkcji przy każdym przepisaniu. Alternatywa „zamrozić fork” odcina od poprawek adapterów, które zmieniają się najszybciej (adapter Claude: ok. 4 commity dziennie [22]). Recenzja zewnętrzna wskazuje właśnie utrzymanie poprawności adapterów jako najtrudniejszą część [38].

## 7. Dojrzałość i ryzyka (pytanie 6)

**Jakość**
- Ok. 1 790 własnych plików testów (plus kod zewnętrznych repo dołączony do T3 Code) [22].
- CI na Linuksie: lint, knip, typecheck, build desktopu, testy shardowane z budżetem wydajności, smoke release [33].
- Własne reguły oxlint. Kategorie poprawności lintera mają poziom „warn” [33].
- Windows poza CI blokującym scalenie [33].

**Issues** [24]:
- 1 327 otwartych (804 z etykietą bug), 1 383 otwartych PR-ów, 1 711 nowych issues w 30 dni.
- Triage jest w dużej mierze robiony przez AI („Grok responding on behalf of Julius”) [34].

**Otwarte błędy istotne dla Ciebie:**
- utrata danych na Windows przy usuwaniu worktree (#14990; PR naprawczy #17076 ma CHANGES_REQUESTED) [34];
- uszkodzenie SQLite po twardym wyłączeniu na macOS (#13544) [35];
- regresje stanu po wdrożeniu V2 (#15017, #15001, #10517) [36];
- błędy MCP w OpenCode [32].

**Koncentracja i model biznesowy**
- Top-3 autorów robi ok. 61% commitów [22].
- Brak strony cennika. T3 Connect używa hostowanej infrastruktury [12].
- Wypowiedź Theo, że nie będzie płatnego planu, potwierdza tylko źródło wtórne. **Niezweryfikowane.**

**Odbiór zewnętrzny**
- Pozytywny, ale cienki: „genuinely pleasant to use day to day” [37]; „expect bugs, unfinished edges, and frequent changes” [38].
- Na HN tylko pojedyncze komentarze [39].
- **Dowody cienkie:** nie znaleziono wątków retrospektywnych z ruchem ani relacji z YouTube czy X.

**Ryzyko porzucenia: niskie w horyzoncie roku.** Projekt jest bardzo aktywny (27 tys. gwiazdek, firma za nim). Większe ryzyko to **kierunek**: upstream może dodać multi-repo albo zadania na swój sposób. Dla forka oznacza to konflikt koncepcji przy kolejnej synchronizacji. W opcji C nowych narzędzi T3 można po prostu zacząć używać.

## 8. Budowa od zera: zakres i gotowe klocki (pytanie 7)

**Co T3 Code daje gotowe, czego w B nie ma:**
- strumień wiadomości i strukturalne UI rozmowy (diffy, zatwierdzenia, plan);
- checkpointy i cofanie;
- przełączanie dostawcy w wątku;
- dojrzałe adaptery sześciu dostawców;
- tryb zdalny i aplikacja mobilna;
- pakowanie i podpisy na trzy systemy;
- terminal (libghostty) [1][7][12][13].

**Co i tak trzeba napisać w każdej opcji (A, B i C):**
- Workspace, Task z zależnościami i wyliczaniem kolejności merge'a, ChangeRequest (CR) jako byt, Artifact z wersjami, Handoff, profil Agenta;
- narzędzia MCP do tego wszystkiego;
- widoki GUI zbudowane wokół Tasków, worktree i CR.

To jest Twój wyróżnik i jednocześnie większość pracy nad v1.

**Gotowe klocki dla B (stan na 2026-10):**

| Warstwa | Klocek | Uwagi | Źródła |
|---|---|---|---|
| Sesje Claude (strukturalnie) | Claude Agent SDK TS 0.3.296 | Wznawianie i fork sesji, `cwd` + `additionalDirectories`, `canUseTool`, hooki, MCP w procesie, `claude.exe` dla Windows. Otwarte błędy na Windows (#444, #359) | [40][45] |
| Sesje Claude (terminal-first) | Niezmodyfikowane `claude` w PTY | Per sesję: `--mcp-config`, `--strict-mcp-config`, `--add-dir`, `--session-id`, `--settings`, `--plugin-dir`, `--append-system-prompt-file`. 33 zdarzenia hooków (np. `Stop`, `PermissionRequest`, `Notification` z `notification_type`), typy hooków command/http | [61][62] |
| Sesje OpenCode | `opencode serve` / TUI z serwerem + SDK 1.19.0; SDK 2.x w procesie (istnieje od ok. 6 tygodni) | Konfiguracja per sesję przez `OPENCODE_CONFIG_CONTENT` (scalana z konfiguracją użytkownika). Zdarzenia przez SSE `/event` lub pluginy (`session.status`, `session.idle`, `permission.asked`). Trwa przejście na v2 | [46][47][63][64] |
| Protokół wspólny | ACP 1.25.0, `@agentclientprotocol/sdk` 1.8.0 | `opencode acp` wbudowany; Claude przez adapter `claude-agent-acp` (rozszerzenia `_meta` JetBrains); `session/fork` niestabilny | [48][49] |
| Serwer MCP w aplikacji | `@modelcontextprotocol/server` 2.3.1 (linia stabilna v2) | stdio i Streamable HTTP | [50] |
| Terminal | node-pty 1.1.0 (ConPTY) + xterm.js 6 | Moduł natywny przebudowywany pod ABI Electrona. Znane problemy ConPTY: kolejność sekwencji, resize, Shift+Enter | [51][67][68] |
| Powłoka desktopowa | Electron (Node w procesie) vs Tauri 2 (Node tylko jako sidecar) | Dla tej klasy aplikacji Electron jest prostszy | [52][53] |
| Worktree | wywołania `git` | Biblioteki (simple-git, isomorphic-git) nie mają API worktree. Emdash dodaje retry, walidację ścieżek i rollback | [53] |

**Warunki Anthropic.**
- Twórcy aplikacji zewnętrznych nie mogą oferować logowania claude.ai. Produkty na Agent SDK mają używać kluczy API.
- Użytkownik *może* zalogować się do **niezmodyfikowanego** binarnego Claude Code własną subskrypcją [41].
- Potwierdzają to niezależnie [42] i [43]; ten drugi opisuje egzekwowanie i działania prawne wobec OpenCode w 2026.
- Korekta rozliczeń: planowaną zmianę rozliczania Agent SDK wstrzymano. Na 2026-10-07 użycie SDK, `claude -p` i aplikacji zewnętrznych liczy się do limitów subskrypcji [44].
- Dla narzędzia osobistego **ścieżka przez CLI jest wprost dozwolona**.
- SDK z własną subskrypcją w narzędziu, które sam piszesz i którego sam używasz lokalnie, **wygląda na wspierane**. Za tym przemawiają: artykuł Anthropic „Use the Claude Agent SDK with your Claude plan” [44] oraz cytat pracownika Anthropic zachęcający do „local development and experimentation with the Agent SDK” [42]. Zakaz z [40][41] dotyczy twórców, którzy *oferują* logowanie claude.ai w swoich produktach. To interpretacja: Anthropic nie wypowiedział się jednoznacznie o osobistych narzędziach. Pewność średnia.
- T3 Code korzysta z lokalnie zalogowanego Claude Code przez SDK [74].

**Realny zakres B (terminal-first).** Elementy:
- powłoka Electron;
- menedżer procesów PTY;
- hooki statusu dla Claude i plugin lub SSE dla OpenCode;
- operacje git worktree;
- SQLite i serwer MCP;
- GUI domeny.

Wcześniejsze projekty tego typu:
- Superset i Emdash produkcyjnie stosują wzorzec PTY + hooki [53][65].
- Crystal: v0.1 w 7 dni od utworzenia repo, jeden autor robił 92% commitów; projekt wycofany po ok. 9 miesiącach [54].

**Brak danych o godzinach:** nie znaleziono żadnej retrospektywy z liczbami. Ceną wariantu terminal-first jest brak strukturalnych diffów i zatwierdzeń dla Claude, chyba że parsuje się transkrypt (`transcript_path` jest w każdym hooku) [62]. Wymagania v1 tego nie obejmują.

## 9. Trzecia droga C: sidecar na niezmodyfikowanym T3 Code

Runda 2 sprawdziła w kodzie, że osobna aplikacja może sterować T3 Code przez udokumentowany `/mcp` jako „outside agent” [14][15][16]. **Ocena: wykonalne, z lukami.**

**Co działa:**
- `t3_project_create` dla dowolnego katalogu, także niegitowego. Dla takiego katalogu checkpointy są pomijane [16].
- `t3_thread_launch` przyjmuje projekt, instancję dostawcy i model, pierwszy prompt oraz `existing_worktree`. Worktree może leżeć w dowolnym miejscu, pod warunkiem że jest w `git worktree list` repo projektu [16]. Opiera się to na jednym odczycie kodu i wymaga potwierdzenia w spike'u.
- `t3_thread_read`, `t3_thread_wait`, `t3_thread_send` (tryby auto/queue/steer/restart), `t3_thread_interrupt`, a także `t3_pending_request_*`, czyli zatwierdzenia [16].
- Deep link `t3code://threads/<env>/<thread>` otwiera wątek w GUI; `t3 app <path>` otwiera projekt [17].
- Własny serwer MCP aplikacji trafia do agentów przez ich natywną konfigurację. OpenCode zachowuje konfigurację użytkownika (potwierdzone w kodzie). W Claude, według zgłoszenia użytkownika, działają serwery z `~/.claude.json` [73]. Projektowy `.mcp.json` w sesjach T3 jest **niezweryfikowany**.

**Luki:**
1. **Logowanie.** Tylko OAuth z PKCE, bez refresh tokena, sesja ważna 30 dni. Każde logowanie zatwierdzasz kodem parowania. Tworzenie projektów wymaga uprawnień full-access [15]. Czy `t3 auth session issue` daje długoterminowy token honorowany przez `/mcp`, jest **niezweryfikowane**.
2. **Interfejs jest nowy i bez wersji.** Narzędzia wątków i projektów powstały 2026-10-02, OAuth dla zewnętrznych agentów 2026-10-06. Było 57 commitów w toolkitach w 60 dni. Serwer MCP raportuje jedynie wersję aplikacji [9][22].
3. **Dwa GUI.** Rozmowa z agentem toczy się w T3, a stan domeny w Twojej aplikacji. Spina je deep link.
4. **Model.** Jeden projekt T3 to jeden katalog, więc Workspace obejmujący kilka repo staje się jednym projektem T3 na każde repo, plus projektem dla katalogu Workspace. Ktoś, kto budował zewnętrzną aplikację do wybierania pracy, trafił na to samo: każdy wątek wymaga projektu, a wątki pomocnicze zaśmiecają listę wątków repo [86].

**Luki znalezione przez red team 2** (rozdz. 13):

5. **Interfejs istnieje tylko w nightly.**
   - OAuth dla zewnętrznych agentów wszedł 2026-10-06, a ostatnie stabilne wydanie to v0.0.45 z 2026-10-02 [88].
   - Klient OAuth, który nie wysyłał nagłówka `MCP-Protocol-Version`, dostawał 400 na wywołania narzędzi. Serwer akceptuje tylko jedną wersję protokołu. Naprawiono to 2026-10-09 [89]. Pokazuje to kruchość interfejsu, nie jego stałą awarię.
   - `t3_thread_launch` nie ma klucza ponowienia; PR dodający go jest otwarty [91].
   - Deep linki `t3code://` nie działały na desktopie od 0.0.38 aż do 2026-10-12 [87].
6. **Ryzyko utraty danych.** Okno usuwania wątku w GUI T3 proponuje usunięcie *dowolnego* worktree wątku, także dołączonego przez `existing_worktree`. Nie sprawdza, czy worktree leży w katalogu zarządzanym przez T3 [90]. Na Windows usuwanie worktree podąża za junction i kasuje ich cele (#14990) [34]. Automatyczny cleanup T3 omija worktree spoza swojego katalogu [90].
7. **Kierunek upstreamu.** Upstream buduje system pluginów: 19 PR-ów w recenzji, narzędzia pluginów przez MCP, panele, zatwierdzenia [81]. Starszy RFC opisywał integrację jako „Twoja aplikacja renderuje się wewnątrz T3” [92]. Sidecar celuje więc w interfejs, w który upstream inwestuje mniej.

## 10. Alternatywy (pytanie 8)

Żaden inny otwarty projekt nie jest wyraźnie lepszą bazą do forka.

| Projekt | Licencja | Windows | Multi-repo | Stan aplikacji dla agentów | Werdykt |
|---|---|---|---|---|---|
| Vibe Kanban | Apache-2.0 | Prawdopodobnie | **Tak** | **Tak** (lokalny MCP) | Najlepsze dopasowanie funkcji, ale firma zamknięta 2026-04-10, ostatni release 2026-04-24, brak roadmapy społeczności. Stack: Rust + TS web [55][56] |
| Pane | AGPL-3.0 | Tak | Repo w Workspace, ale „1 feature = 1 worktree” | Tak (CLI `runpane` + MCP) | Aktywny, ale copyleft i bez zadań obejmujących kilka repo [57] |
| Emdash | Apache-2.0 | Tak | Nie | Nie | Dobre źródło wzorców (PTY, worktree), nie baza [53] |
| Superset | ELv2 (nie OSI) | **Nie** | — | Tak | Odpada [58] |
| Conductor | zamknięty | **Nie** (macOS) | — | — | Odpada [60] |
| ivar | Apache-2.0 | **Nie** | **Tak** (rdzeń idei) | CLI | Odpada jako baza; dobry wzorzec modelu „View Dir” [59] |

Vibe Kanban warto potraktować jako **wzorzec projektowy** dla multi-repo i MCP, nie jako bazę.

## 11. Ważona macierz decyzyjna

Skala 1–5 (5 = najlepiej). Oceny to **mój osąd na podstawie dowodów** z rozdziałów 2–10 i dwóch przebiegów red teamu (rozdz. 13), a nie pomiar. Macierz jest po to, żebyś mógł zmienić wagi i sprawdzić, czy werdykt się utrzyma.

Warianty B1 i B2 definiuje rozdz. 1.

| Kryterium (waga) | A: fork T3 Code | B1: od zera, PTY | B2: od zera, host strukturalny | C: sidecar na T3 Code |
|---|---|---|---|---|
| Dopasowanie domeny i wymagań v1 (30) | **3.** Warstwa sesji, MCP, worktree i tryb zdalny są gotowe; rdzeń domeny trzeba dopisać; zakres Workspace koliduje z „wątek = jeden cwd” [3][22][30] | **4.** Domena w 100%; sesje PTY na Windows ucinają wejście, a detekcja stanu jest zawodna [77][79][80] | **4,5.** Domena w 100%; zdarzenia i zatwierdzenia strukturalne [40][46]; otwarte błędy SDK na Windows [45] | **3.** Domena Twoja, ale dwa GUI, deep linki naprawione dopiero 2026-10-12, projekt T3 wymagany dla każdego wątku [16][86][87] |
| Kontrola nad architekturą (20) | **1.** Effect-TS importowany w 67–99% plików, event sourcing upstreamu, wewnętrzne API bez wersji [1][6][22] | **5** | **5** | **4.** Domena pełna; host sesji poza kontrolą, ale wymienny |
| Wysiłek do v1 (20) | **3.** Dużo gotowe; funkcja domenowa ≈ 50 plików w obcym idiomie [69]; fork t3kanban wydał tablicę zadań w dniu utworzenia repo (czas pracy nieznany) [82] | **1,5.** PTY, hooki, worktree, SQLite, MCP, GUI + utwardzanie PTY na Windows [77][78] | **1,5.** Minimalne adaptery dla 2 dostawców + prosty widok rozmowy (adapter Claude w T3 ma 9,3 tys. linii, ale obsługuje wszystko [5]) | **3,5.** Bez warstwy sesji; OAuth i narzędzia dostępne tylko w nightly i już raz zepsute [88][89] |
| Koszt utrzymania (15) | **1.** 50–80 commitów dziennie; hooki w plikach zmienianych codziennie; przepisania kasują funkcje forków [22][26][28] | **4.** Dryf flag i hooków dwóch CLI [66] | **3,5.** Dryf API SDK (Agent SDK 0.3.x co kilka dni; OpenCode w trakcie przejścia na v2) [45][47] | **2.** Interfejs ma 6 dni, nie ma wersji, a upstream inwestuje w pluginy zamiast integracji z zewnątrz [81][88] |
| Droga do trybu serwera i telefonu (10) | **5.** Gotowe [12] | **2.** Do zbudowania; terminal na telefonie to słaby UX | **2,5.** Do zbudowania; strukturalne zdarzenia łatwo przesłać do zdalnego klienta | **3.** Sesje zdalnie przez T3 [12]; GUI domeny trzeba udostępnić samemu |
| Ryzyko organizacyjne (5) | **3.** MIT i poparcie forków [18][19], ale alfa i Windows poza CI [33] | **4.** Ścieżka CLI jest zgodna z warunkami Anthropic [41] | **4.** Osobiste lokalne użycie SDK z własnym planem wygląda na wspierane [42][44] | **2,5.** Zależność od kierunku i jakości nightly T3; ryzyko utraty danych worktree [34][90] |
| **Wynik ważony (maks. 5)** | **2,50** | **3,50** | **3,63** | **3,13** |

**Jak liczony jest wynik:** Σ(ocena × waga) / 100. Przykład dla B2: (4,5·30 + 5·20 + 1,5·20 + 3,5·15 + 2,5·10 + 4·5) / 100 = 3,625.

**Historia ocen (dla przejrzystości):**
- **Przed red teamem:** B (wtedy tylko PTY) = 3,90, C = 3,55.
- **Po red teamie 1:** problemy PTY na Windows obniżyły B1 do 3,50. Wariant B2 ominął te problemy.
- **Po red teamie 2:** niedojrzałość i nightly-only interfejsu T3 dla zewnętrznych agentów obniżyły C do 3,13.

**Wrażliwość werdyktu:**
- **B2 i B1 różnią się mało.** Wybór hosta wewnątrz B rozstrzygają spike'i S2 i S7, nie research.
- **C wyprzedza B2**, gdy waga wysiłku sięga ok. 40, a waga kontroli spada do ok. 10, albo gdy interfejs T3 dla zewnętrznych agentów trafi do stabilnego wydania, a jego ocena utrzymania wzrośnie do ≥3,5.
- **A wyprzedza B2** dopiero wtedy, gdy tryb serwera i telefon dostaną wagę porównywalną z dopasowaniem domeny, a koszt utrzymania zniknie. Tak byłoby przy zamrożonym forku, który traci jednak poprawki adapterów.
- **Rozstrzyga kryterium wysiłku**, które opiera się na wskaźnikach pośrednich. Weryfikują je spike'i S2, S5 i S7.

## 12. Wnioski przekrojowe

1. **Twojego wyróżnika nie ma w żadnym kandydacie na bazę.** Workspace, Task z zależnościami i kolejnością merge'a, Artifact i Handoff trzeba napisać w A, B i C [3][8]. Fork oszczędza warstwę wykonawczą, nie domenową. A to domena jest większością pracy nad v1 i jedynym powodem istnienia produktu.
2. **Warstwa wykonawcza jest w T3 najmocniejsza i właśnie tam zmienia się najszybciej.** Adapter Claude ma ok. 4 commity dziennie [22], a autor recenzji wskazuje adaptery jako najtrudniejszą część [38]. Fork wiąże Cię dokładnie z tą częścią, która wymaga ciągłej synchronizacji.
3. **Wymaganie 7 dotyczy stanu pracy, nie rozmowy.** Przełączanie dostawcy w T3 [13] jest zaletą UX, ale nie spełnia wymagania samo z siebie. Spełnia je trwały stan domeny (Taski, Artefakty, Handoffy) dostępny przez MCP. Ten stan jest niezależny od hosta sesji.
4. **Rdzeń domeny z MCP jest identyczny w B1, B2 i C.** Wybór hosta sesji (PTY, strukturalny, T3 z zewnątrz, w przyszłości plugin T3) można więc **odłożyć**. Wystarczy zbudować najpierw rdzeń domeny i serwer MCP, a host sesji schować za portem `SessionHost`. To najtańsze zabezpieczenie odwracalności w tym raporcie.
5. **Model T3 nie jest wrogi Twojemu w zakresie Worktree.** Sesja Worktree ≈ wątek T3 z `existing_worktree`. Kolizja dotyczy tylko zakresu Workspace i grupy repo [3][16][31]. Tłumaczy to, czemu C jest wykonalne mimo „wątek = jeden cwd”.
6. **Ryzyko kierunku upstreamu działa w dwie strony.** Jeśli T3 doda multi-repo i zadania, fork wpada w konflikt koncepcji, a historia forków pokazuje porzucanie funkcji przy przepisaniach [26][27]. Red team nie znalazł jednak żadnych planów T3 dotyczących multi-repo ani zależności zadań [29].
7. **Najlepsza przyszła ścieżka integracji z T3 to nie fork ani sidecar, tylko plugin.** Jeśli system pluginów [81] zostanie scalony i ustabilizowany, warstwę domenową da się wpiąć w T3 bez forka: jako narzędzia MCP dla agentów, panel i statusy wątków. To realna opcja D, ale **dziś niedostępna**. Rdzeń domeny napisany jako niezależna biblioteka i serwis zachowuje tę możliwość.
8. **Windows przesądza o wariancie hosta.** Problemy PTY [77][79], błędy Agent SDK [45] i błąd #14990 w T3 [34] dotyczą właśnie Twojej platformy. Stąd przewaga hosta strukturalnego i potrzeba spike'ów przed wyborem.

## 13. Dowody przeciwne (red team)

Były dwa przebiegi. Każdy prowadził sceptyk bez kontekstu tego badania, który dostał tylko tezę do obalenia. Oba zmieniły werdykt, nie tylko go skomentowały.

**Red team 1 obalał: „nie forkuj, buduj od zera w wariancie terminal-first”.**
- **T3 ma już trudną hydraulikę.** Wbudowany MCP w sesjach, delegowanie subagentów, linkowanie PR-ów, zadania cykliczne, przełączanie dostawcy z jawnym przekazaniem kontekstu (ContextHandoff) [8][13]. W recenzji jest system pluginów z narzędziami MCP i statusami wątków [81].
- **Fork potrafi szybko dowieźć.** t3kanban wydał tablicę zadań z wydaniami na trzy systemy w dniu utworzenia repo [82]. Ile trwała praca przed publikacją, nie wiadomo. mtcode jest 1 492 commity do przodu i tylko 6 z tyłu [83].
- **PTY na Windows ma zmierzone awarie:**
  - ucinanie i dzielenie wiadomości [77];
  - osierocone procesy `claude.exe` [78];
  - Shift+Enter nieodróżnialny od Enter [79];
  - brak wiarygodnego sygnału stanu sesji: „months battling detection bugs”, a prośba o API stanu zamknięta jako NOT_PLANNED [80].
- **Projekty budowane od zera utykały albo zmieniały platformę** (opcode [85], Crystal [54]). To były jednak zespoły, nie samotni autorzy, więc analogia jest słaba.
- **Co z tego wynika.** Wariant terminal-first spadł (B1 = 3,50) i wydzieliłem B2. **Zarzut się utrzymuje w części:** wysiłek B jest realnie wysoki. **Nie utrzymuje się w części o forku:** t3kanban po szybkim starcie przestał się synchronizować i jest 1 650 commitów z tyłu [82]. Szybkość forka kupuje v1, ale nie utrzymanie.

**Red team 2 obalał: „v1 = sidecar sterujący T3 z zewnątrz”.**
- **Interfejs jest bardzo świeży.** Istnieje tylko w nightly, ma 6 dni, a klienci trafiali już na błędy protokołu [88][89]. Kontrakt uruchamiania wątku wciąż się zmienia [91]. Deep linki działają dopiero od dnia pisania raportu [87].
- **Ryzyko utraty danych** worktree dołączonych z zewnątrz, przez okno usuwania wątku i #14990 [34][90].
- **Upstream inwestuje w pluginy**, nie w integrację z zewnątrz [81][92].
- **Co z tego wynika.** C spadło do 3,13, a werdykt przeszedł na B2. C zostaje jako opcja warunkowa.

**Czego red team szukał i nie znalazł** (to też ustalenia):
- planów T3 dotyczących multi-repo albo zależności zadań;
- relacji samotnego autora, który porzucił budowę takiego GUI od zera;
- danych o czasie budowy w którąkolwiek stronę;
- deklaracji stabilności lub wersjonowania interfejsu `/mcp` dla zewnętrznych agentów;
- dowodów, że agenci systematycznie ignorują narzędzia MCP aplikacji. Problemem bywa ładowanie narzędzi [84], a nie ich ignorowanie.

## 14. Rekomendacja

**Rekomendacja: nie forkuj T3 Code. Buduj własną aplikację (opcja B, wariant B2): rdzeń domeny z serwerem MCP jako niezależny serwis, a host sesji za portem `SessionHost`. Domyślnym hostem w v1 jest host strukturalny: Claude Agent SDK z Twoją lokalną subskrypcją oraz `opencode serve` z SSE. Osadzony terminal służy tylko jako wyjście awaryjne** („otwórz tę sesję w terminalu” przez `claude --resume`).

Podstawa pewności:
- **Wysoka** dla odrzucenia A. Koszt utrzymania i utrata kontroli są zmierzone na kodzie i na realnych forkach [22][26][28].
- **Średnia** dla wyboru B2 zamiast C. Rozstrzyga niedojrzałość interfejsu T3, potwierdzona w issues [87][88][89].
- **Niska** dla wysiłku, bo wszystkie oceny wysiłku są pośrednie.

**Drugi wybór: C (sidecar na T3), warunkowo.** Wygrywa, gdy spełnione są oba warunki:
1. Interfejs `/mcp` dla zewnętrznych agentów trafi do stabilnego wydania T3 i przejdzie spike S3.
2. Spike'i S2 i S7 pokażą, że własny host sesji na Windows kosztuje więcej, niż jesteś gotów zapłacić.

**Warunkowa opcja D:** jeśli T3 scali system pluginów [81], ponownie oceń wpięcie rdzenia domeny jako pluginu T3. Rdzeń napisany jako niezależny serwis to umożliwia.

**Najmocniejszy argument przeciwko rekomendacji.** Budujesz sam, po godzinach, warstwę, którą T3 już ma, a utrzymuje ją zespół robiący ok. 1 500 commitów miesięcznie. Chodzi o strukturalne sesje dwóch dostawców na Windows, zatwierdzenia, przełączanie dostawcy, tryb zdalny i aplikację mobilną [5][12][13]. Robisz to na platformie, na której Agent SDK ma otwarte błędy awarii [45]. Wcześniejsze GUI tego typu budowane od zera utykały albo zmieniały platformę [54][85], a fork t3kanban pokazał, że tablicę zadań da się wpiąć w T3 i wydać na trzy systemy [82].

Ten argument jest prawdziwy co do *wysiłku do v1* i dlatego wysiłek ma w macierzy ocenę 1,5. Nie przeważa, bo:
- Twoje wymagania v1 potrzebują tylko 2 dostawców i minimalnego widoku rozmowy;
- wymaganie 7 spełnia stan domeny, nie przełączanie rozmowy;
- historia forków pokazuje, że szybki start forka kończy się dryfem albo stałą synchronizacją [26][82];
- kontrola nad architekturą ma wagę 20, nadaną z powodu rozjazdu poprzedniego prototypu.

**Najtańsze zabezpieczenie odwracalności.** Kolejność budowy: najpierw rdzeń domeny + MCP + GUI stanu, z hostem sesji jako portem. Pierwsza implementacja portu powinna być najtańsza z tych, które przejdą spike'i. Jeśli S7 (host strukturalny) wypadnie źle, a S3 dobrze, podmieniasz host na adapter T3 bez ruszania domeny.

### Wejścia dla `bmad-spec` i `bmad-architecture`

**Dla `bmad-spec`** (kryteria akceptacji i zakres):
- Wymaganie 7 sformułuj jako trwałość *stanu domeny* (Task, Artifact, Handoff) niezależnie od sesji i dostawcy, a nie jako przenoszenie rozmowy [13].
- Zapisz, że sesje uruchamia aplikacja z profilu Agenta (prompt, dostawca, model) i z zakresem (Workspace = katalog z worktree jako podkatalogami; Worktree = jeden worktree).
- Handoff musi być wymuszalny (hook `Stop` lub równoważny), jeśli S6 pokaże, że agenci go pomijają.
- Doprecyzuj Windows jako platformę v1 i to, że worktree mogą zawierać junction, np. node_modules [34].

**Dla `bmad-architecture`** (decyzje i ograniczenia):
- **Proces.** Wzorzec z T3 jest wart skopiowania [2]: jeden lokalny serwer jest właścicielem stanu, procesów agentów i gita; GUI jest cienkim klientem po typowanym RPC lub WebSocket. Daje to drogę do trybu serwera i telefonu bez przepisywania.
- **Porty.** `SessionHost` (implementacje: AgentSdk, OpenCodeServe; w razie potrzeby PTY lub T3-outside) oraz `Forge` (poza v1; CR rejestrują agenci).
- **Persystencja.** SQLite z własnymi migracjami. Stan domeny jest źródłem prawdy, a sesja jest tylko referencją.
- **MCP.** Serwer w procesie (MCP TS SDK v2 [50]), wstrzykiwany per sesja (`mcpServers` w SDK [40]; `OPENCODE_CONFIG_CONTENT` lub `mcp.add` [46][63]). Token powiąż z sesją, wzorem T3 [9].
- **Worktree.** Wywołania `git` z retry, walidacją ścieżek i rollbackiem, wzorem Emdash [53]. Worktree jako podkatalogi katalogu Workspace, dzięki czemu sesja w zakresie Workspace widzi całą inicjatywę.
- **Czego nie kopiować.** Effect-TS jako warunek wejścia (to wybór zespołu T3, nie wymóg domeny) i normy, że już w v1 każda funkcja działa na webie, desktopie i mobile.
- **Wzorce projektowe do przejrzenia:** model „View Dir” z ivar [59], multi-repo i MCP w Vibe Kanban [56], ADR-y Emdash [53].

## 15. Czego sieć nie rozstrzygnie: lista spike'ów

**Priorytet: najpierw S1 → S7 → S2** (przed `bmad-architecture`). S3 tylko wtedy, gdy rozważasz C; S5 tylko wtedy, gdy wraca A. Numery spike'ów nie oznaczają kolejności. Dla każdego: co sprawdzić i po czym poznać wynik.

| # | Spike | Co sprawdzić | Po czym poznać wynik |
|---|---|---|---|
| S1 | **Własny MCP widoczny w sesjach** (rozstrzyga B i C) | Postaw minimalny serwer MCP z 2 narzędziami (`workspace_read`, `handoff_write`). Podepnij go do `claude` przez `--mcp-config` i do `opencode` przez `OPENCODE_CONFIG_CONTENT` [61][63]. Osobno przez `~/.claude.json` / `opencode.json` w sesji uruchomionej z T3 [73] | Agent w każdej z 4 konfiguracji wywołuje oba narzędzia bez przypominania. Porażka: narzędzia nie ładują się albo agent je ignoruje (znane błędy OpenCode w T3 [32]) |
| S2 | **Terminal-first na Windows** (B) | Electron + node-pty + xterm.js. Uruchom `claude --session-id <uuid> --mcp-config … --add-dir … --settings <hooki>` i `opencode --port`. Hooki `Stop` / `PermissionRequest` / `Notification` mają trafiać przez HTTP do aplikacji [62][64] | Status working / needs-permission / done jest poprawny w ≥95% przejść w 20-minutowej sesji. Resize, Shift+Enter i kolory działają (znane problemy ConPTY [67]). Hooki z `--settings` działają bez edycji konfiguracji użytkownika: to inferencja, niesprawdzona |
| S3 | **Sterowanie T3 z zewnątrz** (C) | Własny skrypt: OAuth z kodem parowania → `t3_project_create` (katalog Workspace, niegitowy) → `t3_project_create` (repo) → `git worktree add <workspace>/<repo>-1` → `t3_thread_launch` z `existing_worktree` → `t3_thread_wait` / `read` → deep link `t3code://threads/…` [15][16][17]. Do tego: czy token z `t3 auth session issue` działa na `/mcp` | Wątek startuje w worktree spoza `~/.t3/worktrees`, a deep link otwiera go w GUI. Testuj wyłącznie na **stabilnym** wydaniu, w którym jest OAuth dla zewnętrznych agentów [88]. Czerwone flagi: okno usuwania wątku proponuje skasowanie Twojego worktree [90]; po 30 dniach wymagane ponowne parowanie bez obejścia |
| S7 | **Host strukturalny na Windows** (B2, rekomendacja) | W Electronie: Claude Agent SDK `query()` z `cwd` = worktree, `additionalDirectories`, `mcpServers` (własny MCP), `resume` i `interrupt`; zatwierdzenia przez `canUseTool` [40]. Do tego `opencode serve` z `/event` SSE i odpowiedziami na uprawnienia przez HTTP [46]. Logowanie: Twoja subskrypcja przez lokalne Claude Code | Sesja Claude i sesja OpenCode przechodzą: start → narzędzie MCP → zatwierdzenie → przerwanie → wznowienie, bez awarii (sprawdź #444 interrupt i #359 stream closed [45]). Minimalny widok rozmowy da się zrobić w kilka wieczorów. Jeśli nie, rozważ C po S3 |
| S4 | **Zakres Workspace w T3** (A i C) | Projekt T3 na katalogu Workspace z zagnieżdżonymi worktree kilku repo. Agent dostaje zadanie obejmujące 2 repo [31] | Agent widzi i edytuje oba repo. Wiesz dokładnie, które funkcje gita T3 przestają działać (diff, PR, checkpoint) i czy Ci ich brakuje |
| S5 | **Koszt jednej funkcji domenowej w forku** (tylko jeśli A wraca do gry) | W forku dodaj byt `Task` z zależnościami według wzorca commita 1f6df6b [69]: osobny rejestr migracji [70][76], toolkit MCP, widok web. Po tygodniu zmerguj upstream | Mierzysz godziny implementacji i godziny merge'a. Jeśli merge po tygodniu kosztuje więcej niż ok. 1 h albo psuje hook bez konfliktu (zjawisko opisane w [28]), A odpada definitywnie |
| S6 | **Dyscyplina agentów wobec stanu** (wszystkie opcje) | Prompt workera wymaga `handoff_write` na końcu pracy. 10 przebiegów na Claude i OpenCode | Handoff zapisany w ≥9/10 przebiegów z wypełnionymi polami (zmiany, dowody weryfikacji, pytania). Inaczej potrzebny jest hook `Stop`, który wymusza handoff |

## 16. Pytania otwarte i miejsca o cienkich dowodach

- **Godziny pracy.** Żadne źródło nie podaje czasu budowy GUI dla agentów ani czasu utrzymania forka. Wszystkie oceny wysiłku to proxy: historia repo i liczba plików. *Rozstrzygają S2 i S5.*
- **Czy `--settings` / `--plugin-dir` wstrzykuje hooki bez edycji konfiguracji użytkownika.** To inferencja z dokumentacji [61][62]. *Rozstrzyga S2.*
- **Projektowy `.mcp.json` w sesjach Claude uruchamianych przez T3** jest niezweryfikowany. Działanie `~/.claude.json` potwierdza tylko zgłoszenie użytkownika [73]. *Rozstrzyga S1.*
- **Długoterminowy token do `/mcp` T3** jest niezweryfikowany [15]. *Rozstrzyga S3.*
- **Warunki Anthropic dla osobistego narzędzia na Agent SDK z subskrypcją.** Brak jednoznacznego stanowiska. Ścieżka CLI jest jednoznacznie dozwolona [41][42], a zmiana rozliczeń SDK jest wstrzymana [44] (pojedyncze źródło). *Korzystaj z CLI albo klucza API, dopóki Anthropic tego nie doprecyzuje.*
- **Plany T3 wobec multi-repo i zadań.** Nie znaleziono wypowiedzi Theo ani Juliusa poza komentarzem [71]. Wyszukiwanie na X i YouTube było ograniczone (X zwraca 402). *Obserwuj #7041 i #15752.*
- **Wypowiedź o braku płatnego planu T3** pochodzi tylko ze źródła wtórnego. *Niezweryfikowane.*
- **Stan Vibe Kanban na Windows** i działanie jego MCP po wyłączeniu chmury: *niezweryfikowane* [56].
- **Dojrzałość OpenCode 2 SDK** (istnieje od ok. 6 tygodni, dokumentacja tylko w README) [47]. Dla B to argument, żeby zacząć od CLI, TUI i SSE, a nie od SDK 2.x.

## Źródła

Repozytorium T3 Code cytowane jest jako `pingdotgg/t3code@0e7346c`, a data publikacji to data ostatniego commita dotykającego pliku. Dostęp do wszystkich źródeł: 2026-10-12.

| [n] | Twierdzenie / ustalenie | Wydawca | Data publ. | Dostęp | Pewność |
|---|---|---|---|---|---|
| [1] | Pozycjonowanie, stack, normy (multi-surface, forki), autorstwo przez agentów | [pingdotgg/t3code AGENTS.md](https://github.com/pingdotgg/t3code/blob/0e7346c/AGENTS.md) | 2026-10-10 | 2026-10-12 | wysoka |
| [2] | Procesy, granica serwer–klient, event sourcing, checkpointy | [t3code docs/internals/overview.md](https://github.com/pingdotgg/t3code/blob/0e7346c/docs/internals/overview.md) | 2026-10-02 | 2026-10-12 | wysoka |
| [3] | Schemat wątku i projektu (1 projekt, ≤1 worktree), linki PR | [t3code contracts/orchestrationV2.ts](https://github.com/pingdotgg/t3code/blob/0e7346c/packages/contracts/src/orchestrationV2.ts) | 2026-10-10 | 2026-10-12 | wysoka |
| [4] | cwd = worktreePath ?? workspaceRoot | [t3code RuntimePolicy.ts](https://github.com/pingdotgg/t3code/blob/0e7346c/apps/server/src/orchestration-v2/RuntimePolicy.ts) | 2026-10-09 | 2026-10-12 | wysoka |
| [5] | Adapter Claude (Agent SDK, additionalDirectories, 9,3 tys. linii) | [t3code ClaudeAdapterV2.ts](https://github.com/pingdotgg/t3code/blob/0e7346c/apps/server/src/orchestration-v2/Adapters/ClaudeAdapterV2.ts) | 2026-10-11 | 2026-10-12 | wysoka |
| [6] | Architektura docelowa V2, „ignores backward compatibility” | [t3code docs/orchestration-v2/README.md](https://github.com/pingdotgg/t3code/blob/0e7346c/docs/orchestration-v2/README.md) | 2026-10-10 | 2026-10-12 | wysoka |
| [7] | Warstwa driverów i adapterów dostawców, transporty | [t3code docs/internals/providers.md](https://github.com/pingdotgg/t3code/blob/0e7346c/docs/internals/providers.md) | 2026-10-08 | 2026-10-12 | wysoka |
| [8] | MCP orkiestratora wstrzykiwany do sesji; narzędzia | [t3code orchestrator-mcp-server.md](https://github.com/pingdotgg/t3code/blob/0e7346c/docs/orchestration-v2/orchestrator-mcp-server.md) | 2026-10-10 | 2026-10-12 | wysoka |
| [9] | 11 toolkitów MCP, rejestracja przez Layer.mergeAll, wersja = wersja aplikacji | [t3code McpHttpServer.ts](https://github.com/pingdotgg/t3code/blob/0e7346c/apps/server/src/mcp/McpHttpServer.ts) | 2026-10-09 | 2026-10-12 | wysoka |
| [10] | SQLite, jeden rejestr migracji | [t3code persistence/Migrations.ts](https://github.com/pingdotgg/t3code/blob/0e7346c/apps/server/src/persistence/Migrations.ts) | 2026-10 | 2026-10-12 | wysoka |
| [11] | Tworzenie worktree przy starcie wątku, strategie | [t3code ThreadLaunchService.ts](https://github.com/pingdotgg/t3code/blob/0e7346c/apps/server/src/orchestration-v2/ThreadLaunchService.ts) | 2026-10-11 | 2026-10-12 | wysoka |
| [12] | Zdalny dostęp: LAN, Tailscale, SSH, T3 Connect (relay, Clerk) | [t3code docs/user/remote-access.md](https://github.com/pingdotgg/t3code/blob/0e7346c/docs/user/remote-access.md) | 2026-10-10 | 2026-10-12 | wysoka |
| [13] | Przełączanie dostawcy i ContextHandoff | [t3code provider-switching-and-context.md](https://github.com/pingdotgg/t3code/blob/0e7346c/docs/orchestration-v2/provider-switching-and-context.md) | 2026-10-02 | 2026-10-12 | średnia (dokumentacja, nie test) |
| [14] | Zewnętrzni agenci przez /mcp | [t3code docs/user/outside-agents.md](https://github.com/pingdotgg/t3code/blob/0e7346c/docs/user/outside-agents.md) | 2026-10-06 | 2026-10-12 | wysoka |
| [15] | OAuth tylko z authorization_code, bez refresh tokena, 30 dni | [t3code auth/McpOAuth.ts](https://github.com/pingdotgg/t3code/blob/0e7346c/apps/server/src/auth/McpOAuth.ts) | 2026-10-06 | 2026-10-12 | wysoka |
| [16] | Klasy dostępu do narzędzi; t3_thread_launch, existing_worktree, niegitowe projekty | [t3code McpToolAccess.ts](https://github.com/pingdotgg/t3code/blob/0e7346c/apps/server/src/mcp/McpToolAccess.ts) | 2026-10 | 2026-10-12 | wysoka (kod) / średnia (zachowanie) |
| [17] | Deep linki t3code://, `t3 app` | [t3code DesktopClerk.ts](https://github.com/pingdotgg/t3code/blob/0e7346c/apps/desktop/src/app/DesktopClerk.ts) | 2026-10 | 2026-10-12 | wysoka |
| [18] | Licencja MIT, T3 Tools Inc. | [t3code LICENSE](https://github.com/pingdotgg/t3code/blob/0e7346c/LICENSE) | 2026-03-05 | 2026-10-12 | wysoka |
| [19] | Cytat o forkach, „very very early” | [t3code README.md](https://github.com/pingdotgg/t3code/blob/0e7346c/README.md) | 2026-07-28 | 2026-10-12 | wysoka |
| [20] | Brak zainteresowania kontrybucjami z zewnątrz | [t3code CONTRIBUTING.md](https://github.com/pingdotgg/t3code/blob/0e7346c/CONTRIBUTING.md) | 2026-09-30 | 2026-10-12 | wysoka |
| [21] | Brak bezpiecznego zakresu migracji dla forków; noty licencji | [t3code legacy-orchestration-migration.md](https://github.com/pingdotgg/t3code/blob/0e7346c/docs/internals/legacy-orchestration-migration.md) | 2026-10-02 | 2026-10-12 | wysoka |
| [22] | Metryki z gita: tempo, autorzy, rotacja, LOC, udział Effect | [lokalny klon, git log/shortlog @0e7346c](https://github.com/pingdotgg/t3code/commits/main) | 2026-10-12 | 2026-10-12 | wysoka (pomiar zgrubny) |
| [23] | PR #2829 nowy orkiestrator, importer V1→V2 | [GitHub pingdotgg/t3code#2829](https://github.com/pingdotgg/t3code/pull/2829) | 2026-10-02 | 2026-10-12 | wysoka |
| [24] | Statystyki repo, issues, PR | [GitHub API pingdotgg/t3code](https://github.com/pingdotgg/t3code) | 2026-10-12 | 2026-10-12 | wysoka |
| [25] | Rozjazd forków (compare API) | [GitHub forks pingdotgg/t3code](https://github.com/pingdotgg/t3code/forks) | 2026-10-12 | 2026-10-12 | wysoka |
| [26] | TrogonStack: porzucone rozbieżności przy V2 | [TrogonStack/t3code#71](https://github.com/TrogonStack/t3code/pull/71) | 2026-10-02 | 2026-10-12 | wysoka |
| [27] | Fork aaditagrawal zarchiwizowany (upstream wchłonął funkcje) | [aaditagrawal/t3code README](https://github.com/aaditagrawal/t3code) | 2026-10-09 | 2026-10-12 | wysoka |
| [28] | Wzorzec „Fork add-on”, kontrola markerów, automatyczny merge | [travisjneuman/t3code fork-add-ons.md](https://github.com/travisjneuman/t3code/blob/main/docs/internals/fork-add-ons.md) | 2026-10 | 2026-10-12 | wysoka (istnienie) / niska (skuteczność długoterminowa) |
| [29] | Prośba o multi-repo otwarta, bez maintainera | [pingdotgg/t3code discussion #7041](https://github.com/pingdotgg/t3code/discussions/7041) | 2026-03-27 | 2026-10-12 | wysoka |
| [30] | PR multi-repo otwarty, bez recenzji, `vouch:unvouched` | [pingdotgg/t3code#15752](https://github.com/pingdotgg/t3code/pull/15752) | 2026-10-04 | 2026-10-12 | wysoka |
| [31] | Niegitowy katalog z zagnieżdżonymi repo działa, git UX nie | [pingdotgg/t3code#12902](https://github.com/pingdotgg/t3code/issues/12902) | 2026-09-25 | 2026-10-12 | średnia |
| [32] | Błędy MCP i współdzielonej bazy w OpenCode | [pingdotgg/t3code#16141](https://github.com/pingdotgg/t3code/issues/16141) | 2026-10 | 2026-10-12 | wysoka |
| [33] | CI tylko Linux; testy Windows ręczne i czerwone; lint na „warn” | [t3code windows-tests.yml](https://github.com/pingdotgg/t3code/blob/0e7346c/.github/workflows/windows-tests.yml) | 2026-10 | 2026-10-12 | wysoka |
| [34] | Utrata danych na Windows przy usuwaniu worktree; triage przez AI | [pingdotgg/t3code#14990](https://github.com/pingdotgg/t3code/issues/14990) | 2026-10-03 | 2026-10-12 | wysoka |
| [35] | Uszkodzenie SQLite na macOS | [pingdotgg/t3code#13544](https://github.com/pingdotgg/t3code/issues/13544) | 2026-09-25 | 2026-10-12 | wysoka |
| [36] | Regresje stanu po V2 | [pingdotgg/t3code#15017](https://github.com/pingdotgg/t3code/issues/15017) | 2026-10-03 | 2026-10-12 | wysoka |
| [37] | Recenzja użytkowa po kilku tygodniach | [bmateas.com](https://bmateas.com/blog/t3-code-review) | 2026-07-28 | 2026-10-12 | średnia |
| [38] | Analiza architektury: alfa, adaptery to najtrudniejsza część | [flaviocopes.com](https://flaviocopes.com/t3-code) | 2026-08-28 | 2026-10-12 | średnia |
| [39] | Komentarze HN (cienkie, mieszane) | [Hacker News via Algolia](https://hn.algolia.com/?q=t3%20code) | 2026-04..10 | 2026-10-12 | średnia |
| [40] | Możliwości Claude Agent SDK; zakaz logowania claude.ai w produktach | [Anthropic agent-sdk overview](https://code.claude.com/docs/en/agent-sdk/overview) | b.d. (dok. żywa) | 2026-10-12 | wysoka |
| [41] | OAuth tylko dla użycia zwykłego; niezmodyfikowane CLI dozwolone | [Anthropic legal-and-compliance](https://code.claude.com/docs/en/legal-and-compliance) | b.d. (dok. żywa) | 2026-10-12 | wysoka |
| [42] | Niezależne potwierdzenie zakazu (cytat Anthropic) | [GIGAZINE](https://gigazine.net/gsc_news/en/20260220-anthropic-third-party-block) | 2026-02-20 | 2026-10-12 | wysoka |
| [43] | Oś czasu egzekwowania wobec narzędzi zewnętrznych | [falcao.org](https://falcao.org/posts/anthropic-claude-access-crackdown-ecosystem-fallout/) | 2026-05-24 | 2026-10-12 | średnia |
| [44] | Wstrzymana zmiana rozliczeń Agent SDK | [Anthropic Help Center](https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-claude-plan) | 2026-10-07 | 2026-10-12 | średnia (pojedyncze źródło) |
| [45] | Agent SDK TS 0.3.296; błędy Windows #444, #359 | [anthropics/claude-agent-sdk-typescript](https://github.com/anthropics/claude-agent-sdk-typescript/releases) | 2026-10-09 | 2026-10-12 | wysoka |
| [46] | `opencode serve`: OpenAPI, SSE, MCP dynamicznie | [OpenCode docs: server](https://opencode.ai/docs/server/) | 2026-10-12 | 2026-10-12 | wysoka |
| [47] | OpenCode 1.19.0, przejście na v2, SDK 2.x w procesie | [anomalyco/opencode v1.19.0](https://github.com/anomalyco/opencode/releases/tag/v1.19.0) | 2026-10-11 | 2026-10-12 | wysoka (istnienie) / niska (dojrzałość v2) |
| [48] | ACP 1.25.0, metody stabilne i niestabilne | [agentclientprotocol/agent-client-protocol](https://github.com/agentclientprotocol/agent-client-protocol) | 2026-10-09 | 2026-10-12 | wysoka |
| [49] | Adapter Claude dla ACP i jego ograniczenia | [agentclientprotocol/claude-agent-acp](https://github.com/agentclientprotocol/claude-agent-acp) | 2026-10-10 | 2026-10-12 | wysoka |
| [50] | MCP TS SDK v2 stabilne | [modelcontextprotocol/typescript-sdk](https://github.com/modelcontextprotocol/typescript-sdk) | 2026-10-05 | 2026-10-12 | wysoka |
| [51] | node-pty: ConPTY, ostrzeżenia | [microsoft/node-pty](https://github.com/microsoft/node-pty) | 2026-08-03 | 2026-10-12 | wysoka |
| [52] | Tauri: Node tylko jako sidecar | [Tauri docs: sidecar-nodejs](https://v2.tauri.app/learn/sidecar-nodejs/) | 2026-09-21 | 2026-10-12 | wysoka |
| [53] | Emdash: Electron, node-pty, worktree przez git, ryzyka PTY | [generalaction/emdash](https://github.com/generalaction/emdash) | 2026-10-10 | 2026-10-12 | wysoka |
| [54] | Crystal: v0.1 w 7 dni, jeden autor, wycofany | [stravu/crystal](https://github.com/stravu/crystal) | 2026-02-26 | 2026-10-12 | wysoka |
| [55] | Zamknięcie firmy Vibe Kanban | [vibekanban.com/blog/shutdown](https://www.vibekanban.com/blog/shutdown) | 2026-04-10 | 2026-10-12 | wysoka |
| [56] | Vibe Kanban: multi-repo, lokalny MCP, ostatnie commity | [BloopAI/vibe-kanban](https://github.com/BloopAI/vibe-kanban) | 2026-09-19 | 2026-10-12 | wysoka |
| [57] | Pane: AGPL, Windows, CLI + MCP | [greenfield-inc/Pane](https://github.com/greenfield-inc/Pane) | 2026-10-11 | 2026-10-12 | średnia |
| [58] | Superset: ELv2, brak Windows | [superset-sh/superset](https://github.com/superset-sh/superset) | 2026-10-08 | 2026-10-12 | wysoka |
| [59] | ivar: multi-repo worktree, tylko macOS i Linux | [mnzsss/ivar](https://github.com/mnzsss/ivar) | 2026-10-08 | 2026-10-12 | wysoka |
| [60] | Conductor: tylko Mac | [conductor.build docs](https://www.conductor.build/docs/) | b.d. | 2026-10-12 | średnia |
| [61] | Flagi Claude Code CLI (MCP, add-dir, session-id, settings, plugin-dir) | [Anthropic CLI reference](https://code.claude.com/docs/en/cli-reference) | b.d. (dok. żywa) | 2026-10-12 | wysoka |
| [62] | 33 zdarzenia hooków, typy, notification_type, transcript_path | [Anthropic hooks](https://code.claude.com/docs/en/hooks) | b.d. (dok. żywa) | 2026-10-12 | wysoka |
| [63] | OPENCODE_CONFIG_CONTENT, scalanie konfiguracji | [OpenCode docs: config](https://opencode.ai/docs/config/) | 2026-10-12 | 2026-10-12 | wysoka |
| [64] | Zdarzenia pluginów OpenCode | [OpenCode docs: plugins](https://opencode.ai/docs/plugins/) | 2026-10-12 | 2026-10-12 | wysoka |
| [65] | Status agentów przez hooki w produkcji (Superset), ograniczenia | [Superset docs: agent-status](https://docs.superset.sh/agent-status) | b.d. | 2026-10-12 | wysoka |
| [66] | Zmiany w hookach między wersjami, OSC 7501 | [anthropics/claude-code CHANGELOG](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md) | 2026-10 | 2026-10-12 | wysoka |
| [67] | Błędy ConPTY w Claude Code (resize, mysz, Shift+Enter) | [anthropics/claude-code#80123](https://github.com/anthropics/claude-code/issues/80123) | 2026-07..10 | 2026-10-12 | średnia |
| [68] | ConPTY gubi lub przestawia sekwencje (historia) | [Warp blog](https://www.warp.dev/blog/building-warp-on-windows) | 2025-01-22 | 2026-10-12 | średnia (historia) |
| [69] | Rozmiar przykładowej funkcji domenowej (~50 plików) | [t3code commit 1f6df6b](https://github.com/pingdotgg/t3code/commit/1f6df6b) | 2026-10-11 | 2026-10-12 | wysoka |
| [70] | Osobny rejestr migracji forka (precedens) | [TrogonStack/t3code#13](https://github.com/TrogonStack/t3code/pull/13) | 2026-07-31 | 2026-10-12 | wysoka |
| [71] | Maintainer: brak zmian w orkiestracji przed V2; zaproszenie do nowego PR | [pingdotgg/t3code#11251](https://github.com/pingdotgg/t3code/pull/11251) | 2026-09-19 | 2026-10-12 | wysoka |
| [72] | V2 odrzuca stare klienty mobilne | [SmartScope](https://smartscope.blog/en/blog/t3-code-client-not-supported-mobile-v2-2026/) | 2026-10-03 | 2026-10-12 | średnia |
| [73] | Serwery MCP użytkownika działają w sesjach Claude w T3 | [pingdotgg/t3code#6632](https://github.com/pingdotgg/t3code/issues/6632) | 2026-08-14 | 2026-10-12 | średnia (zgłoszenie użytkownika) |
| [74] | T3 używa logowania i konfiguracji Claude Code | [t3code docs/user/providers-claude.md](https://github.com/pingdotgg/t3code/blob/0e7346c/docs/user/providers-claude.md) | 2026-10 | 2026-10-12 | wysoka |
| [75] | OpenCode: wymagane wersje, „Limited support” dla 1.x | [t3code docs/user/providers-opencode.md](https://github.com/pingdotgg/t3code/blob/0e7346c/docs/user/providers-opencode.md) | 2026-10 | 2026-10-12 | wysoka |
| [76] | `Migrator.make({ table })` w Effect | [t3code .repos/effect-smol Migrator.ts](https://github.com/pingdotgg/t3code/tree/0e7346c/.repos) | 2026-10 | 2026-10-12 | średnia (kopia dołączona, wersja niezweryfikowana) |
| [77] | `claude` w PTY na Windows/ConPTY: ucinanie i dzielenie wiadomości | [nimbalyst/nimbalyst#987](https://github.com/nimbalyst/nimbalyst/issues/987) | 2026-07-25 | 2026-10-12 | wysoka (jeden projekt) |
| [78] | Osierocone drzewa `claude.exe`, pętla awarii renderera w trybie CLI | [nimbalyst/nimbalyst#903](https://github.com/nimbalyst/nimbalyst/issues/903) | 2026-07-17 | 2026-10-12 | średnia |
| [79] | Shift+Enter nieodróżnialny od Enter na Windows (Electron + xterm + ConPTY) | [anthropics/claude-code#92771](https://github.com/anthropics/claude-code/issues/92771) | 2026-09-08 | 2026-10-12 | wysoka |
| [80] | Brak wiarygodnego sygnału stanu sesji; „months battling detection bugs”; NOT_PLANNED | [anthropics/claude-code#38184](https://github.com/anthropics/claude-code/issues/38184) | 2026-03-24 | 2026-10-12 | wysoka |
| [81] | System pluginów T3 w recenzji (stos PR, niescalony) | [pingdotgg/t3code#16047](https://github.com/pingdotgg/t3code/pull/16047) | 2026-10-05 | 2026-10-12 | średnia (niescalone) |
| [82] | Fork t3kanban: tablica zadań wydana w dniu utworzenia repo (2026-08-31), potem 1 650 commitów z tyłu | [what1f/t3kanban](https://github.com/what1f/t3kanban) | 2026-09-10 | 2026-10-12 | wysoka |
| [83] | Fork mtcode: 1 492 do przodu, 6 z tyłu, merge'e paczkami | [munimtechnologies/mtcode](https://github.com/munimtechnologies/mtcode) | 2026-10-12 | 2026-10-12 | średnia |
| [84] | Narzędzia MCP nie zawsze docierają do modelu/subagentów w Claude Code | [anthropics/claude-code#30280](https://github.com/anthropics/claude-code/issues/30280) | 2026-02..04 | 2026-10-12 | średnia |
| [85] | opcode (Claudia): brak commitów kodu od 2025-10 (historia) | [winfunc/opcode](https://github.com/winfunc/opcode) | 2025-10-13 | 2026-10-12 | średnia |
| [86] | Zewnętrzna aplikacja sterująca T3: każdy wątek wymaga projektu | [pingdotgg/t3code#17894](https://github.com/pingdotgg/t3code/issues/17894) | 2026-10-10 | 2026-10-12 | średnia |
| [87] | Deep linki `t3code://` nie działały od 0.0.38 do 2026-10-12 | [pingdotgg/t3code#9745](https://github.com/pingdotgg/t3code/issues/9745) | 2026-09-04 | 2026-10-12 | wysoka |
| [88] | OAuth dla zewnętrznych agentów scalony 2026-10-06; ostatnie stabilne v0.0.45 (2026-10-02) | [pingdotgg/t3code#16336](https://github.com/pingdotgg/t3code/pull/16336) | 2026-10-06 | 2026-10-12 | wysoka |
| [89] | Klient OAuth bez nagłówka MCP-Protocol-Version dostawał 400 (naprawione 10-09) | [pingdotgg/t3code#16906](https://github.com/pingdotgg/t3code/issues/16906) | 2026-10-07 | 2026-10-12 | wysoka |
| [90] | Okno usuwania wątku bez sprawdzenia katalogu zarządzanego; cleanup automatyczny omija obce worktree | [t3code apps/web/src/worktreeCleanup.ts](https://github.com/pingdotgg/t3code/blob/main/apps/web/src/worktreeCleanup.ts) | 2026-10-12 | 2026-10-12 | średnia (odczyt kodu red teamu) |
| [91] | `t3_thread_launch` bez klucza ponowienia; PR otwarty | [pingdotgg/t3code#16654](https://github.com/pingdotgg/t3code/pull/16654) | 2026-10-07 | 2026-10-12 | wysoka |
| [92] | RFC: zewnętrzne aplikacje renderowane wewnątrz T3 | [pingdotgg/t3code#6419](https://github.com/pingdotgg/t3code/issues/6419) | 2026-08-13 | 2026-10-12 | średnia |

## Mapa nieaktualności

Raport opisuje stan upstreamu z 2026-10-12 (commit `0e7346c`). Upstream dostaje ok. 50–80 commitów dziennie, więc twierdzenia o kodzie T3 Code starzeją się w tygodniach, nie w miesiącach.

**Dwie reguły odświeżania:**

- **Ponowne sprawdzenie interfejsu zewnętrznego T3** (twierdzenia o kodzie): do 2026-11-06 albo natychmiast, gdy T3 scali system pluginów [81].
- **Odświeżenie całego raportu (Refresh):** raport wyboru starszy niż dwa kwartały trzeba odświeżyć przed działaniem, czyli przed decyzją podjętą po 2027-04-12.

Mapę wyliczył `recon_kit.py staleness`. Okna w miesiącach: kod i architektura upstreamu (code-architecture) 1, wersje (version) 1, sygnały ekosystemu (velocity) 6, licencja (license) 12, warunki dostawców (terms) 3, krajobraz AI (landscape) 3, wzorce (pattern) 24.

| Twierdzenie | Klasa | Publ. | Ponowne sprawdzenie |
|---|---|---|---|
| [14][15] /mcp dla zewnętrznych agentów, OAuth 30 dni | code-architecture | 2026-10-06 | **2026-11-06** |
| [9] /mcp wstrzykiwany do sesji, własny toolkit | code-architecture | 2026-10-09 | 2026-11-09 |
| [45] Agent SDK 0.3.296 | version | 2026-10-09 | 2026-11-09 |
| [3] wątek = 1 projekt, ≤1 worktree | code-architecture | 2026-10-10 | 2026-11-10 |
| [47] OpenCode 1.19 / przejście na v2 | version | 2026-10-11 | 2026-11-11 |
| [16] `existing_worktree` poza katalogiem T3 | code-architecture | 2026-10-12 | 2026-11-12 |
| [61][62] flagi i hooki Claude Code | version | 2026-10-12 | 2026-11-12 |
| [55] Vibe Kanban bez firmy | landscape | 2026-09-19 | 2026-12-19 |
| [44] wstrzymana zmiana rozliczeń SDK | terms | 2026-10-07 | 2027-01-07 |
| [58] Superset ELv2, brak Windows | landscape | 2026-10-08 | 2027-01-08 |
| [41] warunki Anthropic: CLI z subskrypcją | terms | 2026-10-12 | 2027-01-12 |
| [18] licencja MIT | license | 2026-03-05 | 2027-03-05 |
| [22] tempo upstreamu | velocity | 2026-10-12 | 2027-04-12 |
| [29][30] multi-repo niezaimplementowane | velocity | 2026-10-12 | 2027-04-12 |
| [33][34] Windows poza CI, #14990 | velocity | 2026-10-12 | 2027-04-12 |
| [19] autorzy popierają forki | license | 2026-07-28 | 2027-07-28 |
| [54] Crystal | pattern | 2026-02-26 | 2028-02-26 |
| [28] wzorzec forka plug-in | pattern | 2026-10-12 | 2028-10-12 |

**Najwcześniejsze ponowne sprawdzenie: 2026-11-06.** Dotyczy interfejsu `/mcp` dla zewnętrznych agentów w T3. Jeśli wybierasz C, sprawdź go przed spike'iem S3.

# TUI dla użytkownika — plan implementacyjny

Status: gotowy plan do implementacji; ten dokument nie oznacza wykonania backlogu.
Data projektu: 2026-09-15. Baza analizy: commit `58f4cc29c0d8a62d40187c3587b2aa7824752f01`.
Język interfejsu: angielski, zgodnie z istniejącym CLI. Dokumentacja dla implementera: polski.

## 1. Polecenie dla implementera

Zaimplementuj cały zakres tego dokumentu, etapami z sekcji 13. Przed pracą przeczytaj
AGENTS.md oraz wskazane dokumenty i testy. Zachowaj zastane zmiany. Nie realizuj innych
pozycji TODO.md. Nie publikuj zmian i nie wywołuj prawdziwych klientów modeli, trackera
ani forge podczas weryfikacji. Nie zmieniaj modelu Task/Session/Run dla wygody interfejsu.

Ten dokument rozstrzyga produkt, nawigację, architekturę i zachowanie recovery.
Nazwy nowych plików i API są docelowym podziałem odpowiedzialności; drobna korekta nazwy
jest dopuszczalna, jeśli koliduje z istniejącym symbolem. Nie pomijaj żadnego etapu
ani kryterium akceptacji. Na końcu podaj wykonane testy i konkretne ograniczenia.
Nie oznaczaj pozycji TODO jako ukończonej, jeśli integracja tmux nie została zweryfikowana.

## 2. Decyzje i zakres

1. Nowa jawna komenda `workspace tui` uruchamia interfejs. Samo `workspace`, `open`,
   `attach`, `menu`, istniejące komendy i formaty odpowiedzi zachowują swój kontrakt.
2. W katalogu projektu pokazujemy wybór workspace. W workspace lub jego potomku —
   dashboard tego workspace. Projekt z jednym workspace nadal pokazuje wybór.
3. Główna oś nawigacji: **Workspace → Tasks → Task → Sessions / Results**.
   Worktrees to równoległa perspektywa infrastruktury i przejście do tych samych encji.
4. Dashboard pokazuje stan, liczniki, orkiestratora i maksymalnie pięć problemów.
   Nie pokazuje wszystkich agentów, sesji, tasków ani worktrees.
5. TUI korzysta bezpośrednio z typowanego core. Nie uruchamia komend `workspace ...`
   jako subprocessów w celu pobierania JSON ani wykonania operacji domenowych.
6. Jeden zarządzany panel TUI w oknie orkiestratora. Supervisor odtwarza utracony
   panel. TUI nie jest Agent, Session, Run ani BackgroundService.
7. Automatyczny panel pozostaje przydatny w paused i po zakończeniu procesów.
   Nie blokuje archive/clean, nie zajmuje writer lease, limitów modeli ani tasków.
8. Stan procesu, zaakceptowanie wyniku i faza workflow to osobne informacje.
9. Interfejs działa przy 40×12 znaków; obsługuje szerszy niski panel i wąski wysoki.
   Przy mniejszych rozmiarach pokazuje bezpieczny ekran minimalny i możliwość wyjścia.
10. V1 obejmuje przegląd i operacje wymienione w sekcji 7. Nie implementuje całego CLI
    w formularzach. Tworzenie tasków/person/worktrees, publikacja CR, decyzje live test,
    release, akceptacja handoffów, migracje i usuwanie danych pozostają w istniejącym CLI.
    Stan tych procesów i decyzje oczekujące są widoczne w TUI.

Poza zakresem: edytor kodu, terminal w terminalu, przechwytywanie klawiszy klienta
agenta, dashboard kosztów/tokenów, zdalne workspace, osobny daemon UI, globalny fuzzy
search po treści wszystkich plików, drag-and-drop, GUI/web, automatyczne decyzje workflow.

### 2.1. Stack i wersje

Zachowaj `go 1.24.0`. Użyj jednej rodziny API v1:

| Moduł | Wersja | Rola |
|---|---|---|
| `github.com/charmbracelet/bubbletea` | `v1.3.10` | Model/Update/View, event loop, terminal |
| `github.com/charmbracelet/bubbles` | `v0.21.0` | list, viewport, textinput, help, key, spinner |
| `github.com/charmbracelet/lipgloss` | `v1.1.0` | layout i style |
| `github.com/charmbracelet/huh` | `v0.7.0` | osadzone formularze i potwierdzenia |
| `github.com/charmbracelet/x/ansi` | `v0.10.1` | szerokość, zawijanie i skracanie ANSI-aware |
| `github.com/charmbracelet/x/term` | `v0.2.1` | wykrywanie terminala |

To świadomy wybór zgodności z obecnym minimum Go. Zweryfikowano pliki go.mod tych
wydań: Bubble Tea wymaga Go 1.24; Bubbles i Huh — Go 1.23. Huh 0.7.0 zależy od
Bubbles 0.21.0. Huh 0.8.0 wprowadza pseudowersję Bubbles; nie wybieraj go przypadkiem.
Aktualne gałęzie main Bubble Tea/Huh używają API v2 i nowszego Go. Nie kopiuj przykładów
z main do implementacji v1. Nie używaj `@latest`. Po `go mod tidy` sprawdź cały graf
modułów z `GOTOOLCHAIN=local` i Go 1.24; nie akceptuj cichego podniesienia minimum.

Glamour: **nie dodawaj w tym zakresie**. Podgląd Markdown to zawijany tekst w viewport,
z zachowaniem nagłówków i bloków kodu. To kompletny podgląd V1. Renderowanie Markdown
może być kolejną zmianą; nie jest warunkiem ukończenia tej pozycji.

Źródła wersji i API:

- [Bubble Tea 1.3.10 — go.mod](https://github.com/charmbracelet/bubbletea/blob/v1.3.10/go.mod)
- [Bubbles 0.21.0 — go.mod](https://github.com/charmbracelet/bubbles/blob/v0.21.0/go.mod)
- [Lip Gloss 1.1.0 — go.mod](https://github.com/charmbracelet/lipgloss/blob/v1.1.0/go.mod)
- [Huh 0.7.0 — go.mod](https://github.com/charmbracelet/huh/blob/v0.7.0/go.mod)
- [Bubble Tea — oddawanie terminala przez Exec](https://github.com/charmbracelet/bubbletea/blob/v1.3.10/exec.go)
- [Bubble Tea main — go.mod](https://github.com/charmbracelet/bubbletea/blob/main/go.mod)
- [Huh main — go.mod](https://github.com/charmbracelet/huh/blob/main/go.mod)
- [tmux — dokumentacja referencyjna](https://man.openbsd.org/tmux.1)

## 3. Co istnieje i co rzeczywiście trzeba wydzielić

Przeczytaj [PRODUCT.md](../../PRODUCT.md), [ARCHITECTURE.md](../../ARCHITECTURE.md),
[README.md](../../README.md), [runtime](../runtime.md), [operacje](../operations.md).
Przy szczegółach historycznych wyników przeczytaj też [rewizje](../revisions.md),
[checks](../checks.md) i [klientów](../clients.md).

| Obecny kod | Znaczenie dla implementacji |
|---|---|
| `internal/core/model.go` | Workspace, Registry, Status, Session i Run; `Status()` synchronizuje projekcje sesji |
| `internal/core/workflow_model.go` | Task wskazuje bieżący worktree/session/run; artefakty i handoffy mają pochodzenie |
| `internal/core/project.go` | Service, DiscoverProject, InferWorkspace, With, List, Status; blokada projektu |
| `internal/core/files.go` | loadDocument, recovery zapisu, atomowy zapis, contained; nie omijać przy odczycie |
| `internal/core/session.go` | start/stop/resume/reconcile, lineage i runtime ownership |
| `internal/core/runtime.go` | Tmux, Launch, Recover, Inspect, Attach, shellQuote |
| `internal/core/supervisor.go` | EnsureSupervisor, Tick, tickWorkspace, recovery orkiestratora |
| `internal/core/actor.go` | pusty Actor = użytkownik; role i stale_actor egzekwuje core |
| `internal/core/lifecycle.go` | pause/interrupt, archive i clean — TUI nie może blokować tych kontraktów |
| `internal/cli/cli.go` | bootstrap z env/flag/CWD, wybór workspace, resume i attach częściowo realizowane w adapterze |
| `internal/cli/help.go`, `cli_test.go` | kompletność pomocy i flag dla każdej widocznej komendy |

Core już istnieje. Nie wprowadzaj repozytoriów SQL, event busa, generycznego CQRS ani
masowego przenoszenia plików core do nowych pakietów. Wydziel konkretne fragmenty
z CLI, których potrzebuje TUI, i dodaj spójny odczyt agregatu.

### 3.1. Relacje agregatu

```text
Project
└─ Workspace
   ├─ Task (bieżąca Attempt, zależności, wynik zaakceptowany)
   │  ├─ Sessions po TaskID (bieżąca i historyczne próby)
   │  │  └─ Runs po SessionID
   │  └─ Results: Handoffs / Artifacts / Checks
   ├─ Worktree (checkout, branch, purpose, lifecycle)
   │  ├─ Tasks: bieżące Task.WorktreeID + historia po Session.WorktreeID
   │  ├─ Sessions → Agent persona / Runs
   │  └─ BackgroundServices
   ├─ Orchestrator: Agent → Sessions → Runs; WorktreeID pusty
   └─ Workflow / Decisions / Integration / ChangeRequests / LiveTest / Release
```

Okno `orchestrator` nie jest checkoutem Git i nie ma rekordu Worktree. Nie twórz
sztucznego worktree. W nawigacji Worktrees dodaj osobny wiersz `Orchestrator · workspace
directory`, wyraźnie oddzielony od checkoutów; otwiera tę samą stronę co kafel dashboardu.

Jedna persona może mieć wiele sesji w historii. Sesje mogą nie mieć TaskID. Nie zakładaj
relacji jeden task = jeden agent = jeden worktree. Indeksy buduj po ID, nie po nazwach.
Do nazw historycznej sesji używaj AgentSnapshot, a bieżącą definicję persony pokazuj
osobno. Historyczny Run nie jest aktualnym właścicielem panelu.

## 4. Wejście, scope i zgodność CLI

### 4.1. Komendy

```sh
workspace tui
workspace tui --project /repo
workspace tui --workspace ws_ID
workspace tui --theme dark
workspace tui --theme light
workspace tui --theme auto
workspace tui --no-color
workspace tui show --workspace ws_ID
workspace tui hide --workspace ws_ID
workspace tui status --workspace ws_ID --json
```

`tui show/hide/status` dotyczą **zarządzanego panelu**, nie ręcznych instancji.
`show` ustawia desired=true i próbuje odtworzyć panel przy istniejącej sesji tmux;
nie startuje orkiestratora. Bez sesji zapisuje preferencję i zwraca `waiting_for_runtime`.
`hide` ustawia desired=false i usuwa tylko zweryfikowany własny panel.
`status` pokazuje desired, stan, pane/window ID, last_error i next_retry_at.
Te trzy polecenia są normalnym CLI i obsługują JSON/short/non-interactive.
Mutacje show/hide obsługują operation-key według sekcji 10.3.

Właściwy `workspace tui` wymaga terminalowego stdin **i** stdout. Przy pipe, TERM=dumb,
`--json`, `--short` lub `--non-interactive` zwraca `interactive_required` bez wejścia
w raw mode i bez uruchomienia tmux/supervisora. JSON błędu zachowuje standard CLI.
`--operation-key` dla samej pętli TUI odrzuć jako `invalid_option`: każda akcja ma swój
klucz. Używaj wstrzykiwanych io.Reader/io.Writer, a nie ukrytego otwierania `/dev/tty`.

### 4.2. Rozpoznanie lokalizacji

- Zachowaj istniejące pierwszeństwo dla ręcznego uruchomienia: flagi → WORKSPACE_*
  → CWD. TUI pokazuje nazwę i ścieżkę rozstrzygniętego projektu/workspace w nagłówku.
- Wspólny bootstrap tworzy Service i Actor tak samo jak obecne options.service().
- InferWorkspace: `workspace_required` oznacza ekran projektu; inne błędy, np.
  uszkodzony frontmatter lub odmowa dostępu, pokaż jako błąd, nie jako brak workspace.
- Flaga --workspace w V1 przyjmuje ID, zgodnie z globalną flagą CLI. Wybór po nazwie
  odbywa się w liście. Nie dodawaj innego resolvera nazw z innymi zasadami niejednoznaczności.
- Zweryfikuj, że wskazane/inferowane ID należy do wybranego projektu. Nie przeskakuj
  między projektami na podstawie odziedziczonego obcego WORKSPACE_ID.
- Normalizuj CWD przez Abs/EvalSymlinks, użyj istniejących reguł workspaces_dir.
- W checkoutach wewnątrz workspace działa przeszukiwanie rodziców. Dla zarejestrowanego
  checkoutu poza tą hierarchią dodaj dopasowanie kanonicznego CWD do Worktree.Path
  danego projektu, tylko jeśli zwykłe InferWorkspace nie znalazło workspace. Wybierz
  najdłuższą pasującą ścieżkę; różne workspace z tym samym dopasowaniem to konflikt.
  Nie traktuj katalogu projektu jako checkoutu workspace.
- Brak projektu: instrukcja `workspace project init`; bez automatycznej inicjalizacji.
- Brak workspace: ekran pusty z poleceniem create, bez automatycznego tworzenia.

## 5. Nawigacja i zawartość ekranów

### 5.1. Ekran projektu

Nagłówek projektu, filtr `/`, lista workspace: title, status, phase, liczba aktywnych
runów, liczba problemów, krótki ID. Szczegóły zaznaczenia: pełny ID, ścieżka, źródło
inputu, data utworzenia. `Enter` otwiera dashboard, `a` wybiera akcję (m.in. attach).
Domyślne sortowanie: workspace niearchiwalne przed archiwalnymi, potem CreatedAt
malejąco, ID jako tie-breaker. Archiwalne pozostają dostępne przez filtr statusu.
Nie zmieniaj automatycznie kolejności przy każdym heartbeat; odśwież dane zaznaczenia
po ID i sortuj po zmianie filtrów lub jawnym `r`.

### 5.2. Dashboard workspace

Zawsze obecne: breadcrumb, title/ID, osobno status i phase, czas ostatniego udanego
odczytu, wskaźnik błędu/nieaktualnych danych. Treść to cztery panele:

1. **Overview**: tasks accepted/total, running, blocked/needs_changes; active runs;
   ready worktrees; usługi aktywne. To liczniki z rekordów, nie lista encji.
2. **Orchestrator**: persona, lifecycle sesji, stan bieżącego/ostatniego Run, model,
   przycisk `Jump` albo `Start/Resume` w menu akcji.
3. **Needs attention**: PendingDecision; blocked/needs_changes tasks; niezaakceptowane
   aktualne handoffy; failed/interrupted bieżące/ostatnie wykonania; problem supervisora,
   runtime lub UI. Maks. pięć wierszy, potem `View all (N)`.
4. **Activity**: maks. pięć ostatnich zdarzeń WYPROJEKTOWANYCH z dostępnych dat Run,
   Handoff, Artifact i odpowiedzi Decision. Etykieta `Recent recorded activity`;
   to nie pełny audit log ani nowa trwała tabela zdarzeń.

Panel uwagi deduplikuje problem task/run do jednego wiersza z linkami. Stan stale
handoff ma oddzielną etykietę, nie proponuje akceptacji. Priorytet: wymagana decyzja,
awaria odczytu/runtime, blocked/needs_changes, aktualny handoff do review, reszta;
w obrębie priorytetu stabilnie po ID. Widok pełny ma search.

Nawigacja główna: `1 Overview`, `2 Tasks`, `3 Worktrees`, `4 Results`, `5 More`.
More zawiera Sessions (cały workspace), Agents, Services, Decisions, Change requests
i Runtime. Są to jawnie wybierane strony — żadnych rozwiniętych list na dashboardzie.
`w` otwiera selektor workspace w bieżącym projekcie, `o` stronę orkiestratora.

### 5.3. Task

Lista: nazwa/title, Task.State, attempt, badge aktywności Run, bieżący worktree.
Szczegóły: goal, role/profile, acceptance criteria, required artifacts/checks,
zależności z linkami, reason i accepted handoff. Sekcje otwierane Enter:

- Sessions: domyślnie bieżąca próba; przełącznik `History` pokazuje poprzednie
  próby/input lineage. Każda sesja pokazuje agenta, model, lifecycle i Run state.
- Worktrees: bieżący oraz historyczne powiązane przez sesje, z etykietą history.
- Results: Handoffs, Artifacts i Checks tylko tego taska, z filtrami prób/history.

Nie przypisuj artefaktu do bieżącej próby jedynie po TaskID. Połącz go z Run/Session
i SourceHandoff, a stary/nieustalony lineage wyświetl jako history/unknown.
Task.accepted pozostaje zaakceptowany, nawet gdy stare wykonanie zakończyło się błędem.

### 5.4. Worktree i orkiestrator

Worktree: path, branch, purpose, lifecycle, writer/readers/active services, powiązane
taski i sesje. Rozdziel `ready` (checkout istnieje według rejestru) od `active` (ma
aktywny Run). Git dirty/HEAD badaj dopiero na tej stronie, w osobnym zapytaniu,
z timeoutem; nie uruchamiaj git status dla każdego checkoutu co dwie sekundy.
Wynik git jest obserwacją z własnym timestampem, nie zmianą Worktree.State.

Strona orkiestratora: bieżąca sesja + historia, katalog workspace, nadzorowany panel
TUI, link do okna tmux i akcje. Brak udawanego branch/WorktreeID.

### 5.5. Session, Run, Agent, Service

- Session: AgentSnapshot, lineage TaskID/attempt/WorktreeID, lifecycle,
  CurrentRunID/LastRunID, read-only, native thread jeśli jest; Runs w historii.
- Run: dokładny stan, model/provider/client, czas, kod wyjścia i error, pochodzenie.
  `Jump` tylko dla aktualnego, żywego, zweryfikowanego panelu. Stare wykonanie nie
  przeskakuje po cichu do nowego Run; osobny link `Current session` jest dozwolony.
- Agent: definicja i lista sesji; brak wymyślonego trwałego Agent.State.
- Service: stan/exit code, worktree, argv jako tekst, jump i stop w menu akcji.

### 5.6. Results i dokumenty

Results ma zakładki Artifacts/Handoffs/Checks, wspólny filtr po nazwie/ID i opcjonalny
filtr taska. Artefakt: metadane pochodzenia, digest, rozmiar, commit i podgląd.
Handoff: outcome, state, stale, summary, feedback, risks, linki do artefaktów/checks.
Check: state, exit_code, SHA, argv i podgląd outputu. `completed` check z exit != 0
nie jest sukcesem; pokaż exit code niezależnie od stanu.

Dokumenty WORKSPACE.md/WORKFLOW.md/input snapshot dostępne z Overview → Actions →
View documents. Preview tylko lokalnie, bez otwierania URL z treści. Pliki binarne
pokazują metadane. Limit tekstowego podglądu 256 KiB, oznaczenie ucięcia; nie odczytuj
całego dużego pliku przed przycięciem. Przewijanie i search w widocznej liście to
osobne mechanizmy; wyszukiwanie pełnej treści plików nie jest wymagane.

### 5.7. Klawiatura i routing zdarzeń

| Klawisz | Działanie |
|---|---|
| Up/Down, j/k | lista lub przewijanie aktywnego panelu |
| Enter | otwarcie zaznaczenia; nigdy automatyczny start/stop |
| Esc | formularz → anuluj; edycja filtra → wróć; aktywny filtr → wyczyść; inaczej poprzedni ekran |
| Tab / Shift+Tab | następny/poprzedni panel; w formularzu pola formularza |
| 1–5 | główne strony, tylko poza edycją tekstu |
| / | edycja filtra aktualnej kolekcji |
| f | menu status/history filtrów, tylko na stronach kolekcji |
| a | menu akcji zaznaczenia/bieżącej strony |
| g | Jump: sesja tmux / window / pane według zaznaczenia |
| w / o | wybór workspace / orkiestrator |
| r | odśwież odczyt; nie wykonuje reconcile |
| ? | pomoc ze skrótami dostępnymi w tym kontekście |
| q / Ctrl+C | wyjdź z ręcznej instancji; ukryj zarządzany panel (sekcja 10) |
| PgUp/PgDn, Home/End | lista albo viewport |

Pole tekstowe otrzymuje litery q, g, r, cyfry i spację; nie są wtedy skrótami aplikacji.
Ctrl+C w formularzu anuluje formularz, dopiero poza nim wychodzi/ukrywa panel.
Nie nadpisuj globalnych bindings tmux ani prefiksu użytkownika.

Filtr: case-insensitive substring po title/name i ID; sesja dodatkowo po nazwie
AgentSnapshot, Run po ID/modelu, worktree po name/branch. Znormalizuj wielkość liter,
nie stosuj regex, nie filtruj ukrytych kolekcji. Pokazuj `N / total` i treść filtra.
Search działa lokalnie na snapshot, bez I/O per klawisz. Pusty wynik ma jasny komunikat
i `Esc clear`. Zapamiętuj query, ID zaznaczenia i scroll per route podczas jednej sesji TUI.

## 6. Layout i theme

Użyj Lip Gloss, bez emoji i bez wymogu Nerd Fonts. Ikona + tekst, np. `[>] running`,
`[x] failed`, `[!] blocked`, `[+] accepted`, `[-] idle`, `[#] stopped`.
Ikony są ASCII i mają stałą szerokość; kolor jest dodatkowym kanałem informacji.
Ramki mogą być Unicode, z wariantem ASCII przy braku odpowiedniego locale.

Theme jako semantyczne tokeny: background, surface, text, muted, border, focus,
info, success, warning, danger, selection. Dark: tło terminala, surface #1E293B,
text #E2E8F0, muted #94A3B8, border #475569, focus/info #67E8F9, success #86EFAC,
warning #FDE68A, danger #FDA4AF. Light: tło terminala, surface #F1F5F9, text #0F172A,
muted #475569, border #94A3B8, focus/info #0E7490, success #166534,
warning #92400E, danger #BE123C. Aktywny panel ma wyraźny border i znacznik `>`.
Wariant ANSI-16 dobierz po rolach; no-color nie emituje sekwencji kolorów.

`--theme auto|dark|light`, domyślnie auto; `--no-color` oraz niepuste NO_COLOR mają
pierwszeństwo. Użyj jednego renderera/palety na instancję, bez mutowania globalnych
stylów w testach. Auto korzysta z możliwości terminala; przy braku detekcji wybiera
dark z przezroczystym tłem. Huh otrzymuje tę samą paletę, nie własny kontrastowy theme.

Reguły wymiarów (kolumny × wiersze, wymiary faktycznego panelu):

- **Wide**: width >= 100 i height >= 24. Dashboard 2×2; kolekcje lista 40%, szczegóły
  60% z minimalnymi szerokościami 32/40; główne zakładki u góry.
- **Short**: width >= 80 i 12 <= height < 24. Nagłówek/zakładki/stopka po jednym
  wierszu; panel Overview w dwóch krótkich kolumnach, pozostałe przez Tab.
  Listy mają jeden wiersz na rekord; szczegóły po Enter jako osobna strona.
- **Narrow**: pozostałe width >= 40 i height >= 12. Jedna kolumna, jeden panel treści,
  zwarte zakładki `1 Home 2 Tasks …`; dashboard jako przewijane sekcje.
  Szczegóły zawsze osobną stroną. Brak trwałego lewego sidebara zabierającego szerokość.
- **Tiny**: width < 40 lub height < 12. Nazwa workspace skrócona, `Terminal too small`,
  wskazanie 40×12 i q; tekst przycinany także przy 1×1. Bez ujemnych SetSize.

Na resize przelicz wszystkie komponenty i formularze, zachowaj route/query/focus/ID.
Obliczenia uwzględniają ramki, padding, nagłówek i stopkę. Nie licz szerokości len(bytes).
Długi tytuł/ścieżka nie może wypchnąć statusu poza ekran; pełny tekst w szczegółach.
Formularze na narrow/short pokazują jedno pole/grupę naraz i przewijalne wyjaśnienie.

Przykład wide (treść poglądowa, liczby pochodzą ze snapshotu):

```text
Project / Checkout fix       [>] active  · implementing      updated 2s ago
1 Overview   2 Tasks   3 Worktrees   4 Results   5 More
┌ Overview ──────────────────┐ ┌ Orchestrator ─────────────────────────┐
│ Accepted 3/8 · Running 2   │ │ [>] running · session active           │
│ Blocked 1 · Worktrees 4    │ │ client / provider / model              │
└───────────────────────────┘ └────────────────────────────────────────┘
┌ Needs attention (2) ───────┐ ┌ Recent recorded activity ──────────────┐
│ [!] parser: blocked       │ │ [+] implementation handoff submitted    │
│ [!] pending user decision │ │ [-] planner process exited              │
└───────────────────────────┘ └────────────────────────────────────────┘
Tab panel   Enter open   / filter   a actions   g jump   ? help   q quit
```

Przykład narrow: nagłówek → zwarte zakładki → Overview z licznikami → Orchestrator →
Needs attention, wszystko w jednej przewijanej powierzchni. Stopka pozostaje widoczna.

### 6.1. Statusy: nie mieszaj osi

| Encja | Wartości / sposób prezentacji |
|---|---|
| Workspace | active, needs_workflow, paused, blocked, needs_attention, completed, archived; phase obok |
| Task | pending, running, awaiting_review, accepted, needs_changes, blocked; nieznane jako neutralny surowy tekst |
| Worktree | creating, ready, failed, removing, removed; obok osobno writer/readers/services |
| Session | LifecycleState active/idle/closed; obok current/last Run.State |
| Run | starting, running, exited, failed, stopped, interrupted; exited = proces zakończony, nie accepted |
| Service | starting, running, exited, failed, stopped, interrupted |
| Runtime observation | present, missing, unknown/unavailable; to nie nowy stan Run w rejestrze |

Nie zakładaj, że tabela wyczerpuje przyszłe stany. Testuj nieznaną wartość. Licznik
aktywnych runów bazuje na `Run.Active()` i zgodności Session.CurrentRunID; przy błędzie
tmux oznacz go jako stan zapisany/niezweryfikowany. Nie zmieniaj runu na interrupted
na podstawie timeoutu w samym TUI. To obowiązek core.Reconcile/supervisora.

## 7. Operacje V1 i ich kontrakty

Akcje wyświetlają nazwę, target i powód niedostępności. Core zawsze waliduje uprawnienia
i stan ponownie. Nie traktuj menu jako autoryzacji. Mapowanie ID akcji na metodę jest
statyczne; nie wykonuj stringów MenuAction.Command jako shell.

| Ekran / akcja | Core / zachowanie | Formularz |
|---|---|---|
| Workspace: Start orchestrator | nowy StartSupervisedOrchestrator → istniejący StartOrchestrator | potwierdzenie startu klienta, workspace i profil |
| Workspace: Pause | Pause(..., false, key) | krótki confirm, aktywne runy pozostają |
| Workspace: Pause and interrupt | Pause(..., true, key), z guardem listy runów/usług | potwierdzenie z dokładną listą zatrzymywanych wykonań |
| Workspace: Resume workspace | SetPaused(..., false, key) | confirm; nie obiecuje wznowienia procesów |
| Workspace: Reconcile | Reconcile(..., key) + uzgodnienie UI po zwolnieniu locka | confirm; jawna operacja, nie odświeżenie |
| Workspace needs_workflow | SelectWorkflow(..., name, key) | wybór z WorkflowNames + confirm |
| Task: Retry | RetryTask(..., reason, key), z guardem attempt/revision | reason + lista zależności unieważnianych przez retry |
| Session: Resume | nowy ResumeSession (dokładna wskazana sesja) z supervision | confirm session/task/attempt/worktree/model |
| Session: Stop current run | nowy StopRun, delegujący istniejącą logikę stop | confirm z konkretnym RunID |
| Session: Close | CloseSession(..., reason, key) | idle only, reason + confirm |
| Service: Stop | StopService(..., id, key) | confirm nazwy/worktree i ID |
| Każdy właściwy ekran: Jump | ResolveNavigationTarget → adapter terminala | przy wielu celach selektor |
| Runtime: Show/Hide managed TUI | SetUIPaneDesired | informacja o przywracaniu/ukryciu |

Bez automatycznego ACK inboxa, wysyłania wiadomości, akceptacji wyniku ani zatwierdzania
decyzji. Dla czynności poza V1 pokaż stan i odpowiednie istniejące polecenie CLI jako
tekst. Nie deklaruj przycisku jako działającego, jeśli kończy się tylko placeholderem.

### 7.1. Formy, idempotencja, konkurencja

Huh działa jako zagnieżdżony model Update/View w jednym programie Bubble Tea.
Nie uruchamiaj osobnego Form.Run(), NewProgram ani promptu stdin podczas pracy TUI.
Anulowanie formularza nie wywołuje core. Confirm domyślnie zaznacza Cancel.

Gdy użytkownik otwiera akcję, zapamiętaj target ID, RunID/attempt i payload. Bezpośrednio
przed confirm pokaż świeże dane. Zmiana znaczących pól wymaga ponownego pokazania
formularza. Sam odczyt przed zapisem nie usuwa wyścigu — guard opisany niżej jest
sprawdzany w core pod tą samą blokadą co właściwa mutacja.

- Nowa potwierdzona intencja otrzymuje `tui_<ULID>` (można użyć core.ID("tui")).
- Double Enter nie startuje drugiej operacji; jedna mutacja w toku na instancję TUI.
- Timeout/launch_uncertain: pokaż kod, klucz i `Reconcile / Retry same operation`.
  Nie generuj nowego klucza automatycznie. Retry zachowuje identyczny payload i guard.
- Po sukcesie pobierz snapshot; receipt może przedstawiać wcześniejszy stan.
- Nowy payload = nowa świadoma intencja i nowy klucz.
- Nie powtarzaj mutacji w ticku i nie buduj ogólnego automatycznego retry zmian.
- W zamknięciu programu podczas mutacji anuluj context, zaczekaj na wynik sprzątania
  komendy; nie porzucaj goroutine trzymającej lock. Przy niepewnym wyniku wyświetl klucz
  w komunikacie końcowym/logu i nie twierdź, że anulowanie cofnęło efekt.

Nowy `MutationGuard` dla TUI: ExpectedRevision (opcjonalne), ExpectedRunID,
ExpectedAttempt oraz dla pause-interrupt zbiór aktywnych RunID/ServiceID.
Dodaj typowane warianty guarded dla StopRun, PauseInterrupt i RetryTask. Wspólne
prywatne helpery przeprowadzają walidację i istniejącą operację; bez wywoływania
publicznej metody pod już zajętym s.With i bez zagnieżdżania effect z tym samym kluczem.
Sprawdzenie już istniejącego receipt ma pierwszeństwo przed porównaniem guard ze
stanem bieżącym, po weryfikacji aktora i zgodności payloadu. Guard należy do digestu.
Nieudany guard zwraca `revision_conflict` lub `target_changed`, niczego nie zatrzymuje.
CLI bez guard zachowuje dotychczasowe zachowanie. `StopRun` nie może zatrzymać nowego
Run tej samej Session, jeśli proces zdążył się zmienić podczas formularza.

Dla pause-interrupt nie trzymaj jednego locka przez wywołania publicznych StopSession.
Pod effect-lock operacji: pod project lock sprawdź guard, zapisz paused i trwałą listę
dokładnych RunID/ServiceID do zatrzymania; następnie zwolnij project lock i zatrzymuj
wyłącznie te wykonania przez helpery z własnymi krótkimi lockami. Każdy krok ma stabilny
podklucz z ID wykonania, a ponowienie czyta utrwaloną listę intencji. Proces już
zakończony to ukończony krok; nowy Run nie jest automatycznie dołączany do listy.
Zachowaj dotychczasową kolejność zatrzymania wywołującego aktora jako ostatniego.
Receipt całej operacji powstaje po krokach; przerwana operacja zachowuje intencję.
Wykorzystaj istniejący mechanizm effect/intencji, nie twórz drugiego systemu transakcji.

## 8. Architektura kodu i wspólne API

```text
cmd/workspace → internal/cli (Cobra, flagi, JSON/YAML, uruchomienie TUI)
                         ├→ internal/bootstrap (scope, Service, env)
                         ├→ internal/tui (Bubble Tea, views/forms/theme)
                         └→ internal/terminal (TTY, attach/switch, stdio)
internal/tui ──────────────→ internal/core (typowane odczyty i operacje)
internal/terminal ─────────→ internal/core (zweryfikowane cele runtime)
internal/core ─────────────→ obecne pliki/Git/tmux/klienci/supervisor
```

Zakaz importów cli/tui/bootstrap/Charm/Cobra w core. Core może nadal zawierać obecny
adapter tmux zgodnie z ARCHITECTURE.md. Nie przenoś wszystkich adapterów procesowych.
Terminal nie zna ekranów ani formularzy. TUI nie parsuje frontmatter/index.json.

### 8.1. Nowe i zmieniane pliki

| Plik | Odpowiedzialność |
|---|---|
| `internal/bootstrap/context.go` | flag/env/CWD → Service, Actor, wybrany scope; CLI i TUI |
| `internal/core/query.go` | WorkspaceSnapshot, projektowe podsumowania, spójny odczyt |
| `internal/core/query_relations.go` | indeksy task/worktree/session/run/result, liczniki i uwaga bez stylów |
| `internal/core/preview.go` | bezpieczny ograniczony odczyt dokumentów/artefaktów/check output |
| `internal/core/session_actions.go` | ResumeSession i nadzorowane entry points wydzielone z CLI |
| `internal/core/navigation.go` | referencje encji, weryfikacja i rozwiązanie celu jump |
| `internal/core/ui_runtime.go` | trwała preferencja/instancja panelu i reconciliation |
| `internal/core/tmux_ui.go` | oznaczanie/wyszukiwanie/uruchamianie/odtwarzanie UI i okien |
| `internal/core/runtime.go` | bezpieczny wybór panelu orkiestratora, metadata i topology |
| `internal/core/supervisor.go` | niezależne uzgadnianie UI w Tick |
| `internal/core/lifecycle.go`, `task.go`, `session.go` | helpery dla guarded operations, bez zmiany starych reguł |
| `internal/terminal/navigation.go` | IO oraz attach/switch/jump; fake port do testów |
| `internal/tui/app.go`, `model.go`, `update.go` | Run, stan aplikacji i event routing |
| `internal/tui/backend.go`, `commands.go` | wąskie interfejsy core i asynchroniczne Cmd |
| `internal/tui/routes.go`, `keys.go` | historia nawigacji, focus, skróty |
| `internal/tui/layout.go`, `theme.go`, `status.go` | wymiary, paleta, badge |
| `internal/tui/project.go`, `dashboard.go`, `collection.go`, `detail.go` | prezentacja ekranów |
| `internal/tui/forms.go`, `preview.go` | embedded Huh i viewport |
| `internal/cli/tui.go` | tui/show/hide/status i ukryty runner |
| `internal/cli/cli.go`, `help.go` | rejestracja, odchudzone resume/attach/bootstrap, kompletna pomoc |

Nie dziel na osobny package każdego panelu. Dopisz testy obok plików, wspólne fixtures
TUI w `internal/tui/testdata/`. Nie buduj osobnego binarium TUI.

Do nowych możliwości runtime dodaj nazwane wąskie interfejsy: obserwacja topology,
operacje zarządzanego panelu i nawigacja. Tmux implementuje je, testy UI mają własny
fake z tymi możliwościami. Dotychczasowy Runtime Launch/Inspect/Stop/Attach może
pozostać kompatybilny; brak opcjonalnej możliwości oznacza runtime_unsupported,
a nie wywołanie prawdziwego tmux w testach. Nie twórz zależności core od fake TUI.

### 8.2. Queries i DTO

Dodaj do core.Service:

- `WorkspaceSnapshot(ctx, selector)` → nowy typ zawierający Status, Body, Services,
  Checks, Handoffs, Messages i ObservedAt. Wszystko z jednego s.With/loadDocument,
  bez zagnieżdżonego wywołania Status/Menu/Inbox pod blokadą. Nie zwracaj *Document.
- `ProjectOverview(ctx)` → lista lekkich podsumowań oraz błędy per workspace.
  Nie zmieniaj fail-fast kontraktu istniejącego List. W nowym odczycie jeden uszkodzony
  workspace pozostaje wierszem z path/error, a pozostałe są dostępne. Enumeruj według
  tej samej konfiguracji storageRoot; nie wywołuj fail-fast workspaceDirs w pętli,
  która ma obsłużyć uszkodzony WORKSPACE.md per element.
- `ObserveWorkspaceRuntime(ctx, workspaceID)` → odczyt topology tmux i supervisora,
  z osobnym timestampem; failure nie unieważnia poprawnego snapshotu dokumentu.
- `InspectWorktree(ctx, workspaceID, worktreeID)` → HEAD/dirty/error, tylko na żądanie.
- `ReadPreview(ctx, workspaceID, kind, resourceID)` → metadata, text, truncated,
  binary, warning/error; kind to zamknięty enum document/artifact/check.
- `ResolveNavigationTarget(ctx, workspaceID, EntityRef)` → NavigationTarget.

WorkspaceSnapshot jest prywatnym kontraktem współdzielenia interfejsów wewnątrz
binarium. Nie zastępuje `status --json` i nie podnosi jego schema_version: 2.
Nie dopisuj UI do publicznych list sessions/runs/services. Snapshot nie udostępnia
map Mutations/Operations, tokenu supervisora ani konfiguracji sekretów.

Buduj mapy po ID raz na snapshot. DTO relacji przechowują IDs i flagi history/current,
nie wskaźniki do mutowanego Document. TUI może wykonywać sortowanie, filtrowanie i
formatowanie, ale reguły lineage, aktywności, uprawnień i runtime ownership są w core.
Odczyty korzystają z loadDocument i dotychczasowego recovery/migracji. „Read” nie
oznacza obejścia koniecznego dokończenia pending write; nie wykonuje jednak reconcile
tmux, nowych decyzji ani startu procesów.

### 8.3. Wydzielenie istniejących przypadków użycia

- Nowe `ResumeSession(ctx, selector, sessionOrLegacyRunID, key)` zawiera obecną
  logikę session resume/emitSessionResume: resolve alias, zachowanie read-only,
  parent/profile/task/worktree i ResumeSession; start supervisora poza lockiem,
  potem StartSession. CLI i TUI wywołują tę samą metodę.
- Dodaj `StartSupervisedSession`, `StartSupervisedOrchestrator`,
  `ResumeSupervisedAgent`: EnsureSupervisor, następnie istniejąca metoda core.
  CLI start/session start/agent resume używają tych entry points. Supervisor nadal
  korzysta z prymitywów StartSession/ResumeAgent i nie startuje samego siebie.
- Po udanym starcie orkiestratora spróbuj ReconcileInterface poza s.With. Niepowodzenie
  companion UI zapisz jako UI error i pozostaw do retry; nie zamieniaj udanego Run
  w failed i nie zwracaj błędu sugerującego, że start agenta się nie odbył.
- Także start orkiestratora przez session start i agent/session resume musi włączyć
  companion. Hook po powodzeniu prymitywu StartSession może to scentralizować, ale
  dopiero po zwolnieniu jego locka, wyłącznie gdy wynik dotyczy orkiestratora i bez
  wywoływania EnsureSupervisor z wnętrza tego hooka.
- Move obecne sprawdzanie ownership z CLI session attach do core resolvera.
  Stare attach/open wywołują ten sam adapter terminala co TUI, z zachowaniem ich
  semantyki selektora i formatu błędów. Utrzymaj kompatybilne legacy Run ID.

## 9. Event loop, odczyt i błędy

TUI Model zawiera: scope, route stack, selection ID, queries per route, focus,
snapshot, runtime observation, width/height, palette, form, pending action,
last success/error, request generations i flagi in-flight. Nie trzyma Service z
mutowanym Actorem; wstrzyknięty backend ma stały scope/actor per operacja.

- Init zleca odczyt. Każde I/O to tea.Cmd z contextem; Update/View nie czytają plików
  i nie wykonują Git/tmux. Cmd zwraca wiadomość; nie modyfikuje modelu w goroutine.
- Jeden timer po zakończeniu odczytu; refresh co 2 s workspace, 5 s projekt.
  Gdy odczyt trwa, nie startuj kolejnego. Timeout odczytu 3 s (lock uwzględnia context).
- Zmiana workspace zwiększa generation; spóźnione snapshot/runtime/preview z innego
  scope są ignorowane. Po mutacji unieważnij poprzednią generację odczytu i odśwież.
- Runtime observation osobno, maks. jedno w toku, timeout 2 s. Preview timeout 3 s,
  git detail timeout 3 s. Zbyt wolny proces kończy się jawnym błędem, nie blokuje UI.
- Nie filtruj zmian jedynie po Workspace.Revision: stan runu/checków/UI może mieć
  inny cykl zapisu. Nowy snapshot zastępuje cały zestaw powiązanych rekordów.
- Nie zeruj formularza podczas heartbeat; po zmianie znaczącego targetu zaznacz
  konflikt i zablokuj confirm do odświeżenia intencji.
- Przechowuj ostatni poprawny snapshot przy błędzie. Nagłówek `Stale · last update …`
  po nieudanym refresh; po 6 s bez udanego odczytu blokuj nowe mutacje do odświeżenia.
- Na usunięcie zaznaczonego zasobu: informacja, przejście do rodzica i najbliższego
  dostępnego zaznaczenia. Na usunięcie workspace: ekran projektu, bez crasha.
- Widoki unknown/loading/empty/filtered-empty/error/stale muszą być odrębne.
- Runtime unavailable nie blokuje przeglądania danych; jump/start są niedostępne
  z powodem. Windows: przegląd TUI działa, tmux actions zwracają runtime_unsupported
  z instrukcją użycia linuxowego binarium w WSL.
- Logowanie nie trafia na stdout podczas renderowania. Błędy mają code + czytelny
  message, details w viewport; nie spamuj toastem co heartbeat.

### 9.1. Podgląd i bezpieczeństwo terminala

ReadPreview rozwiązuje ID z rejestru, nigdy dowolny path przekazany przez TUI. Dla
dokumentów dozwolona lista WORKSPACE.md/WORKFLOW.md/Input.Snapshot; dla artefaktu
kanoniczny katalog artifacts, dla check output zatwierdzona ścieżka core. Użyj
istniejących kontroli containment i sprawdź symlinki przed otwarciem; regular file,
bounded read, bez FIFO/urządzeń i ścieżek poza dozwolonym katalogiem. Błąd digestu
artefaktu oznacz jawnie, jeśli weryfikacja jest wykonywana; nie deklaruj pełnej
weryfikacji digestu po przeczytaniu tylko pierwszych 256 KiB.

Każdy zewnętrzny tekst (tytuł, reason, output, argv, ścieżka) usuwa terminalowe
sekwencje sterujące CSI/OSC/DCS/APC i niedozwolone control chars PRZED stylowaniem.
Samo ansi.Strip sprawdź testami dla OSC52/OSC8; uzupełnij sanitizer, jeśli trzeba.
Tekst multiline zachowuje newline/tab (tab rozwijany), lista zamienia newline na
spację. Nie generuj automatycznych hiperłączy ani poleceń z tekstu artefaktów.

## 10. Zarządzany panel i reconciliation tmux

### 10.1. Właściciel, dane i pożądany stan

Dodaj osobny plik `<workspace>/.runtime/ui.json`, schema_version 1. Nie zmieniaj
WORKSPACE.md ani Registry/Status schema tylko z powodu UI. Minimalny rekord:

- workspace_id, project_id;
- desired (bool), ui_id (trwałe `ui_...`), generation (int);
- launch_token (unikalny na generację), state;
- pane_id, window_id, anchor_run_id;
- last_error, failures, next_retry_at, started_at, updated_at;
- receipts show/hide: key → digest, result (mały, jawny kontrakt tego pliku).

Stany UI: disabled, waiting_for_runtime, starting, running, backoff.
desired=false dominuje nad stanem procesu. Brak pliku oznacza default desired=true,
ale plik i panel tworzy się dopiero, gdy workspace ma historię orkiestratora oraz
istnieje jego sesja tmux. Sam odczyt TUI/project list nie tworzy UI ani tmux.

Wszystkie odczyty/zapisy ui.json realizuje core pod istniejącym project lock;
atomicWrite. Nie zwiększaj Workspace.Revision od heartbeat, focus ani odbudowy UI.
Nieznany schema_version lub uszkodzony ui.json: błąd, żadnego nadpisania/nowego panelu.

Metadata tmux:

- session: `@workspace_project_id`, `@workspace_id`;
- window: `@workspace_kind=orchestrator|worktree`, `@workspace_worktree_id` gdy dotyczy;
- UI pane: `@workspace_kind=tui`, `@workspace_id`, `@workspace_ui_id`,
  `@workspace_ui_generation`, `@workspace_ui_token`;
- agent pane: `@workspace_kind=agent` + istniejące session_id/run_id;
- service pane: `@workspace_kind=service` + dotychczasowa tożsamość usługi.

Nazwa okna to etykieta; identyfikatorem jest @window_id. Workspace tmux session nadal
ma nazwę TmuxName(workspaceID). Nowe pola Pane/Launch muszą być dodawane ze sprawdzeniem
wszystkich konstruktorów, fakeRuntime i testów. Legacy paneOwns nadal działa dla agentów;
UI nigdy nie dostaje @workspace_session_id ani @workspace_run_id.

### 10.2. Krytyczna naprawa launch orkiestratora

Obecne Launch robi display-message na `name:orchestrator`, a następnie respawn-pane -k
na otrzymanym panelu. Gdy aktywny będzie TUI, zabije TUI. Zmień to PRZED dodaniem UI.

Algorytm wyboru panelu agenta:

1. Odszukaj dedykowane okno po metadata lub zweryfikowanych Runach orkiestratora.
   Dla legacy bez metadata użyj istniejących pane IDs/runner command i dopiero
   potwierdzone powiązanie oznacz nowymi metadata. Sama nazwa okna nie dowodzi ownership.
2. Jeżeli pasujący panel nowego Run już istnieje (Recover), odzyskaj go.
3. Jeśli istnieje nieaktywny panel poprzedniego Run orkiestratora, wolno respawn tylko
   po porównaniu poprzedniego session/run ownership i sprawdzeniu braku aktywnej
   rezerwacji. Przekaż spod locka jawne ReplacePaneID/ReplaceSessionID/ReplaceRunID
   w Launch; nie wybieraj arbitralnie dowolnego panelu z rodziny agent.
4. Inaczej utwórz nowy panel przez split w tym oknie, albo nowe okno, jeśli go brak.
   Gdy tworzysz nową sesję tmux, uruchom właściwy runner bezpośrednio jako jej pierwszy
   panel; nie pozostawiaj nieoznaczonego shell placeholdera do późniejszego zabicia.
5. Nigdy nie respawn panelu kind=tui/service, obcego shell lub aktywnego innego Run.
6. Brak całego okna orkiestratora przy żyjącym oknie workera musi być obsłużony przez
   utworzenie okna, nie zakończenie błędem przy display-message.

Pozostałe tworzenie okien worktree również oznacz ID. Przy duplikatach nazw nie
wybieraj ostatniego wiersza list-windows. Legacy rozstrzygaj po żywych panelach
powiązanych z WorktreeID; przy niejednoznaczności zgłoś konflikt. Reconcile i stop
zawsze ponownie weryfikują ownership przed użyciem zapisanego pane_id.

### 10.3. Cykl życia panelu

Ukryty runner: `workspace ... _tui-exec <ui-id> <generation> <token>`.
Komenda uruchomienia zawiera bezwzględny Executable, --project, --workspace,
--tmux-socket. Każdy argument przechodzi shellQuote, tak jak runner agentowy.
Pełny unikalny command umożliwia Recover po awarii pomiędzy split a set-option/save.

`_tui-exec` pod lockiem sprawdza ui_id/generation/token/desired, zapisuje running
dopiero po skutecznym claim. Jeśli zastał inną generację lub desired=false — kończy
się bez renderowania i bez zapisywania stanu nowej generacji. Claim jest pojedynczy:
drugi proces z tym samym tokenem nie może nadpisać żywego właściciela. PID nie stanowi
samodzielnego dowodu ownership; sprawdzaj aktualny TMUX_PANE i metadata/command.

Pane TUI jest interfejsem użytkownika. Launcher usuwa odziedziczone tożsamości
WORKSPACE_AGENT_ID/SESSION_ID/RUN_ID/TASK_ID/ROLE/PARENT_* i ORCHESTRATOR_* z jego
środowiska (nie usuwa TERM/TMUX/TMUX_PANE/PATH). Handler po zweryfikowanym claim
tworzy Service z pustym Actor i jawnym projektem/workspace/socket. Nie zmieniaj
globalnie os.Environ w procesie supervisora. Ręczny `workspace tui` zachowuje Actor
z bootstrap i pokazuje ograniczenia roli; nie podnosi jej przez wyczyszczenie env.

`SetUIPaneDesired(ctx, selector, desired, key)`:

1. requireUser, project lock, resolve/loadDocument i ui.json.
2. Receipt replay po key/digest przed nową zmianą. Inny desired z tym samym key = conflict.
3. Zapis desired i receipt atomowo razem w ui.json. Hide jest skuteczne nawet jeśli
   tmux jest niedostępny; efekt zabicia własnego panelu jest uzgadniany później.
4. Po puszczeniu locka uruchom ReconcileInterface; wynik show/hide ma stan efektu UI.
   Powtórzony receipt nie tworzy drugiego panelu. Oddziel od domenowych receipts
   opisanych w operations.md; nie wkładaj ui.json do Document.PendingFiles.

Receipt ma własne operation_id i nie ma Workspace.Revision. Rozdziel niezmienny
wynik ustawienia desired od bieżącej obserwacji panelu: replay zwraca ten sam zapisany
wynik intencji; aktualny stan pobiera `tui status`. W JSON show/hide zwracaj operation_id
w standardowej kopercie, bez udawanej rewizji dokumentu. Dodaj typowaną ścieżkę emit
z przekazanym OperationMetadata dla tych komend; nie szukaj ich kluczy w
Registry.Operations przez obecne options.emit. Pozostałe komendy emit pozostają bez zmian.

Zarządzane `q`/Ctrl+C: najpierw zapisz desired=false, potem opuść raw/alt screen i
zakończ program. Stopka ma `q hide`, ręczne TUI ma `q quit`. Nie zabijaj własnego
panelu z wnętrza procesu przed zakończeniem zapisu i odtworzeniem terminala;
supervisor usuwa jego dead pane po zweryfikowaniu ownership. Nieudany zapis hide
pokazuje błąd i pozostawia program uruchomiony. `tui show` włącza go ponownie.
Zewnętrzne kill-pane, SIGKILL, awaria TUI = utrata, desired pozostaje true → recovery.

### 10.4. Algorytm ReconcileInterface

Oddzielna metoda, wywoływana przez Tick i jawny reconcile oraz po starcie orkiestratora.
Nie wołaj s.With pod istniejącym project lock. Korzystaj z prywatnego helpera
operującego na już odczytanym stanie. Wszystkie starty UI serializuj project lockiem;
tmux ma krótki timeout, nie trzymaj locka podczas działania TUI ani oczekiwania na claim.

1. Odczytaj workspace/ui.json. Przy archived wymuś brak zarządzanego panelu; usuń
   tylko należący do niego pane. Ui.json/historia pozostają. Zrób to również, gdy
   tickWorkspace zwykle wcześnie wraca dla archived.
2. Odczytaj topology wskazanego socketu. Transient tmux error = zachowaj IDs i
   rezerwację, zapisz bounded diagnostic/backoff; NIE traktuj jako braku panelu.
3. desired=false → usuń tylko zweryfikowane UI pane; nie dotykaj agentów i usług.
4. Brak sesji tmux → waiting_for_runtime. UI samo nie odtwarza całej sesji, nie
   startuje orkiestratora ani nie restartuje przerwanego paused/completed workspace.
5. Brak historii orkiestratora → waiting_for_runtime; nie twórz okna tylko dla UI.
6. Odzyskaj zgodny UI pane po metadata lub dokładnym runnerCommand/token. Ponowne
   wywołanie po utracie odpowiedzi ze split musi adoptować istniejący panel.
7. Żywy poprawny panel → running. Przemieszczenie/resize przez użytkownika nie
   powoduje ciągłego narzucania layoutu. Zmienione ID okna aktualizuj po weryfikacji.
8. Zweryfikowany dead/missing UI: jeśli nie ma niepewnej rezerwacji i minął backoff,
   zapisz nową generation/token + starting przed split/respawn. Dotychczasowy dead
   panel można respawn, jeśli nadal jest własny i znajduje się w docelowym oknie.
9. Docelowe okno: aktualne okno Run orkiestratora; jeśli brak aktywnego Run — jego
   oznaczone okno historyczne. Jeśli okno zniknęło i nie ma Run do odtworzenia,
   waiting_for_runtime; nie uruchamiaj shell/orkiestratora tylko dla TUI.
10. Split jest detached i nie kradnie focusu. Preferuj po prawej, gdy dostępne
    width >= 120: TUI 40%, min. 40 kolumn; inaczej na dole, gdy height >= 30:
    TUI 35%, min. 12 wierszy. Jeśli żaden wariant nie daje minimum przy zachowaniu
    używalnego panelu agenta, waiting_for_runtime z powodem `not enough pane space`.
    Supervisor ponawia po zmianie wymiarów. Nie zmniejszaj agentowi panelu do 1 wiersza.
11. Po utworzeniu zapisz pane/window IDs i metadata. Awaria po split to starting
    z niepewnym efektem; kolejny tick najpierw Recover. Nie rezerwuj nowej generacji
    dopóki nie potwierdzisz braku starego polecenia.
12. Przy wielu własnych UI pane adoptuj pasujący do aktualnej rezerwacji; stare
    generacje usuń wyłącznie po potwierdzeniu workspace/ui ownership. Niejednoznaczne
    obce panele zostaw i pokaż konflikt.

Backoff awarii/krótkich crashy: 2, 4, 8, 16, 30, 60 s, potem maks. 60 s; reset po
30 s stabilnego running. `tui show` jako nowa intencja może wyzerować backoff.
Nie zapisuj ui.json co tick, gdy nic się nie zmieniło. Upadek renderera nie może
prowadzić do setek nowych paneli. Nie ma restartowania workerów w ramach tego algorytmu.

Nowo tworzona detached sesja tmux dostaje początkowo 160×48 (`new-session -x/-y`),
aby start z nieterminalowego CLI mógł od razu stworzyć oba panele. Dotyczy to tylko
nowej sesji; nie zmieniaj wymiarów istniejących klientów/okien. Po attach tmux
dostosowuje rozmiar, a TUI przechodzi w właściwy layout, w skrajnym przypadku Tiny.
Brak miejsca na NOWY split nie oznacza usuwania już istniejącego panelu po resize.
Panel w starting nie staje się running tylko dlatego, że jest żywy: musi wykonać
claim. Jeśli przez 10 s pozostaje starting, pokaż błąd startu; kolejny krok recovery
może zatrzymać tylko zweryfikowany własny runner tej generacji i wejść w backoff.

Tick: uzgodnij agentów według obecnych reguł, wykonaj recovery orkiestratora, następnie
UI. Dostarczanie wiadomości i UI uruchamiaj tak, by błąd jednego nie pomijał drugiego;
zbierz błędy (errors.Join). TUI nigdy nie jest jedynym procesem odpowiedzialnym za
odtworzenie samego siebie. Zatrzymany supervisor oznacza brak automatycznego recovery;
pokazuj ten stan, nie startuj supervisora z samego read/refresh TUI.

Paused: UI żyje/odtwarza się przy istniejącej sesji i oknie, także po pause --interrupt.
Completed: UI żyje do archive lub hide, ułatwia przegląd wyników. Archived: brak
zarządzanego UI; ręczne `workspace tui --workspace …` może czytać archiwum.
Clean nie liczy UI jako aktywnego procesu domenowego i nie usuwa ui.json jako worktree.

### 10.5. Migracja i zgodność

Brak ui.json w starym workspace nie wymaga migracji Registry. Metadata okien/paneli
uzupełniaj tylko po udowodnieniu tożsamości istniejącymi rekordami/runner command.
Reconcile jest bezpieczny dla mieszanki starych i nowych paneli. Nie zmieniaj aliasów
sess_* ani numeru publicznego status schema. Starsze binarium nie zna nowego panelu;
README ostrzega, że downgrade z aktywnym UI wymaga `tui hide` przed podmianą binarium,
bo stary launcher orkiestratora nie rozróżnia paneli.

## 11. Jump do tmux i oddawanie terminala

`EntityRef` ma Kind + ID (workspace/worktree/session/run/service/orchestrator/ui).
`NavigationTarget` ma workspaceID, rzeczywistą nazwę/ID sesji tmux, windowID, paneID,
oraz oczekiwane session/run/service/ui ownership i server socket. Nie przyjmuje
dowolnego tmux target-string z formularza użytkownika.

Resolver:

- Workspace → istniejąca sesja tmux, bez tworzenia; wybór: session/orchestrator/UI.
- Worktree → okno po @workspace_worktree_id, fallback po zweryfikowanych panelach
  sesji/usług tego worktree. Brak okna = informacja, nie automatyczny start.
- Session → wyłącznie CurrentRunID, Run.Active i żywy panel należący do tego Run.
- Run → tylko gdy nadal jest current danej sesji; historyczny ma niedostępny Jump.
- Service/UI → sprawdzona własna tożsamość i żywy panel.
- Jeżeli jest wiele sensownych celów, jawny selektor; nigdy „pierwszy pasujący model”.

Oddziel `Select` (nieinteraktywne select-window/select-pane/switch-client) od `Attach`
(proces potrzebujący stdin/stdout). Adapter terminala używa argv i timeoutów, nie
shell strings. Dla attach zwolnij terminal przez tea.Exec z własnym ExecCommand
albo tea.ExecProcess; powrót/detach/błąd odtwarza terminal i odświeża widok.
Wewnątrz tmux zwykły jump odbywa się przez tea.Cmd i nie zatrzymuje pętli TUI.

Rozróżnij klienta i serwer tmux:

- Poza tmux: attach do jawnego socketu/session z przekazanym stdio; TUI wraca po detach.
- W tym samym serwerze: znajdź klienta po TMUX_PANE i list-clients/client_tty,
  jawnie wskaż `switch-client -c CLIENT`. Gdy wielu klientów ogląda to samo źródłowe
  okno/pane i nie da się wskazać jednego, pokaż wybór klientów zamiast zgadywać.
- W innym serwerze (np. --tmux-socket wskazuje izolowany serwer): zwróć czytelny
  `tmux_server_mismatch` i instrukcję attach z terminala poza bieżącym tmux.
  V1 nie robi zagnieżdżonego attach i nie przełącza losowego klienta innego serwera.
- Brak klienta attachowanego do managed UI: jump informuje `no attached client`.
  Nie startuje nowego terminala i nie przełącza cudzej sesji.
- Waliduj przynależność pane do session/window tuż przed select. Nie da się objąć
  plików i tmux jedną transakcją; błędy wyścigu pokazuj i odśwież, nie stosuj fallbacku
  do niesprawdzonego panelu. Własne operacje start/stop/focus serializuj odpowiednio
  krótką blokadą core; nie trzymaj blokady przez interaktywny attach.

Nie ustawiaj globalnego keybinding powrotu. W pomocy opisz standardowy wybór okna/panelu
tmux oraz `workspace tui show`. Samo przejście do innego window nie zmienia route
działającego w tle panelu TUI. Po ponownym zaznaczeniu panel pokazuje świeży snapshot.

## 12. Testy i weryfikacja

Nie wystarczy screenshot jednego ekranu. Testuj przejścia, ownership i failure windows.
W unit testach użyj fake backend/runtime/clock i deterministycznych IDs/dat; żadnych
sieciowych klientów ani zależności od terminala dewelopera. Nie zapisuj tysięcy
identycznych goldenów; poniżej minimalny zestaw mający znaczenie.

### 12.1. Core / bootstrap / kompatybilność

- Scope: root projektu, podkatalog projektu, workspace, podkatalog checkoutu,
  external workspaces_dir, symlink, explicit flags, odziedziczony inny workspace,
  brak config, uszkodzony frontmatter, konflikt projektu i workspace.
- Snapshot: wiele sesji tej samej persony, historyczne próby, sesja bez taska,
  reader+writer, services, wyniki bez możliwości ustalenia lineage; brak duplikatów.
- ProjectOverview pokazuje zdrowe workspace obok uszkodzonego; List zachowuje dawny
  fail-fast kontrakt. Runtime timeout nie psuje snapshotu trwałego stanu.
- ResumeSession: read-only, snapshot persony, legacy Run alias, closed/active/invalid
  lineage, operation replay; CLI i TUI dają taki sam stan i błędy.
- Guarded stop: Run zmienia się między formularzem a operacją → nowy Run nietknięty.
  Retry zmienionej attempt i pause-interrupt zmienionego zbioru → konflikt.
  Replay po sukcesie z historycznym guard nadal zwraca receipt, nie konflikt rewizji.
- ReadPreview: symlink/traversal/FIFO/binary/duży plik/brak pliku/znaki ANSI/OSC52;
  limit dotyczy odczytu, a nie tylko renderowania.
- `status/list/session list/run list/menu --json/--short` nie dostają UI rekordów
  ani nowej wersji schematu. Zachowaj bieżące testy CLI.

### 12.2. Model TUI

- Enter/Esc/Tab/1–5 i link task → session → run → back zachowują zaznaczenie/query.
- Search substring, różne wielkości liter, powtarzające się nazwy, ID, pusty wynik;
  q/g/cyfry w input nie wykonują akcji; Huh otrzymuje wyłącznie należne mu zdarzenia.
- Generacja A → workspace B → spóźniony snapshot A nie podmienia ekranu B.
- Jedna operacja/refresh w toku; double submit, retry z tym samym kluczem,
  anulowanie form, zmiana targetu, error/stale i odzyskanie odczytu.
- `exited` vs `accepted`, idle + interrupted, ready + active writer, unknown state,
  failed check z exit code; nie mieszaj statusów.
- Sanitizer chroni wszystkie renderowane pola; test długich Unicode/CJK/combining
  chars i wielowierszowych nazw bez przekroczenia szerokości.
- Test rendered width/height (po usunięciu ANSI) dla 160×48, 120×18, 80×24,
  60×40, 40×12, 30×8 i 1×1. Golden dla dashboard/task/form w wide/narrow/short,
  osobno podstawowa paleta light/no-color. Test resize wielokrotnie w obu kierunkach.
- Fake terminal adapter: jump success/failure, attach zwraca terminal, cross-server,
  wybór klienta przy niejednoznaczności. Program przywraca terminal po quit/cancel/error.

### 12.3. Recovery UI — fake runtime

- Default bez orkiestratora nie startuje niczego. Po starcie dokładnie jeden UI pane.
- Dwa równoległe wywołania ensure/reconcile → jeden panel i jeden token claim.
- Awaria przed split, po split/przed metadata, po metadata/przed zapisem IDs,
  przed claim i po claim. Powtórzenie adoptuje istniejący panel, nie duplikuje.
- Inny pane pod tym samym pane_id, zły token/generation/workspace → brak kill/respawn.
- timeout/niepewny tmux ≠ missing. Backoff ogranicza crash loop.
- q/hide jest trwałe; zewnętrzny kill odtwarza. Stary runner nie nadpisuje nowej
  generacji. Show/hide receipts i konkurencja hide vs launch.
- paused/completed działają według tabeli; archived sprząta tylko UI. UI nie liczy
  się do limitów/writer/service_active/session_active/clean.
- Błąd UI nie blokuje recovery/delivery agentów i odwrotnie; supervisor stop nie
  zatrzymuje paneli. Odczyt TUI nie uruchamia supervisora.

### 12.4. Prawdziwe tmux, Linux/WSL

Rozszerz istniejące fixture i prywatny socket z tmux_integration_test.go,
supervisor_test.go, workflow_tmux_test.go. Binarium buduj w t.TempDir, także w ścieżce
ze spacją i apostrofem. Domyślny testowy klient jest lokalnym procesem fixture.

1. Start orkiestratora tworzy jego panel i TUI w tym samym window; focus pozostaje
   na orkiestratorze. Capture-pane TUI zawiera title/status/Overview.
2. Ustaw aktywnym TUI, zakończ/utrac orkiestratora i wznów. Panel TUI nie jest
   respawn jako agent; nowy Run ma poprawne IDs, orkiestrator działa obok niego.
3. Kill UI pane → supervisor odtwarza jeden; kill całego okna → recovery orkiestratora
   i UI, gdy workspace aktywny. Nie startuj workerów od nowa.
4. Zamień/usuń metadata UI w oknie z obcym pane; obcy shell nie jest zabijany.
5. q w TUI → desired=false i brak odtworzenia; tui show → jeden panel ponownie.
6. Pause --interrupt → agent i service zatrzymani, UI działa. Archived po spełnieniu
   istniejących gate → UI znika i nie blokuje archive/clean.
7. Zmień wymiary okna na szeroki niski i wąski wysoki; capture-pane mieści tekst,
   zachowuje stronę, input i widoczny status. Małe rozmiary nie powodują crasha.
8. Dwa klienty tmux + inny socket: jump wybiera właściwy klient/okno/pane;
   cross-server nie przełącza obcego klienta. Attach/detach testuj z pseudo-TTY.
9. Reconcile nie kradnie focusu i nie zmienia ręcznie poprawionego layoutu co tick.
10. Uruchom także dotychczasowe testy Session/Run/replay/readonly/supervisor/workflow.

Testy czekają na stan z deadline, nie na arbitralne długie sleep. Przy błędzie
capture-pane/log supervisora trafiają do diagnostyki testu. Prywatny socket i procesy
sprzątaj przez t.Cleanup. Użycie send-keys w testach do sterowania samym TUI jest
dozwolone; aplikacja nie dostarcza w ten sposób wiadomości do agentów.

### 12.5. Wymagane komendy końcowe

```sh
gofmt -w <zmienione-pliki-Go>
go test ./...
go vet ./...
WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s
```

W Linux/WSL zbuduj binarium do nowego katalogu tymczasowego i uruchom:

```sh
go build -o "$TEMP_BUILD_DIR/workspace" ./cmd/workspace
python3 scripts/check-install.py "$TEMP_BUILD_DIR/workspace"
```

TEMP_BUILD_DIR musi wskazywać nowy katalog utworzony dla tej weryfikacji poza repo.
Sprawdź też `GOTOOLCHAIN=local` przy Go 1.24 oraz build/test core i TUI na Windows.
Jeśli środowisko nie ma WSL/tmux, wykonaj dostępne testy, dokładnie odnotuj brak
integracji i nie deklaruj kryteriów tmux jako zaliczonych.

## 13. Kolejność prac dla implementera

Etapy są sekwencyjne; każdy pozostawia kompilujący się kod. Nie twórz całego TUI w
jednym wielkim pliku. Po każdym etapie uruchom testy zmienionych pakietów.

### Etap 0 — inwentaryzacja i punkt odniesienia

- [ ] git status, przeczytanie wskazanych dokumentów i tests; sprawdzenie różnic od bazy planu.
- [ ] Zanotowanie bieżącego wyniku go test ./... oraz dostępności Go/WSL/tmux.
- [ ] Odszukanie wszystkich wywołań Runtime.Attach/Launch, konstruktorów Pane/Launch,
      session resume, EnsureSupervisor, testów z liczbą paneli.

Gotowe, gdy znane są aktualne punkty integracji i nie nadpisano zastanej pracy.

### Etap 1 — wspólny core i bootstrap

- [ ] query.go/query_relations.go i spójny snapshot, projekcje/testy relacji.
- [ ] Preview i testy ograniczeń odczytu.
- [ ] Bootstrap z preserve CLI semantics; ekstrakcja ResumeSession i supervised entry points.
- [ ] Core resolver celu i wspólne helpery guardów; testy replay i konkurencji.
- [ ] CLI korzysta z wydzielonych przypadków użycia; stare formaty nadal przechodzą testy.

Gotowe, gdy core bez importów UI wystarcza do obsługi tabeli akcji i odczytu ekranów.

### Etap 2 — bezpieczny tmux przed drugim pane

- [ ] Metadata sesji/windows/panes i odczyt topology.
- [ ] Naprawa wyboru panelu orkiestratora, tworzenia brakującego okna i recovery.
- [ ] Rozwiązanie/nawigacja ID-based; adapter terminala z wyborem klienta/stdio.
- [ ] Test legacy pane oraz aktywny obcy panel w oknie orkiestratora.

Gotowe, gdy start/resume nie może nadpisać panelu innej roli i dotychczasowy runtime działa.

### Etap 3 — shell TUI i layout

- [ ] Przypięte zależności, go mod tidy, kontrola Go 1.24.
- [ ] `workspace tui`, walidacja TTY/flag, app/model/update/routes/keys/theme/layout.
- [ ] Project picker + Dashboard na fake backend i prawdziwym WorkspaceSnapshot.
- [ ] Async refresh, stale/error/loading i generation fences; zero mutacji z ticka.
- [ ] Testy wide/narrow/short/tiny, input, focus, no-color i terminal restore.

Gotowe, gdy ręczne TUI daje używalny dashboard projektu/workspace przy obu orientacjach.

### Etap 4 — wszystkie widoki i przejścia

- [ ] Tasks/detail/sessions/runs/results i powroty z history/query preservation.
- [ ] Worktrees/orchestrator, usługi, More/Agents/Decisions/CR/Runtime.
- [ ] Filtry/name search i preview; odrębne stany i prawidłowe liczniki.
- [ ] Jump z każdego wymaganego poziomu oraz obsługa braku/utraty celu.

Gotowe, gdy żadna wymagana encja nie jest ukryta bez ścieżki nawigacji i dashboard
nie zalewa pełnymi listami. Wszystkie strony mają empty/error cases.

### Etap 5 — akcje i formularze

- [ ] Embedded Huh, statyczny dispatch tabeli akcji z sekcji 7.
- [ ] Potwierdzenia, reason, current target guards, właściwe klucze i same-key retry.
- [ ] Error details, blokada double submit i refresh po wyniku.
- [ ] Sprawdzenie roli aktora w core i testy stale_actor/forbidden/read-only.

Gotowe, gdy wszystkie akcje V1 działają z tym samym core co CLI i są testowane
na fake backend oraz przynajmniej reprezentatywnych rzeczywistych operacjach core.

### Etap 6 — zarządzany panel

- [ ] ui.json, SetUIPaneDesired/receipts, ukryty _tui-exec i generation claim.
- [ ] ReconcileInterface, recovery po command/token, backoff, split/layout bez focus steal.
- [ ] Hook po starcie orkiestratora i Tick; niezależność błędów UI/delivery.
- [ ] show/hide/status CLI, q hide, paused/completed/archived i concurrency tests.

Gotowe, gdy ręczne TUI i zarządzane mogą współistnieć, ale supervisor zarządza tylko
jednym pane na workspace; utrata oraz świadome ukrycie mają różne zachowanie.

### Etap 7 — integracja i dokumentacja kontraktów

- [ ] Wszystkie testy z sekcji 12, w szczególności real tmux i race.
- [ ] README: wejście, scope, skróty, theme, wymagania wymiarów, q hide vs quit,
      show/hide/status, Windows/WSL, downgrade i zachowanie attach.
- [ ] PRODUCT: TUI jako istniejący interfejs i nawigacja task-first.
- [ ] ARCHITECTURE: dwa adaptery UI, bootstrap/terminal, queries, osobny ui.json.
- [ ] docs/runtime.md: ownership paneli, supervisor recovery UI, wyjście i pause/archive.
- [ ] docs/operations.md: guards, keys/retry TUI i oddzielne receipts UI.
- [ ] docs/tui.md: zwięzły bieżący kontrakt ekranów, keymap i macierz layoutu;
      nie kopiuj całego planu jako dokumentacji aktualnej implementacji.
- [ ] help.go: opisy/argumenty/flagi nowych widocznych komend; test kompletności.
- [ ] TODO.md: oznacz pierwszą pozycję dopiero po wszystkich kryteriach; nie zmieniaj releasów.
- [ ] Raport końcowy: co działa, testy, faktyczne ograniczenia, bez deklaracji z samego planu.

## 14. Definicja ukończenia — macierz wymagań użytkownika

| Wymaganie | Dowód ukończenia |
|---|---|
| Projekt → wybór workspace | test scope + działający project picker, także 0/1/wiele workspace |
| Workspace → dashboard | poprawny snapshot i dashboard także external storage/CWD |
| Nawigacja po agregacie | task → sessions → runs/results; worktree → tasks/sessions/services; orphan/history dostępne |
| Brak zalewu encji | dashboard tylko liczniki + maks. 5 uwag/5 activity; pełne listy po wyborze |
| Statusy | odrębne Task/Session/Run/Worktree, active vs failed/stopped/interrupted/accepted |
| Search po nazwie | wszystkie kolekcje, substring case-insensitive, zachowanie query i selection |
| Estetyczne panele/theme | wide/short/narrow, focus, dark/light/no-color i tekstowe badge |
| Poziomy/pionowy pane | automatyczne layouty, testy render width/height + real tmux resize |
| Skok tmux session/window/pane | zweryfikowane cele/klient/socket, attach/detach i błędy wyścigu |
| Automatyczny TUI obok orkiestratora | real start/resume z aktywnym TUI nie niszczy jego pane |
| Reconciliation utraconego UI | kill/recover, concurrent ensure, crash windows, backoff i q hide |
| Wspólny core CLI/TUI | brak shell-out workspace, wspólne resume/guards/queries/navigation |
| Zachowanie dotychczasowego CLI | wszystkie obecne testy, stare komendy, schema i formaty bez regresji |

W przypadku rozbieżności kodu z bazą analizy zachowaj kontrakt tego planu, adaptując
punkty integracji do aktualnego kodu. Jeśli brakuje możliwości weryfikacji runtime,
opisz dokładnie niewykonane scenariusze. Nie zastępuj ich deklaracją „powinno działać”.

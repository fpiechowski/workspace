# Architektura `workspace`

## Kontekst systemu

`workspace` jest pojedynczym programem Go uruchamianym lokalnie w repozytorium Git.
CLI wywołuje logikę domenową, która utrwala stan w plikach i steruje zewnętrznymi
procesami: Git, tmux, klientami agentów oraz opcjonalnym adapterem forge lub trackera.
Projektowy supervisor działa lokalnie i uzgadnia zapisany stan z runtime; nie podejmuje
decyzji workflow.

```text
użytkownik / agent
        │
        ▼
   CLI (`cmd/workspace`, `internal/cli`)
        ├── bootstrap (flag/env/CWD → scope)
        ├── TUI (`internal/tui`)
        └── terminal (attach/switch/jump)
                  │
                  ▼
   domena i przypadki użycia (`internal/core`)
        │
        ├── pliki stanu, blokady i write-ahead record
        ├── Git worktrees i commity
        ├── tmux, procesy klientów i supervisor
        └── adaptery klientów, trackerów i forge
```

Runtime sesji wymaga Linuxa, macOS albo Linuxa w WSL, ponieważ opiera się na tmux.
Rdzeń CLI buduje się i jest testowany także na Windows; kod zależny od procesów ma
implementacje rozdzielone według platform.

## Podział kodu

- `cmd/workspace/` — punkt wejścia binarium.
- `internal/cli/` — drzewo komend Cobra, flagi, pomoc oraz serializacja odpowiedzi.
- `internal/bootstrap/` — wspólne rozpoznanie projektu/workspace'u, Service i aktora
  z flag, środowiska oraz CWD.
- `internal/tui/` — Bubble Tea model, trasy, widoki, formularze Huh i theme; bez
  bezpośrednich zapisów do plików domenowych.
- `internal/terminal/` — zweryfikowane przełączanie klienta tmux albo attach z
  przekazaniem stdio; nie przyjmuje surowych target-stringów z interfejsu.
- `internal/core/` — model domenowy, przypadki użycia, trwałość i adaptery procesowe.
- `internal/core/templates/` — wbudowane szablony instalowane przez `project init`.
- `.workspace/templates/` — projektowa, edytowalna kopia szablonów i workflow.
- `scripts/` — sprawdzenia instalacji i narzędzia testujące integracje.
- `docs/` — węższe kontrakty operacyjne, do których odsyła ten dokument i README.

Pakiet `core` jest obecnie świadomie spójny i plikowy, zamiast dzielić każdą domenę
na osobny pakiet. Granice odpowiedzialności wyznaczają pliki (`project`, `workflow`,
`session`, `runtime`, `routing`, `mailbox`, `handoff`, `forge` itd.) oraz interfejsy
zewnętrznych adapterów.

## Model domenowy

| Obiekt | Odpowiedzialność |
|---|---|
| Project | Repozytorium Git, konfiguracja klientów, profili, forge, trackera i workflow. |
| Workspace | Trwały kontekst jednej inicjatywy od wejścia do potwierdzonego release'u. |
| Worktree | Izolowany checkout i branch do planowania, implementacji, integracji lub testów. |
| Task | Delegowana jednostka pracy z próbą, zależnościami i kryteriami akceptacji. |
| Agent | Stabilna definicja persony: rola, instrukcje, prompt i profil. |
| Session | Logiczny kontekst rozmowy dla jednego lineage agenta i zadania. |
| Run | Konkretne uruchomienie klienta i panelu wraz z routingiem oraz wynikiem procesu. |
| Message | Trwała wiadomość adresowana do Agent ID. |
| Handoff | Zgłoszenie wyniku zadania do osobnej oceny przez odbiorcę. |
| Artifact | Niezmienna kopia produktu pracy z digestem i pochodzeniem. |
| Check | Receipt rzeczywiście uruchomionego polecenia weryfikacyjnego. |
| Change request | Lokalnie przygotowany albo opublikowany PR/MR związany z rewizją kodu. |

Project posiada wiele workspace'ów. Workspace zawiera zadania, worktrees i agentów.
Agent może mieć wiele logicznych Session, a każda Session — wiele kolejnych Runów.
Wyniki wskazują zadanie, próbę, Session, dokładny Run i rewizję Git. Identyfikatory są
niezmienne; nazwy służą wyłącznie prezentacji i wyborowi zasobu.

Usunięcie Task lub Session z TUI jest logiczne: rekord otrzymuje `deleted_at` i pozostaje
w trwałym stanie jako tombstone dla receiptów i historycznych referencji, ale znika z
normalnych kolekcji TUI i liczników postępu. Core odrzuca tombstone aktywnej Session,
zaakceptowanego lub zależnego Taska oraz rekordów z utrwalonymi wynikami. Usunięcie
całego workspace'u jest fizyczną operacją projektu i ma receipt poza usuwanym katalogiem.
Jest jawnym discardem niezależnym od archive/release: zatrzymuje sesję runtime, wymusza
usunięcie należących do workspace'u worktrees i lokalnych gałęzi, a następnie usuwa cały
katalog stanu. Ścieżki i prefiksy gałęzi są sprawdzane przed destrukcyjnymi efektami.

Najważniejsze niezmienniki:

- tylko jeden Run danej Session może być aktywny;
- orkiestrator ma jedną aktywną linię wykonania chronioną generacją;
- w worktree może działać najwyżej jeden writer zarządzany przez `workspace`;
- kod wyjścia procesu nie akceptuje automatycznie wyniku zadania;
- ACK wiadomości i akceptacja handoffu są osobnymi operacjami;
- nowy input, próba zadania lub niezgodny lineage nie wznawia starej Session;
- live test i release odnoszą się do konkretnego SHA.

## Trwały stan

Projekt przechowuje wersjonowalną konfigurację i szablony pod `.workspace/`.
Domyślnie lokalne workspace'y również znajdują się w `.workspace/ws_*`, ale
`workspaces_dir` może wskazać katalog zewnętrzny. Lokalne reguły Git ignorują runtime
i produkty robocze bez ukrywania konfiguracji oraz szablonów.

W każdym workspace:

```text
ws_ID/
├── WORKSPACE.md          # kanoniczny stan workflow i narracja
├── WORKFLOW.md           # zamrożony snapshot wybranego workflow
├── AGENTS.md             # instrukcje roli orkiestratora
├── inputs/               # snapshot wejścia
├── prompts/              # zamrożone prompty i instrukcje wykonawców
├── tasks/                # specyfikacje zadań
├── artifacts/            # niezmienne kopie przekazanych produktów i dowodów
├── worktrees/            # checkouty Git, jeśli storage jest wewnętrzny
└── .runtime/
    ├── index.json        # rejestry operacyjne, Session i Run
    ├── ui.json           # oddzielny desired state, receipts i generation TUI
    ├── pending.json      # intencja przerwanego zapisu
    └── ...               # inbox, prompty, receipts i stan supervisora
```

`WORKSPACE.md` ma walidowany frontmatter i opisową treść dla kolejnego orkiestratora.
Jest źródłem prawdy dla fazy workflow, zadań, decyzji i zaakceptowanych wyników.
`.runtime/index.json` jest indeksem operacyjnym, a nie konkurencyjną wersją workflow.
Publiczny `status --json` ma własny jawny numer schematu. TUI korzysta z prywatnego
`WorkspaceSnapshot`; nie dodaje encji do `index.json`, nie zmienia schematu publicznego
Status i nie zapisuje Session/Run dla własnego procesu.

Szablony są kopiowane do workspace przy jego tworzeniu. Późniejsza zmiana szablonu
projektowego nie zmienia trwającej pracy. Jawna migracja zachowuje wcześniejsze pliki,
hashe i historię oraz unieważnia zależne wyniki. Szczegóły opisuje
[docs/revisions.md](docs/revisions.md).

## Mutacje, współbieżność i odtwarzanie

Operacja zmieniająca stan przebiega pod blokadą projektu: odczytuje i waliduje dokument,
sprawdza rolę oraz oczekiwaną rewizję, zapisuje intencję, przygotowuje pliki tymczasowe,
wykonuje atomową podmianę i utrwala receipt. Przerwany zapis jest kończony albo
uzgadniany przy następnym odczycie.

`--operation-key` identyfikuje logiczną mutację. Digest obejmuje jej payload, dlatego
ponowienie zwraca zachowany wynik, a użycie tego samego klucza z inną treścią kończy
się konfliktem. Efektów Git, tmux i usług zewnętrznych nie da się objąć transakcją
plikową; ich intencja i stan są zapisywane przed wywołaniem, a niepewny rezultat jest
sprawdzany przed retry. Pełny kontrakt znajduje się w
[docs/operations.md](docs/operations.md).

Destrukcyjne akcje TUI dodatkowo wymagają przepisania pełnego ID. Delete Task/Session
korzysta z guardów rewizji i attempt/ostatniego Run, natomiast Delete Workspace zapisuje
idempotentny receipt w projektowym rejestrze operacji, zatrzymuje runtime oraz usuwa
worktrees i gałęzie przed katalogiem. Retry po zakończonym fizycznym usunięciu odtwarza
wynik z project-scoped receiptu. Archive z TUI ma osobny guard rewizji i nie kasuje danych.

Kontrola ról, tryb read-only i dzierżawa worktree są kontraktem współpracujących
procesów działających na jednym koncie systemowym, nie granicą bezpieczeństwa systemu
operacyjnego.

## Runtime procesów

Każdy workspace otrzymuje sesję tmux. Worktrees są oknami, konkretne Runy agentów —
panelami, a orkiestrator ma osobne okno uruchomione z katalogu workspace. Usługi
pomocnicze są osobnym typem rekordu i nie dziedziczą tożsamości agenta.

Runner otrzymuje m.in. `WORKSPACE_AGENT_ID`, `WORKSPACE_SESSION_ID`,
`WORKSPACE_RUN_ID`, `WORKSPACE_TASK_ID`, `WORKSPACE_ROLE` oraz identyfikatory rodzica,
orkiestratora i worktree. To Run jest właścicielem aktualnego panelu; recyklingowany
identyfikator tmux nie daje staremu procesowi prawa do mutacji.

Supervisor porównuje aktywne rezerwacje z rzeczywistymi panelami, dostarcza wiadomości
i wykrywa utracone procesy. Może wznowić utraconego orkiestratora w kompatybilnej
Session, ale nie restartuje w ciemno jawnie zatrzymanych lub zakończonych błędem
wykonawców. Szczegółowe stany i procedury opisuje [docs/runtime.md](docs/runtime.md).

Zarządzany TUI jest odrębnym typem runtime ownership zapisanym w workspace
`.runtime/ui.json`. Po wykryciu historii orkiestratora supervisor uruchamia najwyżej
jeden panel obok orkiestratora bez zmiany focusu. Launch token i generation są
claimowane przez dokładny pane; reconcile usuwa tylko panele, których tożsamość
potwierdzają metadata albo konkretna komenda runnera. Błąd UI i błąd agentów są
uzgadniane niezależnie. UI nie liczy się jako writer, service ani aktywny Run i nie
blokuje cleanup. Pełny kontrakt opisuje [docs/runtime.md](docs/runtime.md).

## Adaptery klientów i routing

Adapter klienta rozdziela pięć możliwości: launch, resume, deliver, observe i interrupt.
Argumenty procesów są tablicami argv bez interpolacji shell. Wbudowane adaptery
obsługują Codex, Claude i OpenCode; adapter `command` pozwala podłączyć własne wrappery.
Natywne ID rozmowy jest opcjonalnym bindingiem Session, nigdy jej tożsamością ani
adresem inboxa. Szczegóły i format wrappera opisuje [docs/clients.md](docs/clients.md).

Profil zawiera trasy klient/provider/model oraz wymagane capabilities. Router odrzuca
trasy niedostępne, będące w cooldownie albo przekraczające limity. Następnie równoważy
providerów na podstawie lokalnych uruchomień i rezerwacji z okna 24 godzin, a wynik
decyzji zapisuje w Run. Te liczniki są przybliżeniem lokalnego obciążenia projektu,
nie pomiarem tokenów, kosztu ani limitów całego konta.

## Workflow i delegowanie

Workflow jest wersjonowanym szablonem opisującym fazy, warunki wejścia, wymagane wyniki
i ścieżki naprawy. CLI egzekwuje przejścia, role, zależności i akceptację; tekst
`WORKFLOW.md` instruuje orkiestratora, jak składać te operacje idempotentnie.

Podstawowy `plan-first` prowadzi od planowania do implementacji. Rozbudowany workflow
`issue-resolution`, zachowany dla zgodności i pełnego procesu, obejmuje:

```text
planning → plan review → implementation → integration → change requests
         → oferta live testing → live test lub jawne pominięcie
         → oczekiwanie na release → potwierdzone zakończenie
```

`paused`, `blocked` i `needs_attention` są stanami operacyjnymi niezależnymi od fazy.
Zmiana wejścia lub retry zwiększa próbę i unieważnia wyniki zależne bez kasowania
historii. Integracja odbywa się w dedykowanym worktree z manifestem wejściowych SHA.

## Komunikacja, artefakty i dowody

Wiadomości trafiają najpierw do trwałego inboxa. Adapter z `deliver` może obudzić
rozmowę; bez niego agent świadomie odpytuje inbox. Dostarczenie jest co najmniej
jednokrotne i deduplikowane po ID.

`handoff submit` waliduje jawnie wskazane pliki, kopiuje je do artifacts, oblicza hashe
i zapisuje pochodzenie przed wysłaniem wiadomości. Nie archiwizuje automatycznie całego
checkoutu. Akceptowalna implementacja wskazuje czysty commit i wymagane dowody.
`check run` przechwytuje rzeczywiste argv, kod wyjścia, output digest i SHA; sam sukces
komendy CLI oznacza zapisanie receipt, nie powodzenie uruchomionego testu. Format i
ograniczenia opisuje [docs/checks.md](docs/checks.md).

## Integracje zewnętrzne

Tracker pobiera snapshot issue przez `gh`, `glab` albo skonfigurowany adapter polecenia.
Nie komentuje i nie zmienia stanu ticketa. Kontrakt wejścia opisuje
[docs/trackers.md](docs/trackers.md).

Forge przygotowuje, publikuje, wyszukuje i synchronizuje change requests dla GitHub,
GitLab albo własnego adaptera. Bez integracji tworzy lokalny pakiet CR. Publikacja
respektuje politykę `ask`/`allowed`; niepewna odpowiedź jest uzgadniana przez lookup,
aby nie utworzyć duplikatu.

## Bezpieczeństwo danych i granice zaufania

- Input z trackera i treść zadania są danymi, nie instrukcjami podnoszącymi uprawnienia.
- Ścieżki artefaktów są ograniczane do przypisanego worktree; traversal i wychodzące
  symlinki są odrzucane.
- Sprzątanie sprawdza aktywność, lokalne zmiany i nieopublikowane commity; opcjonalny
  backup używa weryfikowanego Git bundle.
- Lokalne pliki nie chronią przed innym procesem tego samego użytkownika. Model ról
  zapewnia spójność współpracy, a nie izolację przed złośliwym kodem.
- Potwierdzenie użytkownika jest audytowalnym kontraktem procesu, nie kryptograficznym
  dowodem ludzkiej tożsamości.

## Weryfikacja zmian

Podstawowy zestaw jakości dla kodu Go to `gofmt`, `go test ./...` i `go vet ./...`.
Zmiany procesów, współbieżności lub integracji tmux wymagają pełnego testu race/tmux
na Linuxie lub WSL. Zmiany instalacji albo budowania wymagają dodatkowo zbudowania
binarium w nowej lokalizacji i uruchomienia `scripts/check-install.py` zgodnie z
[README.md](README.md#weryfikacja).

Testy korzystają z izolowanych repozytoriów, prywatnych serwerów tmux i lokalnych
fixture'ów klientów/forge. Zwykła weryfikacja nie publikuje change requests ani nie
wywołuje płatnych tur modeli.

## Referencje operacyjne

Dokumenty w `docs/` są celowo węższe od tej architektury:

- [clients.md](docs/clients.md) — możliwości adapterów i wrapperów klientów;
- [runtime.md](docs/runtime.md) — stany Session/Run, supervisor, usługi i cleanup;
- [tui.md](docs/tui.md) — ekrany, nawigacja i akcje interfejsu terminalowego;
- [operations.md](docs/operations.md) — idempotencja, receipts i niepewne efekty;
- [revisions.md](docs/revisions.md) — zmiana inputu i migracja workflow;
- [checks.md](docs/checks.md) — przechwytywanie i import dowodów testów;
- [trackers.md](docs/trackers.md) — pobieranie oraz snapshotowanie issue.

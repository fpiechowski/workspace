# workspace

CLI do zarządzania trwałym kontekstem zadania, agentami, Git worktrees i tmux.
Orkiestrator realizuje prosty workflow `plan-first`: planowanie w osobnym worktree
i delegowanie implementacji do kolejnych worktree’ów.

- **Agent**: persona z rolą, instrukcjami, szablonem promptu i profilem modelu.
- **Session**: konkretne uruchomienie persony; model, prompt i native thread ID są utrwalone.
- **Workspace**: WORKSPACE.md, AGENTS.md, WORKFLOW.md, zadania, artefakty i worktrees.

Projekt i zakres: [plan](workspace-cli-design.md). Zakres implementacji i wyniki weryfikacji:
[IMPLEMENTATION.md](IMPLEMENTATION.md).

## Instalacja

Wymagania: Go 1.24+, Git i tmux. Runtime działa w Linux/macOS lub Linux w WSL.
Windows obsługuje budowanie i testy rdzenia; sesje uruchamiaj linuxowym binarium w WSL.

```sh
go build -o bin/workspace ./cmd/workspace
export PATH="$PWD/bin:$PATH"
workspace --help
```

Binarium musi pozostać dostępne pod tą ścieżką, ponieważ tmux uruchamia je także później.
Do uruchamiania sesji nie używaj `go run`.

W projekcie Git z przynajmniej jednym commitem:

```sh
workspace project init
workspace doctor --json
workspace skill install --client codex
```

Skill instaluje się w `.agents/skills/workspace` dla Codex/OpenCode lub
`.claude/skills/workspace` dla Claude. Istniejący zmieniony skill nie jest nadpisywany.
`init` zachowuje konfigurację i instaluje edytowalne templates w `.workspace/templates`.
Konfiguracja i templates mogą być wersjonowane; dane robocze mają lokalne reguły Git ignore.

## Konfiguracja

Zachowaj wygenerowane `schema_version`, `project_id` i `runtime`. Dodaj klientów i
profile do `.workspace/config.yaml`. Nazwy modeli zastąp identyfikatorami dostępnymi
na własnym koncie; profile nie zawierają wbudowanych założeń o cenach ani abonamencie.

```yaml
clients:
  codex:
    adapter: codex
  claude:
    adapter: claude
  opencode:
    adapter: opencode
profiles:
  thinker:
    routes:
      - {id: planner, client: opencode, provider: YOUR_PROVIDER, model: YOUR_PROVIDER/YOUR_THINKER_MODEL, max_concurrency: 1}
  orchestrator:
    routes:
      - {id: orchestrator, client: opencode, provider: YOUR_PROVIDER, model: YOUR_PROVIDER/YOUR_ORCHESTRATOR_MODEL, max_concurrency: 1}
  supervisor:
    routes:
      - {id: supervisor, client: opencode, provider: YOUR_PROVIDER, model: YOUR_PROVIDER/YOUR_SUPERVISOR_MODEL, max_concurrency: 1}
  worker:
    routes:
      - {id: worker, client: opencode, provider: YOUR_PROVIDER, model: YOUR_PROVIDER/YOUR_WORKER_MODEL, max_concurrency: 3}
defaults:
  orchestrator_profile: orchestrator
workflows:
  plan-first:
    profiles:
      orchestrator: orchestrator
      planning: thinker
      implementation: worker
    max_parallel_tasks: 3
forge:
  adapter: github
  remote: origin
  publication: ask
```

`forge.adapter` może być `github`, `gitlab` albo `command`. Bez adaptera powstaje lokalny
pakiet CR; użytkownik może dołączyć rzeczywisty request lub jawnie pominąć publikację.
Polityka `allowed` zapisuje uprzednią zgodę na publikację. Przy `ask` orkiestrator
pokazuje przygotowany opis i diff, a następnie zapisuje otrzymaną odpowiedź.
W trybie `per-task` przygotuj requesty w kolejności zależności zadań. Pole `merge_after`
i opis CR wskazują poprzedniki; przygotowanie zależnego CR wymaga ich aktualnych requestów.

Codex używa app-server i obsługuje wybudzanie między turami oraz native resume.
Claude/OpenCode uruchamiają interaktywny klient. OpenCode z `deliver_argv` automatycznie
wykrywa i zapisuje swój native thread, aby supervisor mógł dostarczać wiadomości między
agentami. `session bind-thread` pozostaje narzędziem awaryjnym. Bez `deliver_argv`
komunikacja wymaga odczytu inboxa przez agenta. [Adaptery i ich możliwości](docs/clients.md).

Routing liczy uruchomienia i aktywne rezerwacje w projekcie przez ostatnie 24 h,
uwzględnia wagi providerów, równoległość, capabilities i cooldown po błędzie startu.
`profile explain NAME` pokazuje aktualną ocenę tras; historia oceny jest w Session.
To przybliżenie obciążenia, nie licznik tokenów, kosztów ani limitów całego konta.
Opcjonalne `max_launches_24h` na trasie ogranicza liczbę lokalnych uruchomień danego
klienta/providera/modelu w projekcie; zero oznacza brak tego limitu.

Opcjonalne `workspaces_dir: /absolute/path/project-workspaces` wybiera katalog poza
repozytorium. Skonfiguruj go przed tworzeniem workspaces; zmiana nie przenosi istniejących
katalogów. Można nadal wskazać projekt przez `--project`; CWD w zewnętrznym workspace
również zawiera informację umożliwiającą jego odkrycie.

## Rozpoczęcie pracy

Podaj agentowi ticket lub opis i użyj skilla `workspace`, albo wykonaj:

```sh
workspace create --issue https://github.com/OWNER/REPO/issues/142 \
  --workflow plan-first --operation-key issue-142
# Opis można też podać bezpośrednio jako argument albo przez --input-file issue.md.
workspace create "Improve workspace creation" --workflow plan-first
# --input-file można opcjonalnie połączyć z --issue URL, aby zachować źródło.
workspace start --workspace ws_ID_Z_ODPOWIEDZI --operation-key orchestrator-1
workspace attach --workspace ws_ID_Z_ODPOWIEDZI
```

`create` utrwala opis; `start` uruchamia supervisora i orkiestratora bez zmiany widoku.
`attach` przełącza istniejącego klienta tmux lub dołącza z zewnątrz. Bez `--workflow`
powstaje stan `needs_workflow`; orkiestrator pyta o wybór przed delegowaniem.
[Trackery i snapshoty](docs/trackers.md).

Powrót do istniejącego workspace nie wymaga pamiętania ID:

```sh
workspace list --short
workspace open
# albo bez menu:
workspace open "specification"
workspace open ws_ID
```

Globalne `--short` wypisuje zwięzłe podsumowanie dla każdej komendy. Dla nazwanych
zasobów (workspace, agentów, zadań i worktree) jest to mapa `nazwa: ID`; rekordy bez
naturalnej nazwy zachowują najważniejsze identyfikatory i stan. `list --map` pozostaje
aliasem `list --short`. `open` pokazuje interaktywny selector z tytułem, stanem, fazą,
ID i źródłem wejścia, po czym dołącza do sesji tmux.

Sesja tmux odpowiada workspace, okno worktree, panel konkretnej Session. Orkiestrator
ma własne okno w katalogu workspace. Odłączenie użytkownika nie zatrzymuje procesów.

## Zadania i wyniki

Orkiestrator definiuje zadania z celem, rolą, kryteriami akceptacji i zależnościami:

```yaml
name: planning
title: Diagnoza issue
goal: Wyjaśnij przyczynę i zaproponuj plan realizacji.
role: planner
acceptance_criteria:
  - Diagnoza wskazuje dowody z kodu.
  - Plan zawiera zadania i testy akceptacyjne.
required_artifacts: [PLAN.md]
```

```sh
workspace task create --spec-file planning.yaml --operation-key planning-task
workspace agent create planner --role planner
workspace worktree create planning --purpose planning
workspace session start --agent planner --task task_ID --worktree planning \
  --operation-key planning-start-1
workspace status
workspace menu
```

Klient otrzymuje IDs agenta, sesji, zadania, worktree, rodzica i orkiestratora w
`WORKSPACE_*`. Wiadomości adresuje się do **Agent ID**. Wykonawcy zapisują lokalne
produkty i przekazują je przez `handoff submit`; CLI kopiuje jawnie wskazane pliki do
`artifacts/`. ACK wiadomości i akceptacja zadania są osobnymi decyzjami.

```sh
workspace message send --to agent_PARENT --kind question --body-file question.md
workspace inbox list
workspace inbox read msg_ID
workspace inbox ack msg_ID
workspace check run --operation-key tests-attempt-1 -- npm test
workspace handoff submit --task task_ID --to agent_PARENT \
  --summary-file work-products/SUMMARY.md \
  --artifact work-products/IMPLEMENTATION.md --check check_ID
workspace handoff accept handoff_ID
workspace workflow advance
```

`check run` zwraca receipt; odczytaj jego `exit_code`. Rejestrowanie receipt nie oznacza
sukcesu testu. Akceptacja sprawdza kryteria workflow, wymagane artefakty i wyniki.
[Dowody testów](docs/checks.md).

Orkiestrator deleguje integrację do osobnego wykonawcy po `integration prepare`.
`change-request prepare/publish/sync` zachowują rewizję i identyfikator CR. Po publikacji
menu proponuje live testing z osobnym profilem i sesją w zintegrowanym worktree.
Test albo jawne pominięcie prowadzi do oczekiwania na release. Sam merge nie kończy
workflow; `release confirm --reference REF` zapisuje potwierdzenie użytkownika.
`--user-confirmed` w sesji orkiestratora oznacza przekazanie rzeczywiście otrzymanej
odpowiedzi, nie samodzielne udzielenie zgody przez model.

## Wznowienia i stan

`agent resume NAME` zachowuje personę i tworzy nową Session, preferując dostępny native
thread. `pause` wstrzymuje delegowanie, `pause --interrupt` zatrzymuje aktywne sesje,
`resume` odblokowuje pracę. `reconcile` uzgadnia utracone panele i przerwane operacje.
`archive` i `clean --dry-run` są osobne od potwierdzenia release’u.
[Runtime, komunikacja i sprzątanie](docs/runtime.md).

WORKSPACE.md jest kanonicznym stanem workflow; `.runtime/index.json` zawiera rejestry
operacyjne. Aktualizuj opis przez `state update --expected-revision N`; `state edit`
służy do kontrolowanej edycji w stanie paused. Zmiana inputu i migracja templates
zachowują historię i unieważniają zależne wyniki: [rewizje](docs/revisions.md).

`--json` zwraca pełne `{ok,data}` lub `{ok:false,error:{code,message}}` i ma pierwszeństwo
przed `--short`. Mutacje z kluczem
zawierają także `operation_id` i rewizję workspace. `--non-interactive` zwraca
brakujące decyzje do rozstrzygnięcia przez użytkownika. `--operation-key` zachowuje
wynik logicznej operacji; ponowienie z innym payloadem zwraca konflikt.
[Kontrakt ponowień i rewizji](docs/operations.md). Nie edytuj rejestrów ani
WORKSPACE.md poza CLI podczas aktywnej pracy.

## Weryfikacja

```sh
go test ./...
go vet ./...
WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s
go build -o bin/workspace ./cmd/workspace
python3 scripts/check-install.py bin/workspace
```

Testy używają izolowanych repozytoriów i prywatnych serwerów tmux. Pełny test workflow
uruchamia deterministyczne procesy agentów, rzeczywiste Git/CLI/handoff/check i testera.
Forge oraz model są fixture’ami; testy nie publikują zewnętrznych PR ani nie zużywają
kredytów modelowych. Test protokołu Codex sprawdza wybudzanie i native resume.

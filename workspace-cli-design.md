# Projekt CLI `workspace`

Status: propozycja architektury i kontraktu CLI, przed implementacją.

`workspace` zarządza trwałym kontekstem zadania, worktrees, sesjami agentów i komunikacją między nimi. Orkiestrator podejmuje decyzje na podstawie `WORKFLOW.md` i `WORKSPACE.md`, a CLI wykonuje i rejestruje operacje. tmux zapewnia widoczność i interaktywny dostęp do uruchomionych klientów agentów.

Zakres pierwszej wersji: lokalny projekt Git, jeden komputer, tmux, workflow `issue-resolution`, wymienne klienty agentów. Środowisko wykonawcze: Linux/macOS lub Linux w WSL. Praca na wielu repozytoriach i zdalne workery pozostają poza MVP.

## 1. Model domenowy

| Obiekt | Znaczenie |
|---|---|
| Project | Zarejestrowany katalog projektu Git, konfiguracja klientów, modeli i templates. |
| Workspace | Trwały kontekst jednej inicjatywy; żyje aż do potwierdzenia wdrożenia/release'u przez użytkownika. |
| Worktree | Checkout Git przypisany do workspace. Może służyć planowaniu, implementacji, integracji lub testowaniu. |
| Task | Jednostka delegowanej pracy: cel, zależności, kryteria akceptacji, wejścia i wyniki. |
| Agent | Template/definicja persony agenta, np. orkiestratora, planisty lub wykonawcy, ze stabilną tożsamością w workspace. |
| Session | Trwały logiczny kontekst rozmowy w jednym agent/task/attempt/input/worktree lineage; może być idle bez panelu. |
| Run | Jedno konkretne uruchomienie klienta i panelu tmux; token ownership, jednostka routingu i dokładnego provenance. |
| Handoff | Trwałe przekazanie wyniku sesji do agenta będącego odbiorcą. |
| Artifact | Niezmienna kopia produktu pracy, z pochodzeniem i sumą kontrolną. |
| Change request | Zewnętrzny PR/MR lub lokalnie przygotowany materiał do jego utworzenia. |

Project ma wiele workspaces; workspace ma wiele tasks, worktrees i agents; Agent ma wiele logicznych Sessions, a Session wiele kolejnych Runs. Task może wymagać kilku sesji, a jeden worktree może obsługiwać kilka zadań. Wyniki zawsze wskazują task, Session, dokładny Run i konkretną rewizję Git.

Agent definiuje personę: rolę, instrukcje, template promptu i domyślny profil modelu. Session utrwala wersję persony i kontekst semantyczny; Run utrwala trasę/model, argv, CWD, prompt, client state, pane/window i wynik procesu. Zmiana definicji agenta nie zmienia istniejących Sessions. Wiadomości są adresowane do stabilnego ID agenta, a pochodzenie wskazuje Session i Run.

Identyfikatory są niezmienne. Nazwy i slugi służą prezentacji. Przykłady `ws_01…`, `agent_01…`, `sess_01…` są skrócone; implementacja używa pełnych ULID. Nazwa brancha zawiera identyfikator workspace i zadania.

**Niezmienniki:**

- W workspace działa najwyżej jeden aktywna sesja orkiestratora; dzierżawa z numerem generacji odrzuca operacje starego procesu.
- W jednym worktree działa najwyżej jedna sesja z prawem zapisu. Pozostałe sesje pracują sekwencyjnie albo są czytelnikami.
- Proces zakończony kodem zero nie oznacza zaakceptowanego wyniku.
- Wysłany handoff nie oznacza zakończonego taska: wynik akceptuje orkiestrator.
- Wszystkie wznowienia korzystają z trwałego stanu i artefaktów. Historia czatu jest pomocnicza.

Dzierżawa zapisu koordynuje klientów `workspace`; sama nie jest sandboxem dla dowolnych procesów uruchomionych poza narzędziem.

## 2. Struktura projektu

```text
project/
├── .workspace/
│   ├── config.yaml
│   ├── templates/
│   │   ├── WORKSPACE.md.tmpl
│   │   ├── orchestrator.AGENTS.md.tmpl
│   │   ├── worker.AGENTS.md.tmpl
│   │   └── workflows/issue-resolution/
│   │       ├── WORKFLOW.md.tmpl
│   │       └── prompts/
│   │           ├── planning.md.tmpl
│   │           ├── implementation.md.tmpl
│   │           ├── integration.md.tmpl
│   │           └── live-testing.md.tmpl
│   ├── .runtime/                 # rejestr projektu, routing, lokalne blokady
│   └── ws_01-fix-checkout/
│       ├── AGENTS.md
│       ├── WORKSPACE.md
│       ├── WORKFLOW.md
│       ├── inputs/issue.md
│       ├── prompts/              # utrwalone prompty, bez ponownego renderowania przy resume
│       ├── tasks/task_01.md
│       ├── artifacts/
│       │   └── art_01/PLAN.md
│       ├── worktrees/
│       │   ├── planning/
│       │   ├── checkout-api/
│       │   └── integration/
│       └── .runtime/
│           ├── events/           # zatwierdzone zdarzenia i operacje do uzgodnienia
│           ├── inbox/            # trwałe wiadomości, indeksowane per odbiorca
│           ├── agents/             # definicje person agentów
│           ├── sessions/           # logiczne sesje i historia ich Runów
│           ├── handoffs/
│           └── locks/
└── …
```

Konfiguracja i templates mogą być wersjonowane. Katalogi `ws_*` oraz `.runtime/` są lokalnie ignorowane przez Git. `project init` przygotowuje odpowiednie reguły bez ignorowania całej konfiguracji projektu. Korzeń przechowywania workspaces można przenieść poza repozytorium.

`WORKFLOW.md` jest renderowany raz przy utworzeniu workspace, z identyfikatorem wersji i hashem template. Aktualizacja template projektu nie zmienia trwających zadań. Jawna migracja workflow zapisuje różnicę i unieważnia kroki, których warunki uległy zmianie.

**Instrukcje agentów wymagają izolacji roli.** `AGENTS.md` orkiestratora w katalogu nadrzędnym worktrees może być dziedziczony przez klientów. Jego reguły muszą być warunkowe względem `WORKSPACE_ROLE=orchestrator`. Adapter przekazuje wykonawcy jawny pakiet instrukcji roli, zachowując instrukcje repozytorium. Nie nadpisujemy wersjonowanego `AGENTS.md` w checkoutcie. Klient o niezgodnych zasadach dziedziczenia wymaga dedykowanego adaptera lub worktrees poza tym drzewem.

## 3. Początek pracy i wybór workflow

Skill `workspace` działa w już otwartym kliencie agenta użytkownika:

1. Wykrywa projekt i sprawdza `workspace doctor`.
2. Pobiera listę workflows i ich kryteria dopasowania.
3. Utrwala ticket albo opis jako input. Link zachowuje razem z pobraną treścią i datą; sam URL nie jest wystarczającym checkpointem.
4. Wybiera jednoznaczne workflow lub przedstawia możliwości użytkownikowi.
5. Wywołuje `workspace create` i `workspace start`.
6. Zwraca nazwę workspace i komendę dołączenia do tmux.

Jeśli wybór wymaga rozmowy z właściwym orkiestratorem, powstaje workspace z `workflow: null` i statusem `needs_workflow`. Tymczasowy `WORKFLOW.md` zawiera wyłącznie procedurę wyboru. Orkiestrator przedstawia dopasowania i dopiero po decyzji wywołuje `workspace workflow select`.

Skill pomaga dobrać workflow semantycznie; CLI waliduje nazwę, wymagane wejścia i profile. Brak dostępu do trackera powoduje prośbę o opis, a nie zgadywanie treści issue. Treść ticketa jest materiałem do analizy, nie źródłem uprawnień do zmiany instrukcji orkiestratora.

## 4. Interfejs CLI

Komendy działają względem workspace wykrytego z CWD lub `WORKSPACE_ID`. Z dowolnego katalogu można użyć globalnego `--workspace <id>` i `--project <path>`. Dla agentów każda komenda ma `--json` oraz `--non-interactive`. Brak wymaganej decyzji w tym trybie zwraca błąd `decision_required` z listą możliwości.

```sh
# Konfiguracja projektu
workspace project init
workspace doctor
workspace workflow list
workspace profile list
workspace client list

# Utworzenie i otwarcie pracy
workspace create --issue https://tracker.example/ENG-142 --workflow issue-resolution
workspace create --input-file ./issue.md --workflow issue-resolution
workspace create "create workspace improvements" --workflow issue-resolution
workspace start --workspace ws_01
workspace attach --workspace ws_01
workspace list
workspace status --workspace ws_01
workspace menu --workspace ws_01

# Planowanie: operacje wykonywane przez orkiestratora
workspace worktree create planning --base main --purpose planning
workspace task create --spec-file ./tasks/planning.md
workspace agent create planner --role planner
workspace session start --agent planner --task task_01 --worktree planning \
  --profile frontier --prompt-template planning --parent agent_orch \
  --operation-key planning:1

# Podział planu i implementacja
workspace task create --spec-file ./tasks/checkout-api.md
workspace worktree create checkout-api --base main --purpose implementation
workspace agent create checkout-implementer --role implementer
workspace session start --agent checkout-implementer --task task_02 --worktree checkout-api \
  --profile implementation --prompt-template implementation --parent agent_orch \
  --operation-key implementation:task_02:1

# Trwała komunikacja
workspace message send --to agent_orch --body-file ./question.md --kind question
workspace inbox list
workspace inbox read msg_01
workspace inbox ack msg_01
workspace handoff submit --to agent_orch --task task_01 \
  --summary-file ./work-products/SUMMARY.md --artifact ./work-products/PLAN.md \
  --operation-key result:task_01:1
workspace handoff accept handoff_01
workspace handoff reject handoff_01 --reason-file ./feedback.md

# Cykl życia i obsługa awarii
workspace agent list
workspace session list
workspace session attach sess_01
workspace session history sess_01
workspace run list
workspace run inspect run_01
workspace agent resume agent_01
workspace session stop sess_01
workspace session close sess_01 --reason completed
workspace pause
workspace resume
workspace reconcile
workspace task retry task_02

# Integracja i publikacja wyników
workspace integration prepare --tasks task_02,task_03 --base main
workspace change-request prepare --worktree integration --target main
workspace change-request publish cr_01
workspace change-request sync
workspace release confirm --reference release-2026.09.10
workspace archive
workspace clean --dry-run
```

`create` nie otwiera klienta; `start` uruchamia orkiestratora w tmux, ale nie przełącza automatycznie widoku użytkownika. `attach` przełącza klienta tmux, jeżeli polecenie już działa wewnątrz tmux, albo dołącza z zewnątrz. Dzięki temu skill nie porzuca przypadkowo swojej rozmowy w połowie odpowiedzi.

`task create` przyjmuje specyfikację z tytułem, celem, kryteriami akceptacji, zależnościami, bazą Git, profilem i wymaganymi artefaktami. Specyfikacja jest walidowana i kopiowana do workspace.

Komendy mutujące przyjmują `--operation-key`. Ponowienie tej samej operacji z tym samym payloadem zwraca poprzedni wynik; inny payload z tym samym kluczem kończy się konfliktem. Przykładowa odpowiedź:

```json
{
  "ok": true,
  "data": {"agent_id": "agent_01", "session_id": "sess_01", "run_id": "run_01", "state": "starting"},
  "operation_id": "op_01",
  "revision": 14
}
```

CLI zwraca zaakceptowanie uruchomienia, nie obietnicę ukończenia pracy. Błędy mają stabilne kody, m.in. `decision_required`, `revision_conflict`, `worktree_busy`, `client_unavailable`, `no_route`, `artifact_invalid`.

## 5. Trwały stan: `WORKSPACE.md`

`WORKSPACE.md` jest kanonicznym stanem semantycznym: workflow, tasks, decyzje, zaakceptowane wyniki i warunki zakończenia. Frontmatter jest maszynowy, treść opisowa służy przekazaniu kontekstu kolejnemu orkiestratorowi. Rejestry w `.runtime/` zawierają szczegóły uruchomień, dostarczenia wiadomości oraz metadane operacji, nie drugą niezależną wersję stanu workflow.

```yaml
---
schema_version: 1
id: ws_01
project_id: prj_01
title: Naprawa ponawiania płatności
revision: 14
status: active
workflow:
  id: issue-resolution
  version: 1
  template_digest: sha256:...
  phase: implementing
input:
  source: https://tracker.example/ENG-142
  snapshot: inputs/issue.md
base:
  ref: main
  commit: abc123...
orchestrator:
  agent_id: agent_orch
tasks:
  - id: task_01
    role: planner
    state: accepted
    worktree: planning
    accepted_handoff: handoff_01
  - id: task_02
    role: implementer
    state: running
    depends_on: [task_01]
    worktree: checkout-api
artifacts:
  - id: art_01
    kind: plan
    path: artifacts/art_01/PLAN.md
    digest: sha256:...
    source_handoff: handoff_01
change_requests: []
pending_decision: null
release:
  user_confirmed: false
  reference: null
---

## Cel i kryteria akceptacji
...

## Aktualny obraz sytuacji
...

## Decyzje i uzasadnienia
...

## Ryzyka, pytania i następny krok
...
```

Agent aktualizuje stan przez `workspace state update --patch-file ... --expected-revision 14`, nie przez nadpisanie całego pliku. CLI sprawdza dozwolone przejścia, zależności i prawa roli. Wykonawcy zgłaszają wyniki i blokady, ale nie akceptują swoich tasks ani nie zmieniają fazy workflow.

Zapis: blokada workspace → sprawdzenie rewizji → zapis intencji operacji → plik tymczasowy i atomowa podmiana → oznaczenie operacji jako zatwierdzonej. Reconcile rozpoznaje przerwany zapis po identyfikatorze operacji i rewizji. Zewnętrzne efekty, jak Git, tmux i PR, mają osobny stan `pending/applied/confirmed`; nie da się objąć ich jedną transakcją plikową.

Ręczna edycja jest możliwa w stanie paused, przez `workspace state edit`, z walidacją przy zapisie. Nieznany hash pliku podczas pracy zatrzymuje mutacje do uzgodnienia zmian. Treść opisowa nie może przestawiać stanu sprzecznie z frontmatterem.

## 6. tmux i sesje agentów

```text
tmux session: ws-project-fix-checkout-01
├── window 0: orchestrator       cwd: katalog workspace
│   └── pane: bieżący Run orkiestratora
├── window 1: planning           cwd: worktrees/planning
│   └── pane: planista
├── window 2: checkout-api       cwd: worktrees/checkout-api
│   ├── pane: implementer
│   └── pane: reviewer           po zwolnieniu zapisu lub w trybie odczytu
└── window 3: integration        cwd: worktrees/integration
    ├── pane: integrator/tester  kolejno, bez dwóch aktywnych zapisów
    └── pane: serwer aplikacji
```

Jedyny wyjątek od „okno = worktree” to okno orkiestratora, który pracuje w katalogu workspace. Panel usługi pomocniczej ma typ `service`, nie tożsamość agenta. Czytelnik w aktywnie zmienianym worktree widzi ruchomy stan; review wymagające stabilnej rewizji dostaje osobny checkout.

Mapowanie zapisuje identyfikatory logicznej Session i konkretnego Run na panelu oraz identyfikatory okna i panelu; nazwy i indeksy okien mogą się zmieniać. Zniszczenie panelu przerywa Run, ale nie usuwa Session, agenta ani wyników. Odłączenie użytkownika od tmux niczego nie zatrzymuje. Podział tmux session → windows → panes wynika z modelu [tmux](https://man.openbsd.org/tmux).

Każda sesja otrzymuje środowisko:

```text
WORKSPACE_PROJECT_ID=prj_01
WORKSPACE_ID=ws_01
WORKSPACE_DIR=/project/.workspace/ws_01-fix-checkout
WORKSPACE_AGENT_ID=agent_worker
WORKSPACE_SESSION_ID=sess_01
WORKSPACE_RUN_ID=run_01
WORKSPACE_ORCHESTRATOR_ID=agent_orch
WORKSPACE_PARENT_AGENT_ID=agent_orch
WORKSPACE_TASK_ID=task_02
WORKSPACE_ROLE=implementer
WORKSPACE_WORKTREE_ID=wt_02
```

`agent_id` jest adresem komunikacji workspace. `session_id` identyfikuje logiczny kontekst, `run_id` właściciela bieżącego runtime, `client_thread_id` opcjonalny binding adaptera, a `pane_id` jedynie lokalizację terminala. Rodzic delegujący pracę i orkiestrator workspace są osobnymi polami, nawet jeśli na początku mają tę samą wartość.

Agent orkiestratora po restarcie zachowuje `agent_id` i zgodny `session_id`; powstaje nowy `run_id`. Wznowienie natywnego wątku jest preferowane, ale gdy klient go nie obsługuje, nowy Run otrzymuje bootstrap z bieżącym stanem, workflow, decyzjami i nieodebranymi wiadomościami. Nowa Session powstaje przy zmianie agenta, task/attempt, input digest, worktree lub native thread lineage.

| Stan Session | Znaczenie |
|---|---|
| `active` | istnieje dokładnie jeden `current_run_id` w stanie starting/running |
| `idle` | brak procesu/panelu; zgodny kontekst można wznowić |
| `closed` | kontekst logiczny zamknięty, bez dalszych Runs |

| Stan Run | Znaczenie |
|---|---|
| `starting` / `running` | zarezerwowany launch / proces przejął ownership |
| `exited` / `failed` | proces zakończył się normalnie / błędem |
| `stopped` | jawnie zatrzymany, bez autorestartu |
| `interrupted` | pane utracony lub Run zastąpiony; Session pozostaje resumable |

Rejestr `.runtime/index.json` ma `schema_version: 2` i osobne `sessions` oraz `runs`.
Migracja v1→v2 zachowuje stare `sess_*` jako Run IDs/aliasy i konserwatywnie grupuje
tylko rekordy o tym samym Agent ID, niepustym native thread oraz zgodnym adapterze,
task/attempt/input/worktree. Rekordy bez thread ID pozostają osobnymi Sessions.
Aktualizacja przechodzi przez istniejący write-ahead/atomic recovery; ponowienie jest
idempotentne. Aktywny legacy runner pozostaje rozpoznawalny przez zachowany Run alias.

## 7. Generyczne klienty i niezawodne dostarczenie

Sama komenda uruchomienia dowolnego CLI zapewnia start procesu. Nie gwarantuje wysłania wiadomości do zajętej rozmowy, rozpoznania końca odpowiedzi ani wznowienia konwersacji. Te możliwości są jawne w adapterze:

```text
launch(cwd, env, model, prompt_file) -> process/session
resume(client_thread_id, bootstrap) -> process/session     [opcjonalne]
deliver(client_thread_id, message_file) -> receipt     [opcjonalne]
observe(session) -> lifecycle events                      [opcjonalne]
interrupt(session)                                        [opcjonalne]
```

Konfiguracja uruchomienia jest tablicą argumentów, nie interpolowanym poleceniem shell. Adapter zamienia model i prompt na format obsługiwany przez klienta. Dla klienta przyjmującego tylko tekst promptu adapter odczytuje plik. Duży input pozostaje w plikach, do których bootstrap kieruje agenta.

Obsługiwane poziomy:

- `command`: generyczny launcher; agent może używać `workspace inbox` i `handoff` przez terminal.
- `interactive`: dodatkowo potrafi bezpiecznie dostarczyć wiadomość do gotowej rozmowy.
- `native`: dodatkowo raportuje zdarzenia i identyfikator wątku, wspiera resume.

Przykładowe klienty docelowe to opencode, codex i claude; dokładne flagi i wykrywanie możliwości należą do wersjonowanych adapterów, nie do workflow.

Projektowy supervisor `workspace serve`, automatycznie uruchamiany przy `start`, pilnuje procesów, wiadomości, dzierżaw i powiadomień tmux. Nie wykonuje decyzji workflow. Po awarii odczytuje trwałe kolejki i uzgadnia istniejące procesy.

Handoff najpierw trafia do trwałego inboxa. Adapter z `deliver` może obudzić orkiestratora. Bez tej funkcji orkiestrator używa blokującego `workspace inbox wait --timeout 30` między turami pracy, a tmux pokazuje powiadomienie. Jeśli rozmowa już czeka na użytkownika i klient nie umie przyjąć zdarzenia, wiadomość pozostaje w kolejce do następnej tury; nie obiecujemy automatycznego wybudzenia. Pełna autonomiczna orkiestracja wymaga adaptera z dostarczeniem lub klienta działającego w pętli obsługiwanej przez adapter.

Nie używamy wpisywania tekstu w nieznany stan terminala jako protokołu wiadomości. Początkowy prompt przekazuje launcher; późniejsze wiadomości idą przez adapter albo świadomy odczyt inboxa.

## 8. Handoff i artefakty

Wykonawca zapisuje produkty w swoim worktree, np. w lokalnie ignorowanym `work-products/`. Kod produktu pozostaje normalnie wersjonowany. Każdy workflow definiuje wymagane produkty i ich format.

```yaml
id: handoff_01
from_agent: agent_worker
from_session: sess_01
from_run: run_07
to_agent: agent_orch
task_id: task_02
attempt: 1
outcome: succeeded              # succeeded | blocked | failed
summary: Dodano obsługę ponowień oraz test regresji.
git:
  worktree_id: wt_02
  base_commit: abc123...
  head_commit: def456...
  dirty: false
artifacts:
  - id: art_02
    kind: report
    path: artifacts/art_02/IMPLEMENTATION.md
    digest: sha256:...
checks:
  - command: npm test
    exit_code: 0
    evidence_artifact: art_03
risks: []
requested_action: review_result
```

`handoff submit` waliduje źródła, kopiuje pliki do tymczasowego katalogu artefaktów, oblicza hashe, atomowo finalizuje manifest i dopiero wtedy publikuje wiadomość. Wynik `submit` zawiera identyfikator i ścieżkę zachowanego artefaktu — to stabilny link dla orkiestratora. Pliki źródłowe pozostają w worktree.

Ścieżki są rozwiązywane względem przypisanego worktree; symlinki wychodzące poza dozwolony katalog i traversal są odrzucane. Obowiązuje limit rozmiaru, jawna lista plików i brak automatycznego archiwizowania całego checkoutu. Manifest przypisuje wynik do sesji, modelu, bazy Git, promptu i taska.

Dostarczenie jest co najmniej jednokrotne, a obsługa deduplikuje po ID. ACK oznacza odbiór; `handoff accept` oznacza decyzję merytoryczną. Przerwany odbiór może być ponowiony bez ponownego wykonania zadania. Wynik starej próby po retry jest oznaczony jako spóźniony i nie nadpisuje aktualnej próby.

Akceptacja implementacji wymaga wskazania commitów oraz wymaganych dowodów testów. Brudny checkout może dostarczyć raport blokady, ale nie zaakceptowany wynik gotowy do integracji. Samo oświadczenie modelu o testach nie zastępuje zachowanych wyników poleceń.

## 9. Profile modeli i routing

Profil określa zastosowanie i zbiór tras. Trasa wiąże klienta, model oraz providera; ta sama nazwa modelu w dwóch klientach nie musi oznaczać tej samej możliwości.

```yaml
schema_version: 1
runtime: tmux
clients:
  client-a:
    adapter: command
    launch_argv: ["/usr/local/bin/my-agent-wrapper", "--model", "{model}", "--prompt-file", "{prompt_file}"]
    capabilities: [launch]
profiles:
  frontier:
    strategy: provider-balanced
    provider_weights: {provider-a: 1, provider-b: 1}
    routes:
      - {id: frontier-a, client: client-a, provider: provider-a, model: MODEL_A, max_concurrency: 2}
      - {id: frontier-b, client: client-a, provider: provider-b, model: MODEL_B, max_concurrency: 2}
  implementation:
    strategy: provider-balanced
    routes:
      - {id: implementation-a, client: client-a, provider: provider-a, model: MODEL_C, max_concurrency: 4}
  live-testing:
    strategy: provider-balanced
    routes:
      - {id: testing-b, client: client-a, provider: provider-b, model: MODEL_D, max_concurrency: 1}
workflows:
  issue-resolution:
    profiles:
      orchestrator: frontier
      planning: frontier
      implementation: implementation
      integration: implementation
      live-testing: live-testing
    max_parallel_tasks: 3
    change_requests: integrated
```

To konfiguracja ilustrująca kontrakt własnego wrappera; `MODEL_A` itd. wymagają zastąpienia identyfikatorami obsługiwanymi przez zainstalowane klienty.

Algorytm MVP:

1. Odfiltruj trasy niedostępne, w cooldownie, bez wymaganych możliwości lub z wyczerpaną równoległością/budżetem.
2. Wybierz providera o najmniejszej wartości `(uruchomienia_w_oknie + rezerwacje) / waga`. Okno domyślne: 24 h. Brak wagi oznacza 1.
3. Wybierz najmniej zajętą trasę tego providera; remis rozstrzygnij deterministycznym round-robin.
4. Atomowo zarezerwuj trasę przed uruchomieniem. Udany start zamienia rezerwację w licznik uruchomienia; nieudany zwalnia ją i zapisuje błąd.

Liczniki są projektowe, współdzielone przez workspaces. Wybór po providerze zapobiega faworyzowaniu providera tylko dlatego, że ma więcej modeli w profilu. Historia decyzji zawiera rozważane trasy i powód wyboru, dostępne przez `workspace profile explain frontier`.

Liczba uruchomień jest przybliżeniem obciążenia, nie miarą tokenów ani kosztu. MVP pokazuje to wprost. Rozliczanie tokenów/kosztów wymaga telemetrii wszystkich porównywanych tras; brak danych nie jest zerowym użyciem. Limity kont współdzielonych z innymi narzędziami nie są znane bez osobnej integracji.

Model jest przypięty do konkretnej sesji. Resume natywnego wątku domyślnie zachowuje trasę. Fallback po błędzie startu jest możliwy przed rozpoczęciem pracy; po rozpoczęciu wymaga zatrzymania poprzedniej sesji, checkpointu i nowej sesji. Nigdy nie uruchamiamy drugiego wykonawcy na tym samym zadaniu tylko dlatego, że pierwszy milczy.

## 10. `issue-resolution`: przebieg

```text
needs_workflow → planning → plan_review → implementing → integrating
                                                        ↓
                                  change_requests → live_test_offer
                                                        ↓
                              live_testing / jawne pominięcie
                                                        ↓
                                            awaiting_release → completed
```

`paused`, `blocked` i `needs_attention` to stan operacyjny workspace, niezależny od fazy. Testy i review mogą wracać do implementacji. Zmiana celu może wymagać nowej rewizji planu.

| Krok | Działanie | Warunek ukończenia |
|---|---|---|
| Intake | Utrwal input, cel, kryteria akceptacji, bazę Git i wybór workflow. | Wystarczający opis problemu i rozwiązane pytania blokujące. |
| Planning | Utwórz worktree planning i sesję planisty z profilem `frontier`. | Handoff z `PLAN.md` zaakceptowany przez orkiestratora. |
| Plan review | Orkiestrator analizuje plan, zapisuje decyzje, tworzy DAG zadań. | Tasks mają zakres, kryteria, zależności i strategię integracji. |
| Implementation | Uruchamiaj gotowe tasks w nowych albo przydzielonych worktrees. | Wszystkie wymagane wyniki zaakceptowane; commity i dowody testów zachowane. |
| Integration | Deleguj połączenie zmian i testy integracyjne do wykonawcy. | Jeden wskazany commit integracji z poprawnymi wymaganymi testami. |
| Change requests | Przygotuj/utwórz PR/MR i zapisz zdalne ID. | Utworzone CR odpowiadają zaakceptowanemu zakresowi i rewizji. |
| Live test offer | Zaproponuj test z profilem `live-testing`, środowiskiem i scenariuszami. | Użytkownik wybrał test lub jawne pominięcie z uzasadnieniem. |
| Live testing | Osobna sesja testera na zintegrowanym worktree. | Raport `LIVE_TEST.md`, wskazany commit i wynik scenariuszy. |
| Awaiting release | Obsługuj feedback review/CI, pokaż gotowość, czekaj na informację o wdrożeniu. | Użytkownik potwierdził wdrożenie/release. |
| Completed | Zapisz potwierdzenie i raport końcowy. | Trwałe zamknięcie; archiwizacja i sprzątanie są osobnymi operacjami. |

`PLAN.md` zawiera diagnozę, dowody z kodu, proponowane rozwiązanie, alternatywy, tasks, zależności, ryzyka, strategię integracji i testy akceptacyjne. Planista nie implementuje poprawki. Gdy analiza zmienia się w eksperyment z kodem, oznacza go jako eksperyment i nie przedstawia jako gotowej implementacji.

Domyślnie orkiestrator sam przyjmuje plan i deleguje pracę. Pyta użytkownika, gdy pozostaje wybór produktowy, zmiana zakresu lub blokująca niejasność. Interaktywność oznacza dostępne opcje i pytania w punktach decyzji, a nie obowiązek zatwierdzania każdego utworzenia worktree.

Szablon promptu planisty:

```text
Rola: planner. Task: {{task_id}}. Orkiestrator: {{orchestrator_agent_id}}.
Przeczytaj pakiet instrukcji roli, input {{input_path}} i kryteria taska.
Przeanalizuj problem na bazie {{base_commit}}. Nie implementuj poprawki.
Zapisz work-products/PLAN.md zgodnie ze schematem planu.
Zapisz work-products/SUMMARY.md.
Przekaż oba pliki przez workspace handoff submit do wskazanego orkiestratora.
Jeśli brakuje informacji, wyślij question i zgłoś blokadę zamiast zgadywać.
```

Prompt implementera zawiera zaakceptowany plan jako artefakt, dokładny zakres taska, bazę, zależności, kryteria akceptacji oraz komendę handoffu. Prompt testera zawiera commit integracji, adres środowiska, sposób uruchomienia, scenariusze i format dowodów. Każdy prompt podaje ID odbiorcy wyników.

### Idempotentny `WORKFLOW.md`

Każdy krok określa: stabilne ID, wymagane wejścia i ich hashe, warunek wejścia, operację zapewniającą zasoby, wymagane wyniki, warunek akceptacji i ścieżkę naprawy.

Przykład instrukcji planning:

```text
Jeżeli istnieje zaakceptowany PLAN.md dla bieżącego hasha inputu i bazy Git:
  przejdź do plan_review.
Jeżeli istnieje aktywna próba planningu:
  odbierz jej wiadomości; nie uruchamiaj duplikatu.
Jeżeli istnieje nieodebrany wynik bieżącej próby:
  oceń go, zamiast ponawiać analizę.
W przeciwnym razie:
  zapewnij worktree i task; uruchom sesję ze stabilnym operation-key.
```

Idempotencję egzekwuje CLI poprzez klucze operacji, stan prób i warunki przejść. Tekst workflow mówi agentowi, jak z tego korzystać. Nowy input, baza lub zaakceptowany plan tworzą nową rewizję wykonania i oznaczają zależne wyniki jako wymagające ponownej oceny; nie kasują historii.

## 11. Gałęzie, integracja i change requests

Git worktrees izolują pliki robocze, ale współdzielą repozytorium. Każdy zapisywalny worktree dostaje osobną gałąź, np. `workspace/ws_01/task_02`; nie próbujemy checkoutować tej samej gałęzi w kilku miejscach. Baza jest utrwalonym SHA, nie tylko nazwą `main`. Zasady worktrees opisuje [dokumentacja Git](https://git-scm.com/docs/git-worktree).

Niezależne tasks mogą startować z tej samej bazy. Task zależny czeka na zaakceptowane commity poprzednika i dostaje jawnie wyliczoną bazę. Kilku agentów może pracować na jednym istniejącym worktree po kolei, z checkpointem i przekazaniem dzierżawy.

`integration prepare` tworzy dedykowany worktree i manifest wybranych headów. Integrator łączy zmiany w kolejności zależności i rozwiązuje konflikty jako osobny delegowany task. Orkiestrator nie edytuje kodu. Wynik integracji jest checkpointem o konkretnym SHA, a następne testy odnoszą się do niego.

Domyślnie powstaje jeden zintegrowany PR/MR na issue. Projekt może wybrać wiele CR per task, ze zdefiniowaną kolejnością merge'ów; to zwiększa koszt obsługi zależności. Live testing zawsze testuje pełny zestaw zmian, niezależnie od liczby CR.

Adapter forge zapewnia `prepare`, `publish`, `lookup`, `sync`. Przy ponowieniu publikacji wyszukuje zapisany identyfikator lub znacznik workspace/operacji, zanim utworzy nowy PR/MR. Nieudana odpowiedź sieciowa oznacza stan niepewny do uzgodnienia, nie automatyczny retry tworzenia.

Autoryzacja publikacji jest polityką projektu/workspace: `ask` albo wcześniej udzielone `allowed`. Przy `ask` orkiestrator prezentuje gotowy tytuł, opis i diff. Bez integracji z forge powstaje lokalny pakiet CR; workflow wymaga dołączenia rzeczywistego CR lub jawnej decyzji o pominięciu, a nie udaje publikacji.

Nowy commit po live testing unieważnia status testu poprzedniej rewizji. Merge PR nie kończy workflow. `release confirm` wymaga decyzji użytkownika zapisanej z datą, referencją i rewizją zmiany; wykonawca nie może sam jej wystawić. CLI lokalne nie zapewnia kryptograficznego dowodu ludzkiej decyzji — to jawny kontrakt roli, rejestrowany jako zdarzenie użytkownika.

## 12. Menu orkiestratora

Menu jest generowane z aktualnego stanu przez `workspace menu --json`; orkiestrator przedstawia je językiem rozmowy. Każda opcja ma stabilne ID akcji, etykietę, warunki dostępności i rewizję stanu. Odpowiedź użytkownika dotyczy zapisanego pytania; po zmianie rewizji warunki są sprawdzane ponownie.

Przykład w trakcie implementacji:

```text
ENG-142 · implementacja · 2/3 zadania przyjęte
API gotowe. Test regresji trwa. Integracja czeka na wynik testów.

1. Pokaż plan i podział pracy
2. Otwórz sesję wykonawcy testów
3. Pokaż artefakty i wyniki weryfikacji
4. Zmień zakres lub dodaj uwagę
5. Wstrzymaj delegowanie nowych zadań

Możesz też napisać własne polecenie.
```

Przykład po utworzeniu CR:

```text
PR gotowy. Zintegrowany commit: def456. Testy automatyczne przeszły.
Proponuję live testing ścieżki płatności profilem live-testing.

1. Uruchom live testing
2. Ustal środowisko i scenariusze
3. Pomiń live testing i zapisz powód
4. Wróć do implementacji
```

`pause` zatrzymuje nowe delegacje, pozwalając aktywnym sesjom dostarczyć wyniki. `pause --interrupt` dodatkowo prosi adaptery o przerwanie aktywnej pracy i zachowuje stan do uzgodnienia. Wyciszenie powiadomień, zatrzymanie procesu i zakończenie workflow to osobne działania.

## 13. Awarie i odtwarzanie

| Sytuacja | Zachowanie |
|---|---|
| Użytkownik odłącza tmux | Praca trwa, stan i inbox pozostają dostępne. |
| Pada klient wykonawcy | Run oznaczony jako interrupted; Session pozostaje idle/resumable, task nie staje się accepted. Najpierw sprawdzenie commitów i handoffów. |
| Pada orkiestrator | Wyniki trafiają do jego trwałego inboxa; resume odtwarza kontekst pod tym samym ID. |
| Pada tmux lub komputer | Reconcile odnajduje worktrees i wyniki, oznacza utracone Runs. Odtwarza layout i uruchamia nowe Runs w zgodnych Sessions. |
| Timeout providera | Rejestracja błędu i cooldown; bez równoległego duplikowania niepewnej sesji. |
| Worktree usunięty ręcznie | Błąd wymagający uwagi, zachowane artefakty dostępne; brak cichego odtworzenia pustego checkoutu. |
| Brudny worktree przy retry | Zachowaj pliki, pokaż różnicę, deleguj odzyskanie pracy; nie stosuj reset --hard. |
| Powtórzony handoff | Deduplikacja; brak podwójnej akceptacji. |
| PR opublikowany, brak odpowiedzi | Lookup po zapisanej tożsamości operacji; brak drugiego PR. |

`clean --dry-run` pokazuje plan sprzątania. Wykonanie odmawia usunięcia aktywnych, brudnych albo zawierających niezabezpieczone commity worktrees. Po archiwizacji pozostają dokumenty, artefakty i historia decyzji. Worktrees usuwa mechanizm Git; usuwanie katalogów nie zastępuje operacji Git.

## 14. Podział implementacji i MVP

Proponuję pojedynczy binarny program w Go: CLI, supervisor, walidacja stanu i adapter tmux. Git i klienty agentów są procesami zewnętrznymi. Nie potrzeba osobnego serwera sieciowego; supervisor udostępnia lokalny socket. Rozszerzenia klientów i forge mogą być zewnętrznymi programami z wersjonowanym protokołem JSON.

Moduły: `project`, `state`, `workflow`, `task`, `git`, `runtime/tmux`, `client`, `routing`, `mailbox`, `artifact`, `forge`. Backend trwałości zaczyna od plików, blokad i atomowych podmian. Migracja indeksów operacyjnych do SQLite nie może zmienić roli `WORKSPACE.md` jako kanonicznego stanu workflow.

Pierwszy pionowy zakres:

1. `project init`, `create`, templates i walidowany stan workspace.
2. Worktrees, tmux i jeden adapter klienta, ze stabilnymi Agent/Session IDs oraz osobnym Run fencing.
3. Inbox, handoff, kopiowanie artefaktów i resume orkiestratora.
4. `issue-resolution`: planista → wykonawca → integrator → oferta live testing → potwierdzenie release.
5. Profile i routing providerów, następnie dodatkowe klienty i adapter forge.

Kryteria odbioru obejmują przejście całego workflow oraz próby awarii: podwójny `session start` nie uruchamia drugiego agenta; ponowiony handoff tworzy jeden wynik; restart orkiestratora nie gubi inboxa; przerwany zapis odtwarza spójną rewizję; dwie sesje nie dostają równocześnie dzierżawy zapisu; stare wyniki nie zamykają nowej próby; niepewna publikacja nie tworzy drugiego CR; live test jest związany z SHA; workspace nie kończy się bez potwierdzenia release przez użytkownika.

Pełny tryb automatyczny wymaga przynajmniej jednego adaptera potrafiącego wybudzić orkiestratora. Generyczny launcher pozostaje obsługiwany z jawnie ograniczoną automatyzacją komunikacji.

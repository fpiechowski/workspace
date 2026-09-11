# Audyt zgodności z planem

Zakres odniesienia: `workspace-cli-design.md`, commit `59f7fb1`. Agent jest
trwałą definicją persony, Session — konkretnym uruchomieniem. Poniższa macierz
obejmuje wszystkie 14 części planu i pierwszy workflow `issue-resolution`.

| Część planu | Implementacja i sprawdzone zachowanie | Dowody |
|---|---|---|
| 1. Model domenowy | Project, Workspace, Worktree, Task, Agent, Session, Handoff, Artifact i CR; stałe ULID, snapshot persony, jedna aktywna sesja persony i jeden writer worktree; oddzielna akceptacja wyniku | `model.go`, `session.go`, `core_test.go`, `readonly_test.go`, `handoff_test.go` |
| 2. Struktura projektu | Dokumenty i frontmatter, lokalne Git excludes, nadpisywalne templates, zamrożony workflow, jawne migracje i zewnętrzny katalog workspace; izolacja projektów we wspólnym katalogu | `project.go`, `migration.go`, `storage_test.go`, `migration_test.go`, test naprawy brakujących templates |
| 3. Rozpoczęcie pracy | Instalowalny skill, doctor, discovery, ticket URL lub plik, snapshot treści i źródła, `needs_workflow` oraz jawny wybór | bundled `skill/workspace/SKILL.md`, `issue.go`, `issue_test.go`, `skill_test.go`, `check-install.py` |
| 4. CLI | Komendy projektu, workflow, agentów, sesji, zadań, worktrees, wiadomości, handoffów, CR, testów i cyklu życia; CWD/env/global scope; JSON, non-interactive i operation keys | `internal/cli`, `mutation_test.go`, `cli_test.go`, `check-install.py`; zasady w `operations.md` |
| 5. Trwały stan | WORKSPACE.md jako stan workflow; kontrola rewizji, wykrywanie edycji zewnętrznej, lock i atomowa podmiana, WAL dla stanu i plików migracji; receipt razem ze zmianą | `files.go`, `state.go`, `mutation.go`; testy recovery, rollback, równoległego replay i odtworzenia receipt z pending write |
| 6. tmux i sesje | Workspace jako sesja tmux, worktrees jako okna, wykonania jako panele, osobne okno orkiestratora; ID rodzica i orkiestratora w środowisku; osobne panele usług | `runtime.go`, `session.go`, `service.go`, `tmux_integration_test.go`, `workflow_tmux_test.go` |
| 7. Klienty i dostarczenie | Generyczne argv, adaptery Codex/Claude/OpenCode, snapshoty promptów, native thread resume, trwały inbox, supervisor i powiadomienia; Codex app-server budzi orkiestratora między turami | `client.go`, `codex.go`, `supervisor.go`, `codex_test.go`, `codex_input_test.go`, `supervisor_test.go`, rzeczywisty handshake app-server |
| 8. Handoff i artefakty | Produkty zostają w worktree; kopie, manifest, pochodzenie i hash w artifacts; kontrola ścieżek/rozmiaru, ACK niezależny od akceptacji, stale attempts, captured check receipts | `handoff.go`, `check.go`, `handoff_test.go`, `check_test.go`; testy podmiany dowodów i zmiany HEAD |
| 9. Profile i routing | Trasy klient/model/provider, wagi providerów, okno 24 h, limity równoległości i uruchomień, cooldown, capability filters, zapis oceny tras, przypięcie trasy do sesji | `routing.go`, `routing_test.go`, test balansowania między workspaces; explain i Session.routing_decision |
| 10. issue-resolution | Planning → plan review → implementation DAG → integration → CR → oferta live testing → test lub decyzja pominięcia → awaiting release → potwierdzenie użytkownika | templates workflow/roles/prompts, `workflow.go`, `task.go`, `workflow_test.go`; cały przepływ w `TestTmuxCompleteIssueWorkflow` |
| 11. Git, integracja i CR | Oddzielne branche i zamrożone SHA, zależności, manifest integracji, CR zintegrowany lub per-task z kolejnością merge, polityka publikacji, lookup po niepewnej publikacji, związanie testu i release z SHA | `integration.go`, `change_request.go`, `forge.go`, `workflow_test.go`, `change_request_dependencies_test.go`, `forge_test.go` |
| 12. Menu | Akcje zależne od stanu, wolny tekst, trwałe pytania i odpowiedzi z rewizją, odświeżenie nieaktualnego pytania z zachowaniem historii; pause i interrupt | `workflow.go`, `decision.go`, `decision_test.go`, `lifecycle_test.go`, templates orkiestratora |
| 13. Awarie i odtwarzanie | Utrata panelu, transient runtime outage, wznowienie orkiestratora, niepewny launch/publish, deduplikacja, chronione lokalne zmiany; archive i clean z weryfikacją Git bundle | `session.go`, `supervisor.go`, `lifecycle.go`, testy tmux/recovery/cleanup oraz `mutation_test.go` |
| 14. Program i MVP | Binarium Go, lokalny supervisor/socket, zewnętrzne Git/tmux/klienty, protokoły JSON; kompletny pionowy workflow, dokumentacja i skrypt sprawdzający instalację | `cmd/workspace`, `go.mod`, README, docs, testy Go i `scripts/check-install.py` |

## Interpretacja dowodów

Test całego workflow uruchamia rzeczywiste procesy wykonawców w tmux, przekazuje
wyniki przez CLI, zapisuje commity, integruje kod i wykonuje polecenia sprawdzające.
Wykonawcy są deterministycznymi klientami testowymi. Testy adapterów forge uruchamiają
lokalne procesy kontrolne dla argumentów gh/glab/git i protokołu command. Publikacja
w zewnętrznym repozytorium i płatne wywołania modeli nie były wykonywane.

Testy native Codex sprawdzają wakeup, resume i pytania protokołu. Dodatkowo zainstalowany
app-server przyjął initialize/initialized w izolowanym katalogu runtime. Użytkownik
konfiguruje modele i uwierzytelnienie swoich klientów przed pierwszym uruchomieniem.
Generyczny klient bez adaptera dostarczenia odbiera inbox w swojej aktywnej turze;
pełna automatyzacja korzysta z adaptera native lub deliver.

Dane operacyjne są indeksowane w `.runtime/index.json`, a intencja atomowego zapisu
w `.runtime/pending.json`. Są to plikowe indeksy odpowiadające rejestrom z diagramu
planu. WORKSPACE.md pozostaje stanem workflow. Tożsamość konkretnej Session i znaczniki
własności panelu zabezpieczają przed operacjami poprzedniego wykonania i odziedziczeniem
recyklingowanego pane ID. Kontrola ról i read-only są kontraktem współpracujących
klientów działających na jednym koncie systemowym.

## Odtwarzalna weryfikacja

```sh
WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s
go vet ./...
go build -o bin/workspace ./cmd/workspace
python3 scripts/check-install.py bin/workspace
```

Osobno wykonywane są testy i build Windows. Runtime tmux jest testowany w Linux/WSL;
kompilacja dla macOS ARM64 sprawdza przenośność kodu, bez uruchamiania na komputerze Mac.

Końcowy przebieg 2026-09-11: cały zestaw WSL/race/tmux PASS (core 51.082 s), Windows
PASS (core 48.642 s), vet i build obu platform PASS, smoke gotowego CLI PASS oraz
cross-build macOS ARM64 PASS. Wymagania MVP z powyższej macierzy są zaimplementowane
i objęte wskazaną weryfikacją.

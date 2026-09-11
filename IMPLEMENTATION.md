# Stan implementacji

Zaimplementowano zakres MVP z `workspace-cli-design.md` (commit planu `59f7fb1`),
z modelem Agent = definicja persony, Session = konkretne wykonanie.
Szczegółowa macierz wszystkich 14 części planu znajduje się w [docs/audit.md](docs/audit.md).

## Dostępne funkcje

- Inicjalizacja projektu, skill, templates, workspace z dokumentami i YAML frontmatterem,
  issue URL/opis, wybór workflow, migracja inputu/templates i historia rewizji.
- Agent personas, sesje klientów, Git worktrees, tmux, supervisor, usługi pomocnicze,
  wznowienia, read-only analysis i wyłączne rezerwacje zapisu.
- Zadania i zależności, trwały inbox, pytania, ACK, handoffy, niezmienne artefakty,
  przechwytywanie wyników rzeczywistych poleceń i kontrola SHA.
- Profile i routing providerów z diagnostyką, limitami, cooldownem i historią wyboru.
- Cały issue-resolution: planowanie, implementacja, integracja, PR/MR, oferta live
  testing, osobna sesja testera i zakończenie po potwierdzeniu release przez użytkownika.
- Interaktywne menu i decyzje, kontrola rewizji i roli, idempotentne mutacje,
  atomowy zapis z odtwarzaniem, archive oraz clean z zachowaniem niezabezpieczonej pracy.
- Adaptery Codex, Claude, OpenCode i generyczny command; GitHub, GitLab i command forge.

## Weryfikacja końcowa

Wykonano 2026-09-11:

| Sprawdzenie | Wynik |
|---|---|
| Linux/WSL: `WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s` | PASS; pakiet core 51.082 s |
| Windows: `go test ./... -timeout 90s` | PASS; pakiet core 48.642 s |
| `go vet ./...` na Linux i Windows | PASS |
| Build Linux i Windows | PASS |
| Cross-build macOS ARM64 | PASS; bez testu runtime na Macu |
| `scripts/check-install.py bin/workspace` | PASS; init, skill, create/replay, dokumenty, identity, menu, CWD discovery, replay mutacji i JSON receipts |
| Pełny workflow w prawdziwym tmux | PASS; procesy wykonawców, commity, integracja, CLI handoff/check i osobna sesja testera |
| Adaptery forge | PASS; protokół command oraz procesowe testy argumentów gh/glab/git i rozpoznawania własnego CR |
| Native Codex | PASS; test protokołu wakeup/resume/pytania oraz rzeczywisty izolowany handshake app-server |

Testy nie publikowały zewnętrznych PR i nie wykonywały płatnych tur modeli.
Przed pracą należy skonfigurować profile, dostępne modele i uwierzytelnienie klientów.
Generyczny launcher bez deliver wymaga aktywnego odczytu inboxa; pełna automatyzacja
jest dostępna przez adapter native/deliver, zgodnie z planem.

## Pliki do użycia

Instrukcje instalacji i konfiguracji: [README.md](README.md).
Binarium Linux/WSL: `bin/workspace`; Windows: `bin/workspace.exe` (rdzeń CLI,
bez natywnego tmux). Kod znajduje się w `cmd/` i `internal/`, a szczegółowe kontrakty
runtime, klientów, artefaktów, rewizji i ponowień w `docs/`.

Plan jest zacommitowany. Kod implementacji i dokumentacja są pozostawione lokalnie
do przeglądu; nie opublikowano repozytorium ani release'u tego narzędzia.

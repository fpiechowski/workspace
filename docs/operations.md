# Ponowienia operacji

Używaj `--operation-key` jako identyfikatora jednej logicznej mutacji. Powtórz ten
sam klucz przy utracie odpowiedzi. Zmienione argumenty lub treść pliku wejściowego
z tym samym kluczem powodują `operation_conflict`. Nowa decyzja, poprawiony payload
lub kolejna próba pracy wymagają nowego klucza.

Mutacje workspace obejmują tworzenie zasobów i zadań, start/resume/stop sesji,
wiadomości i ACK, handoffy i ich ocenę, workflow, aktualizacje stanu, migracje,
integrację, CR, decyzje, release, pause/resume, reconcile, archive i clean.
`project init`, `skill install` i `server stop` zapisują receipts w zakresie projektu.
Odczyty, `clean --dry-run`, interaktywne attach oraz stale działający `serve` nie
są jednorazowymi mutacjami wymagającymi receipt.

Udane ponowienie zwraca zachowany wynik. Może on opisywać starszy stan: ponowienie
`session start` po zatrzymaniu sesji zwraca wcześniejszą odpowiedź i nie uruchamia
jej ponownie. Bieżący stan odczytaj przez `status`, `session list` lub właściwe `list`.
Identyfikatory zasobów i numer operacji pozostają stałe.

JSON mutacji z kluczem zawiera `operation_id` i, dla workspace, `revision` operacji.
Rewizja ta odnosi się do utrwalonego wyniku. `data.workspace.revision` w odpowiedzi
`status` jest bieżącą rewizją do następnego `state update --expected-revision`.
Projekt nie ma wspólnej rewizji WORKSPACE.md, więc jego receipts nie zawierają tego pola.

Zmiana dokumentu i jej wynik są zatwierdzane razem przez write-ahead record. Błąd
walidacji nie utrwala częściowej aktualizacji. Równoległe wywołania z tym samym
kluczem nie wykonują tej zmiany dwa razy. Autoryzacja roli jest sprawdzana także przy
odczycie utrwalonego wyniku.

Operacje zewnętrzne zapisują intencję przed wywołaniem procesu lub sieci i korzystają
z oddzielnej blokady operacji. Po awarii rezultat pozostaje niepewny do uzgodnienia:
PR jest wyszukiwany przed kolejną próbą publikacji; sesja zachowuje zarezerwowaną
identyfikację; usunięcie worktree jest uzgadniane z Git. Zajęta blokada nie oznacza,
że poprzednie wykonanie się zakończyło. Historyczny receipt nie zastępuje polecenia
reconcile w celu odczytania aktualnego stanu runtime.

Dane z wczesnej wersji rejestru, bez zapisanego snapshotu odpowiedzi, zachowują
odwołanie do utworzonego zasobu. Ich replay może pokazać jego aktualny stan.

## Operacje TUI

Mutacje uruchamiane z TUI korzystają z tych samych przypadków użycia core co CLI.
Potwierdzenie zachowuje rewizję workspace, attempt taska albo dokładny bieżący Run jako
guard; „Pause and interrupt” zapisuje i porównuje dokładny zbiór aktywnych Runów oraz
usług przed zatrzymaniem któregokolwiek procesu. Zmiana targetu po otwarciu formularza
kończy się konfliktem zamiast wykonania akcji na nowym stanie. Retry taska pokazuje
zależne zadania i wymaga powodu.

Każda potwierdzona operacja otrzymuje klucz `tui_<ULID>`. Powtórzenie po niepewnej
odpowiedzi używa tego samego klucza i payloadu; podwójne zatwierdzenie nie wykonuje
mutacji ponownie. `tui show` i `tui hide` zapisują osobne receipts w `.runtime/ui.json`,
nie w rejestrze Session/Run. `q` w zarządzanym panelu zapisuje ten sam trwały zamiar hide
przed przywróceniem terminala i zakończeniem procesu; usunięcie panelu wykonuje później
supervisor po zweryfikowaniu jego tożsamości.

Delete w TUI wymaga przepisania pełnego ID. Task i Session są logicznie usuwane przez
audytowalny tombstone; receipt pozostaje w rejestrze workspace'u, a guard obejmuje
rewizję oraz odpowiednio attempt lub ostatni Run. Fizyczne Delete Workspace korzysta
z project-scoped receipt poza katalogiem docelowym, więc ten sam klucz i payload można
bezpiecznie ponowić po usunięciu katalogu. Zmiana rewizji albo targetu kończy się konfliktem.

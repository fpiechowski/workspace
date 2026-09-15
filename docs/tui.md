# Interfejs terminalowy

`workspace tui` jest interaktywną prezentacją lokalnego stanu workspace'u i używa tych
samych zapytań oraz mutacji core co CLI. Ręczna instancja nie tworzy workspace'u,
orkiestratora ani supervisora przez sam odczyt lub odświeżenie.

## Ekrany i nawigacja

Picker projektu pokazuje tytuł, status, fazę, aktywne Runy, problemy, pełny ID, ścieżkę,
źródło wejścia i datę utworzenia. Wybór workspace'u otwiera dashboard z licznikami,
orkiestratorem, maksymalnie pięcioma uwagami i pięcioma ostatnimi zapisanymi
zdarzeniami. Pełne listy są pod Tasks, Worktrees, Results i More.

More zawiera Sessions, Agents, Services, Decisions, Change requests, Runtime, Needs
attention, Recent recorded activity i Documents. Task prowadzi do bieżących sesji,
powiązanych worktrees i wyników; `f` przełącza widok bieżącej próby i historii.
Artefakt bez możliwego do ustalenia lineage jest oznaczony jako unknown i widoczny
w historii. Detail Worktree pokazuje powiązane zadania/sesje/usługi, aktywnych writerów
i read-only readerów oraz osobną, oznaczoną czasem obserwację Git.

`Enter` otwiera zaznaczenie, a `Esc` wraca lub czyści filtr. Na kolekcjach `/` edytuje
filtr substring po nazwie, ID i podtytule; porównanie nie rozróżnia wielkości liter.
`Esc` podczas edycji przywraca poprzednią wartość. `f` przełącza aktywny/archiwalny
widok workspace'ów i statusowe lub historyczne filtry odpowiednich kolekcji. Na Dashboard
`Tab`/`Shift+Tab` zmienia fokus między Overview, Orchestrator, Needs attention i Recent
recorded activity. Na Results zmienia typ wyniku: artifacts, handoffs lub checks. Użyj
`1`–`5`, aby przejść do głównych stron.

| Klawisz | Działanie |
|---|---|
| `1`–`5` | Overview, Tasks, Worktrees, Results, More |
| `Up`/`Down`, `j`/`k` | Zmiana zaznaczenia lub przewijanie szczegółów |
| `Enter` | Otwórz zaznaczenie; nie uruchamia mutacji |
| `Esc` | Anuluj formularz, wyjdź z edycji filtra, wyczyść filtr albo wróć |
| `/` | Edytuj filtr kolekcji |
| `f` | Przełącz status albo historię na wspieranych listach |
| `Tab` / `Shift+Tab` | Zmień fokus panelu Dashboard albo typ wyników |
| `a` | Otwórz dostępne akcje zaznaczenia lub workspace'u |
| `g` | Skocz do zweryfikowanego celu tmux |
| `w` / `o` | Picker workspace'u / strona orkiestratora |
| `r` | Odśwież snapshot i runtime; nie wykonuje reconcile |
| `?` | Pomoc kontekstowa |
| `q` / `Ctrl+C` | Zakończ ręczną instancję; ukryj zarządzany panel |

W formularzu Huh klawisze należą do formularza. `Ctrl+C` anuluje otwarty formularz;
w zarządzanym panelu poza formularzem zapisuje hide. Podczas edycji tekstu filtr przejmuje
klawisze, więc `q`, `g` i cyfry nie uruchomią skrótów aplikacji.

## Akcje i synchronizacja

Akcje są ograniczone do istniejącego kontraktu core. Dostępne są m.in. start/resume
orkiestratora, pause, guarded pause-and-interrupt, resume workspace, reconcile, wybór
workflow, retry task, session resume/stop/close i stop service. Runtime udostępnia show
i hide zarządzanego TUI. TUI nie wysyła wiadomości do agentów, nie ACK-uje inboxa,
nie akceptuje handoffów ani decyzji.

Każda nowa, potwierdzona intencja dostaje klucz `tui_<ULID>`. Podwójne zatwierdzenie
nie uruchamia drugiej mutacji. Retry po błędzie używa tego samego klucza i payloadu;
po sukcesie TUI pobiera nowy snapshot. Guardy w core sprawdzają rewizję, task attempt,
albo konkretny bieżący Run. „Pause and interrupt” przed potwierdzeniem pokazuje
dokładne aktywne Runy i usługi; zmiana którejkolwiek listy odrzuca operację przed
zatrzymaniem procesów. Retry taska pokazuje próbę, zadania zależne i powód.

Odczyty snapshotu, tmux, UI i Git mają osobne stany asynchroniczne. Spóźniona odpowiedź
ze starego workspace'u nie może nadpisać aktualnego wyboru. Nieudany odczyt pokazuje
błąd i zachowuje ostatni dobry snapshot jako nieaktualny. Runtime niedostępny nie
blokuje przeglądania zapisanego stanu.

## Terminal i zarządzany panel

Widoki dopasowują szerokość i wysokość, a dla terminala poniżej 40×12 pokazują krótki
komunikat zamiast ciasnego layoutu. Dostępne są motywy `auto`, `dark`, `light` oraz
`--no-color`. TUI nie wymaga Git status wszystkich worktrees: Git jest badany po
otwarciu konkretnego szczegółu Worktree.

`g` rozwiązuje cel z ID i sprawdza jego ownership przed `switch-client`, wyborem okna
lub attach. Historyczny Run nie przeskakuje do nowszego wykonania. Z zewnątrz tmux TUI
może oddać terminal na czas attach; po powrocie odświeża dane. Błąd lub brak runtime
jest pokazywany użytkownikowi bez wymyślania alternatywnego celu. Wewnątrz tmux
nawigacja wybiera klienta po jego TTY i bieżącym panelu; brak klienta, wielu klientów
oglądających panel albo inny socket kończą się jawnym błędem zamiast przełączenia
przypadkowego terminala.

Supervisor utrzymuje najwyżej jeden zarządzany panel na workspace po zapisaniu historii
orkiestratora. Panel trafia do tego samego okna orkiestratora z nieaktywnym splitem, nie
przejmuje focusu i nie jest Agentem, Session ani Runem. `workspace tui show` zapisuje
desired state i uzgadnia panel; `hide` wyłącza go, a `status` pokazuje generację,
ownership, błąd i backoff. `q`/`Ctrl+C` zapisuje żądanie hide, kończy TUI dopiero po
przywróceniu terminala, a supervisor usuwa panel w następnym reconcile. Zewnętrzne
zabicie panelu pozostawia desired=true, więc supervisor może go odtworzyć.
Jeżeli proces TUI nadal działa, ale jego metadane panelu znikną lub zostaną uszkodzone,
reconcile potwierdza dokładną komendę runnera i odtwarza metadane. Nie usuwa przy tym
sąsiednich, niezweryfikowanych paneli. Utrata całego okna orkiestratora powoduje najpierw
odtworzenie orkiestratora, a następnie jednego panelu TUI w nowym oknie.

Panel pozostaje dostępny po pause i completed, dopóki użytkownik go nie ukryje albo
workspace nie zostanie zarchiwizowany. Archive sprząta zweryfikowany panel; UI nie
blokuje `clean` i nie jest liczone jako aktywny proces domenowy. Przed downgrade
binarium wykonaj `workspace tui hide`, bo stary launcher nie rozpoznaje nowego pane'a.

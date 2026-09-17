# Interfejs terminalowy

`workspace tui` jest interaktywną prezentacją lokalnego stanu workspace'u i używa tych
samych zapytań oraz mutacji core co CLI. Sam odczyt lub odświeżenie niczego nie tworzy
i nie uruchamia orkiestratora ani supervisora; jawna akcja w pickerze może utworzyć workspace.

## Ekrany i nawigacja

Picker projektu pokazuje tytuł, status, fazę, aktywne Runy, problemy, pełny ID, ścieżkę,
źródło wejścia i datę utworzenia. Gdy lista jest pusta, pokazuje tytuł, jednozdaniowe
wyjaśnienie i jedną prawidłową akcję (`a` tworzy workspace), więc nie zaprzecza już
akcji Create workspace. W trybie szerokim obok listy pokazuje szczegóły zaznaczenia
(ID, ścieżkę, źródło wejścia, datę utworzenia i stan) bez ramki panelu. Wybór workspace'u
otwiera Tasks: każdy wiersz pokazuje stan zadania oraz liczbę aktywnych wykonań, sesji
i runów jego bieżącej próby, a zaznaczenie pozostaje przypięte do ID także po sortowaniu.
Zakończenie runu nie oznacza akceptacji zadania; stan zadania, sesji i procesu są
rozróżnione. Główna nawigacja to kolejno `1 Tasks`, `2 Sessions`, `3 Worktrees`,
`4 Results` i `5 More`. Sessions to zwykła kolekcja bez wtórnych zakładek: `f` przełącza
widok bieżący/historię, `Enter` otwiera szczegóły sesji, a `t` otwiera albo wznawia jej
terminal.

Stan ma symbol i podpis; running/starting mają animowany wskaźnik, także bez koloru.
`s` zmienia sortowanie (priorytet pracy, nazwa, ostatnie wykonanie); zaznaczenie
pozostaje przypięte do ID. Widok szeroki pokazuje nazwane panele listy i `Preview`
z jedną wyraźną krawędzią fokusu, a linia akcji podglądu reklamuje tylko komendy
obsługiwane przez zaznaczony rodzaj (`t`, `g`, `Enter`). Wąski widok zachowuje listę
i dwuwierszowe wpisy. Przy małej wysokości wpis zajmuje jeden wiersz. Wpisy są
rozdzielone linią o niższym nacisku, a zaznaczenie obejmuje tytuł i opis na wspólnym
tle; bez koloru pozostaje znacznik wyboru i separator. Pasek skrótów ma zawsze zarezerwowany ostatni wiersz.

`a` w pickerze projektu udostępnia utworzenie workspace'u (tytuł, opis oraz jawny wybór
trybu: nazwany workflow, `Choose later` albo `No workflow (manual orchestration)`) oraz
trwałe usunięcie zaznaczonego workspace'u. Usunięcie jest dostępne dopiero
po wpisaniu pełnego ID i nie wymaga release ani archive. Jest pełnym discardem: zatrzymuje
runtime i usuwa stan, wszystkie worktrees wraz z niezacommitowanymi plikami oraz lokalne
gałęzie `workspace/<id>/…`. Tej operacji nie można cofnąć. Wybór workflow jest dostępny
tylko dla stanu `needs_workflow`; workspace manualny nie proponuje później wyboru ani
advance.

Na Orchestratorze (`o`) oraz na stronie Runtime w More `a` udostępnia
`Complete this manual workspace` dla aktywnego manualnego workspace'u oraz
`Archive completed workspace`, gdy stan to `completed`.
Archive wymaga potwierdzenia, sprawdza rewizję oraz zachowuje wszystkie dane. Workflow
nadal wymaga potwierdzonego release'u, a ukończony manualny workspace — wcześniejszego
`complete`; w obu przypadkach obowiązuje brak aktywnych Session oraz usług.

More zawiera Agents, Services, Decisions, Change requests, Runtime, Needs
attention, Recent recorded activity i Documents. Detail Runtime renderuje topologię
tmux jako tabelę (bubbles/table) z kolumnami window, pane, kind, owner, run i state
w trybie szerokim oraz równoważny układ wierszy w trybie compact; błędy i stan
zarządzanego interfejsu pozostają nad topologią. Task prowadzi do bieżących sesji,
powiązanych worktrees i wyników; `f` przełącza widok bieżącej próby i historii.
Artefakt bez możliwego do ustalenia lineage jest oznaczony jako unknown i widoczny
w historii. Detail Worktree pokazuje powiązane zadania/sesje/usługi, aktywnych writerów
i read-only readerów oraz osobną, oznaczoną czasem obserwację Git.

Szczegóły zadań, sesji, runów, worktrees, handoffów, checków, decyzji, change
requestów, runtime'u i orkiestratora oraz podglądy plików są dokumentami o stałej
hierarchii: najpierw tożsamość i status, potem fakty operacyjne, dalej sekcje
narracyjne i powiązane zasoby, a na końcu ID, digesty i znaczniki czasu. Długie
cele, instrukcje, podsumowania, powody, linie poleceń, ścieżki i treści change
requestów zawijają się do szerokości viewportu, a nieprzerywalne tokeny są
łamane; jawne znaki nowej linii w podglądzie pozostają zachowane. Treść binarna
lub obcięta ma jawny komunikat `[!]`, a brak encji — komunikat odzyskania z `Esc`
i `r`. Każdy dokument przewija się strzałkami lub `PgUp`/`PgDn`; wiersz statusu
pokazuje `line x–y of n` tylko przy przewijaniu, a pozycja wraca po powrocie z
innej trasy.

`Enter` otwiera zaznaczenie, a `Esc` wraca lub czyści filtr. Na kolekcjach `/` edytuje
filtr substring po nazwie, ID i podtytule; porównanie nie rozróżnia wielkości liter.
`Esc` podczas edycji przywraca poprzednią wartość i zaznaczenie. Poza edycją czyści najpierw filtr tekstowy, następnie statusowy, zanim wróci do poprzedniej strony. `f` przełącza aktywny/archiwalny
widok workspace'ów i statusowe lub historyczne filtry odpowiednich kolekcji. Na Results
`Tab`/`Shift+Tab` zmienia typ wyniku: artifacts, handoffs lub checks; aktywny
typ jest nazwany w linii pomocniczej. Trasy szczegółów i kolekcji zależnych pokazują
breadcrumb, a na szczegółach zadania `1`–`3` są skrótami do powiązanych zasobów
(Sessions, Worktrees, Results), nie do stron głównych. Użyj
`1`–`5`, aby przejść kolejno do Tasks, Sessions, Worktrees, Results i More.

| Klawisz | Działanie |
|---|---|
| `1`–`5` | Tasks, Sessions, Worktrees, Results, More |
| `Up`/`Down`, `j`/`k` | Zmiana zaznaczenia lub przewijanie szczegółów |
| `PgUp`/`PgDn` | Przewiń dokument albo przesuń zaznaczenie o stronę na kolekcji |
| `Home`/`End` | Początek lub koniec dokumentu albo kolekcji |
| `Enter` | Otwórz zaznaczenie; nie uruchamia mutacji |
| `Esc` | Anuluj formularz, wyjdź z edycji filtra, wyczyść filtr albo wróć |
| `/` | Edytuj filtr kolekcji |
| `f` | Przełącz status albo historię na wspieranych listach |
| `Tab` / `Shift+Tab` | Zmień typ wyników na Results (Artifacts, Handoffs, Checks) |
| `s` | Sortuj według priorytetu, nazwy lub ostatniego wykonania |
| `t` | Otwórz terminal agenta albo potwierdź start/wznowienie |
| `a` | Otwórz dostępne akcje zaznaczenia lub workspace'u |
| `g` | Skocz do zweryfikowanego celu tmux |
| `w` / `o` | Picker workspace'u / strona orkiestratora |
| `r` | Odśwież snapshot i runtime; nie wykonuje reconcile |
| `?` | Pomoc kontekstowa |
| `q` / `Ctrl+C` | Zakończ ręczną instancję; ukryj zarządzany panel |

`Esc` i `Ctrl+C` anulują otwarty formularz, także jego filtr i potwierdzenie; pozostałe klawisze należą do formularza. `Ctrl+C`
w zarządzanym panelu poza formularzem zapisuje hide. Podczas edycji tekstu filtr przejmuje
klawisze, więc `q`, `g` i cyfry nie uruchomią skrótów aplikacji.

Stopka pokazuje wyłącznie polecenia dostępne dla bieżącego zaznaczenia i możliwości
backendu; nieobsługiwane terminal, skok i akcje nie są reklamowane. `?` otwiera
przewijalną pomoc pogrupowaną na Navigation, View, Runtime, Actions i Exit; pozycję
przewijania widać w wierszu statusu, a `?` lub `Esc` wracają do poprzedniego widoku.

`t` na aktywnej sesji otwiera jej dokładny bieżący Run po weryfikacji ownership.
Na nieaktywnej sesji prosi o potwierdzenie wznowienia, tworzy nowy Run przez core
i po sukcesie otwiera jego terminal. Jest to wznowienie tej konkretnej Session, a nie
retry Taska. Gdy Task tej Session oczekuje na review, niezmienione wznowienie nadal
tworzy Run, ale zachowuje stan `awaiting_review` i pochodzenie oczekującego handoffu.
`o`, a następnie `t`, otwiera orkiestratora
lub potwierdzenie jego uruchomienia. Zadanie z wieloma sesjami otwiera ich listę do
jawnego wyboru; zadanie bez sesji wskazuje potrzebę delegacji przez orkiestratora.
Historyczny Run pozostaje dokładnym celem i nie jest automatycznie wznawiany.

Jeżeli nawigacja zgłosi brak sesji lub panelu tmux, TUI proponuje reconcile
w formularzu potwierdzenia. Esc i Cancel pozostawiają runtime bez zmian.
Po potwierdzeniu wykonuje jedną operację reconcile i ponawia nawigację do tego
samego celu. Reconcile może odtworzyć kwalifikującego się orkiestratora; nie
restartuje automatycznie zatrzymanych workerów. Gdy cel nadal jest niedostępny,
TUI wskazuje Sessions (`2`) albo Orchestrator (`o`) i `t` do otwarcia lub wznowienia
sesji, bez pętli potwierdzeń. Niejednoznaczny cel, niezgodne ownership lub socket nie
wywołują propozycji reconcile.

## Akcje i synchronizacja

Akcje są ograniczone do istniejącego kontraktu core. Dostępne są m.in. start/resume
orkiestratora, pause, guarded pause-and-interrupt, resume workspace, reconcile, wybór
workflow (tylko dla `needs_workflow`), `complete` manualnego workspace'u, retry task,
session resume/stop/close i stop service. Runtime udostępnia show
i hide zarządzanego TUI. Menu Task i Session udostępnia też Delete. Wymaga ono wpisania
pełnego ID i zapisuje tombstone `deleted_at`; rekord znika z normalnych kolekcji, ale
pozostaje w historii dla receiptów. Core odrzuca aktywną sesję, sesję z referencjami
wynikowymi, zaakceptowany lub zależny task oraz task z aktywnymi sesjami albo utrwalonymi
handoffami, artefaktami, checks, integracją lub change requestem. TUI nie wysyła wiadomości do agentów, nie ACK-uje inboxa,
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

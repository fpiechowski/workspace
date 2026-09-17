# Wizja produktu `workspace`

## Cel

`workspace` jest lokalnym narzędziem do prowadzenia złożonych zmian w repozytorium Git
przez jednego orkiestratora i wielu wyspecjalizowanych agentów. Utrwala kontekst pracy,
oddziela planowanie od implementacji, izoluje zmiany w Git worktrees i pozostawia
człowiekowi kontrolę nad decyzjami produktowymi, publikacją oraz zakończeniem pracy.
Praca może korzystać z nazwanego workflow albo być prowadzona manualnie bez workflow.

Produkt rozwiązuje problem sesji agentowych, które są łatwe do uruchomienia, ale trudne
do bezpiecznego wznowienia i skoordynowania. Sam terminal lub historia czatu nie mówi,
który wynik został zaakceptowany, do jakiej rewizji kodu się odnosi ani czy operację
można bezpiecznie ponowić. `workspace` zapisuje te informacje jako jawny stan projektu.

## Dla kogo

Podstawowym użytkownikiem jest programista korzystający z agentów CLI podczas pracy
nad istniejącym repozytorium. Drugim odbiorcą jest sam agent: stabilny interfejs CLI,
JSON/YAML i instalowany skill pozwalają mu wykonywać operacje bez zgadywania stanu.

Narzędzie jest szczególnie przydatne, gdy zadanie:

- wymaga najpierw diagnozy i planu;
- można podzielić na zależne lub równoległe części;
- angażuje różne role, modele albo klientów agentowych;
- musi przetrwać przerwanie procesu, terminala lub komputera;
- wymaga śladu decyzji, wyników testów i pochodzenia artefaktów.

## Obietnica produktu

Użytkownik przekazuje ticket albo opis problemu i wybiera jedną z trzech jawnych dróg:
nazwany workflow, świadome odroczenie wyboru (`needs_workflow`) albo pracę manualną bez
workflow (`--no-workflow`). Pracę można obserwować w tmux. Orkiestrator deleguje
planowanie, implementację, integrację i testy w workflow, a w trybie manualnym sam
tworzy jawne zadania i worktrees, bez faz i release'u. Każdy wynik ma wskazane zadanie,
wykonanie, commit i dowody weryfikacji. Po przerwaniu pracy system odtwarza stan
z plików, zamiast polegać wyłącznie na pamięci rozmowy.

Sukces oznacza, że użytkownik może:

1. utworzyć workspace z trwałym snapshotem wejścia;
2. bezpiecznie delegować pracę do izolowanych worktrees;
3. sprawdzić aktualny stan, decyzje, artefakty i historię wykonań;
4. wznowić przerwaną pracę bez duplikowania niepewnych operacji;
5. świadomie zatwierdzić publikację i live testing w workflow oraz potwierdzić
   zakończenie workflow albo manualnego workspace'u.

## Zasady produktu

### Stan jest ważniejszy niż historia czatu

`WORKSPACE.md`, snapshot workflow i rejestry runtime są źródłem odtwarzalnego stanu.
Historia rozmowy może poprawić ciągłość pracy, ale nie zastępuje zadań, decyzji,
artefaktów ani identyfikatorów operacji.

### Delegowanie jest jawne

Zadanie ma cel, rolę, kryteria akceptacji, zależności i wymagane produkty. Zakończenie
procesu nie oznacza przyjęcia wyniku, a odebranie wiadomości nie oznacza akceptacji
handoffu. Orkiestrator ocenia wynik przed akceptacją zadania, a w workflow także przed
przesunięciem fazy.

### Człowiek zachowuje decyzje o skutkach zewnętrznych

Agent może przygotować change request i przedstawić diff, ale publikacja zależy od
polityki skonfigurowanej przez użytkownika. Workflow kończy się dopiero po otrzymanym
od użytkownika potwierdzeniu wdrożenia albo release'u; manualny workspace zamyka się
wyłącznie jawną, potwierdzoną przez użytkownika operacją `complete`. Treść ticketa nie
rozszerza uprawnień agenta.

### Ponowienie nie może duplikować pracy

Mutacje mają klucze operacji i trwałe receipts. Powtórzenie tej samej intencji zwraca
poprzedni wynik; zmieniona intencja wymaga nowego klucza. Niepewny efekt zewnętrzny
jest najpierw uzgadniany, a nie wykonywany ponownie w ciemno.

### Lokalna praca użytkownika jest chroniona

Worktrees izolują zapisywalne zadania. Sprzątanie nie usuwa aktywnych, brudnych ani
niezabezpieczonych zmian. Narzędzie nie traktuje dzierżawy zapisu jako systemowego
sandboxa i nie obiecuje ochrony przed dowolnym procesem działającym poza nim.

### Możliwości klienta są jawne

Uruchomienie procesu, wznowienie natywnej rozmowy, dostarczenie wiadomości, obserwacja
i przerwanie to osobne możliwości adaptera. Generyczny launcher pozostaje użyteczny,
ale pełna autonomiczna komunikacja wymaga klienta obsługującego dostarczenie.

## Zakres produktu

Aktualny zakres obejmuje:

- pojedynczy lokalny projekt Git i wiele workspace'ów;
- trzy jawne tryby utworzenia workspace'u: nazwany workflow, odroczony wybór
  (`needs_workflow`) i manualna orkiestracja bez workflow (`--no-workflow`);
- planowanie i implementację w oddzielnych worktrees;
- tmux jako widoczny runtime procesów;
- persony agentów, logiczne sesje oraz historię konkretnych uruchomień;
- trwały inbox, handoffy, niezmienne artefakty i przechwycone wyniki poleceń;
- routing klientów, providerów i modeli według profili;
- adaptery Codex, Claude, OpenCode i własnych poleceń;
- integrację zmian oraz przygotowanie/publikację change requests;
- kontrolowane wznowienie, uzgadnianie awarii, archiwizację i sprzątanie;
- workflow `plan-first` oraz rozbudowany, zgodny wstecznie `issue-resolution`.
- manualny tryb bez workflow: delegowanie zadań i worktrees, handoffy, checks i
  akceptacja z ustalonym limitem 3 równoległych workerów, bez faz, advance, release'u
  ani konwersji na workflow;
- interaktywny TUI do przeglądania tego samego stanu, nawigacji po taskach i
  uruchamiania jawnie dozwolonych operacji core.

Poza aktualnym zakresem pozostają zdalne workery, koordynacja wielu komputerów,
kryptograficzne potwierdzanie tożsamości człowieka, rozliczanie tokenów lub kosztów
całego konta oraz ochrona przed procesami działającymi z tymi samymi uprawnieniami
systemowymi.

## Doświadczenie użytkownika

CLI i formaty maszynowe pozostają podstawowym interfejsem agentów i automatyzacji.
Programista może użyć TUI do przeglądania workspace'ów, tasków, wykonania, worktrees,
wyników i runtime, a następnie skoczyć do zweryfikowanego panelu tmux. TUI korzysta
z tych samych zapytań i operacji core co CLI; każda mutacja ma potwierdzenie oraz
guardy bieżącej rewizji, próby lub RunID. Nie dodaje akcji wysyłania wiadomości,
ACK-owania inboxa ani automatycznej akceptacji wyników.
Native OpenCode delivery is visible in the active parent TUI: the Run-scoped loopback
server selects the current session, appends the marker-bearing workspace prompt, and
submits it through the active TUI control API. Delivery is confirmed against the
current Run only after the marker appears in session history; it remains distinct from
inbox ACK and handoff acceptance.

W pickerze projektu użytkownik może utworzyć workspace, wybierając nazwany workflow,
świadomie odkładając wybór (`needs_workflow`) albo tworząc workspace manualny bez
workflow. Tryby odróżnia etykieta fazy: `manual` dla orkiestracji manualnej i `-`
dla odroczonego wyboru. Manualny workspace ukończy jawna, potwierdzona przez
użytkownika operacja `complete`, po której archive nie wymaga release'u; workflow
pozostaje przy potwierdzonym release. Alternatywnie użytkownik może świadomie odrzucić
workspace w całości bez wymogu release/archive. Pełne usunięcie wymaga przepisania ID,
zatrzymuje runtime i usuwa stan, worktrees, niezacommitowane pliki oraz lokalne gałęzie
workspace'u. Wewnątrz zakończonego workspace'u TUI udostępnia archive, które zachowuje
historię i respektuje bramki aktywnego runtime oraz release'u (workflow) albo
wcześniejszego `complete` (tryb manualny). Można też usunąć task bez zależności
i utrwalonych wyników oraz nieaktywną sesję bez referencji wynikowych. Taski i sesje
otrzymują audytowalny tombstone i znikają z normalnych widoków; operacja nie przepisuje
ani nie kasuje historii, na której opierają się inne rekordy.

Pierwszy ekran TUI to Tasks: lista łączy zadanie, stan oraz liczbę aktywnych
wykonań, sesji i runów jego bieżącej próby. Główna nawigacja to kolejno Tasks,
Sessions, Worktrees, Results i More. Sessions jest zwykłą kolekcją bez wtórnych
zakładek, z filtrem bieżące/historia oraz wejściem w szczegóły sesji i jej terminal,
a More udostępnia m.in. Needs attention i Recent recorded activity. Otwarcie zadania
pokazuje powiązane sesje, worktrees i wyniki, odróżnia wykonanie procesu od akceptacji
wyniku oraz umożliwia otwarcie lub jawne wznowienie terminala. Brak panelu podczas
nawigacji prowadzi do propozycji reconcile wymagającej potwierdzenia użytkownika,
a następnie ponownej próby otwarcia tego samego celu.

TUI może działać ręcznie jako przeglądarka albo jako zarządzany panel obok orkiestratora.
Supervisor odtwarza wyłącznie panel o zapisanej, zweryfikowanej tożsamości; q w tym
panelu zapisuje hide przed wyjściem. Podczas pause i completed panel może pozostać
dostępny do przeglądu, a archive go sprząta. Brak tmux ogranicza nawigację runtime,
ale pozostawia dostępny zapisany stan workspace'u. Kontrakt ekranów i skrótów opisuje
[docs/tui.md](docs/tui.md).

Instrukcje instalacji i użycia znajdują się w [README.md](README.md). Szczegóły
techniczne opisuje [ARCHITECTURE.md](ARCHITECTURE.md), a planowane zmiany są utrzymywane
w [TODO.md](TODO.md).

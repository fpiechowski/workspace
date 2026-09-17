# Design System: workspace TUI

## Overview

Tryb **Operate**: natywny interfejs terminalowy do obserwowania pracy agentów,
przeglądania wyników i wykonywania jawnych operacji. Pierwszy ekran Work pokazuje
postęp zaakceptowanych zadań oraz aktualną pracę. Zakończenie procesu nie oznacza
akceptacji wyniku: licznik i procent postępu uwzględniają wyłącznie zadania `accepted`.

Źródłami kontraktu są [PRODUCT.md](PRODUCT.md) i [docs/tui.md](docs/tui.md).
Implementacja wyglądu znajduje się w `internal/tui/theme.go`, `status.go`, `view.go`
i `work.go`.

## Colors

Paleta rozróżnia role semantyczne, a renderery używają ich zamiast surowych
kolorów: tekst podstawowy, pomocniczy i dyskretny, obramowanie i obramowanie
fokusu, akcent, tło i tekst zaznaczenia, sukces, ostrzeżenie, zagrożenie oraz
powierzchnia informacyjna. Motyw `auto` rozwiązuje się przez wykrywanie tła
terminala (Lip Gloss / termenv). Gdy terminal nie pozwala wiarygodnie odczytać
tła (potok, tmux, screen, `dumb`), obowiązuje udokumentowany fallback do palety
ciemnej. Jawne `dark` i `light` ignorują wykrywanie, a `--no-color` nie ustawia
żadnego koloru i pozostaje deterministyczne. Tło terminala poza zaznaczeniem
pozostaje ustawieniem użytkownika; token zaznaczenia wypełnia cały zaznaczony
wpis, łącznie z jego opisem.

| Token | Dark | Light | Rola |
|---|---|---|---|
| `primary` | `#E2E8F0` | `#0F172A` | Tekst podstawowy |
| `secondary` | `#B4C0D3` | `#334155` | Podtytuły i etykiety |
| `subtle` | `#7C8CA3` | `#64748B` | Dyskretne metadane i ID |
| `border` | `#475569` | `#CBD5E1` | Obramowanie bez fokusu |
| `focusedBorder` | `#67E8F9` | `#0E7490` | Obramowanie aktywnego panelu |
| `accent` | `#67E8F9` | `#0E7490` | Nagłówki, fokus, aktywna praca |
| `selectionFg` / `selectionBg` | `#F8FAFC` / `#1E293B` | `#0F172A` / `#E0F2FE` | Tekst i tło zaznaczenia |
| `success` | `#86EFAC` | `#166534` | Accepted, completed, ready |
| `warning` | `#FDE68A` | `#92400E` | Blokady, review i przerwanie |
| `danger` | `#FDA4AF` | `#BE123C` | Failed, error |
| `infoSurface` | `#164E63` | `#E0F2FE` | Tło komunikatu informacyjnego |

`--no-color` usuwa kolory, zachowując tekst, symbole, pogrubienie i animację.
Status musi pozostawać rozpoznawalny bez barwy.

## Typography

Krój i rozmiar pisma wyznacza terminal. Hierarchię tworzą fabryki stylów
(`titleStyle`, `headingStyle`, `sectionStyle`, `labelStyle`, `valueStyle`,
`warningStyle`, `metaStyle`, `keycapStyle`, `noticeStyle`, `selectedStyle`,
`panelStyle`): pogrubiony nagłówek, akcent sekcji, etykieta i wartość pola,
ostrzeżenie, przygaszony podtytuł, podświetlenie zaznaczenia i ramka fokusu.
Szerokości mierzy się w kolumnach terminala, z uwzględnieniem Unicode; długi
tekst jest skracany przez `…`.

## Layout

Powłoka ma stałą kolejność: nagłówek tożsamości i świeżości, główna nawigacja,
opcjonalny breadcrumb/ nawigacja wtórna, treść, wiersz statusu/komunikatu oraz
kontekstowa legenda klawiszy. Wiersz statusu i legenda są rezerwowane zawsze,
więc pozostają widoczne przy każdym wspieranym rozmiarze. Breadcrumb pojawia się
na trasach szczegółów i kolekcjach zależnych, a Results używa tej linii jako
nawigacji wtórnej typu wyniku; Work i picker projektu jej nie pokazują.

- Minimum to **40×12**; mniejszy terminal pokazuje komunikat o rozmiarze.
- Jedna decyzja `layoutFor` steruje wszystkimi stronami: **tiny** poniżej 40×12,
  **compact** dla średnich terminali (jedna kolumna) oraz **wide** od 100×24
  (lista i szczegóły obok siebie). Work korzysta z tej samej decyzji zamiast
  własnego progu szerokości; w trybie wide lista Work zajmuje trzy piąte
  szerokości.
- Kolekcje w trybie wide pokazują dwa nazwane, obramowane panele (lista oraz
  `Preview`) obok siebie, z jedną wyraźną krawędzią fokusu; szerokość listy jest
  oparta na dwóch piątych szerokości terminala. Picker projektu w trybie wide
  pokazuje listę workspace'ów i obok szczegóły zaznaczenia bez ramki, z
  wyróżnionym wierszem wyboru.
- Wpis ma dwa wiersze: znacznik, status i tytuł, następnie kontekst. Gdy na listę
  pozostają mniej niż cztery wiersze, wpis zwija się do jednego. W pozostałych
  rozmiarach sąsiednie wpisy oddziela linia o niższym nacisku (`subtle`).
  Zaznaczenie pozostaje widoczne, a linia akcji podglądu reklamuje tylko komendy
  wspierane przez zaznaczony rodzaj.
- Etykiety sekcji Work skracają się poniżej 75 kolumn, a główne zakładki,
  podsumowanie i skróty używają krótszej wersji poniżej 60 kolumn.

## Elevation & Depth

Układ jest płaski. Relacje tworzą odstępy, kolumny, nagłówki oraz obramowania
terminalowe. Fokus wyróżnia barwa i pogrubienie, bez cieni.

## Shapes

Renderer paneli używa zaokrąglonych ramek znakowych Lip Gloss. Aktywne zakładki
otrzymują nawiasy `[ ]`, zaznaczony wpis znacznik `›`, a pasek postępu znaki `━` i `─`.

## Components

**Agents & runs.** Wybieralny wpis łączy agenta, zadanie, stan wykonania i model.
Lista obejmuje orkiestratora, aktywne wykonania i niezamknięte sesje bieżących prób
niezaakceptowanych zadań. Zaznaczenie jest związane z ID także po sortowaniu. Sekcja
jest pierwszą z czterech sekcji Dashboardu (Agents & runs, Tasks, Needs attention,
Recent recorded activity); aktywna sekcja ma znaczniki tekstowe `[ ]`, dzięki czemu
fokus pozostaje widoczny bez koloru.

**Postęp i status.** Pasek postępu używa `bubbles/progress` z tokenem `accent`
i zawsze towarzyszy mu tekst `accepted/total` oraz procent; liczone są wyłącznie
zadania `accepted`. Poniżej osobny pasek statusu pokazuje live, review, blocked
i attention, więc podsumowanie nie opiera się na kolorze ani na jednej liczbie.
Puste kolekcje pokazują tytuł, jednozdaniowe wyjaśnienie i jedną prawidłową akcję;
picker projektu reklamuje `a` (Create workspace) zamiast twierdzić, że TUI nigdy
nie tworzy workspace'u.

**Board.** Tasks mają dwa widoki: domyślny **List** i **Board** przełączany klawiszem
`b` (stopka pokazuje `b board`/`b list`). Board to jedna kolumna na stan zadania w
stałej kolejności pending, running, blocked, needs_changes, awaiting_review, accepted,
z nierozpoznanymi stanami dopisanymi na końcu i renderowanymi jako neutralny tekst.
Karty używają tych samych danych co lista i zachowują znacznik zaznaczenia `›`; kolumny
pochodzą z nieusuniętych zadań strony Tasks, więc filtrowanie usuwa karty, a nie kolumny.
Nagłówek kolumny to badge stanu z liczbą widocznych kart, a aktywna kolumna ma obramowanie
fokusu. W trybie wide kolumny sąsiadują, a przy braku miejsca przewijają się oknem wokół
aktywnej kolumny z pozycją w linii licznika; w trybie compact widoczna jest tylko aktywna
kolumna z pagerem `‹ stan (n) › k/m`, dzięki czemu board pozostaje czytelny już od 40×12.
Zaznaczenie pozostaje przypięte do `route.SelectedID`, a wybór List/Board jest pamiętany
tylko przez czas sesji (jak `route.Sort` i filtry) i nie jest zapisywany między uruchomieniami.

**Runtime.** Topologia tmux jest tabelą `bubbles/table` (window, pane, kind,
owner, run, state) w trybie wide, a w trybie compact tym samym danym w układzie
wierszy. Błędy runtime i stan zarządzanego interfejsu pozostają nad topologią.

**Szczegóły i podglądy.** Każdy szczegół i podgląd jest dokumentem złożonym z
jednego zestawu bloków: nagłówka tożsamości i statusu, par etykieta/wartość,
nagłówków sekcji, punktów, ostrzeżeń `[!]`, linków do powiązanych zasobów oraz
dyskretnej proveniencji (ID, digest, znaczniki czasu). Kolejność jest stała:
tożsamość i status, fakty operacyjne, narracja, powiązane zasoby, a na końcu
proveniencja. Wartości zewnętrzne są sanityzowane przed nadaniem stylu, a długie
cele, instrukcje, podsumowania, powody, linie poleceń, ścieżki i treści change
requestów zawijają się do szerokości viewportu, z twardym łamaniem
nieprzerywalnych tokenów. Jawne znaki nowej linii w tekście podglądu są
zachowane, a taby rozwijane. Każdy dokument przewija się w istniejącym
viewportcie (`↑`/`↓`, `PgUp`/`PgDn`); wiersz statusu pokazuje `line x–y of n`
tylko wtedy, gdy treść przekracza widok, a pozycja przewijania wraca po powrocie
na trasę.

**Status.** Symbolowi zawsze towarzyszy podpis: `✓` sukces, `×` błąd, `!` blokada,
`◈` review, `○` oczekiwanie, `■` zatrzymanie lub zamknięcie, `◇` przerwanie lub wyjście.
`running` i `starting` używają animacji brajlowskiej co 120 ms. Podsumowanie żywych
agentów animuje się tylko przy aktywnym bieżącym Runie; inaczej pokazuje `○`.

**Nawigacja i terminal.** Strzałki lub `j`/`k` wybierają wpis; `Enter` otwiera
szczegóły. `Tab` zmienia listę Work lub typ wyników. `t` otwiera zweryfikowany
bieżący terminal, a start lub wznowienie wymaga jawnego potwierdzenia formularza.
Wiele sesji zadania wymaga wyboru konkretnej sesji. Historyczny Run zachowuje
dokładny cel i nie przekierowuje automatycznie do nowszego wykonania.

**Filtr.** `/` edytuje wyszukiwanie po nazwie, ID i podtytule bez rozróżniania
wielkości liter. `Enter` zatwierdza. `Esc` podczas edycji przywraca wcześniejszy filtr
i zaznaczenie; poza edycją usuwa najpierw filtr tekstowy, potem statusowy, a dopiero
następnie wraca do poprzedniej strony. Stopka pokazuje dostępne działanie.

**Pomoc i skróty.** Wszystkie skróty pochodzą z jednego, scentralizowanego zestawu
`bubbles/key`. Stopka i pełna pomoc renderują wyłącznie powiązania włączone dla
bieżącego zaznaczenia; nieobsługiwane `t`, `g` i `a` nie są reklamowane dla wpisów
bez procesu lub akcji. Pełna pomoc jest przewijana przez `bubbles/viewport` i
pogrupowana na **Navigation, View, Runtime, Actions, Exit**, więc każda grupa jest
osiągalna już przy 40×12; pozycję przewijania pokazuje wiersz statusu. Na szczegółach
zadania `1`–`3` występują jako skróty do powiązanych zasobów, nie jako nawigacja
główna. Breadcrumb oraz linia typów Results (`[Artifacts] Handoffs Checks`) nazywają
miejsce bez polegania na kolorze.

## Do's and Don'ts

- Zachowuj rozróżnienie między stanem zadania, sesji i procesu oraz liczbą
  zaakceptowanych wyników.
- Utrzymuj czytelność symboli i podpisów bez koloru oraz widoczność zaznaczenia
  i stopki przy zmianie rozmiaru terminala.
- Pokazuj brak danych, błąd odświeżenia i nieaktualny snapshot jawnym tekstem.
- Nie uruchamiaj procesów przez samo otwarcie szczegółów lub odświeżenie widoku.
- Rozwijaj istniejący system znakowy terminala; fonty webowe, obrazy rastrowe
  i komponenty przeglądarkowe nie należą do tego interfejsu.

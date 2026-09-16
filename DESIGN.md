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
(`titleStyle`, `headingStyle`, `labelStyle`, `metaStyle`, `keycapStyle`,
`noticeStyle`, `selectedStyle`, `panelStyle`): pogrubiony nagłówek, akcent
sekcji, przygaszony podtytuł, podświetlenie zaznaczenia i ramka fokusu.
Szerokości mierzy się w kolumnach terminala, z uwzględnieniem Unicode; długi
tekst jest skracany przez `…`.

## Layout

Powłoka ma stałą kolejność: nagłówek tożsamości i świeżości, główna nawigacja,
opcjonalny breadcrumb/ nawigacja wtórna, treść, wiersz statusu/komunikatu oraz
kontekstowa legenda klawiszy. Wiersz statusu i legenda są rezerwowane zawsze,
więc pozostają widoczne przy każdym wspieranym rozmiarze. Breadcrumb pojawia się
na trasach szczegółów i kolekcjach zależnych; trasy główne go nie pokazują.

- Minimum to **40×12**; mniejszy terminal pokazuje komunikat o rozmiarze.
- Jedna decyzja `layoutFor` steruje wszystkimi stronami: **tiny** poniżej 40×12,
  **compact** dla średnich terminali (jedna kolumna) oraz **wide** od 100×24
  (lista i szczegóły obok siebie). Work korzysta z tej samej decyzji zamiast
  własnego progu szerokości. Lista w trybie wide zajmuje trzy piąte szerokości.
- Kolekcje i picker projektu w trybie wide pokazują listę i szczegóły obok
  siebie; szerokość listy jest oparta na dwóch piątych szerokości terminala.
- Wpis ma dwa wiersze: stan i tytuł, następnie kontekst. Gdy na listę pozostają
  mniej niż cztery wiersze, wpis zwija się do jednego. W pozostałych rozmiarach
  sąsiednie wpisy oddziela pozioma linia. Zaznaczenie pozostaje widoczne.
- Etykiety sekcji Work skracają się poniżej 75 kolumn, a główne zakładki,
  podsumowanie i skróty używają krótszej wersji poniżej 60 kolumn.

## Elevation & Depth

Układ jest płaski. Relacje tworzą odstępy, kolumny, nagłówki oraz obramowania
terminalowe. Fokus wyróżnia barwa i pogrubienie, bez cieni.

## Shapes

Renderer paneli używa zaokrąglonych ramek znakowych Lip Gloss. Aktywne zakładki
otrzymują nawiasy `[ ]`, zaznaczony wpis znacznik `›`, a pasek postępu znaki `━` i `─`.

## Components

**Current work.** Wybieralny wpis łączy agenta, zadanie, stan wykonania i model.
Lista obejmuje orkiestratora, aktywne wykonania i niezamknięte sesje bieżących prób
niezaakceptowanych zadań. Zaznaczenie jest związane z ID także po sortowaniu.

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

## Do's and Don'ts

- Zachowuj rozróżnienie między stanem zadania, sesji i procesu oraz liczbą
  zaakceptowanych wyników.
- Utrzymuj czytelność symboli i podpisów bez koloru oraz widoczność zaznaczenia
  i stopki przy zmianie rozmiaru terminala.
- Pokazuj brak danych, błąd odświeżenia i nieaktualny snapshot jawnym tekstem.
- Nie uruchamiaj procesów przez samo otwarcie szczegółów lub odświeżenie widoku.
- Rozwijaj istniejący system znakowy terminala; fonty webowe, obrazy rastrowe
  i komponenty przeglądarkowe nie należą do tego interfejsu.

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

Paleta rozróżnia tekst podstawowy, informacje pomocnicze, fokus i znaczenie statusu.
`auto` oraz pusty wybór używają palety ciemnej. Tło terminala pozostaje ustawieniem
użytkownika; token `selection` wypełnia cały zaznaczony wpis, łącznie z jego opisem.

| Token | Dark | Light | Rola |
|---|---|---|---|
| `text` | `#E2E8F0` | `#0F172A` | Tekst podstawowy |
| `muted` | `#94A3B8` | `#475569` | Podtytuły i stany neutralne |
| `border` | `#475569` | `#94A3B8` | Obramowanie bez fokusu |
| `focus`, `info` | `#67E8F9` | `#0E7490` | Zaznaczenie, nagłówki, aktywna praca |
| `success` | `#86EFAC` | `#166534` | Accepted, completed, ready |
| `warning` | `#FDE68A` | `#92400E` | Blokady, review i przerwanie |
| `danger` | `#FDA4AF` | `#BE123C` | Failed, error |
| `selection` | `#1E293B` | `#E0F2FE` | Tło zaznaczonego wpisu |

`--no-color` usuwa kolory, zachowując tekst, symbole, pogrubienie i animację.
Status musi pozostawać rozpoznawalny bez barwy.

## Typography

Krój i rozmiar pisma wyznacza terminal. Hierarchię tworzą pogrubione nagłówki,
kolor fokusu, tło zaznaczonego wpisu, nawiasy aktywnej zakładki i przygaszony podtytuł. Szerokości mierzy się
w kolumnach terminala, z uwzględnieniem Unicode; długi tekst jest skracany przez `…`.

## Layout

Nagłówek identyfikuje workspace i świeżość danych. Pod nim są główne zakładki.
Work utrzymuje podsumowanie postępu nad przełączanymi listami Current work, Tasks,
Needs attention i Recent recorded activity. Ostatni wiersz jest zarezerwowany na
kontekstowe skróty, a komunikat zajmuje wiersz bezpośrednio nad nimi.

- Minimum to **40×12**; mniejszy terminal pokazuje komunikat o rozmiarze.
- Work od **110 kolumn** pokazuje listę i szczegóły obok siebie; lista zajmuje
  trzy piąte szerokości. Przy mniejszej szerokości pozostaje jedna kolumna.
- Kolekcje i picker projektu od **100×24** pokazują listę i szczegóły obok siebie;
  szerokość listy jest oparta na dwóch piątych szerokości terminala.
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

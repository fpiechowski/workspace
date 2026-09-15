# Wskazówki dla agentów

Cel i zakres zmian określa bieżące polecenie użytkownika. Ten plik opisuje zasady pracy i weryfikacji.

## Planowanie i zakres

- Przed edycją sprawdź stan repozytorium i przeczytaj odpowiednie pliki oraz testy. Zachowaj zastane zmiany; nie resetuj ani nie nadpisuj pracy użytkownika.
- [TODO.md](TODO.md) zawiera backlog. Przeczytaj go podczas planowania nowej pracy; realizuj pozycje tylko wtedy, gdy obejmuje je bieżące zlecenie. Aktualizuj backlog zgodnie z ustaleniami z użytkownikiem.
- Ograniczaj zmiany do zleconego celu i stosuj istniejące konwencje. Unikaj niezwiązanych refaktorów i nowych zależności bez uzasadnionej potrzeby.
- Traktuj nieśledzone i ignorowane pliki jako potencjalne dane użytkownika. Nie usuwaj ich ani nie zastępuj; pliki tymczasowe i wyniki budowania zapisuj poza repozytorium, jeśli to możliwe.
- Nie publikuj zmian ani nie wykonuj operacji na zewnętrznych usługach w ramach zwykłej weryfikacji. Rób to tylko wtedy, gdy mieści się to wprost w zleceniu.
- Gdy wymaganie wpływa na zakres lub zgodność, a nie da się go rozstrzygnąć na podstawie kodu i testów, jasno opisz przyjęte założenie lub ograniczenie.

## Narzędzia i weryfikacja

- Wymagana wersja Go to 1.24 lub nowsza (zob. `go.mod`). Formatuj zmienione pliki Go poleceniem `gofmt`.
- Podczas pracy uruchamiaj testy właściwego pakietu, a przed zakończeniem zmian w Go wykonaj `go test ./...` oraz `go vet ./...`.
- Pełny zestaw testów z tmux wymaga Linuxa lub WSL z zainstalowanym tmux. Uruchom go, gdy zmiana dotyczy procesów, współbieżności lub integracji tmux:

  ```sh
  WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s
  ```

- Przy zmianach instalacji lub budowania uruchom sprawdzenie instalacyjne z `README.md`: zbuduj binarium do nowej lokalizacji tymczasowej i przekaż jego ścieżkę do `python3 scripts/check-install.py <ścieżka-do-binarium>`.
- Testy powinny korzystać z istniejących atrap i fixture’ów. Nie łącz ich z prawdziwymi usługami, kontami ani publikacją zmian.
- Dobieraj weryfikację do zakresu: przy zmianach dokumentacji sprawdź poprawność treści i odnośników, a przy kodzie podaj wykonane testy oraz te, których nie dało się uruchomić.

## Git w środowisku sandbox

Jeśli Git zgłasza `dubious ownership`, ogranicz wyjątek do pojedynczego polecenia, np. `git -c safe.directory=<katalog-repo> status --short`. Nie zmieniaj w tym celu globalnej konfiguracji Git.

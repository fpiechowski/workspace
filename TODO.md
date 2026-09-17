# Roadmapa / backlog

Lista planowanych usprawnień. Pozycje są ułożone według priorytetu.

## Planowane

- [ ] **Komenda `workspace prime`** — dodać polecenie CLI dostarczające agentowi aktualne instrukcje operacyjne.
- [ ] **TUI jako sidebar dla tmuxa** — przemyśleć jedną instancję TUI obsługującą wszystkie workspace’y w danym projekcie; rozważyć zagnieżdżenie tmuxa, gdzie jedna instancja pełni rolę selektora projektów, a druga selektora workspace’ów.
- [ ] **Status serwera w TUI** — dodać wskaźnik stanu procesu supervisora i powiązanych usług w TUI (zielona/czerwona kropka).
- [ ] **Issues jako element pierwszej klasy** — dodać widok Issues na poziomie projektu w TUI; umożliwić tworzenie workspace’ów dla issue, traktując issue jako wejście do ich tworzenia, oraz pokazywać tę relację w widoku listy.
- [ ] **Statystyki monitoringu współbieżności** — dodać monitoring liczby równolegle działających agentów i sesji.
- [ ] **Wskaźnik odświeżania w TUI** — zastąpić bieżące powiadomienie o odświeżeniu ikoną sygnalizującą, że odświeżanie jest w toku.
- [ ] **Status `idle` sesji** — rozszerzyć statusy sesji o `idle` i wykrywać go, gdy agent nic nie wykonuje.

- [x] **TUI dla użytkownika** — dodać terminalowy interfejs oparty na Bubble Tea i Bubbles.
  - **Cel:** ułatwić człowiekowi przeglądanie stanu i obsługę projektu. Obecne CLI udostępnia YAML/JSON, co dobrze sprawdza się w pracy agentów i skryptów, ale jest mniej wygodne do codziennego użycia przez człowieka.
  - **Pierwszy zakres:** czytelny przegląd workspace’ów i ich zadań/sesji, widok szczegółów wybranego elementu oraz możliwość uruchamiania najczęstszych istniejących operacji z klawiatury.
  - **Kryteria ukończenia:** TUI obsługuje nawigację klawiaturą i zmianę rozmiaru terminala, pokazuje stany oraz błędy w zrozumiały sposób, a logikę operacji współdzieli z istniejącym CLI. Dotychczasowe polecenia i wyjście YAML/JSON pozostają dostępne dla agentów i automatyzacji.

- [ ] **Releasy, dystrybucja i aktualizacje** — uprościć instalowanie i uaktualnianie `workspace`.
  - **Cel:** użytkownik może szybko zainstalować narzędzie i utrzymywać je w aktualnej wersji bez ręcznego budowania binarium.
  - **Instalacja:** przygotować one-liner w README, który pobiera właściwe wydanie i instaluje je na maszynie użytkownika.
  - **Aktualizacja:** dodać polecenie `workspace upgrade`, które pobiera i instaluje nowsze wydanie.
  - **Proces wydań:** automatycznie budować i publikować wersjonowane paczki/binaria dla wspieranych platform; dokumentować obsługiwane systemy i architektury.
  - **Kryteria ukończenia:** nowy użytkownik może zainstalować `workspace` poleceniem z README, a istniejący — zaktualizować przez `workspace upgrade`, bez ręcznej podmiany binarium.

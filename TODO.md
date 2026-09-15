# Roadmapa / backlog

Lista planowanych usprawnień. Pozycje są ułożone według priorytetu.

## Planowane

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

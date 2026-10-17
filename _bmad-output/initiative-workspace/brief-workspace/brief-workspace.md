---
title: "Product Brief: Workspace"
status: draft
created: 2026-10-08
updated: 2026-10-09
---

# Product Brief: Workspace

## Podsumowanie

Workspace to aplikacja desktopowa do pracy z agentami AI nad zadaniami, które obejmują kilka repozytoriów. Typowy przykład to system mikroserwisów, w którym jedna zmiana dotyka serwisu, jego BFF i repo infrastruktury (GitOps). Dla każdej inicjatywy pracy, np. ticketu z issue trackera, użytkownik zakłada osobny **Workspace**. W nim uruchamia agentów, którzy znają cały Workspace i mogą przekazywać pracę agentom działającym w osobnych git worktree poszczególnych repo. Aplikacja zarządza stanem pracy (Taskami, worktree, Change Requestami, artefaktami), a agenci wykonują pracę wymagającą osądu.

Narzędzia, które sprawdziłem (Conductor, super.engineering, Orca i inne), robią głównie to samo co gołe Claude Code czy Codex: dają osobny worktree na zadanie i pozwalają uruchomić kilka agentów obok siebie. Żadne nie łączy w jednym lokalnym narzędziu kilku repo, kilku worktree tego samego repo, przekazywania pracy między agentami i stanu pracy, który przetrwa zmianę sesji i dostawcy modelu.

Narzędzie robię dla siebie, ale traktuję je poważnie. Pierwszy prototyp rozjechał się w kolejnych iteracjach, więc tym razem produkt powstaje według specyfikacji (brief → PRD → UX → architektura → implementacja), a nie przez vibe-coding.

## Problem

Duża inicjatywa pracy, np. ticket na 8 SP, w praktyce oznacza stack kilku PR-ów w repo serwisu, żeby review było lekkie, plus zmiany w repo infrastruktury i w BFF. Ostatnio przeprowadziłem taki ticket przez Codexa w aplikacji ChatGPT, z osobnym projektem na ticket i skillami, które tworzyły worktree. Zadziałało, ale:

- **LLM zajmuje się rutyną.** Tworzy worktree, przenosi kontekst między sesjami, pilnuje gałęzi, czyli robi to, co powinna robić aplikacja, i niepotrzebnie zużywa na to kontekst.
- **Nie widać stanu.** Nie wiadomo, co już zrobiono, co jest w toku i w jakiej kolejności to mergować. Ten sam problem rozbił poprzedni prototyp.
- **Ogólne GUI czatu nie nadaje się do tej pracy.** Każda inicjatywa zaśmieca sidebar i brakuje widoku zbudowanego wokół Tasków, worktree i Change Requestów.

## Rozwiązanie

Workspace nie narzuca sposobu pracy. Dostarcza model pojęć i mechanikę, a workflow wybiera użytkownik.

**Co daje produkt:**
- **Project**, czyli zbiór repozytoriów, i **Workspace** dla każdej inicjatywy pracy. Workspace ma `intent`: opis pracy, kontekst biznesowy i ograniczenia. Opis wkleja użytkownik albo agent pobiera go przez MCP, który użytkownik sam mu dostarczy.
- **Sesje agentów w dwóch zakresach.** Agent w zakresie Workspace widzi całą inicjatywę i może przekazywać pracę do zakresu Worktree. Sesję w zakresie Worktree można też uruchomić samodzielnie.
- **Taski** z zależnościami, **Worktree** (także kilka w jednym repo), **Change Requesty** i **Artefakty**: wszystko jako obiekty runtime, które agenci odczytują i zmieniają przez CLI/MCP, a użytkownik widzi w GUI.
- **Kolejność merge'a** Change Requestów w kilku repo, wyliczana z zależności Tasków.

**Przykładowy workflow (mój, nie jedyny możliwy):** agent w zakresie Workspace przygotowuje plan i zapisuje go jako Artefakt. Tworzy z niego Taski, a ja je zatwierdzam. Potem agent zakłada worktree, np. pięć w repo serwisu (po jednym na każdy PR ze stacka) oraz po jednym w infrastrukturze i w BFF, i przekazuje Taski workerom. Workerzy oddają wyniki przez handoff: podsumowanie zmian, dowody weryfikacji, otwarte pytania i odnośniki do wytworzonych Artefaktów. Agent w Workspace aktualizuje stan Tasków. Change Requesty otwierają agenci narzędziami, które sami mają (CLI/MCP do GitLaba lub GitHuba), i rejestrują je w Workspace. Merge robię sam, w kolejności, którą pokazuje aplikacja. W każdej chwili mogę wejść w dowolny Task, dopytać, poprawić coś albo zmienić podejście.

### Zasady projektowe

- **Stan żyje w obiektach runtime aplikacji, nie w plikach markdown.** Także wyniki pracy agentów są obiektami runtime (Artifact), a nie luźnymi plikami w katalogu.
- **Aplikacja wykonuje deterministyczne operacje, agent podejmuje decyzje.** Docelowo mechanika, np. rebase stacka, trafia do aplikacji. Świadomy wyjątek w v1: Change Requesty tworzy agent, żeby aplikacja nie była przywiązana do jednego forge'a.
- **Workflow to wybór użytkownika, nie produktu.** Role takie jak „orkiestrator”, „planner” czy „worker” nie należą do modelu domeny. To po prostu Agenci, czyli profile z promptem, dostawcą i modelem, uruchomieni w sesji o danym zakresie.

## Słownik pojęć

| Pojęcie | Znaczenie |
|---|---|
| **Project** | Zbiór repozytoriów, które razem tworzą system. |
| **Repository / Branch** | Repo git należące do Projectu i jego gałęzie. |
| **Workspace** | Odizolowana przestrzeń dla jednej inicjatywy pracy, zwykły katalog, a nie repo. Ma `intent` (opis pracy) i status. Status `completed` nie jest końcowy: Workspace można ponownie otworzyć. |
| **Worktree** | Git worktree jednego repo w ramach Workspace, z jedną wybraną gałęzią. W jednym Workspace może być kilka worktree tego samego repo. |
| **Task** | Jednostka pracy w Workspace: nazwa, status, kryteria akceptacji, zależności (bez cykli). Realizowany w jednym lub kilku worktree i dostarczany przez jeden lub kilka Change Requestów. |
| **ChangeRequest** | Ogólna nazwa PR-a lub MR-a, zapisana jako obiekt runtime z linkiem do forge'a oraz gałęzią źródłową i docelową. |
| **Artifact** | Wynik pracy agenta (np. plan, analiza, raport) zapisany jako obiekt runtime Workspace i dostępny dla innych agentów. Jest wersjonowany: poprawka tworzy nową wersję, a poprzednie zostają. |
| **Handoff** | Raport, który agent przekazuje po skończonej pracy: podsumowanie zmian, dowody weryfikacji (wykonane testy, build, lint itp. i ich wyniki), otwarte pytania (także wątpliwości i decyzje do podjęcia) oraz odnośniki do wytworzonych Artefaktów. |
| **Agent** | Profil: nazwa, prompt, dostawca i model. |
| **Session** | Uruchomiona sesja agenta w określonym zakresie (SessionScope). W v1 są dwa zakresy: Workspace i Worktree. |

## Co odróżnia Workspace od innych narzędzi

Osobny worktree na zadanie i obsługa kilku agent CLI są już standardem (Conductor, Vibe Kanban, Superset, Cursor, Codex, Claude Code). Workspace wyróżnia dopiero połączenie kilku rzeczy, których nie znalazłem razem w żadnym lokalnym narzędziu:

- **Kilka repo i kilka worktree tego samego repo w jednym Workspace.** super.engineering było najbliżej, ale tego nie obsługiwało.
- **Stan pracy z prawdziwego zdarzenia,** czyli Taski, zależności, Change Requesty i Artefakty, który przetrwa zmianę sesji i dostawcy modelu.
- **Kolejność merge'a dla Change Requestów w kilku repo.** GitHub obsługuje stacki tylko w obrębie jednego repo.
- **Dowolny workflow** zamiast jednego wbudowanego procesu.

Uczciwie: to nie jest przewaga konkurencyjna. To narzędzie dopasowane do mojego sposobu pracy. Warto obserwować **ivar** i **Warp Oz**, które częściowo się z nim pokrywają. Żadnego nie testowałem.

## Dla kogo

Dla mnie: programisty pracującego z mikroserwisami w wielu repo na GitLabie, który używa głównie Claude Code i OpenCode, a dostęp do różnych dostawców i modeli ma przez OpenCode. Innych użytkowników w v1 nie zakładam.

## Kryteria sukcesu

- Następną prawdziwą inicjatywę obejmującą kilka repo przeprowadzam w Workspace od początku do końca, bez wracania do Codexa w ChatGPT.
- W każdej chwili wiem z GUI, co jest zrobione, co jest w toku i w jakiej kolejności mergować.
- Nie kopiuję ręcznie kontekstu między sesjami agentów.

## Zakres

**V1 obejmuje:** aplikację desktopową; CLI/MCP do komunikacji agentów z aplikacją; integrację z Claude Code i OpenCode; sesje w zakresach Workspace i Worktree; Taski z zależnościami; kilka worktree jednego repo; Artefakty; Change Requesty otwierane przez agenta i rejestrowane w aplikacji; kolejność merge'a w kilku repo; status Workspace `completed`.

**Poza v1:**
- **Pierwsze kroki po v1:** automatyczny rebase stacka CR, przekazywanie uwag z review do workerów, archiwizacja Workspace. W v1 robię to ręcznie albo proszę o to agenta.
- **Później lub wcale:** sesje w zakresie Project, np. agent zakładający Workspace z opisu pracy; tryb serwera z dostępem z telefonu i przeglądarki; integracja z Codex CLI (modele OpenAI są dostępne przez OpenCode); integracja z issue trackerem (w tym z Jirą) i z forge'em git (użytkownik dostarcza agentom własne CLI/MCP).

## Wizja

Jedno miejsce, w którym prowadzę dowolną inicjatywę pracy od przyjęcia do merge'a w dowolnej liczbie repo, z dowolnymi modelami i własnym workflow. Aplikacja przejmuje coraz więcej mechaniki: rebase stacków, wracające uwagi z review, zakładanie Workspace z opisu pracy. Agenci zajmują się tylko pracą wymagającą osądu. Docelowo aplikacja działa też jako serwer, więc pracę rozpoczętą przy biurku mogę kontynuować z telefonu.

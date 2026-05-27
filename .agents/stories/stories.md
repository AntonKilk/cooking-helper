# User Stories — Cooking Helper MVP

**Источник:** `.agents/PRDs/PRD.md` (Draft v3)
**Сгенерировано:** 2026-05-26 · **Обновлено:** 2026-05-27 под `.agents/tech-design.md` v1 (Go-стек)
**Репозиторий:** AntonKilk/cooking-helper
**Всего историй:** 21 (Phase 1: 6, Phase 2: 5, Phase 3: 7, Phase 4: 3)
**Стек (locked-in):** Go + `html/template` + HTMX, SQLite, Anthropic Go SDK, PWA на iPad,
Docker на Mac mini + Tailscale Serve. Детали — [`tech-design.md`](../tech-design.md).

> **CH-1 закрыт** — tech-design зафиксирован, спайк выполнен. Истории ниже обновлены под
> конкретный Go-стек (раньше содержали TS/JS-предположения, унаследованные от PRD v2).

---

## Phase 1 — Foundation

### CH-1 Tech design document

**Type:** Spike
**Priority:** High
**Complexity:** Medium
**Phase:** 1
**Status:** ✅ Done — `.agents/tech-design.md` v1 (GitHub Issue #1)
**Labels:** `spike`, `architecture`

#### Description
Как разработчик, я хочу зафиксировать tech design (платформа, frontend-фреймворк, хранилище, LLM-провайдер, стратегия изображений), чтобы остальные истории строились на конкретном стеке, а не на абстракциях.

#### Acceptance Criteria
- [x] Документ `.agents/tech-design.md` существует и отвечает на все 6 открытых вопросов из PRD §8
- [x] Решения уважают constraints из PRD §8 (iPad Safari primary, offline для рецептов, миграция на cloud)
- [x] Документ содержит обоснование выбора, альтернативы и «when to revisit» по каждому решению
- [x] Решения отражены в PRD v3 (§6, §8, §9, §15)

#### Итоговые решения
- Архитектура: **home-network-first** (данные на Mac mini, iPad — клиент в tailnet)
- Платформа: **PWA**; Frontend: **Go `html/template` + HTMX** (SSR, не SPA)
- Хранилище: **SQLite** в Docker volume (`database/sql`, без ORM), миграции `golang-migrate`
- LLM: **Anthropic Go SDK** через `internal/llm`; Sonnet 4.6 (генерация) + Haiku 4.5 (категоризация)
- Хостинг: **Docker на Mac mini i7 + Tailscale Serve** (HTTPS в tailnet, без Funnel)
- Изображения: **нет в MVP**, emoji + типографика

#### Dependencies
- Blocks: CH-2, CH-3, CH-7

---

### CH-2 Project skeleton & dev environment

**Type:** Technical
**Priority:** High
**Complexity:** Small
**Phase:** 1
**Labels:** `technical`, `infrastructure`

#### Description
Как разработчик, я хочу базовый скелет Go-проекта с работающим dev-сервером, чтобы можно было запустить пустое приложение и наращивать функциональность.

#### Acceptance Criteria
- [ ] `go run ./cmd/server` поднимает HTTP-сервер локально
- [ ] Layout пакетов соответствует tech-design §4.4 (`cmd/server`, `internal/{domain,handler,service,repository,llm,i18n,shopping}`)
- [ ] `GET /healthz` возвращает 200 (заглушка проверки БД)
- [ ] Настроены `gofmt -s`, `go vet`, `golangci-lint` — прогоняются без ошибок
- [ ] `Dockerfile` + `docker-compose.yml` собирают и запускают контейнер
- [ ] README с инструкцией запуска (локально и через Docker)

#### Technical Notes
- Стек зафиксирован в CH-1 / tech-design: Go + `html/template` + HTMX + SQLite
- Strict typing по умолчанию, избегать `interface{}`/`any` без необходимости
- Тесты — `go test ./...`; CI прогоняет fmt/vet/lint/test
- Tailscale Serve настраивается на хосте, не в этой истории (см. CH-21 / ops)

#### Dependencies
- Blocked by: CH-1
- Blocks: CH-3, CH-4, CH-6

---

### CH-3 Data models & SQLite repository layer

**Type:** Technical
**Priority:** High
**Complexity:** Medium
**Phase:** 1
**Labels:** `technical`, `storage`

#### Description
Как разработчик, я хочу слой персистентности с типизированными моделями `HouseholdProfile`, `Recipe`, `WeeklyPlan`, `ShoppingListItem`, чтобы остальные фичи работали с данными через единый репозиторий-интерфейс.

#### Acceptance Criteria
- [ ] Схемы из PRD §15 Appendix реализованы как Go structs в `internal/domain` (без `sql.Row`/HTTP-типов внутри)
- [ ] Первая миграция `golang-migrate` создаёт все таблицы; применяется на старте
- [ ] `internal/repository` умеет CRUD по каждой модели через `database/sql` (без ORM)
- [ ] SQL присутствует только в `internal/repository` — ни в service, ни в handler
- [ ] `household_id` UUID в каждой таблице (под будущий мультиюзер)
- [ ] Многотабличные записи (WeeklyPlan + ShoppingList) — в одной транзакции
- [ ] Unit-тесты на CRUD каждой модели (SQLite-файл во временной директории)

#### Technical Notes
- См. PRD §15 Appendix и tech-design §3.3
- Не забывать `created_at` / `updated_at`
- Таймауты запросов через `context`; SQLite single-writer — короткие write-транзакции

#### Dependencies
- Blocked by: CH-1, CH-2
- Blocks: CH-5, CH-8, CH-12

---

### CH-4 i18n framework with RU/FI/EN

**Type:** Feature
**Priority:** High
**Complexity:** Small
**Phase:** 1
**Labels:** `feature`, `i18n`

#### Description
Как пользователь, я хочу выбрать язык интерфейса между русским, финским и английским, чтобы читать приложение на родном языке семьи. (US-9)

#### Acceptance Criteria
- [ ] При первом запросе язык определяется из заголовка `Accept-Language`, далее хранится в session cookie
- [ ] Переключатель языка в настройках обновляет cookie и перерисовывает страницу (редирект/HTMX swap)
- [ ] Все строки UI берутся через `t(key, args...)`, зарегистрированную в `template.FuncMap` — нет хардкода
- [ ] Подключены словари `i18n/ru.json` / `fi.json` / `en.json`
- [ ] Тестовый шаблон рендерится на каждом языке корректно (включая финские/русские символы)

#### Technical Notes
- Реализация — `internal/i18n` + `t()` в шаблонах (tech-design §4.1)
- Категории магазина из PRD §15 Appendix — первая локализованная сущность
- Сгенерированные рецепты не локализуются (язык фиксируется при создании, PRD §F-9)

#### Dependencies
- Blocked by: CH-2

---

### CH-5 Household profile screen

**Type:** Feature
**Priority:** High
**Complexity:** Small
**Phase:** 1
**Labels:** `feature`, `onboarding`

#### Description
Как пользователь, я хочу указать состав семьи (взрослых и детей), чтобы порции в подборе считались правильно.

#### Acceptance Criteria
- [ ] Экран профиля доступен из настроек
- [ ] Поля: количество взрослых (1-6), количество детей (0-6), язык
- [ ] Изменения сохраняются на сервере (SQLite через CH-3) и применяются к следующей генерации
- [ ] При первом запуске профиль создаётся с дефолтами (2 взрослых, 0 детей, язык из `Accept-Language`)

#### Technical Notes
- Использует репозиторий из CH-3
- Один профиль на household в MVP (`household_id`)

#### Dependencies
- Blocked by: CH-3, CH-4

---

### CH-6 App navigation & layout shell

**Type:** Technical
**Priority:** Medium
**Complexity:** Small
**Phase:** 1
**Labels:** `technical`, `ui`

#### Description
Как пользователь, я хочу базовую навигацию между главным экраном, рецептом и настройками, чтобы пользоваться приложением как обычным планшетным приложением.

#### Acceptance Criteria
- [ ] HTTP-роуты: главный `/`, рецепт `/recipe/{id}`, настройки `/settings` — рендер через `html/template`
- [ ] Переходы и частичные обновления через HTMX (partial swap), без полной SPA
- [ ] Базовый layout-шаблон (шапка с заголовком и кнопкой настроек) на каждом экране
- [ ] Манифест PWA + регистрация Service Worker подключены в layout
- [ ] Открывается на iPad Safari (через tailnet HTTPS) без визуальных артефактов

#### Technical Notes
- iPad-first вёрстка из PRD §4 «iPad UX», Nordic Kitchen (tech-design §4.5)
- Можно использовать заглушечные шаблоны в этой истории
- SW требует HTTPS — тестировать по Tailscale Serve URL, не по plain-HTTP `go run`

#### Dependencies
- Blocked by: CH-2

---

## Phase 2 — LLM Integration & Core Generation

### CH-7 LLM service abstraction with retry & JSON parsing

**Type:** Technical
**Priority:** High
**Complexity:** Medium
**Phase:** 2
**Labels:** `technical`, `llm`

#### Description
Как разработчик, я хочу провайдер-агностичный LLM-сервис с retry-логикой и строгим парсингом JSON-ответов, чтобы дальнейшие фичи (генерация меню, swap) использовали единый стабильный интерфейс.

#### Acceptance Criteria
- [ ] Интерфейс `Client` в `internal/llm` принимает `prompt + schema`, возвращает типизированный Go-объект или ошибку
- [ ] Реализация `internal/llm/anthropic` использует Anthropic Go SDK
- [ ] Явный таймаут (`context.WithTimeout`) на каждый вызов
- [ ] Retry с экспоненциальной задержкой (2s→4s→8s) на сетевых/5xx, max 3; 4xx не повторяются
- [ ] Невалидный JSON → авто-повтор с уточняющим хинтом (max 1), затем ошибка
- [ ] Промпты — версионируемые файлы в `internal/llm/prompts/` (`*.v1.txt`)
- [ ] Prompt caching: стабильная часть (профиль+disliked+pantry+feedback) кэшируется
- [ ] Логирование token count и latency (бюджет-мониторинг); содержимое промптов в проде не логируется
- [ ] Unit-тесты: успех, retry, fallback на ошибке

#### Technical Notes
- Модели: `claude-sonnet-4-6` (генерация/swap), `claude-haiku-4-5-20251001` (категоризация) — tech-design §3.4
- `ANTHROPIC_API_KEY` только в env на сервере, не в коде/клиенте
- Никаких прямых SDK-вызовов в handler/service — только через `Client`

#### Dependencies
- Blocked by: CH-1

---

### CH-8 Weekly menu generation (F-1)

**Type:** Feature
**Priority:** High
**Complexity:** Large
**Phase:** 2
**Labels:** `feature`, `llm`, `generation`

#### Description
Как пользователь, я хочу одним нажатием получить план недели из 3 разных рецептов с порциями на 7 дней, чтобы не тратить время на ручное планирование. (US-1)

#### Acceptance Criteria
- [ ] Кнопка «Сгенерировать неделю» на главном экране запускает генерацию (HTMX-запрос)
- [ ] Промпт получает household profile, disliked, pantry, последние feedback-записи, историю недели
- [ ] Ответ парсится как 3 объекта `Recipe` со структурой из PRD §15
- [ ] Порции суммарно покрывают `7 дней × family_size`
- [ ] Среди 3 рецептов минимум 2 категории белка (разнообразие)
- [ ] Время от тапа до отрисовки карточек ≤ 30 секунд
- [ ] На главном экране отрисовываются 3 карточки (название, время, краткое описание, emoji белка)

#### Technical Notes
- Использует LLM-сервис из CH-7, промпт `generate_week.v1.txt`
- Сохранять `WeeklyPlan` и `Recipe[]` на сервере через CH-3 репозиторий (одна транзакция)

#### Dependencies
- Blocked by: CH-3, CH-5, CH-6, CH-7
- Blocks: CH-9, CH-10, CH-11, CH-12

---

### CH-9 Recipe swap & full regenerate (F-2)

**Type:** Feature
**Priority:** High
**Complexity:** Medium
**Phase:** 2
**Labels:** `feature`, `llm`, `generation`

#### Description
Как пользователь, я хочу заменить один из 3 рецептов, не теряя остальные, или перегенерировать всю подборку, если ни один не подошёл. (US-2, US-3)

#### Acceptance Criteria
- [ ] Кнопка «Заменить» на карточке заменяет только этот рецепт
- [ ] Промпт замены получает 2 оставшихся рецепта в контексте, чтобы не дублировать профиль блюда
- [ ] Кнопка «Перегенерировать всё» полностью пересоздаёт `WeeklyPlan`
- [ ] При замене порции пересчитываются, чтобы суммарно покрыть 7 дней
- [ ] Shopping list инвалидируется при изменении подборки

#### Technical Notes
- Отдельный промпт `swap_recipe.v1.txt`
- При полной регенерации старый `WeeklyPlan` уходит в архив (не удаляется)

#### Dependencies
- Blocked by: CH-8

---

### CH-10 Disliked-ingredients post-validation

**Type:** Feature
**Priority:** High
**Complexity:** Small
**Phase:** 2
**Labels:** `feature`, `validation`, `llm`

#### Description
Как пользователь, я хочу быть уверен, что мои нелюбимые ингредиенты никогда не попадут в подбор, даже если LLM их «забудет». (US-5, defence in depth)

#### Acceptance Criteria
- [ ] После каждого ответа LLM сравниваем все ингредиенты со списком disliked
- [ ] При обнаружении нарушения генерация повторяется (макс 2 попытки) с явным акцентом в промпте
- [ ] Если после 2 повторов нарушение остаётся — показывается ошибка пользователю, без молчаливого игнорирования
- [ ] Сравнение учитывает падежи и варианты написания (нормализация / LLM-based matching)
- [ ] Метрика «частота нарушений» логируется для мониторинга качества промпта

#### Technical Notes
- См. PRD §14 Risks — «Disliked ingredients игнорируются LLM»
- Нормализация имён ингредиентов — отдельная утилита в `internal/shopping` или `internal/llm`

#### Dependencies
- Blocked by: CH-8

---

### CH-11 Fullscreen recipe view (F-4)

**Type:** Feature
**Priority:** Medium
**Complexity:** Small
**Phase:** 2
**Labels:** `feature`, `ui`, `ipad`

#### Description
Как пользователь, я хочу видеть рецепт крупным шрифтом во время готовки, чтобы читать с iPad на столе без масштабирования. (US-8)

#### Acceptance Criteria
- [ ] Тап по карточке открывает полноэкранный режим
- [ ] Body text минимум 18pt, заголовки минимум 24pt (Nordic Kitchen)
- [ ] Ингредиенты и шаги в двух колонках на iPad landscape, одной — на portrait
- [ ] Активный шаг визуально выделен, есть возможность отметить шаг выполненным
- [ ] Тёмный режим уважается (CSS `prefers-color-scheme`)
- [ ] Прокрутка плавная одним пальцем, touch-targets ≥44×44pt

#### Technical Notes
- Markup генерируется `frontend-design` skill (HTML/CSS only), переносится в `templates/*.gohtml`
- Тестировать на iPad с расстояния 50см; никаких hover-only взаимодействий

#### Dependencies
- Blocked by: CH-8

---

## Phase 3 — Shopping List & Personalization

### CH-12 Shopping list builder with consolidation (F-3)

**Type:** Feature
**Priority:** High
**Complexity:** Large
**Phase:** 3
**Labels:** `feature`, `shopping-list`, `llm`

#### Description
Как пользователь, я хочу автоматический список покупок из всех 3 рецептов с консолидацией одинаковых ингредиентов и приведением единиц, чтобы не считать вручную. (US-4)

#### Acceptance Criteria
- [ ] При создании `WeeklyPlan` автоматически генерируется `shopping_list`
- [ ] Одинаковые ингредиенты суммируются (250г + 100г моркови = 350г)
- [ ] Несовместимые единицы (1 шт + 100г) показываются раздельно
- [ ] Каждый ингредиент имеет `category` (produce / meat_fish / dairy / pantry / frozen / other)
- [ ] Категоризация работает на 95%+ ингредиентов в тестовом наборе (5 разных недель)
- [ ] Ингредиенты из `pantry_basics` исключаются из списка (US-7)

#### Technical Notes
- Логика в `internal/shopping`; категоризация — промпт `categorize_ingredient.v1.txt` (Haiku) или словарь + LLM fallback
- Нормализация единиц: g, kg, ml, l, шт, ст.л., ч.л., dl, tl, rkl + финские эквиваленты
- Кэшировать категорию по имени ингредиента (в БД), чтобы не вызывать LLM повторно

#### Dependencies
- Blocked by: CH-8

---

### CH-13 Shopping list UI (categories, checkboxes, manual remove)

**Type:** Feature
**Priority:** High
**Complexity:** Medium
**Phase:** 3
**Labels:** `feature`, `ui`, `shopping-list`

#### Description
Как пользователь, я хочу видеть список покупок, сгруппированный по отделам магазина, отмечать купленное чекбоксами и удалять лишнее, чтобы быстро пройти магазин. (US-4)

#### Acceptance Criteria
- [ ] Экран shopping list доступен с главного и из меню
- [ ] Группировка по 6 категориям с локализованными заголовками
- [ ] Чекбокс рядом с каждым пунктом, тап = отмечено (HTMX-запрос, состояние в БД)
- [ ] Отмеченные пункты опционально скрываются («показать купленное» переключатель)
- [ ] Кнопка «удалить» убирает пункт из списка (с возможностью отменить)
- [ ] Состояние сохраняется на сервере (offline-изменения реплеятся Service Worker'ом)

#### Technical Notes
- Категории и переводы — из PRD §15 Appendix таблицы
- Использовать iPad-оптимизированную вёрстку из CH-11
- Запись чекбокса должна быть идемпотентной (SW может повторить)

#### Dependencies
- Blocked by: CH-12

---

### CH-14 Pantry basics management (F-6)

**Type:** Feature
**Priority:** Medium
**Complexity:** Small
**Phase:** 3
**Labels:** `feature`, `settings`

#### Description
Как пользователь, я хочу редактировать список «всегда есть дома», чтобы соль, перец и масло не попадали в shopping list. (US-7)

#### Acceptance Criteria
- [ ] Экран pantry basics доступен из настроек
- [ ] Дефолтный список: соль, чёрный перец, растительное масло, сливочное масло, мука пшеничная, сахар (локализованные)
- [ ] Добавление ингредиента через текстовое поле
- [ ] Удаление через свайп или кнопку
- [ ] Изменения применяются к следующей генерации shopping list
- [ ] Список сохраняется в `HouseholdProfile.pantry_basics`

#### Technical Notes
- См. PRD §15 Appendix «Дефолтный список pantry_basics»

#### Dependencies
- Blocked by: CH-5

---

### CH-15 Disliked ingredients management (F-7)

**Type:** Feature
**Priority:** Medium
**Complexity:** Small
**Phase:** 3
**Labels:** `feature`, `settings`

#### Description
Как пользователь, я хочу указать ингредиенты, которые я не люблю, чтобы они никогда не попадали в подбор. (US-5)

#### Acceptance Criteria
- [ ] Экран disliked доступен из настроек
- [ ] Добавление через текстовое поле с автоподсказками из истории
- [ ] Удаление через свайп или кнопку
- [ ] Список передаётся в промпт каждой генерации как hard constraint
- [ ] Изменения применяются к следующей генерации
- [ ] Пустой список валиден

#### Technical Notes
- Хранится в `HouseholdProfile.disliked_ingredients`
- См. также CH-10 (post-validation)

#### Dependencies
- Blocked by: CH-5

---

### CH-16 Recipe feedback collection (F-5)

**Type:** Feature
**Priority:** High
**Complexity:** Small
**Phase:** 3
**Labels:** `feature`, `feedback`

#### Description
Как пользователь, я хочу оценить рецепт после готовки (лайк / дизлайк / готовить снова), чтобы система училась моим вкусам. (US-6)

#### Acceptance Criteria
- [ ] Три независимых булевых поля на карточке и в детальном просмотре
- [ ] Состояние сохраняется в `Recipe.feedback` с timestamp
- [ ] Можно изменить feedback позже (не финальное действие)
- [ ] В архиве и текущей неделе видны иконки feedback

#### Technical Notes
- Не overlay — UI-элементы интегрированы в карточку
- Один тап = одно состояние, без подтверждений; запись идемпотентна (SW-реплей)

#### Dependencies
- Blocked by: CH-8

---

### CH-17 Feedback integration in next-generation prompt

**Type:** Feature
**Priority:** High
**Complexity:** Small
**Phase:** 3
**Labels:** `feature`, `llm`, `personalization`

#### Description
Как пользователь, я хочу, чтобы мои лайки и дизлайки влияли на следующую подборку, без необходимости что-то настраивать. (US-6 продолжение)

#### Acceptance Criteria
- [ ] Промпт генерации недели получает последние N (например, 20) feedback-записей с названиями и оценками
- [ ] Дизлайкнутые рецепты не появляются повторно (или с очень низкой вероятностью)
- [ ] Лайкнутые рецепты влияют на стиль через текст промпта (не фиксированный список)
- [ ] Параметр N конфигурируется (но не через UI)
- [ ] Объём контекста промпта не превышает разумного лимита токенов

#### Technical Notes
- Использует CH-7 LLM-сервис и CH-8 промпт
- Сериализация feedback в текст — отдельная утилита; кэшируемая часть промпта (prompt caching)

#### Dependencies
- Blocked by: CH-8, CH-16

---

### CH-18 Recipe archive with search & "cook again" (F-8)

**Type:** Feature
**Priority:** Medium
**Complexity:** Medium
**Phase:** 3
**Labels:** `feature`, `archive`

#### Description
Как пользователь, я хочу видеть все ранее сгенерированные рецепты, искать их по названию и возвращать в текущую неделю. (часть US-1, бонус)

#### Acceptance Criteria
- [ ] Экран архива доступен из главного меню
- [ ] Список всех `Recipe` упорядочен по дате создания (новые сверху)
- [ ] Поиск по подстроке в названии (HTMX, debounce ~200мс)
- [ ] Кнопка «Приготовить снова» добавляет рецепт в текущий `WeeklyPlan` (заменяет один из 3 — диалог выбора какой)
- [ ] Иконки feedback видны в списке
- [ ] Если архив недоступен (ошибка чтения) — graceful degradation, не 500 на всю страницу

#### Technical Notes
- Использует репозиторий CH-3
- При «cook again» обновить shopping list (см. CH-12)

#### Dependencies
- Blocked by: CH-8

---

## Phase 4 — Polish & Beta

### CH-19 Onboarding flow

**Type:** Feature
**Priority:** Medium
**Complexity:** Small
**Phase:** 4
**Labels:** `feature`, `onboarding`

#### Description
Как новый пользователь, я хочу краткое знакомство с приложением при первом запуске, чтобы понять, как пользоваться основными фичами.

#### Acceptance Criteria
- [ ] При первом запуске показывается 3-4 экрана с ключевыми возможностями
- [ ] Шаг 1: установка размера семьи и языка
- [ ] Шаг 2: первоначальный pantry basics (с возможностью оставить дефолт)
- [ ] Шаг 3: краткое объяснение цикла «генерация → готовка → feedback»
- [ ] Кнопка «пропустить» доступна на каждом шаге
- [ ] Onboarding не показывается повторно (флаг в профиле/БД)

#### Technical Notes
- Использует CH-5, CH-14; markup через `frontend-design` skill

#### Dependencies
- Blocked by: CH-5, CH-14

---

### CH-20 iPad UX polish & accessibility

**Type:** Enhancement
**Priority:** Medium
**Complexity:** Medium
**Phase:** 4
**Labels:** `enhancement`, `ipad`, `accessibility`

#### Description
Как пользователь iPad, я хочу, чтобы приложение выглядело и работало нативно на iPad — без багов вёрстки, с хорошей контрастностью и поддержкой мокрых рук.

#### Acceptance Criteria
- [ ] Никаких визуальных артефактов на iPad Safari portrait/landscape
- [ ] Все интерактивные элементы — минимум 44×44pt (Apple HIG)
- [ ] Контрастность WCAG AA для текста на всех экранах
- [ ] Тёмный режим корректен (Nordic Kitchen инвертированная палитра)
- [ ] Никаких scroll-jacking или нестандартных жестов
- [ ] PWA устанавливается на home-screen, offline-кэш рецептов работает

#### Technical Notes
- Прогоняется по checklist на реальном устройстве через tailnet HTTPS
- Возможны фиксы в каждом из предыдущих компонентов

#### Dependencies
- Blocked by: вся Phase 1-3

---

### CH-21 Beta testing & bug bash

**Type:** Technical
**Priority:** High
**Complexity:** Small
**Phase:** 4
**Labels:** `technical`, `testing`

#### Description
Как разработчик, я хочу провести структурированное тестирование MVP с семьёй автора, чтобы убедиться, что метрики из PRD §11 достигаются.

#### Acceptance Criteria
- [ ] Развёртывание на Mac mini (Docker + Tailscale Serve), iPad видит сервер по tailnet
- [ ] **Отложенные проверки из dev-среды прогнаны на сетевом хосте** (нельзя выполнить в web-sandbox):
  - [ ] `govulncheck ./...` зелёный — **блокер: зависимости, добавленные в CH-3 (`modernc.org/sqlite`, `golang-migrate`, `google/uuid`), ещё ни разу не сканировались**
  - [ ] `docker build` / `docker compose up` собирается на реальных base-образах (не проверялось с CH-2)
  - [ ] Service Worker регистрируется и кэширует shell по tailnet HTTPS на iPad Safari (CH-6)
- [ ] Настроен ежедневный бэкап БД (`launchd` + `sqlite3 .backup`, retention 14 дней)
- [ ] Чеклист бета-тестирования составлен (10+ сценариев из user stories)
- [ ] 2 недели реального использования семьёй автора зафиксированы
- [ ] Метрика «время до shopping list» < 2 минут подтверждена замерами
- [ ] Все P0/P1 баги по итогам — закрыты
- [ ] Финальный отчёт сохранён в `.agents/reports/beta-1.md`

#### Technical Notes
- См. PRD §11 «Success Criteria», §12 Phase 4, tech-design §7 Operations

#### Dependencies
- Blocked by: вся Phase 1-3, CH-20

---

## Сводная таблица

| ID    | Title                                                | Type        | Phase | Complexity | Status |
|-------|------------------------------------------------------|-------------|-------|------------|--------|
| CH-1  | Tech design document                                 | Spike       | 1     | Medium     | ✅ Done |
| CH-2  | Project skeleton & dev environment                   | Technical   | 1     | Small      | —      |
| CH-3  | Data models & SQLite repository layer                | Technical   | 1     | Medium     | —      |
| CH-4  | i18n framework with RU/FI/EN                         | Feature     | 1     | Small      | —      |
| CH-5  | Household profile screen                             | Feature     | 1     | Small      | —      |
| CH-6  | App navigation & layout shell                        | Technical   | 1     | Small      | —      |
| CH-7  | LLM service abstraction with retry & JSON parsing    | Technical   | 2     | Medium     | —      |
| CH-8  | Weekly menu generation (F-1)                         | Feature     | 2     | Large      | —      |
| CH-9  | Recipe swap & full regenerate (F-2)                  | Feature     | 2     | Medium     | —      |
| CH-10 | Disliked-ingredients post-validation                 | Feature     | 2     | Small      | —      |
| CH-11 | Fullscreen recipe view (F-4)                         | Feature     | 2     | Small      | —      |
| CH-12 | Shopping list builder with consolidation (F-3)       | Feature     | 3     | Large      | —      |
| CH-13 | Shopping list UI                                     | Feature     | 3     | Medium     | —      |
| CH-14 | Pantry basics management (F-6)                       | Feature     | 3     | Small      | —      |
| CH-15 | Disliked ingredients management (F-7)                | Feature     | 3     | Small      | —      |
| CH-16 | Recipe feedback collection (F-5)                     | Feature     | 3     | Small      | —      |
| CH-17 | Feedback integration in next-generation prompt       | Feature     | 3     | Small      | —      |
| CH-18 | Recipe archive with search & "cook again" (F-8)      | Feature     | 3     | Medium     | —      |
| CH-19 | Onboarding flow                                      | Feature     | 4     | Small      | —      |
| CH-20 | iPad UX polish & accessibility                       | Enhancement | 4     | Medium     | —      |
| CH-21 | Beta testing & bug bash                              | Technical   | 4     | Small      | —      |

**Покрытие PRD:** US-1…US-9 → CH-8, CH-9, CH-13, CH-15, CH-16, CH-7+CH-14, CH-11, CH-4. F-1…F-9 → CH-8, CH-9, CH-12, CH-11, CH-16, CH-14, CH-15, CH-18, CH-4. Все 4 фазы PRD §12 покрыты.

# Tech Design — Cooking Helper

**Автор:** ant.kilk@gmail.com
**Дата:** 2026-05-26
**Статус:** Draft v1 (закрывает CH-1)
**Связанные документы:** [PRD v2](PRDs/PRD.md), [User Stories](stories/stories.md)

---

## 1. Context

PRD §8 «Technology Stack» был помечен `DEFERRED` с шестью открытыми вопросами.
Этот документ отвечает на каждый из них и фиксирует архитектурные решения,
от которых зависят CH-2, CH-3, CH-7 и дальше.

**Базовые ограничения (из PRD §3, §4, §8):**
- iPad Safari — primary target
- Кухонный UX: крупный шрифт, минимум тапов, читаемость на расстоянии 50см
- RU/FI/EN с первого коммита
- Архитектура под будущий рост (multi-user, миграция на cloud)
- Семейное приложение, single household в MVP

**Базовые ограничения (от автора, добавлено в этой сессии):**
- Hosting дома, бюджет $0 в месяц на инфраструктуру
- LLM API через Anthropic, бюджет — личное использование
- Apple Reminders как target для экспорта shopping list (Phase 2)
- Кругозор и предпочтения автора по языку — Go для маленьких проектов

---

## 2. Главное архитектурное решение: Home-server-first

PRD §6 говорил «local-first данные хранятся локально на устройстве». После
обсуждения мы переходим к **home-server-first**: данные централизованы на
домашнем сервере, iPad — тонкий клиент в домашней сети.

### Альтернативы

| Подход | Где данные | Зависимость от сервера | LLM API key |
|---|---|---|---|
| **Pure local-first PWA** (как было в PRD §6) | IndexedDB / wa-sqlite в браузере | нет | на клиенте или через прокси |
| **Home-server-first** (выбран) | SQLite на Mac mini | iPad без Mac mini не работает | на сервере |
| Cloud-hosted | Managed Postgres + serverless | требуется uptime провайдера | на сервере |

### Решение
Home-server-first. Pure local-first отброшен из-за:
- API-ключ Anthropic на клиенте небезопасно, прокси-сервер всё равно нужен
- Логику генерации меню и shopping list проще держать на сервере
- iPad всегда дома на кухне — кейс «работать в магазине» решается экспортом
  в Apple Reminders, не онлайн-доступом
- Бесплатный hosting дома — соответствует бюджету

### Последствия
- **PRD §6 обновляется в v3:** «Home-network-first данные централизованы
  на домашнем сервере, iPad — клиент в домашней сети»
- iPad без Mac mini не работает. Это accepted trade-off.
- Бэкап БД становится критичным (раздел 7)

### When to revisit
- Если семья начнёт пользоваться приложением вне дома регулярно
- Если Mac mini становится недоступен надолго (миграция в cloud)

---

## 3. Решения по PRD §8 open questions

### 3.1 Платформа: PWA vs Native iOS vs React Native

**Decision: PWA**

| Альтернатива | Плюсы для проекта | Минусы для проекта |
|---|---|---|
| **PWA** (выбрано) | Один codebase, install-to-home-screen на iPad, Service Worker для offline-кэша, бесплатный distribution | iOS PWA ограничения (нет push до iOS 16.4, ограниченное хранилище) |
| Native iOS | Best UX, доступ к iCloud Reminders без Shortcuts | Apple Developer $99/год, Swift вне зоны компетенции, distribution усложнён |
| React Native | Cross-platform | TS обязателен, JS-стек — против выбранного Go-бэкенда |

**Consequences:**
- Manifest.json + Service Worker обязательны
- Установка как home-screen app на iPad — отдельная инструкция для семьи
- Для экспорта в Reminders — через Shortcuts x-callback-url

**When to revisit:** если iOS PWA ограничения станут болезненными (push,
background sync) — рассмотреть Native iOS как Phase 2.

---

### 3.2 Frontend framework: SSR+HTMX vs SPA

**Decision: SSR на Go (`html/template`) + HTMX**

| Альтернатива | Плюсы | Минусы |
|---|---|---|
| **Go + html/template + HTMX** (выбрано) | Один процесс на сервере, минимум JS, нет дублирования модели на клиенте, кэширование Service Worker'ом тривиально | Меньше «фронтовых» библиотек компонентов, новая парадигма для разработчика после React |
| React/Vue/Svelte SPA | Богатая экосистема, привычные паттерны | Дублирует модели на клиенте, требует API-слой, тяжелее offline, отдельная сборка |
| Next.js (SSR + React) | SSR из коробки | TS-стек, тяжелее, overkill для семейного приложения |

**Consequences:**
- Один шаблон = одна страница или partial для HTMX swap
- JavaScript только для: Service Worker, HTMX, минимальный vanilla JS для UX
- Stories CH-2, CH-3, CH-4 переписываются (npm → go, TS-types → Go structs)

**AI-assisted markup generation:**
Для генерации markup на этапе разработки используем Anthropic
[`frontend-design`](https://github.com/anthropics/skills/blob/main/skills/frontend-design/SKILL.md)
skill в HTML/CSS-режиме (не React/JSX). Сгенерированный markup
переносится в `templates/*.gohtml` и обвешивается HTMX-атрибутами
для интерактивности. Это инструмент разработки, не runtime-зависимость.
Детали — раздел 4.5.

**When to revisit:** если потребуется сложная клиентская интерактивность
(drag-and-drop, real-time updates) — добавить локально Alpine.js, не SPA.

---

### 3.3 Локальное хранилище → теперь Server-side storage

Вопрос из PRD §8 был сформулирован под local-first архитектуру. После
перехода на home-server-first вопрос трансформируется: какая БД на сервере?

**Decision: SQLite в Docker volume**

| Альтернатива | Плюсы | Минусы |
|---|---|---|
| **SQLite** (выбрано) | Один файл, бэкап `cp`, нет второго контейнера, отлично подходит single-writer | Один процесс пишет одновременно |
| PostgreSQL | Concurrency, fulltext-search, JSON-операторы | Второй контейнер, второй volume, миграции, overkill для семейного app |
| IndexedDB на клиенте | Pure local-first | Отброшено вместе с pure local-first (раздел 2) |

**Consequences:**
- Схема таблиц = модель данных из PRD §15 Appendix
- `household_id` UUID в каждой таблице (под future multi-user)
- Доступ через `database/sql`, без ORM
- Миграции через `golang-migrate` или `goose`
- Ежедневный бэкап через `launchd` на Mac mini → `cp data.db /backup/$(date).db`

**When to revisit:** если single-writer станет узким местом (concurrent
edits от нескольких членов семьи одновременно) или появится потребность в
fulltext-search — мигрировать на Postgres.

---

### 3.4 LLM-провайдер: Anthropic Claude

**Decision: Anthropic Claude через официальный Go SDK, провайдер-агностичный
интерфейс в `internal/llm`**

| Альтернатива | Плюсы | Минусы |
|---|---|---|
| **Anthropic Go SDK напрямую** (выбрано) | Качество Claude, прямой контроль над промптами, prompt caching | Lock-in (митигируется через провайдер-агностичный интерфейс) |
| OpenAI | Привычка | Качество Claude выше для длинного контекста, у автора нет предпочтения |
| LangChain-Go | «Провайдер-агностичный» из коробки | Лишняя зависимость, тяжёлая абстракция, мало добавляет для single-prompt сценариев |

**Модели:**
- `claude-sonnet-4-6` — генерация недели и swap (требует разнообразия и нюансов)
- `claude-haiku-4-5-20251001` — категоризация ингредиентов в категории магазина,
  нормализация ингредиентов в shopping list (быстро + дёшево)

**Архитектура (PRD §6 «provider-agnostic»):**
```
internal/llm/
├── client.go           # interface Client { Generate(ctx, req) (resp, error) }
├── anthropic/          # реализация для Anthropic
└── prompts/            # версионируемые промпт-файлы
    ├── generate_week.v1.txt
    ├── swap_recipe.v1.txt
    └── categorize_ingredient.v1.txt
```

**Prompt caching:**
- Использовать prompt caching API: кэшировать household profile + disliked +
  pantry + история feedback как «стабильную часть» промпта, варьировать
  только триггер генерации
- Снижение стоимости на повторяющихся вызовах

**Consequences:**
- API-ключ в env-переменной на сервере, не в коде
- Логирование расхода токенов (для мониторинга бюджета) — обязательно
- Retry с экспоненциальной задержкой при сетевых ошибках, max 3 попытки
- Невалидный JSON — повтор с уточняющим хинтом, max 1 раз

**When to revisit:** если бюджет станет проблемой — оценить переход на
Haiku для большего числа задач или альтернативного провайдера через тот же
интерфейс.

---

### 3.5 Хостинг: Mac mini Intel i7 + Docker + Tailscale Serve

**Decision: Mac mini i7 как always-on сервер, приложение в Docker,
Tailscale Serve для HTTPS-доступа из домашней сети**

| Альтернатива | Плюсы | Минусы |
|---|---|---|
| **Mac mini i7 + Docker + Tailscale Serve** (выбрано) | Уже есть, бесплатно, mDNS+TS работают, авто-HTTPS | Single point of failure (но это OK для семейного app) |
| Raspberry Pi | Дешевле электричества | Покупать; ARM-сборка контейнера |
| Cloud (Fly.io / Railway) | 24/7 uptime провайдером | Платное, данные не дома |
| Synology NAS | NAS-режим uptime | У автора нет |

**HTTPS-доступ:**
- Tailscale Serve выдаёт `cooking-helper.tail-xxxx.ts.net` с Let's Encrypt
- HTTPS обязателен для Service Worker (требование браузера)
- Доступ только в tailnet — iPad с Tailscale-клиентом видит сервер
- **Не используем** Tailscale Funnel (внешний доступ не требуется)

**Mac mini настройки:**
- `Energy Saver → Start up automatically after a power failure: ON`
- `Energy Saver → Prevent computer from sleeping when display is off: ON`
- Docker Desktop запускается при логине

**Consequences:**
- Один контейнер `cooking-helper` (Go-приложение)
- Tailscale установлен на хосте macOS, не в контейнере
- Бэкапы БД через `launchd` на Mac mini
- Если Mac mini падает — семья не может пользоваться, accepted trade-off

**When to revisit:** если Mac mini станет недоступен или семья начнёт
пользоваться приложением вне дома регулярно.

---

### 3.6 Стратегия изображений для рецептов

**Decision: Без изображений в MVP, эмодзи + типографика. Изображения в Phase 2.**

| Альтернатива | Плюсы | Минусы |
|---|---|---|
| **Без изображений** (выбрано для MVP) | Простота, скорость генерации, нет проблем с правами/стоимостью | Менее «вкусный» внешний вид |
| AI-генерация (DALL-E / Stable Diffusion) | Картинка под каждый рецепт | Доп. API + бюджет, низкое качество для food, latency |
| Стоковые (Unsplash API по ключевым словам) | Бесплатно, реальные фото | Не соответствуют точно блюду; правовая чистота требует проверки |
| LLM описание + emoji | Лёгкий «визуал» | (выбрано как fallback к отсутствию картинок) |

**MVP реализация:** карточка рецепта = заголовок + emoji-индикатор категории
белка + краткое описание + время приготовления. Никаких `<img>` тегов.

**Consequences:**
- Не нужен image storage / CDN
- Не нужен image generation pipeline
- Карточки рендерятся быстро, оффлайн-кэш минимален

**When to revisit:** после MVP, если визуально приложение «голое».
Кандидат — стоковые с Unsplash + LLM для подбора ключевых слов.

---

## 4. Дополнительные решения

### 4.1 i18n: JSON-словари + `t()` в `html/template`

- Три словаря: `i18n/ru.json`, `i18n/fi.json`, `i18n/en.json`
- Custom func `t(key, args...)` зарегистрирована в `template.FuncMap`
- Язык определяется при первом запросе: `Accept-Language` header или session cookie
- Переключатель языка в настройках — обновляет cookie и редиректит
- Сгенерированные рецепты сохраняют язык создания (не перегенерируются при
  смене UI-языка) — это уже зафиксировано в PRD §F-9

### 4.2 Service Worker и offline strategy

**Что кэшируется:**
- Все статические ассеты (CSS, JS, шрифты, иконки)
- Текущий `WeeklyPlan` (3 рецепта) — обновляется после генерации
- Архив рецептов — last 50 viewed, LRU
- Shopping list текущей недели

**Что НЕ кэшируется:**
- Запросы на генерацию (LLM) — требуют сети
- Запросы на запись feedback — требуют сети, queue для последующего sync

**Strategy:**
- `cache-first` для статики
- `network-first, cache-fallback` для рецептов и shopping list
- `network-only` для генерации

### 4.3 Apple Reminders экспорт (Phase 2, не в MVP)

Не в MVP scope (PRD §4 Out of Scope этого не упоминал, но фича обсуждалась
автором как нужная в Phase 2).

**Подход:** x-callback-url через Shortcuts
- На iPad один раз создаётся Shortcut «Import Shopping List»
- Приложение генерирует ссылку `shortcuts://run-shortcut?name=...&text=...`
- Items появляются в Reminders

**Альтернатива (отброшена для MVP+1):** CalDAV напрямую — требует
app-specific password от iCloud, сложнее, безопаснее не делать в семейном app.

### 4.4 Структура Go-проекта

```
cooking-helper/
├── cmd/
│   └── server/main.go              # entry point
├── internal/
│   ├── domain/                     # модели: Recipe, WeeklyPlan, HouseholdProfile
│   ├── handler/                    # HTTP-обработчики (по фичам, не по слоям)
│   ├── service/                    # бизнес-логика
│   ├── repository/                 # SQL-доступ
│   ├── llm/                        # LLM-абстракция + Anthropic-реализация
│   ├── i18n/                       # переводы и t() func
│   └── shopping/                   # consolidation, categorization
├── migrations/                     # golang-migrate файлы
├── templates/                      # html/template файлы (.gohtml)
├── static/                         # CSS, JS (HTMX), Service Worker, шрифты
├── i18n/                           # ru.json, fi.json, en.json
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── README.md
```

DDD-подход: пакеты группируются по доменной области, не по слою.

### 4.5 AI-assisted development tooling

Помимо runtime LLM-вызовов (раздел 3.4), на этапе разработки используем
Anthropic Skills для ускорения работы:

| Skill | Когда применяется | Что генерирует |
|---|---|---|
| [`frontend-design`](https://github.com/anthropics/skills/blob/main/skills/frontend-design/SKILL.md) | CH-6, CH-11, CH-13, CH-19 — любая UI-задача | HTML/CSS markup в Nordic Kitchen tone, интегрируется в `html/template` |

**Использование `frontend-design`:**
- **Output format строго: HTML/CSS only** (не JSX, не React, не Vue)
- Сгенерированный markup переносится в `templates/*.gohtml` и обвешивается
  HTMX-атрибутами для интерактивности
- Это инструмент разработки, не runtime-зависимость — никаких npm/node
  в проде

**Design system: Nordic Kitchen**

- **Mood:** тёплый домашний, скандинавская функциональная эстетика, между
  Aalto и финским mökki. Утилитарный, но не холодный.
- **Audience:** семейный повар, iPad на кухне с мокрыми руками, чтение с
  расстояния 50см
- **Differentiator:** large readable typography (≥18pt body, ≥24pt headings),
  generous spacing, минимум визуального шума

**Палитра (стартовая, уточняется в Phase 1):**
- Background: warm cream `#F5EFE6`
- Surface: ivory `#FCFAF5`
- Text primary: deep oak `#2B2118`
- Text secondary: muted brown `#6B5F52`
- Accent (action / links / liked): terracotta `#C2603A`
- Success (cooked / liked): moss green `#5C7A4E`
- Border / dividers: soft sand `#E5DBC9`

**Типография:**
- Headings: **Fraunces** (variable, warm serif, открытая графика)
- Body: **Public Sans** (sans, accessible, поддержка финских/русских символов)
- Numbers / measurements: **Public Sans tabular figures**
- Все шрифты Google Fonts / open-source, self-hosted в `static/fonts/`

**Принципы layout:**
- iPad portrait: одна колонка, generous padding 24-32px
- iPad landscape: две колонки только в recipe detail (ingredients | steps)
- Touch-targets: минимум 44×44pt (Apple HIG)
- Никаких hover-only взаимодействий
- Dark mode: respect `prefers-color-scheme`, инвертированная Nordic палитра
  (deep oak background + warm cream text)

**Конкретные правила запуска skill'а:**
При каждом вызове `frontend-design` для нашего проекта явно указывать:
1. Output: HTML + CSS, no React, no Vue, no JSX
2. Design system: Nordic Kitchen (см. палитру и типографику выше)
3. Target: iPad Safari, kitchen-context, 50cm reading distance
4. Constraint: ≥18pt body, ≥24pt headings, 44pt touch targets

---

## 5. Что меняется в PRD v3

Этот документ требует обновлений в PRD:

1. **§6 «Core Architecture & Patterns»:** «Local-first данные» → «Home-network-first»
   с явным trade-off (iPad работает только в домашней сети)
2. **§8 «Technology Stack»:** убрать «DEFERRED», добавить ссылку на этот документ
3. **§9 «Security & Configuration»:** LLM API key — «на сервере в env»
   (раньше было «на стороне сервиса (если есть backend)»)
4. **§15 Appendix «Высокоуровневая модель данных»:** TS-синтаксис заменить
   на язык-нейтральный (поля и типы остаются)
5. **§15 Appendix:** добавить ссылку на используемые Anthropic skills
   (`frontend-design` для UI-генерации)

---

## 6. Что меняется в stories

Stories CH-2, CH-3, CH-4, CH-6, CH-7 содержат TS/JS-предположения.
Требуется перегенерация через `/create-stories` после обновления PRD.
Подробности — отдельная задача (вне этого документа).

---

## 7. Operations

### Backup

```bash
# launchd plist на Mac mini, daily 03:00
/usr/bin/docker exec cooking-helper sqlite3 /data/cooking.db \
  ".backup /backups/$(date +%Y-%m-%d).db"
# Хранить последние 14 дней
find /backups -name "*.db" -mtime +14 -delete
```

### Healthcheck

`GET /healthz` — проверяет DB-подключение, возвращает 200/503.
Tailscale Serve healthcheck к этому endpoint, чтобы видеть деградацию.

### Observability

- `log/slog` (структурированный JSON в stdout)
- Каждый запрос имеет `request_id` (UUID), пробрасывается в LLM-вызовы
- Не логируется содержимое промптов в production-режиме (privacy);
  логируется только token count и latency

### Style checks

- `go vet ./...`
- `golangci-lint run ./...`
- `gofmt -s -d .` в pre-commit hook

---

## 8. When this document needs revision

Триггеры для пересмотра отдельных решений указаны в каждой секции под
заголовком «When to revisit». Документ в целом пересматривается:
- После Phase 1 (CH-2…CH-6) — если базовые предположения не сработали
- Перед Phase 2 — подтвердить промпт-структуру для LLM
- При смене use case (multi-user, доступ извне)

---

## 9. Сводка финального стека

| Слой | Решение |
|---|---|
| Архитектура | Home-server-first |
| Backend | Go + `html/template` + HTMX |
| Database | SQLite в Docker volume |
| Hosting | Docker на Mac mini Intel i7 |
| Network | Tailscale Serve (только tailnet, HTTPS автоматом) |
| LLM | Anthropic Go SDK, Sonnet 4.6 + Haiku 4.5, через `internal/llm` |
| i18n | RU/FI/EN, JSON-словари + `t()` в шаблонах |
| Клиент | PWA на iPad с Service Worker для offline-кэша рецептов |
| Экспорт shopping list | Apple Reminders через Shortcuts x-callback-url (Phase 2) |
| UI generation (dev-time) | Anthropic `frontend-design` skill, HTML/CSS output, Nordic Kitchen design system |

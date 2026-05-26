# User Stories — Cooking Helper MVP

**Источник:** `.agents/PRDs/PRD.md` (Draft v2)
**Сгенерировано:** 2026-05-26
**Репозиторий:** AntonKilk/cooking-helper
**Всего историй:** 21 (Phase 1: 6, Phase 2: 5, Phase 3: 7, Phase 4: 3)

---

## Phase 1 — Foundation

### CH-1 Tech design document

**Type:** Spike
**Priority:** High
**Complexity:** Medium
**Phase:** 1
**Labels:** `spike`, `architecture`

#### Description
Как разработчик, я хочу зафиксировать tech design (платформа, frontend-фреймворк, локальное хранилище, LLM-провайдер, стратегия изображений), чтобы остальные истории строились на конкретном стеке, а не на абстракциях.

#### Acceptance Criteria
- [ ] Документ `.agents/tech-design.md` существует и отвечает на все 6 открытых вопросов из PRD §8
- [ ] Решения уважают constraints из PRD §8 (iPad Safari primary, offline-first, миграция на cloud)
- [ ] Документ содержит обоснование выбора (не только итог)
- [ ] Документ подписан/одобрен автором PRD

#### Technical Notes
- См. PRD §8 «Открытые вопросы для tech design»
- Кандидаты: PWA (React/Vue/Svelte) vs Native iOS vs React Native
- Локальное хранилище: IndexedDB vs SQLite (через wa-sqlite или Capacitor)
- LLM: Claude напрямую vs провайдер-агностичный слой

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
Как разработчик, я хочу базовый скелет проекта с выбранным стеком и работающим dev-сервером, чтобы можно было запустить пустое приложение и начать наращивать функциональность.

#### Acceptance Criteria
- [ ] `npm run dev` (или эквивалент) запускает приложение локально
- [ ] Приложение открывается на iPad Safari через dev-сервер по сети
- [ ] Настроен линтер + форматтер, базовый CI прогоняется без ошибок
- [ ] README с инструкцией запуска

#### Technical Notes
- Конкретный стек — из CH-1
- TypeScript strict mode
- Test runner настроен (vitest/jest)

#### Dependencies
- Blocked by: CH-1
- Blocks: CH-3, CH-4, CH-6

---

### CH-3 Data models & local storage layer

**Type:** Technical
**Priority:** High
**Complexity:** Medium
**Phase:** 1
**Labels:** `technical`, `storage`

#### Description
Как разработчик, я хочу слой персистентности с типизированными моделями `HouseholdProfile`, `Recipe`, `WeeklyPlan`, `ShoppingListItem`, чтобы остальные фичи работали с данными через единый репозитори-интерфейс.

#### Acceptance Criteria
- [ ] Схемы из PRD §15 Appendix реализованы как TS-типы
- [ ] Repository-слой умеет CRUD по каждой модели
- [ ] `household_id` зашит в каждой сущности (под будущий мультиюзер)
- [ ] Storage layer абстрагирован (легко заменить IndexedDB на cloud)
- [ ] Unit-тесты на CRUD каждой модели

#### Technical Notes
- См. PRD §15 Appendix «Высокоуровневая модель данных»
- Не забывать `created_at` / `updated_at`
- Версионирование схемы (миграции) — заложить хук, реализация позже

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
- [ ] При первом запуске язык определяется из системных настроек (`navigator.language`)
- [ ] Переключатель языка доступен в настройках, изменение применяется без перезагрузки
- [ ] Все строки UI берутся из переводов, нет хардкода
- [ ] Подключены словари `ru.json` / `fi.json` / `en.json`
- [ ] Тестовый компонент рендерится на каждом языке корректно

#### Technical Notes
- Конкретная библиотека — из CH-1 (i18next / vue-i18n / svelte-i18n / native solution)
- Категории магазина из PRD §15 Appendix — первая локализованная сущность
- Не локализовать сгенерированные рецепты (их язык фиксируется при создании)

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
- [ ] Изменения сохраняются локально и применяются к следующей генерации
- [ ] При первом запуске профиль создаётся с дефолтами (2 взрослых, 0 детей, язык из системы)

#### Technical Notes
- Использует репозиторий из CH-3
- Один профиль на устройство в MVP

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
Как пользователь, я хочу базовую навигацию между главным экраном, рецептом и настройками, чтобы пользоваться приложением как обычным мобильным/планшетным приложением.

#### Acceptance Criteria
- [ ] Routing настроен (главный экран `/`, рецепт `/recipe/:id`, настройки `/settings`)
- [ ] Layout адаптивен (iPad portrait/landscape, телефон portrait)
- [ ] Базовая шапка с заголовком и кнопкой настроек присутствует на каждом экране
- [ ] Деплой на iPad Safari без визуальных артефактов

#### Technical Notes
- iPad-first вёрстка из PRD §4 «iPad UX»
- Можно использовать заглушечные экраны в этой истории

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
- [ ] Сервис принимает `prompt + schema`, возвращает типизированный объект или ошибку
- [ ] Retry с экспоненциальной задержкой при сетевых ошибках (макс 3 попытки)
- [ ] Невалидный JSON → авто-повтор с уточняющим хинтом (макс 1 раз), затем ошибка
- [ ] Версионирование промптов: каждый промпт — отдельный файл с `version` в коде
- [ ] Логирование расхода токенов (для мониторинга бюджета)
- [ ] Unit-тесты: успех, retry, fallback на ошибке

#### Technical Notes
- См. PRD §6 «Provider-agnostic LLM-слой»
- По умолчанию Claude (модель — из CH-1)
- Промпты живут в репо, не в env

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
- [ ] Кнопка «Сгенерировать неделю» на главном экране запускает генерацию
- [ ] Промпт получает household profile, disliked, pantry, последние feedback-записи, историю недели
- [ ] Ответ парсится как 3 объекта `Recipe` со структурой из PRD §15
- [ ] Порции суммарно покрывают `7 дней × family_size`
- [ ] Среди 3 рецептов минимум 2 категории белка (разнообразие)
- [ ] Время от тапа до отрисовки карточек ≤ 30 секунд
- [ ] На главном экране отрисовываются 3 карточки с превью каждого рецепта

#### Technical Notes
- Использует LLM-сервис из CH-7
- Карточка: название, время приготовления, краткое описание
- Промпт — отдельный артефакт с версией
- Сохранять `WeeklyPlan` и `Recipe[]` локально через CH-3 репозиторий

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
- Отдельный промпт `swap_recipe.v1`
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
- [ ] Если после 2 повторов нарушение остаётся — показывается ошибка пользователю, но не молчаливое игнорирование
- [ ] Сравнение учитывает падежи и варианты написания (нормализация / LLM-based matching)
- [ ] Метрика «частота нарушений» логируется для мониторинга качества промпта

#### Technical Notes
- См. PRD §14 Risks — «Disliked ingredients игнорируются LLM»
- Нормализация имён ингредиентов — отдельная утилита

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
- [ ] Body text минимум 18pt, заголовки минимум 24pt
- [ ] Ингредиенты и шаги в двух колонках на iPad landscape, одной — на portrait
- [ ] Активный шаг визуально выделен, есть возможность отметить шаг выполненным
- [ ] Тёмный режим уважается (CSS prefers-color-scheme)
- [ ] Прокрутка плавная одним пальцем

#### Technical Notes
- Тестировать на iPad с расстояния 50см
- Никаких hover-only взаимодействий

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
- [ ] Несовместимые единицы (например, 1 шт + 100г моркови) показываются раздельно
- [ ] Каждый ингредиент имеет `category` (produce / meat_fish / dairy / pantry / frozen / other)
- [ ] Категоризация работает на 95%+ ингредиентов в тестовом наборе (5 разных недель)
- [ ] Ингредиенты из `pantry_basics` исключаются из списка (US-7)

#### Technical Notes
- Категоризация — отдельный промпт `categorize_ingredient.v1` или маппинг по словарю + LLM fallback
- Нормализация единиц: g, kg, ml, l, шт, ст.л., ч.л., dl, tl, rkl и финские эквиваленты
- Кэшировать категорию по имени ингредиента, чтобы не вызывать LLM повторно

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
- [ ] Чекбокс рядом с каждым пунктом, тап = отмечено
- [ ] Отмеченные пункты опционально скрываются («показать купленное» переключатель)
- [ ] Swipe или кнопка «удалить» убирает пункт из списка (с возможностью отменить)
- [ ] Состояние сохраняется локально

#### Technical Notes
- Категории и переводы — из PRD §15 Appendix таблицы
- Использовать iPad-оптимизированную вёрстку из CH-11

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
- [ ] Список синхронизирован между генерациями

#### Technical Notes
- См. PRD §15 Appendix «Дефолтный список pantry_basics»
- Хранится в `HouseholdProfile.pantry_basics`

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
- Один тап = одно состояние, без подтверждений

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
- [ ] Дизлайкнутые рецепты не появляются повторно (или появляются с очень низкой вероятностью)
- [ ] Лайкнутые рецепты влияют на стиль через текст промпта (не buy a fixed list)
- [ ] Параметр N конфигурируется (но не через UI)
- [ ] Объём контекста промпта не превышает разумного лимита токенов

#### Technical Notes
- Использует CH-7 LLM-сервис и CH-8 промпт
- Сериализация feedback в текст — отдельная утилита

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
- [ ] Поиск по подстроке в названии (debounce 200мс)
- [ ] Кнопка «Приготовить снова» добавляет рецепт в текущий `WeeklyPlan` (заменяет один из 3 — диалог выбора какой)
- [ ] Иконки feedback видны в списке

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
- [ ] Onboarding не показывается повторно (флаг в локальном хранилище)

#### Technical Notes
- Использует CH-5, CH-14

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
- [ ] Тёмный режим корректен
- [ ] Никаких scroll-jacking или нестандартных жестов

#### Technical Notes
- Прогоняется по checklist на реальном устройстве
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
- [ ] Чеклист бета-тестирования составлен (10+ сценариев из user stories)
- [ ] 2 недели реального использования семьёй автора зафиксированы
- [ ] Метрика «время до shopping list» < 2 минут подтверждена замерами
- [ ] Все P0/P1 баги по итогам — закрыты
- [ ] Финальный отчёт сохранён в `.agents/reports/beta-1.md`

#### Technical Notes
- См. PRD §11 «Success Criteria» и §12 Phase 4 «Validation»

#### Dependencies
- Blocked by: вся Phase 1-3, CH-20

---

## Сводная таблица

| ID    | Title                                                | Type        | Phase | Complexity |
|-------|------------------------------------------------------|-------------|-------|------------|
| CH-1  | Tech design document                                 | Spike       | 1     | Medium     |
| CH-2  | Project skeleton & dev environment                   | Technical   | 1     | Small      |
| CH-3  | Data models & local storage layer                    | Technical   | 1     | Medium     |
| CH-4  | i18n framework with RU/FI/EN                         | Feature     | 1     | Small      |
| CH-5  | Household profile screen                             | Feature     | 1     | Small      |
| CH-6  | App navigation & layout shell                        | Technical   | 1     | Small      |
| CH-7  | LLM service abstraction with retry & JSON parsing    | Technical   | 2     | Medium     |
| CH-8  | Weekly menu generation (F-1)                         | Feature     | 2     | Large      |
| CH-9  | Recipe swap & full regenerate (F-2)                  | Feature     | 2     | Medium     |
| CH-10 | Disliked-ingredients post-validation                 | Feature     | 2     | Small      |
| CH-11 | Fullscreen recipe view (F-4)                         | Feature     | 2     | Small      |
| CH-12 | Shopping list builder with consolidation (F-3)       | Feature     | 3     | Large      |
| CH-13 | Shopping list UI                                     | Feature     | 3     | Medium     |
| CH-14 | Pantry basics management (F-6)                       | Feature     | 3     | Small      |
| CH-15 | Disliked ingredients management (F-7)                | Feature     | 3     | Small      |
| CH-16 | Recipe feedback collection (F-5)                     | Feature     | 3     | Small      |
| CH-17 | Feedback integration in next-generation prompt       | Feature     | 3     | Small      |
| CH-18 | Recipe archive with search & "cook again" (F-8)      | Feature     | 3     | Medium     |
| CH-19 | Onboarding flow                                      | Feature     | 4     | Small      |
| CH-20 | iPad UX polish & accessibility                       | Enhancement | 4     | Medium     |
| CH-21 | Beta testing & bug bash                              | Technical   | 4     | Small      |

**Покрытие PRD:** US-1…US-9 → CH-8, CH-9, CH-13, CH-15, CH-16, CH-7+CH-14, CH-11, CH-4. F-1…F-9 → CH-8, CH-9, CH-12, CH-11, CH-16, CH-14, CH-15, CH-18, CH-4. Все 4 фазы PRD §12 покрыты.

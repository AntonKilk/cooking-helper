CREATE TABLE household_profile (
    id                   TEXT PRIMARY KEY,
    language             TEXT      NOT NULL CHECK (language IN ('ru', 'fi', 'en')),
    family_adults        INTEGER   NOT NULL DEFAULT 0,
    family_kids          INTEGER   NOT NULL DEFAULT 0,
    disliked_ingredients TEXT      NOT NULL DEFAULT '[]',
    pantry_basics        TEXT      NOT NULL DEFAULT '[]',
    created_at           TIMESTAMP NOT NULL,
    updated_at           TIMESTAMP NOT NULL
);

CREATE TABLE recipe (
    id                  TEXT PRIMARY KEY,
    household_id        TEXT      NOT NULL REFERENCES household_profile(id) ON DELETE CASCADE,
    language            TEXT      NOT NULL CHECK (language IN ('ru', 'fi', 'en')),
    title               TEXT      NOT NULL,
    description         TEXT      NOT NULL DEFAULT '',
    cook_time_minutes   INTEGER   NOT NULL DEFAULT 0,
    servings            INTEGER   NOT NULL DEFAULT 0,
    ingredients         TEXT      NOT NULL DEFAULT '[]',
    steps               TEXT      NOT NULL DEFAULT '[]',
    source              TEXT      NOT NULL CHECK (source IN ('llm', 'history')),
    feedback_liked      INTEGER,
    feedback_disliked   INTEGER,
    feedback_cook_again INTEGER,
    feedback_created_at TIMESTAMP,
    created_at          TIMESTAMP NOT NULL,
    updated_at          TIMESTAMP NOT NULL
);

CREATE INDEX idx_recipe_household ON recipe(household_id);

CREATE TABLE weekly_plan (
    id           TEXT PRIMARY KEY,
    household_id TEXT      NOT NULL REFERENCES household_profile(id) ON DELETE CASCADE,
    week_start   TEXT      NOT NULL,
    recipe_ids   TEXT      NOT NULL DEFAULT '[]',
    created_at   TIMESTAMP NOT NULL
);

CREATE INDEX idx_weekly_plan_household ON weekly_plan(household_id);

CREATE TABLE shopping_list_item (
    id               TEXT PRIMARY KEY,
    weekly_plan_id   TEXT      NOT NULL REFERENCES weekly_plan(id) ON DELETE CASCADE,
    household_id     TEXT      NOT NULL,
    name             TEXT      NOT NULL,
    amount           REAL      NOT NULL DEFAULT 0,
    unit             TEXT      NOT NULL DEFAULT '',
    category         TEXT      NOT NULL DEFAULT 'other',
    checked          INTEGER   NOT NULL DEFAULT 0,
    manually_removed INTEGER   NOT NULL DEFAULT 0,
    created_at       TIMESTAMP NOT NULL
);

CREATE INDEX idx_shopping_item_plan ON shopping_list_item(weekly_plan_id);

-- Global cache of ingredient name -> store category, so the LLM categorizer
-- (CH-12) is never asked twice for the same ingredient. The key is the
-- normalized ingredient name only: a carrot is produce for every household, so
-- this derived dictionary cache is intentionally household-agnostic (unlike the
-- household data tables, which carry household_id for future multi-user).
CREATE TABLE ingredient_category (
    name_normalized TEXT PRIMARY KEY,
    category        TEXT      NOT NULL CHECK (category IN
                        ('produce', 'meat_fish', 'dairy', 'pantry', 'frozen', 'other')),
    created_at      TIMESTAMP NOT NULL
);

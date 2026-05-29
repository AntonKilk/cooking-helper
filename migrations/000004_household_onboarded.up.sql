-- Gate the first-run onboarding wizard (CH-19): once the household has seen and
-- completed (or skipped) the intro, onboarded flips to 1 and the wizard never
-- shows again. Existing rows default to 0 ("not onboarded"), so a pre-existing
-- single household sees the intro once — acceptable and on-spec.
ALTER TABLE household_profile ADD COLUMN onboarded INTEGER NOT NULL DEFAULT 0;

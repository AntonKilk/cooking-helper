-- Reverse 000004_household_onboarded.up.sql. Requires SQLite >= 3.35 for
-- ALTER TABLE ... DROP COLUMN; the project pins modernc.org/sqlite v1.50+,
-- which bundles a recent SQLite.
ALTER TABLE household_profile DROP COLUMN onboarded;

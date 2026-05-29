-- Reverse 000002_weekly_plan_archive.up.sql. Requires SQLite >= 3.35 for
-- ALTER TABLE ... DROP COLUMN; the project pins modernc.org/sqlite v1.50+,
-- which bundles a recent SQLite.
DROP INDEX IF EXISTS idx_weekly_plan_active;
ALTER TABLE weekly_plan DROP COLUMN archived_at;

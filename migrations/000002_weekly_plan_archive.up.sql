-- Add a nullable archived_at timestamp on weekly_plan so a regenerated plan
-- can supersede the previous one without deleting it (PRD §F-2: previous
-- WeeklyPlan goes to archive, not the trash).
ALTER TABLE weekly_plan ADD COLUMN archived_at TIMESTAMP NULL;

-- Partial index over the single "currently active" plan per household, so
-- CurrentWeeklyPlan(household_id) is O(1). Not UNIQUE: archive-then-create
-- runs in one transaction, but a future race must downgrade to "newest wins"
-- rather than erroring.
CREATE INDEX idx_weekly_plan_active
    ON weekly_plan(household_id)
    WHERE archived_at IS NULL;

ALTER TABLE job_outputs DROP COLUMN IF EXISTS attempt_id;
ALTER TABLE jobs
	DROP CONSTRAINT IF EXISTS jobs_current_attempt_id_fkey,
	DROP CONSTRAINT IF EXISTS jobs_completed_attempt_id_fkey;
DROP TABLE IF EXISTS job_attempts;
ALTER TABLE jobs
	DROP COLUMN IF EXISTS queue_entered_at,
	DROP COLUMN IF EXISTS current_attempt_id,
	DROP COLUMN IF EXISTS completed_attempt_id,
	DROP COLUMN IF EXISTS completed_at;
DROP TYPE IF EXISTS attempt_status;

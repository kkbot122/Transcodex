DO $$
BEGIN
	IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'attempt_status') THEN
		CREATE TYPE attempt_status AS ENUM ('running', 'completed', 'failed', 'expired');
	END IF;
END $$;

ALTER TABLE jobs
	ADD COLUMN IF NOT EXISTS queue_entered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	ADD COLUMN IF NOT EXISTS current_attempt_id UUID,
	ADD COLUMN IF NOT EXISTS completed_attempt_id UUID,
	ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS job_attempts (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
	attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
	worker_id UUID REFERENCES workers(id) ON DELETE SET NULL,
	status attempt_status NOT NULL DEFAULT 'running',
	queue_entered_at TIMESTAMPTZ NOT NULL,
	started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT now(),
	lease_expires_at TIMESTAMPTZ NOT NULL,
	finished_at TIMESTAMPTZ,
	current_phase TEXT NOT NULL DEFAULT 'claiming',
	processing_mode TEXT NOT NULL DEFAULT 'parallel',
	failure_category TEXT,
	failure_detail TEXT,
	download_duration_ms BIGINT,
	processing_duration_ms BIGINT,
	upload_duration_ms BIGINT,
	UNIQUE (job_id, attempt_number)
);

ALTER TABLE jobs
	DROP CONSTRAINT IF EXISTS jobs_current_attempt_id_fkey,
	DROP CONSTRAINT IF EXISTS jobs_completed_attempt_id_fkey;
ALTER TABLE jobs
	ADD CONSTRAINT jobs_current_attempt_id_fkey FOREIGN KEY (current_attempt_id) REFERENCES job_attempts(id),
	ADD CONSTRAINT jobs_completed_attempt_id_fkey FOREIGN KEY (completed_attempt_id) REFERENCES job_attempts(id);

ALTER TABLE job_outputs
	ADD COLUMN IF NOT EXISTS attempt_id UUID REFERENCES job_attempts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_job_attempts_expired ON job_attempts(status, lease_expires_at);
CREATE INDEX IF NOT EXISTS idx_job_attempts_job_started ON job_attempts(job_id, started_at);
CREATE INDEX IF NOT EXISTS idx_jobs_queue_entered ON jobs(status, queue_entered_at);

UPDATE jobs
SET completed_at = updated_at
WHERE status = 'completed' AND completed_at IS NULL;

UPDATE jobs
SET queue_entered_at = created_at
WHERE status = 'queued' AND queue_entered_at > created_at;

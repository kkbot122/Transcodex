CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
	IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'job_status') THEN
		CREATE TYPE job_status AS ENUM ('queued', 'processing', 'completed', 'dead');
	END IF;

	IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'job_output_type') THEN
		CREATE TYPE job_output_type AS ENUM ('video_360p', 'video_720p', 'video_1080p', 'thumbnail');
	END IF;

	IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'worker_status') THEN
		CREATE TYPE worker_status AS ENUM ('idle', 'busy', 'dead');
	END IF;
END $$;

CREATE TABLE IF NOT EXISTS jobs (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	status job_status NOT NULL DEFAULT 'queued',
	retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
	max_retries INTEGER NOT NULL DEFAULT 3 CHECK (max_retries >= 0),
	priority INTEGER NOT NULL DEFAULT 0,
	input_file TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS job_outputs (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
	type job_output_type NOT NULL,
	cdn_url TEXT NOT NULL,
	file_size BIGINT NOT NULL CHECK (file_size >= 0),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (job_id, type)
);

CREATE TABLE IF NOT EXISTS workers (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	status worker_status NOT NULL DEFAULT 'idle',
	current_job UUID REFERENCES jobs(id) ON DELETE SET NULL,
	last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_updated_at ON jobs(updated_at);
CREATE INDEX IF NOT EXISTS idx_workers_status_last_heartbeat ON workers(status, last_heartbeat);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
	NEW.updated_at = now();
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_jobs_updated_at ON jobs;
CREATE TRIGGER trg_jobs_updated_at
BEFORE UPDATE ON jobs
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

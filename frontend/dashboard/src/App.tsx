import { useEffect, useMemo, useState } from "react";
import "./styles.css";

type JobStatus = "queued" | "processing" | "completed" | "dead";
type WorkerStatus = "idle" | "busy" | "dead";
type ConnectionStatus = "connecting" | "connected" | "reconnecting";

type Stats = {
  queue_depth: number;
  throughput_per_min: number;
  workers: Record<string, number>;
  jobs: Record<JobStatus, number>;
  latency: {
    queue_wait_p95_ms?: number;
    processing_p95_ms?: number;
    total_p95_ms?: number;
  };
};

type Worker = {
  id: string;
  status: WorkerStatus;
  current_job: string | null;
  last_heartbeat: string;
};

type Job = {
  job_id: string;
  status: JobStatus;
  retry_count: number;
  priority: number;
  input_file?: string;
  created_at: string;
  updated_at: string;
};

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "/api";
const statusOrder: JobStatus[] = ["queued", "processing", "completed", "dead"];
const statusLabels: Record<JobStatus, string> = {
  queued: "Queued",
  processing: "Processing",
  completed: "Completed",
  dead: "Dead",
};

const emptyStats: Stats = {
  queue_depth: 0,
  throughput_per_min: 0,
  workers: { total: 0, idle: 0, busy: 0, dead: 0 },
  jobs: { queued: 0, processing: 0, completed: 0, dead: 0 },
  latency: {},
};

export default function App() {
  const statsStream = useSSE<Stats>(`${API_BASE}/internal/stats/stream`, emptyStats);
  const [workers, setWorkers] = useState<Worker[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [jobFilter, setJobFilter] = useState<JobStatus | "all">("all");
  const [workersError, setWorkersError] = useState("");
  const [jobsError, setJobsError] = useState("");

  useEffect(() => {
    let cancelled = false;

    async function loadWorkers() {
      try {
        const payload = await fetchJSON<{ workers: Worker[] }>(`${API_BASE}/internal/workers`);
        if (!cancelled) {
          setWorkers(payload.workers);
          setWorkersError("");
        }
      } catch (err) {
        if (!cancelled) {
          setWorkersError(errorMessage(err));
        }
      }
    }

    loadWorkers();
    const interval = window.setInterval(loadWorkers, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    const params = new URLSearchParams({ limit: "50" });
    if (jobFilter !== "all") {
      params.set("status", jobFilter);
    }

    async function loadJobs() {
      try {
        const payload = await fetchJSON<{ jobs: Job[] }>(`${API_BASE}/internal/jobs?${params}`, controller.signal);
        setJobs(payload.jobs);
        setJobsError("");
      } catch (err) {
        if (!controller.signal.aborted) {
          setJobsError(errorMessage(err));
        }
      }
    }

    loadJobs();
    const interval = window.setInterval(loadJobs, 5000);
    return () => {
      controller.abort();
      window.clearInterval(interval);
    };
  }, [jobFilter]);

  return (
    <main className="dashboard-shell">
      <header className="dashboard-header">
        <div>
          <p className="eyebrow">Transcodex Dashboard</p>
          <h1>Pipeline Health</h1>
        </div>
        <ConnectionPill status={statsStream.status} />
      </header>

      <QueueStats stats={statsStream.data} />

      <section className="dashboard-grid">
        <JobCounts stats={statsStream.data} />
        <WorkerGrid workers={workers} error={workersError} />
      </section>

      <JobTable jobs={jobs} filter={jobFilter} error={jobsError} onFilterChange={setJobFilter} />
    </main>
  );
}

function useSSE<T>(url: string, initialData: T) {
  const [data, setData] = useState<T>(initialData);
  const [status, setStatus] = useState<ConnectionStatus>("connecting");

  useEffect(() => {
    const source = new EventSource(url);
    setStatus("connecting");

    source.onopen = () => setStatus("connected");
    source.onmessage = (event) => {
      setData(JSON.parse(event.data) as T);
      setStatus("connected");
    };
    source.onerror = () => {
      setStatus("reconnecting");
    };

    return () => source.close();
  }, [url]);

  return { data, status };
}

function QueueStats({ stats }: { stats: Stats }) {
  const workerTotal = stats.workers.total ?? 0;
  const activeJobs = (stats.jobs.queued ?? 0) + (stats.jobs.processing ?? 0);

  return (
    <section className="stats-grid" aria-label="Queue statistics">
      <Metric label="Queue Depth" value={stats.queue_depth} />
      <Metric label="Throughput/min" value={stats.throughput_per_min} />
      <Metric label="Workers" value={workerTotal} />
      <Metric label="Active Jobs" value={activeJobs} />
      <Metric label="Queue p95" value={formatMilliseconds(stats.latency.queue_wait_p95_ms)} />
      <Metric label="Total p95" value={formatMilliseconds(stats.latency.total_p95_ms)} />
    </section>
  );
}

function Metric({ label, value }: { label: string; value: number | string }) {
  return (
    <article className="metric">
      <span>{label}</span>
      <strong>{value}</strong>
    </article>
  );
}

function formatMilliseconds(value?: number) {
  return value == null ? "-" : `${value} ms`;
}

function JobCounts({ stats }: { stats: Stats }) {
  const total = statusOrder.reduce((sum, status) => sum + (stats.jobs[status] ?? 0), 0);

  return (
    <section className="panel" aria-labelledby="job-counts-title">
      <div className="section-header">
        <h2 id="job-counts-title">Job States</h2>
        <span>{total} total</span>
      </div>
      <div className="status-bars">
        {statusOrder.map((status) => {
          const count = stats.jobs[status] ?? 0;
          const percent = total === 0 ? 0 : Math.round((count / total) * 100);
          return (
            <div className={`status-row ${status}`} key={status}>
              <div className="status-row-label">
                <span>{statusLabels[status]}</span>
                <strong>{count}</strong>
              </div>
              <div className="bar-track" aria-hidden="true">
                <span style={{ width: `${percent}%` }} />
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function WorkerGrid({ workers, error }: { workers: Worker[]; error: string }) {
  return (
    <section className="panel" aria-labelledby="workers-title">
      <div className="section-header">
        <h2 id="workers-title">Workers</h2>
        <span>{workers.length} registered</span>
      </div>

      {error && <div className="inline-alert">{error}</div>}
      {workers.length === 0 && !error ? (
        <div className="empty-state">No workers registered</div>
      ) : (
        <div className="worker-grid">
          {workers.map((worker) => (
            <article className={`worker-card ${worker.status}`} key={worker.id}>
              <div className="worker-card-top">
                <span className={`status-dot ${worker.status}`} />
                <strong>{worker.status}</strong>
              </div>
              <p className="mono">{shortID(worker.id)}</p>
              <dl>
                <div>
                  <dt>Job</dt>
                  <dd className="mono">{worker.current_job ? shortID(worker.current_job) : "-"}</dd>
                </div>
                <div>
                  <dt>Heartbeat</dt>
                  <dd>{formatRelative(worker.last_heartbeat)}</dd>
                </div>
              </dl>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}

function JobTable({
  jobs,
  filter,
  error,
  onFilterChange,
}: {
  jobs: Job[];
  filter: JobStatus | "all";
  error: string;
  onFilterChange: (status: JobStatus | "all") => void;
}) {
  const options = useMemo(() => ["all", ...statusOrder] as const, []);

  return (
    <section className="panel jobs-panel" aria-labelledby="jobs-title">
      <div className="section-header table-header">
        <div>
          <h2 id="jobs-title">Recent Jobs</h2>
          <span>{jobs.length} shown</span>
        </div>
        <div className="segmented-control" aria-label="Filter jobs by status">
          {options.map((status) => (
            <button
              className={filter === status ? "active" : ""}
              type="button"
              onClick={() => onFilterChange(status)}
              key={status}
            >
              {status === "all" ? "All" : statusLabels[status]}
            </button>
          ))}
        </div>
      </div>

      {error && <div className="inline-alert">{error}</div>}
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Job</th>
              <th>Status</th>
              <th>Retries</th>
              <th>Priority</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {jobs.length === 0 ? (
              <tr>
                <td className="empty-cell" colSpan={5}>
                  No jobs found
                </td>
              </tr>
            ) : (
              jobs.map((job) => (
                <tr className={job.status === "dead" ? "dead-row" : ""} key={job.job_id}>
                  <td className="mono">{shortID(job.job_id)}</td>
                  <td>
                    <StatusBadge status={job.status} />
                  </td>
                  <td>{job.retry_count}</td>
                  <td>{job.priority}</td>
                  <td>{formatTime(job.updated_at)}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function ConnectionPill({ status }: { status: ConnectionStatus }) {
  return (
    <div className={`connection-pill ${status}`}>
      <span />
      {status === "connected" ? "Connected" : status === "reconnecting" ? "Reconnecting" : "Connecting"}
    </div>
  );
}

function StatusBadge({ status }: { status: JobStatus }) {
  return <span className={`job-badge ${status}`}>{statusLabels[status]}</span>;
}

async function fetchJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(url, { signal });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(typeof payload.error === "string" ? payload.error : "Request failed");
  }
  return payload as T;
}

function errorMessage(err: unknown) {
  return err instanceof Error ? err.message : "Something went wrong";
}

function shortID(id: string) {
  return id.length > 12 ? `${id.slice(0, 8)}...${id.slice(-4)}` : id;
}

function formatTime(value: string) {
  return new Date(value).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function formatRelative(value: string) {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000));
  if (seconds < 60) {
    return `${seconds}s ago`;
  }
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ago`;
}

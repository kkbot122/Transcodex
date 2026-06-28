import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import "./styles.css";

type JobStatus = "queued" | "processing" | "completed" | "dead";

type JobOutput = {
  type: "video_360p" | "video_720p" | "video_1080p" | "thumbnail";
  cdn_url: string;
  file_size: number;
};

type Job = {
  job_id: string;
  status: JobStatus;
  retry_count: number;
  priority: number;
  created_at: string;
  updated_at: string;
  outputs: JobOutput[];
};

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "/api";
const terminalStatuses = new Set<JobStatus>(["completed", "dead"]);

const statusLabels: Record<JobStatus, string> = {
  queued: "Queued",
  processing: "Processing",
  completed: "Completed",
  dead: "Dead",
};

const outputLabels: Record<JobOutput["type"], string> = {
  video_360p: "360p",
  video_720p: "720p",
  video_1080p: "1080p",
  thumbnail: "Thumbnail",
};

export default function App() {
  const [file, setFile] = useState<File | null>(null);
  const [priority, setPriority] = useState("0");
  const [job, setJob] = useState<Job | null>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  const [lastCheckedAt, setLastCheckedAt] = useState<Date | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const elapsedLabel = useElapsedTime(job?.created_at, job?.updated_at, job ? terminalStatuses.has(job.status) : false);

  useEffect(() => {
    if (!job || terminalStatuses.has(job.status)) {
      return;
    }

    const jobId = job.job_id;
    let ignore = false;
    const controller = new AbortController();

    async function fetchJob() {
      try {
        const nextJob = await getJob(jobId, controller.signal);
        if (!ignore) {
          setJob(nextJob);
          setLastCheckedAt(new Date());
          setError("");
        }
      } catch (err) {
        if (!ignore && !controller.signal.aborted) {
          setError(errorMessage(err));
        }
      }
    }

    fetchJob();
    const interval = window.setInterval(fetchJob, 3000);

    return () => {
      ignore = true;
      controller.abort();
      window.clearInterval(interval);
    };
  }, [job?.job_id, job?.status]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!file) {
      setError("Choose a video file first.");
      fileInputRef.current?.focus();
      return;
    }

    setUploading(true);
    setError("");
    setJob(null);
    setLastCheckedAt(null);

    try {
      const formData = new FormData();
      formData.append("file", file);
      formData.append("priority", priority);

      const response = await fetch(`${API_BASE}/uploads`, {
        method: "POST",
        body: formData,
      });
      const payload = await readJSON(response);
      if (!response.ok) {
        throw new Error(payload.error ?? "Upload failed.");
      }

      const nextJob = await getJob(payload.job_id);
      setJob(nextJob);
      setLastCheckedAt(new Date());
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setUploading(false);
    }
  }

  function resetForm() {
    setFile(null);
    setJob(null);
    setError("");
    setLastCheckedAt(null);
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  }

  const sortedOutputs = useMemo(() => {
    if (!job) {
      return [];
    }
    const order = ["thumbnail", "video_360p", "video_720p", "video_1080p"];
    return [...job.outputs].sort((a, b) => order.indexOf(a.type) - order.indexOf(b.type));
  }, [job]);

  return (
    <main className="app-shell">
      <section className="workspace" aria-labelledby="app-title">
        <header className="topbar">
          <div>
            <p className="eyebrow">Transcodex Demo</p>
            <h1 id="app-title">Video job console</h1>
          </div>
          <div className="api-pill">API {API_BASE}</div>
        </header>

        <div className="content-grid">
          <section className="panel upload-panel" aria-labelledby="upload-title">
            <div className="section-header">
              <h2 id="upload-title">Upload</h2>
              {uploading && <span className="pulse-label">Uploading</span>}
            </div>

            <form className="upload-form" onSubmit={handleSubmit}>
              <label className="file-drop">
                <span>Video file</span>
                <input
                  ref={fileInputRef}
                  type="file"
                  accept="video/*"
                  onChange={(event) => setFile(event.target.files?.[0] ?? null)}
                />
              </label>

              {file && (
                <div className="file-summary">
                  <span>{file.name}</span>
                  <strong>{formatBytes(file.size)}</strong>
                </div>
              )}

              <label>
                <span>Priority</span>
                <select value={priority} onChange={(event) => setPriority(event.target.value)}>
                  <option value="0">Normal</option>
                  <option value="5">High</option>
                  <option value="10">Urgent</option>
                </select>
              </label>

              <div className="button-row">
                <button type="submit" disabled={uploading}>
                  {uploading ? "Uploading..." : "Start job"}
                </button>
                <button className="secondary-button" type="button" onClick={resetForm}>
                  Reset
                </button>
              </div>
            </form>

            {error && <div className="alert">{error}</div>}
          </section>

          <section className="panel status-panel" aria-labelledby="status-title">
            <div className="section-header">
              <h2 id="status-title">Status</h2>
              {job && <StatusBadge status={job.status} />}
            </div>

            {!job ? (
              <EmptyState />
            ) : (
              <div className="job-stack">
                <dl className="job-meta">
                  <div>
                    <dt>Job</dt>
                    <dd className="mono">{job.job_id}</dd>
                  </div>
                  <div>
                    <dt>Elapsed</dt>
                    <dd>{elapsedLabel}</dd>
                  </div>
                  <div>
                    <dt>Retries</dt>
                    <dd>{job.retry_count}</dd>
                  </div>
                  <div>
                    <dt>Priority</dt>
                    <dd>{job.priority}</dd>
                  </div>
                  <div>
                    <dt>Updated</dt>
                    <dd>{formatTime(job.updated_at)}</dd>
                  </div>
                  <div>
                    <dt>Checked</dt>
                    <dd>{lastCheckedAt ? lastCheckedAt.toLocaleTimeString() : "-"}</dd>
                  </div>
                </dl>

                <ProgressRail status={job.status} />

                {job.status === "dead" && (
                  <div className="alert alert-danger">Processing failed after retry attempts.</div>
                )}

                {job.status === "completed" && (
                  <div className="outputs">
                    <h3>Outputs</h3>
                    <div className="output-grid">
                      {sortedOutputs.map((output) => (
                        <a
                          className={output.type === "thumbnail" ? "output-card thumbnail-card" : "output-card"}
                          href={output.cdn_url}
                          target="_blank"
                          rel="noreferrer"
                          key={output.type}
                        >
                          {output.type === "thumbnail" ? (
                            <img src={output.cdn_url} alt="" />
                          ) : (
                            <span className="video-token">{outputLabels[output.type]}</span>
                          )}
                          <span>{outputLabels[output.type]}</span>
                          <strong>{formatBytes(output.file_size)}</strong>
                        </a>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}
          </section>
        </div>
      </section>
    </main>
  );
}

function EmptyState() {
  return (
    <div className="empty-state">
      <span className="empty-mark">TCX</span>
    </div>
  );
}

function StatusBadge({ status }: { status: JobStatus }) {
  return <span className={`status-badge ${status}`}>{statusLabels[status]}</span>;
}

function ProgressRail({ status }: { status: JobStatus }) {
  const steps: JobStatus[] = ["queued", "processing", "completed"];
  const currentIndex = status === "dead" ? 1 : steps.indexOf(status);

  return (
    <ol className="progress-rail" aria-label="Job progress">
      {steps.map((step, index) => (
        <li className={index <= currentIndex ? "active" : ""} key={step}>
          <span />
          {statusLabels[step]}
        </li>
      ))}
      {status === "dead" && (
        <li className="active dead-step">
          <span />
          Dead
        </li>
      )}
    </ol>
  );
}

function useElapsedTime(startedAt?: string, stoppedAt?: string, stopped = false) {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!startedAt || stopped) {
      return;
    }
    const interval = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, [startedAt, stopped]);

  if (!startedAt) {
    return "-";
  }

  const endTime = stopped && stoppedAt ? new Date(stoppedAt).getTime() : now;
  const elapsedSeconds = Math.max(0, Math.floor((endTime - new Date(startedAt).getTime()) / 1000));
  if (elapsedSeconds < 60) {
    return `${elapsedSeconds}s`;
  }
  const minutes = Math.floor(elapsedSeconds / 60);
  const seconds = elapsedSeconds % 60;
  return `${minutes}m ${seconds}s`;
}

async function getJob(jobId: string, signal?: AbortSignal): Promise<Job> {
  const response = await fetch(`${API_BASE}/jobs/${jobId}`, { signal });
  const payload = await readJSON(response);
  if (!response.ok) {
    throw new Error(payload.error ?? "Could not fetch job.");
  }
  return payload;
}

async function readJSON(response: Response) {
  try {
    return await response.json();
  } catch {
    return {};
  }
}

function errorMessage(err: unknown) {
  return err instanceof Error ? err.message : "Something went wrong.";
}

function formatBytes(bytes: number) {
  if (bytes === 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / 1024 ** index;
  return `${value.toFixed(value >= 10 || index === 0 ? 0 : 1)} ${units[index]}`;
}

function formatTime(value: string) {
  return new Date(value).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

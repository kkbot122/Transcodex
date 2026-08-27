# Portfolio-Grade Reliability, Observability, and Local Benchmarking

Status: Ready for implementation

## Problem Statement

Transcodex demonstrates a distributed video-processing architecture, but it does not yet produce reproducible evidence for the performance and reliability claims made about it. The current system exposes queue depth, recent completions, worker state, and a live SSE dashboard, but it does not persist queue wait, processing-phase, end-to-end, or recovery timings. It also has no controlled benchmark that compares sequential and parallel FFmpeg execution or measures horizontal worker scaling.

The current recovery model has correctness gaps that weaken an interview explanation. Processing liveness is inferred from the job's general update timestamp rather than a renewable attempt lease, so a valid long-running transcode can be mistaken for an orphan. A worker's Redis lock expires without renewal, and completion is not fenced by an attempt identity, so a stale worker can continue writing after recovery has assigned the job elsewhere. Redis and PostgreSQL cannot share a transaction, and the existing enqueue/requeue sequence does not make that boundary or its reconciliation behavior explicit. Output upserts prevent duplicate database rows but do not prove that only one attempt can publish a visible output set.

The project therefore needs an honest, testable reliability contract and a local benchmark that works without AWS credits. The resulting measurements must be reproducible enough to support resume bullets while remaining explicitly tied to the machine, workload, and configuration on which they were collected.

## Solution

Introduce an attempt-aware job lifecycle that provides at-least-once attempt execution and at-most-once visible completion. PostgreSQL remains the durable source of truth. Every worker claim creates a uniquely identified attempt with a renewable processing lease. All state transitions are conditional on that attempt still being the job's current attempt. A stale worker may finish local work or leave unreferenced objects, but it cannot change job state or publish outputs to clients.

Centralize lifecycle transitions in one shared coordinator used by the API, workers, and reaper. Replace general job-age orphan detection with expired-attempt recovery. Remove the per-job Redis worker lock once PostgreSQL attempt fencing is in place. Keep the reaper leadership lease as an efficiency mechanism while also making every recovery transition safe if two reapers briefly overlap.

Persist phase timings and attempt outcomes in PostgreSQL, expose operational metrics in Prometheus format, and extend the existing SSE dashboard with latency, retry, lease, and recovery information. Add a host-side benchmark orchestrator that starts isolated Docker Compose environments, submits deterministic synthetic videos through the public API, compares sequential and parallel output generation, scales workers from one to four replicas, and emits raw JSON plus a human-readable Markdown report.

Provide two benchmark profiles. The fast profile verifies the harness and correctness in CI using short synthetic inputs. The portfolio profile runs 30-second 1080p inputs across 10-, 25-, and 50-job workloads, one, two, and four workers, sequential and parallel FFmpeg modes, and three measured repetitions after warm-up. The portfolio report records machine and software metadata so its numbers can be defended rather than generalized beyond the test environment.

## User Stories

1. As a backend candidate, I want every performance claim to be generated from a repeatable benchmark, so that I can defend the claim in an interview.
2. As a backend candidate, I want benchmark reports to identify the workload and machine, so that I do not imply the numbers are universal.
3. As a backend candidate, I want to compare sequential and parallel FFmpeg execution, so that I can explain the benefit and cost of intra-job concurrency.
4. As a backend candidate, I want to compare one, two, and four workers, so that I can explain horizontal scaling and diminishing returns.
5. As a backend candidate, I want measured p50 and p95 values, so that I can discuss latency distributions instead of averages alone.
6. As a backend candidate, I want forced-crash recovery results, so that reliability claims are backed by observed behavior.
7. As a recruiter reviewing the project, I want one command for a short benchmark, so that I can validate the project without a long setup process.
8. As a recruiter reviewing the project, I want a committed benchmark report, so that I can understand the evidence without rerunning a CPU-heavy workload.
9. As an API client, I want an accepted job to remain durable if Redis is temporarily unavailable, so that queue infrastructure failure does not silently discard my request.
10. As an API client, I want only one completed output set to become visible, so that retries do not produce conflicting client-visible results.
11. As an API client, I want job status responses to include useful timing information, so that I can understand where a completed job spent its time.
12. As an API client, I want existing job and output endpoints to remain compatible, so that instrumentation does not break current integrations.
13. As a worker, I want to claim a queued job through one conditional lifecycle operation, so that duplicate or stale queue entries cannot create duplicate current attempts.
14. As a worker, I want a unique attempt identity, so that my updates can be rejected after my authorization becomes stale.
15. As a worker, I want to renew my processing lease while making progress, so that a valid long transcode is not recovered as an orphan.
16. As a worker, I want phase durations recorded durably, so that download, FFmpeg, and upload bottlenecks can be distinguished.
17. As a worker, I want the first failed parallel output task to cancel sibling tasks, so that a doomed attempt does not continue consuming unnecessary CPU.
18. As a worker, I want sequential and parallel modes to use identical encoding arguments, so that their benchmark comparison isolates scheduling mode.
19. As a worker, I want output object keys to be scoped to my attempt, so that stale uploads cannot overwrite the winning attempt's artifacts.
20. As a worker, I want completion to fail cleanly when I am no longer current, so that a recovered job cannot be completed twice.
21. As a worker, I want a failure transition to be conditional on my attempt identity, so that a stale failure cannot requeue a job completed by another attempt.
22. As a worker, I want graceful shutdown to renew the lease while draining, so that an intentionally draining job is not recovered prematurely.
23. As a reaper, I want to detect expired processing leases, so that recovery is based on attempt liveness rather than generic job age.
24. As a reaper, I want recovery transitions to be conditional and idempotent, so that overlapping sweeps cannot increment retries or enqueue the same recovery twice.
25. As a reaper, I want to distinguish worker failure, lease expiry, processing failure, and queue reconciliation, so that recovery metrics explain why work was retried.
26. As a reaper, I want to repair queued jobs missing from Redis, so that the PostgreSQL-to-Redis consistency gap is recoverable.
27. As a reaper, I want stale Redis entries to be harmless, so that reconciliation races cannot create an unauthorized attempt.
28. As an operator, I want queue depth and oldest queued age, so that I can see whether arrival rate is exceeding capacity.
29. As an operator, I want jobs and attempts by status, so that queued demand can be separated from active, failed, expired, and dead work.
30. As an operator, I want queue-wait, FFmpeg, upload, and total-duration percentiles, so that I can locate the dominant source of latency.
31. As an operator, I want retry and recovery counters labeled by bounded reason categories, so that failures can be investigated without high-cardinality metrics.
32. As an operator, I want current lease age and worker heartbeat age, so that silent worker or processing failures are visible.
33. As an operator, I want Prometheus-compatible service metrics, so that the system can be scraped by standard monitoring tools.
34. As an operator, I want the existing SSE dashboard to show the new measurements, so that local demonstrations remain visual and immediate.
35. As an operator, I want metrics endpoints kept on internal service surfaces, so that operational details are not unintentionally public.
36. As a benchmark runner, I want a deterministic synthetic video corpus, so that runs do not depend on copyrighted or network-fetched media.
37. As a benchmark runner, I want an isolated Compose project per run, so that previous jobs and metrics do not contaminate current results.
38. As a benchmark runner, I want a warm-up run excluded from measurements, so that image startup and cold initialization do not distort steady-state comparisons.
39. As a benchmark runner, I want every submitted job checked for successful completion and the complete output set, so that speed is never reported without correctness.
40. As a benchmark runner, I want raw per-job measurements preserved alongside aggregates, so that reported percentiles and percentages can be audited.
41. As a benchmark runner, I want failed and timed-out jobs represented in the report, so that throughput cannot be inflated by ignoring unsuccessful work.
42. As a benchmark runner, I want configuration, image digests, Git revision, FFmpeg version, Docker version, CPU, memory, and operating system recorded, so that results can be reproduced approximately.
43. As a CI maintainer, I want a fast benchmark profile with bounded runtime, so that harness regressions are caught without running the portfolio matrix.
44. As a CI maintainer, I want performance values treated as observations rather than machine-independent pass thresholds, so that shared-runner variance does not create flaky builds.
45. As a maintainer, I want schema migrations to preserve existing jobs and outputs, so that the upgrade does not require resetting local data.
46. As a maintainer, I want lifecycle SQL and queue scripts owned by shared modules, so that API, worker, and reaper semantics cannot drift.
47. As a maintainer, I want current unit tests retained and extended, so that priority, retry, and reaper behavior remain covered during the migration.
48. As a maintainer, I want documentation to state the exact delivery and completion guarantees, so that code, system-design notes, and resume bullets use the same language.
49. As a maintainer, I want AWS Terraform retained but described as authored infrastructure unless a real deployment is verified, so that the portfolio remains truthful without AWS credits.
50. As a maintainer, I want generated resume bullets to contain no placeholder or unmeasured number, so that every quantitative claim points back to a recorded benchmark result.

## Implementation Decisions

### Reliability contract and domain model

- The formal guarantee is **at-least-once attempt execution with at-most-once visible completion**. The project will not claim exactly-once execution or zero job loss under every possible infrastructure failure.
- A **job** remains the durable client request. An **attempt** is one worker's execution of that job. Only the **current attempt** is authorized to renew a processing lease, record terminal state, or publish visible completion.
- PostgreSQL remains the source of truth for jobs, attempts, retry budgets, and visible outputs. Redis remains a replaceable scheduling index whose contents can be reconstructed from queued PostgreSQL jobs.
- The reaper leadership lease reduces duplicate sweep work, but correctness cannot depend solely on that lease. Conditional PostgreSQL updates must make overlapping recovery operations idempotent.

### Primary module and test seam

- Introduce one shared lifecycle coordinator used by the API, worker, and reaper. It owns creation of durable queued jobs, claim and attempt creation, lease renewal, phase recording, conditional completion, conditional failure, expired-attempt recovery, dead-job transitions, and queue reconciliation.
- The coordinator is the primary integration-test seam. Callers provide identifiers and output metadata; callers do not issue lifecycle SQL directly.
- Keep object transfer and FFmpeg execution outside the coordinator. The worker reports phase boundaries and final output metadata to the coordinator, preserving a narrow state-management interface.
- Keep the public upload/status/output API over the complete Docker Compose system as the highest acceptance-test seam.

### Schema and lifecycle state

- Add a durable attempt-status type with `running`, `completed`, `failed`, and `expired` values.
- Add a job-attempt relation containing: attempt identifier, job identifier, monotonically increasing attempt number per job, worker identifier, status, queue-entry time copied at claim, start time, last heartbeat time, lease expiry time, finish time, current processing phase, bounded failure category, sanitized failure detail, processing mode, and nullable phase durations for input download, FFmpeg processing, and output upload.
- Add a uniqueness constraint on job plus attempt number. Index running attempts by lease expiry and attempts by job/start time.
- Add current-attempt identifier, completed-attempt identifier, latest queue-entry time, and completion time to jobs. Current attempt is nullable and exists only while processing. Completed attempt identifies the attempt that produced the visible output set.
- Associate visible output rows with the completed attempt that produced them while retaining uniqueness by job and output type.
- Existing jobs migrate with null attempt references. Existing completed outputs remain valid. No destructive data reset is required.
- Retry count means the number of failed or expired attempts consumed. Maximum retries means additional attempts after the initial attempt, preserving the current user-facing interpretation.

### Job creation and the PostgreSQL/Redis boundary

- Store the source object first, then commit the queued job in PostgreSQL with its latest queue-entry time, then perform a best-effort Redis enqueue.
- Once the PostgreSQL job commit succeeds, the API returns `202 Accepted` with the job identifier even if immediate Redis enqueue fails. The failure is logged and counted; reconciliation will recreate the queue entry. This avoids accepting a queue entry for an uncommitted job and avoids returning an unusable error for a durable request.
- If source storage succeeds but the PostgreSQL job transaction fails, remove the source object on a best-effort basis and return an error as today.
- The cross-system boundary deliberately avoids distributed transactions and transactional-outbox infrastructure. Durability plus periodic reconciliation is the chosen tradeoff and must be documented.

### Redis priority queue

- Replace floating-point composite priority scores with a sorted-set index of non-empty priority tiers plus one FIFO list per tier. The existing queued-job membership set remains the O(1) deduplication and reconciliation index.
- Validate upload priority to a documented bounded integer range. Higher numeric priority is selected first; list order is exact FIFO within a priority tier.
- Implement enqueue as one Lua operation: add membership if absent, append the job identifier to its priority list, and add the tier to the sorted-set index. Duplicate enqueue requests are no-ops.
- Implement pop as one Lua operation: select the highest tier, remove the oldest job identifier from that tier, remove membership, and remove an empty tier from the index. Queue removal and membership removal therefore cannot diverge between commands.
- Store only the job identifier in Redis. Workers read immutable input location, priority, retry configuration, and latest queue timestamp from PostgreSQL when claiming. This prevents serialized queue payloads from becoming stale.
- A worker crash after pop but before claim leaves a durable queued job missing from Redis; reconciliation restores it. A stale or duplicate Redis identifier fails the conditional PostgreSQL claim and is discarded safely.

### Claiming and processing leases

- Claiming a job and creating its attempt occur in one PostgreSQL transaction. The transaction succeeds only when the job is still queued and has no current attempt.
- Claim assigns a new attempt identifier and attempt number, changes the job to processing, and records the attempt as current. Competing workers receive a non-claimed result rather than an error.
- Remove the per-job Redis worker lock after attempt fencing is active. It is redundant with conditional claim and cannot safely represent long FFmpeg ownership without renewal.
- Each running attempt has a PostgreSQL processing lease. The worker renews it at an interval no greater than one-third of the configured lease duration.
- Lease renewal updates only the row that is running and still referenced as the job's current attempt. A zero-row update tells the worker that its work is stale and should be cancelled.
- Worker process heartbeats remain for operator visibility and worker status, but attempt lease expiry is the authority for recovering processing work.
- Graceful shutdown keeps attempt lease renewal active during the bounded drain period. If the drain deadline expires, the processing context is cancelled and ordinary failure or lease-expiry recovery applies.

### Processing modes and phase measurements

- Add explicit `parallel` and `sequential` output-processing modes. Parallel remains the normal runtime default; sequential exists for controlled comparison and diagnostic use.
- Both modes use the same set of outputs, FFmpeg binary, codecs, filters, presets, thread settings, and source file. Only scheduling mode differs.
- The benchmark fixes container CPU and memory limits and records the FFmpeg thread configuration. This prevents a comparison from silently changing available resources.
- In parallel mode, output tasks share a cancellable context. The first task failure cancels sibling FFmpeg subprocesses and the attempt fails after all subprocesses have exited.
- Record input-download, FFmpeg-processing, and output-upload durations for every attempt when those phases occur. Record queue wait as attempt start minus copied queue-entry time and total job duration as visible completion minus job creation.
- Track the current bounded phase (`claiming`, `downloading`, `processing`, `uploading`, or `completing`) for operational display and deterministic failure tests. Phase values are not Prometheus labels.

### Output publication and stale-attempt fencing

- Write generated objects under immutable attempt-scoped keys rather than deterministic job-only keys. A stale attempt can therefore leave unreferenced artifacts but cannot overwrite the winning attempt's objects.
- Uploading objects does not make them visible through the job API. Visible completion occurs only in a PostgreSQL transaction that verifies the attempt is still current and running.
- The completion transaction inserts or updates the job's output metadata from the attempt-scoped objects, marks the attempt completed, records the completed-attempt identifier and job completion time, changes the job to completed, and clears the current-attempt identifier.
- If the conditional job transition affects no row, completion returns a stale-attempt result. No visible output metadata is changed. Best-effort deletion may remove that attempt's unreferenced objects.
- Output URLs remain immutable and cacheable. Existing clients continue to obtain them from job and output responses.

### Failure, expiry, retry, and recovery

- Worker-observed failure conditionally marks only the current running attempt failed. It records a bounded reason, increments retry count, clears the current attempt, and either queues the job again or marks it dead when the retry budget is exhausted.
- The reaper selects running attempts whose lease has expired. A conditional transaction marks each still-current attempt expired and performs the same retry/dead decision. Generic `jobs.updated_at` age is no longer used to infer processing liveness.
- Requeue and dead transitions commit in PostgreSQL before best-effort Redis enqueue. Queue reconciliation handles enqueue failure.
- Recovery reason categories are bounded and include at least processing failure, worker shutdown, lease expiry, worker death observation, and missing queue entry. Raw error strings never become metric labels.
- Dead workers may still be marked from process-heartbeat age for display, but marking a worker dead does not independently consume another retry if its current attempt has already been recovered.
- Recovery timing is measured from the earlier of observed worker termination or lease expiry baseline, as defined by the failure test, until the replacement attempt starts. Reports state which baseline was used.

### API and dashboard contracts

- Preserve existing upload, job, output, health, internal stats, internal jobs, internal workers, and SSE routes.
- Extend completed job responses with a nested timing summary containing queue wait, input download, FFmpeg processing, output upload, total duration, and attempt count in milliseconds. Fields are null or omitted when unavailable for migrated or incomplete jobs.
- Extend internal statistics with p50 and p95 queue wait, FFmpeg processing, output upload, and total duration over a documented rolling window; attempts by status; retry and recovery counts; oldest queued age; and maximum active lease age.
- Continue pushing internal statistics over SSE at the configured interval. Dashboard cards and tables display the new values with explicit units and an empty state when the rolling window has no samples.
- Keep internal and metrics endpoints suitable for private-network or local access. Public authentication and authorization remain outside this change.

### Prometheus metrics

- Expose Prometheus-compatible metrics from API, worker, and reaper service processes on internal endpoints.
- Provide counters for accepted jobs, queue synchronization failures, attempts started/completed/failed/expired, jobs completed/dead, and recoveries by bounded reason.
- Provide gauges for Redis queue depth, oldest queued age, jobs by status, attempts by status, workers by status, and active lease age.
- Provide histograms for queue wait, input download, FFmpeg processing, output upload, total job duration, reaper sweep duration, and recovery duration where the service observes them.
- Do not use job identifiers, attempt identifiers, worker identifiers, filenames, object keys, error strings, or URLs as metric labels.
- Add an optional local observability Compose profile containing Prometheus configured to scrape the service endpoints. The existing dashboard remains the primary visual demo; Grafana is not required.
- Persisted PostgreSQL timings are the source for benchmark reports and historical dashboard aggregates. In-process Prometheus metrics are operational and may reset with a container.

### Benchmark harness

- Provide a host-side benchmark orchestrator rather than mounting the Docker socket into a container. The orchestrator builds images, creates an isolated Compose project, scales workers, invokes a containerized load generator, collects results, and tears down only resources created for that benchmark project.
- Generate source videos locally with FFmpeg using deterministic video and audio filters. Do not download benchmark media from the network.
- The fast profile uses a short synthetic input, a small job count, one measured repetition, both processing modes, and a reduced worker matrix sufficient to validate orchestration and report generation within CI-friendly time.
- The portfolio profile uses a 30-second 1080p H.264 input; 10-, 25-, and 50-job workloads; one, two, and four workers; sequential and parallel modes; one discarded warm-up; and three measured repetitions per matrix cell.
- Submit every job through the public upload API and poll the public job endpoint until completed, dead, or timed out. Verify exactly the configured rendition and thumbnail output types before counting a job as successful.
- Use a fresh isolated database, Redis namespace, and object-storage volume for a benchmark invocation. Generated teardown may remove only the unique Compose project and volumes created by that invocation.
- Record raw per-job status, attempt count, phase timings, wall-clock submission/completion times, output metadata, and errors in machine-readable JSON.
- Produce a Markdown summary containing success rate, completed throughput, p50/p95 queue wait, p50/p95 FFmpeg time, p50/p95 end-to-end time, retry count, dead-job count, and relative changes between configurations.
- Throughput is successful completed jobs divided by measured wall-clock time from first submission to last terminal result. Failed or timed-out jobs remain in the denominator and are reported separately.
- Percentage improvements use the same workload and machine and state the baseline explicitly. The report never replaces missing or failed cells with zero and never generates resume prose from incomplete data.
- Capture benchmark date, Git revision and dirty-state flag, operating system, CPU model/count, available memory, Docker and Compose versions, image digests, FFmpeg version, source-video checksum, container CPU/memory limits, FFmpeg thread setting, worker count, processing mode, job count, repetition, and timeout.
- Commit the benchmark methodology and a representative portfolio report. Raw result files may be retained in a dedicated results area when reasonably sized; generated video files and transient volumes are not committed.

### Failure-injection suite

- Add a Docker integration scenario that submits work, waits for a known processing phase, terminates the active worker container, starts or relies on a replacement worker, and verifies recovery through the public API.
- Cover termination during input download, FFmpeg processing, output upload, and the boundary before completion commit.
- Provide deterministic phase-boundary pauses only in an integration-test build or test-injected processor dependency. Production images do not expose arbitrary pause/failure controls.
- For every scenario, assert that the original attempt becomes failed or expired, a later attempt becomes current, the job reaches one terminal state, retry count changes once, and only the completed attempt's output set is visible.
- Run at least one scenario with two reaper replicas to demonstrate that leadership plus conditional transitions prevents duplicate retry consumption and requeue storms.
- Report recovery latency across repeated forced-crash runs, including p50/p95 and the configured lease/sweep timings that bound the result.

### Documentation and portfolio output

- Update architecture and system-design documentation to use job, attempt, current attempt, processing lease, recovery, and visible completion consistently.
- Replace claims of zero job loss, exactly-once execution, or generic effectively-once behavior with the implemented guarantee and its failure boundaries.
- Document the intentional PostgreSQL/Redis reconciliation tradeoff and explain why a transactional outbox or managed queue was not added for this portfolio scope.
- Document benchmark commands, expected runtime, profile matrix, result methodology, and how to regenerate the report.
- Describe the Terraform as authored AWS deployment infrastructure unless an actual deployment and smoke test have been performed. Local MinIO and Docker Compose remain the verified execution environment.
- Generate final resume bullets only after a successful representative portfolio run. Every number in those bullets must be traceable to the committed report.

### Acceptance criteria

- A long-running attempt that continuously renews its lease is never recovered solely because the job's general update timestamp is old.
- After an attempt's lease expires and recovery makes another attempt current, the stale attempt cannot complete the job or replace visible output metadata.
- Concurrent claim attempts yield at most one current running attempt.
- Concurrent recovery attempts consume at most one retry and create at most one logical queue entry.
- Redis enqueue and pop keep the tier index, FIFO list, and queued-job membership set consistent atomically.
- A PostgreSQL-committed queued job is eventually restored to Redis after an injected enqueue failure.
- The complete Docker acceptance flow produces the configured output set in both sequential and parallel modes.
- The fast benchmark profile completes and emits valid raw JSON and Markdown with environment metadata.
- The portfolio profile supports the accepted full matrix and computes auditable p50/p95, throughput, success, retry, and comparison values.
- Prometheus endpoints parse successfully and contain no high-cardinality identifiers.
- The SSE dashboard continues updating and renders the new metrics without breaking existing queue, job, or worker views.
- Failure-injection tests prove recovery and single visible completion at each specified processing boundary.
- Documentation and resume guidance contain no unmeasured quantitative claim.

## Testing Decisions

- Good tests assert externally observable lifecycle behavior: which attempt is current, whether a lease can be renewed, whether completion becomes visible, whether retry is consumed once, whether a queue entry exists once, and what an API client sees. Tests must not assert goroutine counts, exact SQL statement text, private helper calls, Redis command ordering outside the Lua contract, or dashboard component internals.
- The shared lifecycle coordinator is tested against real PostgreSQL and Redis containers. This is the main integration seam because transaction isolation, conditional updates, lease timestamps, and Lua atomicity are the behavior under test and should not be mocked.
- Claim tests cover one winner under concurrent callers, rejection of non-queued jobs, attempt-number monotonicity, and copied queue-entry time.
- Lease tests cover successful renewal by the current attempt, rejection after recovery or completion, and non-recovery while renewal remains timely.
- Completion tests cover successful visible completion, stale-attempt rejection, output uniqueness, attempt-scoped object metadata, and migrated jobs without timing data.
- Failure tests cover retryable failure, exhausted retry budget, duplicate failure calls, failure from a stale attempt, and bounded/sanitized failure categories.
- Recovery tests cover expired leases, overlapping reapers, leadership loss during a sweep, worker-heartbeat death observation without duplicate retry, and jobs already completed when scanned.
- Queue integration tests cover exact FIFO within each priority tier, higher-priority selection, duplicate enqueue idempotency, atomic membership removal, empty-tier removal, stale entries, and reconstruction of a missing entry.
- API acceptance tests run against the Docker stack and exercise upload, accepted durability, polling, completion timings, output visibility, internal statistics, SSE continuity, and input validation including priority bounds.
- Processor tests use a fake command runner only to verify scheduling semantics, cancellation, phase transitions, and identical task arguments between modes. At least one Docker acceptance test uses real FFmpeg to validate actual output generation.
- Prometheus tests parse emitted text with the Prometheus parser and reject forbidden high-cardinality labels. They test metric names and semantic presence rather than unstable counter values from unrelated test activity.
- Dashboard tests verify rendering from representative SSE payloads, units, empty windows, and reconnect behavior. They do not duplicate backend percentile calculations.
- The fast benchmark runs in CI as a correctness smoke test. It validates result schema, complete outputs, aggregation math, and report generation but does not enforce a minimum speedup.
- The full portfolio benchmark is an explicit local command because its runtime and CPU demand are unsuitable for every commit. Its raw data and report are validated by the same report-schema and aggregation tests used in CI.
- Benchmark aggregation tests use fixed datasets to verify percentile selection, throughput denominator, success/failure accounting, relative-change direction, missing-cell handling, and stable Markdown output.
- Failure-injection tests use isolated Compose project names and test-only phase controls. Each test owns and removes only its generated resources.
- Existing priority-score, retry-decision, and reaper-decision unit tests are prior art for small deterministic rules. They will be retained where still relevant, replaced when their old model is removed, and supplemented by the higher lifecycle and Docker seams above.
- Test timestamps use a controllable clock at the lifecycle seam where practical. Docker recovery tests use real time with generous bounded polling rather than exact sleeps.
- Verification includes all Go unit tests, lifecycle and queue container integration tests, frontend tests/build, the Docker end-to-end suite, the fast benchmark profile, Prometheus parsing, and documentation/report validation.

## Out of Scope

- Running or paying for an AWS deployment, collecting AWS production metrics, or claiming the Terraform deployment has been validated in AWS.
- Exactly-once attempt execution across FFmpeg, PostgreSQL, Redis, and object storage.
- A distributed transaction, transactional outbox, Kafka, SQS, or another managed queue migration.
- User authentication, multi-tenancy, quotas, billing, public API rate limiting, or public hardening of internal metrics endpoints.
- HLS/DASH packaging, adaptive bitrate manifests, additional codecs, GPU transcoding, content-aware encoding, or playback analytics.
- Automatic worker autoscaling or capacity provisioning based on Prometheus metrics.
- Grafana dashboards, OpenTelemetry traces, centralized log aggregation, paging, or alert delivery.
- Machine-independent performance thresholds in CI or promises that another computer will reproduce the same absolute numbers.
- A general garbage collector for all orphaned attempt-scoped objects. Attempts perform best-effort cleanup; durable storage reclamation can be specified separately.
- Redesigning the React dashboard beyond the cards, tables, and status details needed to present the new operational metrics.
- Replacing the existing Terraform architecture solely because the verified benchmark environment is local.

## Further Notes

- The implementation should be decomposed into tracer-bullet tickets after this spec is approved: lifecycle/schema foundation, atomic queue, lease-fenced worker flow, recovery, metrics/dashboard, benchmark harness, failure injection, and documentation/resume evidence are natural slices with explicit dependencies.
- The current system-design document describes output existence checks that are not present in the worker implementation and describes PostgreSQL-before-Redis ordering that the current transaction does not actually provide. Documentation updates must be based on verified code rather than preserving those statements.
- The existing worker lock TTL is five minutes while valid transcodes may exceed that duration. The new processing lease makes liveness renewable and observable rather than relying on a fixed Redis lock expiration.
- Parallel FFmpeg subprocesses may be slower on a CPU-constrained machine because each encoder competes for the same cores. That is a valid result. The report should explain saturation and diminishing returns instead of treating speedup as predetermined.
- The representative report should include confidence limits or at minimum all three repetitions and their spread. Three repetitions are enough for portfolio evidence but not a claim of production capacity planning.
- Resume bullets should prefer measured relative statements tied to the workload, such as “reduced p95 processing latency by X% on a fixed 30-second 1080p local workload,” and should state worker counts when quoting throughput scaling.
- The `ready-for-agent` label means this spec is buildable, not that all work belongs in one implementation change. Ticket decomposition should preserve an end-to-end runnable system after each tracer-bullet slice.

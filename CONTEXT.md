# Transcodex

Transcodex accepts source videos as jobs, executes one or more processing attempts, and publishes a single visible set of derived outputs.

## Language

**Job**:
A durable request to transform one source video into the configured output set. A job survives individual worker and attempt failures.
_Avoid_: Task, message, transcode

**Attempt**:
One worker's time-bounded execution of a job. A job may have multiple attempts, but only its current attempt may complete it.
_Avoid_: Retry, run, execution

**Current attempt**:
The attempt presently authorized to advance a job or publish its completion. Work from an earlier attempt is stale even if that worker is still running.
_Avoid_: Active retry, owner

**Output**:
One derived artifact produced for a job, such as a rendition or thumbnail. The visible output set belongs to the job rather than to an individual attempt.
_Avoid_: Result, file

**Queue entry**:
The temporary Redis representation that makes a queued job available for claiming. PostgreSQL job state remains the durable source of truth.
_Avoid_: Job, message

**Processing lease**:
The renewable authorization held by the current attempt while it is making progress. An expired lease makes the attempt eligible for recovery.
_Avoid_: Lock, heartbeat

**Recovery**:
The transition that retires an expired or failed current attempt and makes its incomplete job eligible for another attempt.
_Avoid_: Restart, resurrection

**Visible completion**:
The single committed job state and output set exposed to clients. At-least-once attempt execution must still produce at most one visible completion.
_Avoid_: Exactly-once execution

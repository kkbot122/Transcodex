package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
)

type timing struct {
	QueueWaitMS  *int64 `json:"queue_wait_ms,omitempty"`
	DownloadMS   *int64 `json:"download_ms,omitempty"`
	ProcessingMS *int64 `json:"processing_ms,omitempty"`
	UploadMS     *int64 `json:"upload_ms,omitempty"`
	TotalMS      *int64 `json:"total_ms,omitempty"`
	AttemptCount int64  `json:"attempt_count"`
}

type jobResult struct {
	JobID   string  `json:"job_id"`
	Status  string  `json:"status"`
	Timings *timing `json:"timings,omitempty"`
	Error   string  `json:"error,omitempty"`
}

type report struct {
	RunID       string      `json:"run_id"`
	Mode        string      `json:"mode"`
	Workers     int         `json:"workers"`
	Jobs        int         `json:"jobs"`
	StartedAt   time.Time   `json:"started_at"`
	FinishedAt  time.Time   `json:"finished_at"`
	Environment environment `json:"environment"`
	Results     []jobResult `json:"results"`
	Summary     summary     `json:"summary"`
}

type environment struct {
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	CPUs        int    `json:"cpus"`
	GitRev      string `json:"git_revision,omitempty"`
	Dirty       string `json:"git_dirty,omitempty"`
	FFmpeg      string `json:"ffmpeg_version,omitempty"`
	Docker      string `json:"docker_version,omitempty"`
	InputSHA256 string `json:"input_sha256,omitempty"`
}

type summary struct {
	Completed        int     `json:"completed"`
	Failed           int     `json:"failed"`
	SuccessRate      float64 `json:"success_rate"`
	ThroughputPerMin float64 `json:"throughput_per_minute"`
	QueueWaitP50MS   *int64  `json:"queue_wait_p50_ms,omitempty"`
	QueueWaitP95MS   *int64  `json:"queue_wait_p95_ms,omitempty"`
	ProcessingP50MS  *int64  `json:"processing_p50_ms,omitempty"`
	ProcessingP95MS  *int64  `json:"processing_p95_ms,omitempty"`
	TotalP50MS       *int64  `json:"total_p50_ms,omitempty"`
	TotalP95MS       *int64  `json:"total_p95_ms,omitempty"`
}

func main() {
	baseURL := flag.String("base-url", "http://localhost:8080", "Transcodex API URL")
	input := flag.String("input", "", "benchmark input video")
	jobs := flag.Int("jobs", 3, "number of jobs")
	runID := flag.String("run-id", time.Now().UTC().Format("20060102T150405Z"), "benchmark run identifier")
	mode := flag.String("mode", "parallel", "processing mode")
	workers := flag.Int("workers", 1, "worker count")
	output := flag.String("output", "benchmark.json", "JSON report path")
	markdown := flag.String("markdown", "", "optional Markdown report path")
	timeout := flag.Duration("timeout", 30*time.Minute, "overall benchmark timeout")
	flag.Parse()
	if *input == "" || *jobs < 1 {
		fatal("input and a positive jobs count are required")
	}

	started := time.Now().UTC()
	client := &http.Client{Timeout: 30 * time.Second}
	ids := make([]string, 0, *jobs)
	for i := 0; i < *jobs; i++ {
		id, err := submit(client, *baseURL, *input)
		if err != nil {
			fatal("submit job: %v", err)
		}
		ids = append(ids, id)
	}

	deadline := time.Now().Add(*timeout)
	results := make([]jobResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, waitForJob(client, *baseURL, id, deadline))
	}
	finished := time.Now().UTC()
	r := report{RunID: *runID, Mode: *mode, Workers: *workers, Jobs: *jobs, StartedAt: started, FinishedAt: finished, Environment: collectEnvironment(*input), Results: results}
	r.Summary = summarize(results, started, finished)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		fatal("encode report: %v", err)
	}
	if err := os.WriteFile(*output, append(data, '\n'), 0o644); err != nil {
		fatal("write report: %v", err)
	}
	if *markdown != "" {
		if err := os.WriteFile(*markdown, []byte(markdownReport(r)), 0o644); err != nil {
			fatal("write Markdown report: %v", err)
		}
	}
	fmt.Printf("completed=%d failed=%d throughput=%.2f jobs/min report=%s\n", r.Summary.Completed, r.Summary.Failed, r.Summary.ThroughputPerMin, *output)
}

func markdownReport(r report) string {
	return fmt.Sprintf("# Transcodex Benchmark\n\n- Run: `%s`\n- Mode: `%s`\n- Workers: `%d`\n- Jobs: `%d`\n- OS/arch: `%s/%s`\n- CPUs: `%d`\n- Git revision: `%s`\n- Git dirty: `%s`\n- Docker: `%s`\n- FFmpeg: `%s`\n- Input SHA-256: `%s`\n\n| Metric | Value |\n|---|---:|\n| Completed | %d/%d |\n| Success rate | %.1f%% |\n| Throughput | %.2f jobs/min |\n| Queue wait p50 | %s ms |\n| Queue wait p95 | %s ms |\n| FFmpeg p50 | %s ms |\n| FFmpeg p95 | %s ms |\n| Total p50 | %s ms |\n| Total p95 | %s ms |\n\nThis report is workload- and machine-specific.\n", r.RunID, r.Mode, r.Workers, r.Jobs, r.Environment.OS, r.Environment.Arch, r.Environment.CPUs, r.Environment.GitRev, r.Environment.Dirty, r.Environment.Docker, r.Environment.FFmpeg, r.Environment.InputSHA256, r.Summary.Completed, r.Jobs, r.Summary.SuccessRate*100, r.Summary.ThroughputPerMin, formatOptional(r.Summary.QueueWaitP50MS), formatOptional(r.Summary.QueueWaitP95MS), formatOptional(r.Summary.ProcessingP50MS), formatOptional(r.Summary.ProcessingP95MS), formatOptional(r.Summary.TotalP50MS), formatOptional(r.Summary.TotalP95MS))
}

func formatOptional(value *int64) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprint(*value)
}

func submit(client *http.Client, baseURL, input string) (string, error) {
	file, err := os.Open(input)
	if err != nil {
		return "", err
	}
	defer file.Close()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "benchmark.mp4")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	if err := form.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/uploads", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload returned %s: %s", resp.Status, raw)
	}
	var payload struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.JobID, nil
}

func waitForJob(client *http.Client, baseURL, id string, deadline time.Time) jobResult {
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/jobs/"+id, nil)
		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var payload struct {
			Status  string  `json:"status"`
			Timings *timing `json:"timings"`
			Outputs []struct {
				Type string `json:"type"`
			} `json:"outputs"`
		}
		err = json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		if err == nil && (payload.Status == "completed" || payload.Status == "dead") {
			if payload.Status == "completed" && !completeOutputSet(payload.Outputs) {
				return jobResult{JobID: id, Status: "failed", Error: "completed job returned incomplete output set"}
			}
			return jobResult{JobID: id, Status: payload.Status, Timings: payload.Timings}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return jobResult{JobID: id, Status: "timeout", Error: "job did not reach a terminal state before timeout"}
}

func completeOutputSet(outputs []struct {
	Type string `json:"type"`
}) bool {
	if len(outputs) != 4 {
		return false
	}
	want := map[string]bool{"video_360p": false, "video_720p": false, "video_1080p": false, "thumbnail": false}
	for _, output := range outputs {
		if _, ok := want[output.Type]; !ok || want[output.Type] {
			return false
		}
		want[output.Type] = true
	}
	for _, found := range want {
		if !found {
			return false
		}
	}
	return true
}

func summarize(results []jobResult, started, finished time.Time) summary {
	var queueWait, processing, total []int64
	completed := 0
	for _, result := range results {
		if result.Status != "completed" {
			continue
		}
		completed++
		if result.Timings == nil {
			continue
		}
		appendValue(&queueWait, result.Timings.QueueWaitMS)
		appendValue(&processing, result.Timings.ProcessingMS)
		appendValue(&total, result.Timings.TotalMS)
	}
	minutes := finished.Sub(started).Minutes()
	if minutes == 0 {
		minutes = 1.0 / 60
	}
	return summary{Completed: completed, Failed: len(results) - completed, SuccessRate: float64(completed) / float64(len(results)), ThroughputPerMin: float64(completed) / minutes, QueueWaitP50MS: percentile(queueWait, .50), QueueWaitP95MS: percentile(queueWait, .95), ProcessingP50MS: percentile(processing, .50), ProcessingP95MS: percentile(processing, .95), TotalP50MS: percentile(total, .50), TotalP95MS: percentile(total, .95)}
}

func appendValue(values *[]int64, value *int64) {
	if value != nil {
		*values = append(*values, *value)
	}
}

func percentile(values []int64, p float64) *int64 {
	if len(values) == 0 {
		return nil
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	index := int(float64(len(values)-1)*p + .5)
	value := values[index]
	return &value
}

func collectEnvironment(input string) environment {
	e := environment{OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), Dirty: commandOutput("git", "status", "--porcelain"), Docker: commandOutput("docker", "version", "--format", "{{.Server.Version}}")}
	if e.Dirty == "" {
		e.Dirty = "clean"
	} else {
		e.Dirty = "dirty"
	}
	e.GitRev = commandOutput("git", "rev-parse", "HEAD")
	e.FFmpeg = commandOutput("ffmpeg", "-version")
	if i := strings.IndexByte(e.FFmpeg, '\n'); i >= 0 {
		e.FFmpeg = e.FFmpeg[:i]
	}
	if file, err := os.Open(input); err == nil {
		hasher := sha256.New()
		if _, err := io.Copy(hasher, file); err == nil {
			e.InputSHA256 = hex.EncodeToString(hasher.Sum(nil))
		}
		_ = file.Close()
	}
	return e
}

func commandOutput(name string, args ...string) string {
	output, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

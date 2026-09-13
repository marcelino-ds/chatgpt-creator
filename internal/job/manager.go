// Package job provides the event-driven registration job manager.
package job

import (
	"sync"
	"time"
)

// EventType identifies the kind of job event.
type EventType string

const (
	EventLog       EventType = "log"
	EventAccount   EventType = "account"
	EventStatus    EventType = "status"
	EventProgress  EventType = "progress"
	EventComplete  EventType = "complete"
)

// Event is a single message broadcast by the job manager.
type Event struct {
	Type      EventType `json:"type"`
	JobID     string    `json:"job_id"`
	Timestamp time.Time `json:"timestamp"`

	// Log fields
	WorkerID   int    `json:"worker_id,omitempty"`
	Tag        string `json:"tag,omitempty"`
	Step       string `json:"step,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
	Message    string `json:"message,omitempty"`

	// Account fields
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
	Success  bool   `json:"success,omitempty"`
	Error    string `json:"error,omitempty"`

	// Status / progress fields
	Status       string `json:"status,omitempty"`
	Target       int    `json:"target,omitempty"`
	Attempts     int    `json:"attempts,omitempty"`
	SuccessCount int    `json:"success_count,omitempty"`
	FailureCount int    `json:"failure_count,omitempty"`
	ElapsedMS    int64  `json:"elapsed_ms,omitempty"`
}

// EmitFunc is called for each event the job produces.
type EmitFunc func(e Event)

// JobManager orchestrates batch registration jobs and broadcasts events.
type JobManager struct {
	mu      sync.Mutex
	current *jobState
	emitter EmitFunc
}

// NewManager creates a job manager that emits events via fn.
func NewManager(fn EmitFunc) *JobManager {
	return &JobManager{emitter: fn}
}

// Current returns the active job id (empty string if none).
func (m *JobManager) Current() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return ""
	}
	return m.current.id
}

// IsRunning returns true when a job is active.
func (m *JobManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current != nil && m.current.status == "running"
}

// emitter helpers

func (m *JobManager) emit(e Event) {
	if m.emitter != nil {
		m.emitter(e)
	}
}

func (m *JobManager) logEvent(jobID string, workerID int, tag, step string, statusCode int, message string) {
	m.emit(Event{
		Type:       EventLog,
		JobID:      jobID,
		Timestamp:  time.Now(),
		WorkerID:   workerID,
		Tag:        tag,
		Step:       step,
		StatusCode: statusCode,
		Message:    message,
	})
}

func (m *JobManager) accountEvent(jobID, email, password string, success bool, errMsg string) {
	m.emit(Event{
		Type:      EventAccount,
		JobID:     jobID,
		Timestamp: time.Now(),
		Email:     email,
		Password:  password,
		Success:   success,
		Error:     errMsg,
	})
}

func (m *JobManager) statusEvent(jobID, status string, target, attempts, successCount, failureCount int, elapsedMS int64) {
	m.emit(Event{
		Type:         EventStatus,
		JobID:        jobID,
		Timestamp:    time.Now(),
		Status:       status,
		Target:       target,
		Attempts:     attempts,
		SuccessCount: successCount,
		FailureCount: failureCount,
		ElapsedMS:    elapsedMS,
	})
}

func (m *JobManager) progressEvent(jobID string, target, attempts, successCount, failureCount int) {
	m.emit(Event{
		Type:         EventProgress,
		JobID:        jobID,
		Timestamp:    time.Now(),
		Target:       target,
		Attempts:     attempts,
		SuccessCount: successCount,
		FailureCount: failureCount,
	})
}

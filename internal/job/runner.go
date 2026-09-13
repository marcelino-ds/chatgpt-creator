package job

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/verssache/chatgpt-creator/internal/email"
	"github.com/verssache/chatgpt-creator/internal/register"
	"github.com/verssache/chatgpt-creator/internal/util"
)

// Config is the parameters for a new job.
type Config struct {
	Target   int
	Workers  int
	Proxy    string
	Domain   string
	Password string
}

// jobState holds the runtime state of an active batch.
type jobState struct {
	id           string
	target       int
	workers      int
	status       string // running, paused, completed, stopped, failed
	successCount int
	failureCount int
	attempts     int
	elapsedMS    int64
	startedAt    time.Time
	done         chan struct{}
	pauseCh      chan struct{} // closed when paused
	resumeCh     chan struct{} // closed to signal resume
	cancelCh     chan struct{} // closed when cancelled
}

// Start launches a new batch registration job.
func (m *JobManager) Start(cfg Config) error {
	m.mu.Lock()
	if m.current != nil && (m.current.status == "running" || m.current.status == "paused") {
		m.mu.Unlock()
		return fmt.Errorf("a job is already active")
	}

	id := uuid.New().String()
	state := &jobState{
		id:        id,
		target:    cfg.Target,
		workers:   cfg.Workers,
		status:    "running",
		done:      make(chan struct{}),
		pauseCh:   make(chan struct{}),
		resumeCh:  make(chan struct{}),
		cancelCh:  make(chan struct{}),
		startedAt: time.Now(),
	}
	// resumeCh starts open (not paused).
	close(state.resumeCh)
	m.current = state
	m.mu.Unlock()

	m.statusEvent(id, "running", cfg.Target, 0, 0, 0, 0)
	go m.run(state, cfg)
	return nil
}

// Pause requests the active job to pause. Workers finish their current
// registration before blocking.
func (m *JobManager) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return fmt.Errorf("no active job")
	}
	if m.current.status != "running" {
		return fmt.Errorf("job is not running")
	}
	m.current.status = "paused"
	close(m.current.pauseCh)
	return nil
}

// Resume unpauses a paused job.
func (m *JobManager) Resume() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return fmt.Errorf("no active job")
	}
	if m.current.status != "paused" {
		return fmt.Errorf("job is not paused")
	}
	m.current.status = "running"
	close(m.current.resumeCh)
	return nil
}

// Stop cancels the active job. In-flight registrations complete; queued ones
// are dropped.
func (m *JobManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return fmt.Errorf("no active job")
	}
	if m.current.status != "running" && m.current.status != "paused" {
		return fmt.Errorf("job is not active")
	}
	close(m.current.cancelCh)
	return nil
}

// Snapshot returns a read-only view of the current job.
func (m *JobManager) Snapshot() (id, status string, target, attempts, success, failures int, elapsedMS int64, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return
	}
	s := m.current
	return s.id, s.status, s.target, s.attempts, s.successCount, s.failureCount, atomic.LoadInt64(&s.elapsedMS), true
}

// run orchestrates worker goroutines until target, cancellation, or stop.
func (m *JobManager) run(s *jobState, cfg Config) {
	defer close(s.done)

	var (
		remaining    int64 = int64(s.target)
		successCount int64
		failureCount int64
		attempts     int64
	)

	var wg sync.WaitGroup
	for w := 1; w <= s.workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				// Pause gate: wait for resume or cancellation.
				if s.status == "paused" {
					select {
					case <-s.resumeCh:
					case <-s.cancelCh:
						return
					}
				}

				// Cancellation check before claiming.
				select {
				case <-s.cancelCh:
					return
				default:
				}

				// Claim a slot.
				if atomic.AddInt64(&remaining, -1) < 0 {
					atomic.AddInt64(&remaining, 1)
					return
				}

				// Check cancellation again after claiming.
				select {
				case <-s.cancelCh:
					atomic.AddInt64(&remaining, 1)
					return
				default:
				}

				attempt := atomic.AddInt64(&attempts, 1)
				tag := fmt.Sprintf("%d/%d", attempt, s.target)

				done, emailAddr, password, errMsg := m.registerOne(s.id, workerID, tag, cfg.Proxy, cfg.Domain, cfg.Password)

				if done {
					count := atomic.AddInt64(&successCount, 1)
					m.accountEvent(s.id, emailAddr, password, true, "")
					m.progressEvent(s.id, s.target, int(attempt), int(count), int(atomic.LoadInt64(&failureCount)))
					if count >= int64(s.target) {
						atomic.AddInt64(&remaining, 1)
						return
					}
				} else {
					count := atomic.AddInt64(&failureCount, 1)
					m.accountEvent(s.id, emailAddr, password, false, errMsg)
					m.progressEvent(s.id, s.target, int(attempt), int(atomic.LoadInt64(&successCount)), int(count))
				}
			}
		}(w)
	}

	wg.Wait()

	elapsed := time.Since(s.startedAt).Milliseconds()
	finalStatus := "completed"
	select {
	case <-s.cancelCh:
		finalStatus = "stopped"
	default:
	}

	m.mu.Lock()
	s.successCount = int(atomic.LoadInt64(&successCount))
	s.failureCount = int(atomic.LoadInt64(&failureCount))
	s.attempts = int(atomic.LoadInt64(&attempts))
	s.elapsedMS = elapsed
	s.status = finalStatus
	m.mu.Unlock()

	m.statusEvent(s.id, finalStatus, s.target, int(atomic.LoadInt64(&attempts)), int(atomic.LoadInt64(&successCount)), int(atomic.LoadInt64(&failureCount)), elapsed)
	m.emit(Event{
		Type:         EventComplete,
		JobID:        s.id,
		Timestamp:    time.Now(),
		Status:       finalStatus,
		Target:       s.target,
		Attempts:     int(atomic.LoadInt64(&attempts)),
		SuccessCount: int(atomic.LoadInt64(&successCount)),
		FailureCount: int(atomic.LoadInt64(&failureCount)),
		ElapsedMS:    elapsed,
	})
}

// registerOne runs a single registration with event-aware logging.
func (m *JobManager) registerOne(jobID string, workerID int, tag, proxy, domain, password string) (success bool, emailAddr, pass, errMsg string) {
	var printMu, fileMu sync.Mutex
	client, err := register.NewClient(proxy, tag, workerID, &printMu, &fileMu)
	if err != nil {
		return false, "", "", fmt.Sprintf("client: %v", err)
	}

	client.SetLogFn(func(wid int, t, msg string) {
		sc := 0
		if _, err := fmt.Sscanf(msg, "%*s | %d", &sc); err == nil {
			// swallow
		}
		step := msg
		if idx := findStatusSep(msg); idx > 0 {
			step = msg[:idx]
		}
		m.logEvent(jobID, wid, t, step, sc, "")
	})

	addr, err := email.CreateTempEmail(domain)
	if err != nil {
		return false, "", "", fmt.Sprintf("email: %v", err)
	}

	pass = password
	if pass == "" {
		pass = util.GeneratePassword(14)
	}

	firstName, lastName := util.RandomName()
	birthdate := util.RandomBirthdate()

	err = client.RunRegister(addr, pass, firstName+" "+lastName, birthdate)
	if err != nil {
		return false, addr, pass, err.Error()
	}
	return true, addr, pass, ""
}

// findStatusSep returns the index of " | " that precedes a status code, or -1.
func findStatusSep(s string) int {
	for i := len(s) - 1; i >= 2; i-- {
		if s[i-2] == ' ' && s[i-1] == '|' && s[i] == ' ' {
			return i - 2
		}
	}
	return -1
}

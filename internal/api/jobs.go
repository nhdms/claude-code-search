package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type Job struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Progress  float64   `json:"progress"`
	Stats     any       `json:"stats,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type Jobs struct {
	mu sync.Mutex
	m  map[string]*Job
}

func NewJobs() *Jobs { return &Jobs{m: map[string]*Job{}} }

func (j *Jobs) Start(kind string, fn func(ctx context.Context, set func(msg string, progress float64), done func(stats any, err error))) *Job {
	id := newID()
	job := &Job{ID: id, Kind: kind, Status: "running", StartedAt: time.Now()}
	j.mu.Lock()
	j.m[id] = job
	j.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
		defer cancel()
		set := func(msg string, p float64) {
			j.mu.Lock()
			job.Message = msg
			job.Progress = p
			j.mu.Unlock()
		}
		done := func(stats any, err error) {
			j.mu.Lock()
			defer j.mu.Unlock()
			t := time.Now()
			job.EndedAt = &t
			job.Stats = stats
			if err != nil {
				job.Status = "failed"
				job.Error = err.Error()
			} else {
				job.Status = "completed"
				job.Progress = 1
			}
		}
		fn(ctx, set, done)
	}()
	return job
}

func (j *Jobs) Get(id string) *Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	v := j.m[id]
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func (j *Jobs) List() []*Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]*Job, 0, len(j.m))
	for _, v := range j.m {
		c := *v
		out = append(out, &c)
	}
	return out
}

package aiworker

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/aievents"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

type Classifier interface {
	Analyze(ctx context.Context, job storage.AnalysisJob) error
}

type Runner struct {
	DB           *storage.DB
	Classifier   Classifier
	WorkerID     string
	Lease        time.Duration
	PollInterval time.Duration
	RetryDelay   func(attempts int) time.Duration
	Events       *aievents.Bus
}

func (r *Runner) Run(ctx context.Context) error {
	if err := r.validate(); err != nil {
		return err
	}
	if _, err := r.DB.RecoverExpiredAnalysisJobs(ctx); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		worked, err := r.ProcessOnce(ctx)
		if err != nil {
			return err
		}
		if worked {
			continue
		}
		timer := time.NewTimer(r.pollInterval())
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func (r *Runner) ProcessOnce(ctx context.Context) (bool, error) {
	if err := r.validate(); err != nil {
		return false, err
	}
	job, err := r.DB.ClaimAnalysisJob(ctx, r.WorkerID, r.Lease)
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}
	if err := r.Classifier.Analyze(ctx, *job); err != nil {
		if errors.Is(err, storage.ErrAnalysisSkipped) {
			if skipErr := r.DB.SkipAnalysisJob(ctx, job.ID, r.WorkerID, err.Error()); skipErr != nil && !errors.Is(skipErr, storage.ErrAnalysisJobLeaseLost) {
				return true, skipErr
			}
			r.publish(*job, storage.AnalysisStateSkipped)
			return true, nil
		}
		if ctx.Err() != nil {
			_ = r.DB.FailAnalysisJob(context.Background(), job.ID, r.WorkerID, ctx.Err().Error())
			r.publish(*job, "failed")
			return true, nil
		}
		retryAt := time.Now().UTC().Add(r.retryDelay(job.Attempts))
		if retryErr := r.DB.RetryAnalysisJob(context.Background(), job.ID, r.WorkerID, err.Error(), retryAt); retryErr != nil && !errors.Is(retryErr, storage.ErrAnalysisJobLeaseLost) {
			return true, retryErr
		}
		log.Printf("ai worker: analysis job %s failed: %v", job.ID, err)
		r.publish(*job, "retry")
		return true, nil
	}
	if err := r.DB.CompleteAnalysisJob(ctx, job.ID, r.WorkerID); err != nil {
		if errors.Is(err, storage.ErrAnalysisJobLeaseLost) {
			return true, nil
		}
		return true, err
	}
	r.publish(*job, storage.AnalysisStateDone)
	return true, nil
}

func (r *Runner) publish(job storage.AnalysisJob, state string) {
	if r.Events == nil {
		return
	}
	r.Events.Publish(aievents.Event{
		Type:      "analysis.job",
		UserID:    job.UserID,
		JobID:     job.ID,
		MessageID: job.MessageID,
		State:     state,
	})
}

func (r *Runner) validate() error {
	if r.DB == nil {
		return errors.New("ai worker requires a database")
	}
	if r.Classifier == nil {
		return errors.New("ai worker requires a classifier")
	}
	if strings.TrimSpace(r.WorkerID) == "" {
		return errors.New("ai worker requires a worker id")
	}
	return nil
}

func (r *Runner) pollInterval() time.Duration {
	if r.PollInterval > 0 {
		return r.PollInterval
	}
	return 5 * time.Second
}

func (r *Runner) retryDelay(attempts int) time.Duration {
	if r.RetryDelay != nil {
		return r.RetryDelay(attempts)
	}
	if attempts < 1 {
		attempts = 1
	}
	delay := 30 * time.Second
	for i := 1; i < attempts; i++ {
		delay *= 2
		if delay >= time.Hour {
			delay = time.Hour
			break
		}
	}
	jitter := time.Duration(0)
	if delay > 0 {
		jitter = time.Duration(rand.Int63n(int64(delay/5) + 1))
	}
	return delay + jitter
}

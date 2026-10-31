package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/aievents"
	"github.com/cristianadrielbraun/gofer/internal/aipipeline"
	"github.com/cristianadrielbraun/gofer/internal/airouting"
	"github.com/cristianadrielbraun/gofer/internal/aiworker"
	"github.com/cristianadrielbraun/gofer/internal/config"
	"github.com/cristianadrielbraun/gofer/internal/jev"
	"github.com/cristianadrielbraun/gofer/internal/policy"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

type managedAIWorker struct {
	central  *storage.DB
	routing  *storage.AccountRouting
	client   *jev.Client
	events   *aievents.Bus
	secrets  *config.AccountStore
	interval time.Duration
}

func startManagedAIWorker(ctx context.Context, central *storage.DB, routing *storage.AccountRouting, events *aievents.Bus, secrets *config.AccountStore) (bool, error) {
	client, endpoint, err := configuredJevClient()
	if err != nil {
		return false, err
	}
	worker := &managedAIWorker{central: central, routing: routing, client: client, events: events, secrets: secrets, interval: 10 * time.Second}
	if client == nil {
		log.Printf("boot: managed AI worker disabled; set GOFER_JEV_API_KEY to enable Jev analysis")
	} else {
		go func() {
			log.Printf("boot: managed AI worker started (endpoint %s)", endpoint)
			worker.run(ctx)
			log.Printf("boot: managed AI worker stopped")
		}()
	}
	go worker.runRouting(ctx)
	return client != nil, nil
}

func (w *managedAIWorker) run(ctx context.Context) {
	w.process(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.process(ctx)
		}
	}
}

func (w *managedAIWorker) runRouting(ctx context.Context) {
	if w == nil || w.central == nil || w.routing == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		w.processRouting(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *managedAIWorker) processRouting(ctx context.Context) {
	rows, err := w.central.Read().QueryContext(ctx, `
		SELECT id FROM users
		WHERE status = 'active' AND deletion_pending = 0 AND user_type <> 'management'
		ORDER BY id`)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("managed routing: list users: %v", err)
		}
		return
	}
	var owners []string
	for rows.Next() {
		var owner string
		if err := rows.Scan(&owner); err != nil {
			rows.Close()
			return
		}
		owners = append(owners, owner)
	}
	rows.Close()
	for _, owner := range owners {
		if ctx.Err() != nil {
			return
		}
		err := w.routing.WithExistingUser(ctx, owner, func(db *storage.DB) error {
			dispatcher := &airouting.Dispatcher{
				DB: db, WorkerID: fmt.Sprintf("gofer-managed-routing-%d-%s", os.Getpid(), owner),
				Logf: log.Printf,
			}
			if w.secrets != nil {
				dispatcher.Resolve = func(_ context.Context, userID, connectorID, encrypted string) (string, error) {
					return w.secrets.DecryptAIConnectorSecret(userID, connectorID, encrypted)
				}
			}
			for i := 0; i < 20; i++ {
				worked, err := dispatcher.ProcessOnce(ctx)
				if err != nil || !worked {
					return err
				}
			}
			return nil
		})
		if err != nil && ctx.Err() == nil && !isMissingUserStore(err) {
			log.Printf("managed routing: %s: %v", owner, err)
		}
	}
}

func (w *managedAIWorker) process(ctx context.Context) {
	if w == nil || w.central == nil || w.routing == nil || w.client == nil {
		return
	}
	rows, err := w.central.Read().QueryContext(ctx, `
		SELECT id FROM users
		WHERE status = 'active' AND deletion_pending = 0 AND user_type <> 'management'
		ORDER BY id`)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("managed AI worker: list users: %v", err)
		}
		return
	}
	var owners []string
	for rows.Next() {
		var owner string
		if err := rows.Scan(&owner); err != nil {
			rows.Close()
			return
		}
		owners = append(owners, owner)
	}
	rows.Close()
	for _, owner := range owners {
		if ctx.Err() != nil {
			return
		}
		if err := w.processOwner(ctx, owner); err != nil && ctx.Err() == nil && !isMissingUserStore(err) {
			log.Printf("managed AI worker: %s: %v", owner, err)
		}
	}
}

func (w *managedAIWorker) processOwner(ctx context.Context, owner string) error {
	return w.routing.WithExistingUser(ctx, owner, func(db *storage.DB) error {
		if _, err := db.RecoverExpiredAnalysisJobs(ctx); err != nil {
			return err
		}
		analyzer := &jev.Analyzer{
			DB: db, Client: w.client,
			PostProcessor: &aipipeline.Processor{DB: db, Config: policy.DefaultConfig(), Logf: log.Printf},
			OnPostError:   func(err error) { log.Printf("ai pipeline: %v", err) },
		}
		runner := &aiworker.Runner{
			DB:         db,
			Classifier: analyzer,
			WorkerID:   fmt.Sprintf("gofer-managed-%d-%s", os.Getpid(), owner),
			Lease:      2 * time.Minute,
			Events:     w.events,
		}
		for i := 0; i < 100; i++ {
			worked, err := runner.ProcessOnce(ctx)
			if err != nil {
				return err
			}
			if !worked {
				return nil
			}
		}
		return nil
	})
}

func isMissingUserStore(err error) bool {
	return err != nil && os.IsNotExist(err)
}

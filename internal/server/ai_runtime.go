package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/aievents"
	"github.com/cristianadrielbraun/gofer/internal/aipipeline"
	"github.com/cristianadrielbraun/gofer/internal/airouting"
	"github.com/cristianadrielbraun/gofer/internal/aiworker"
	"github.com/cristianadrielbraun/gofer/internal/config"
	"github.com/cristianadrielbraun/gofer/internal/jev"
	"github.com/cristianadrielbraun/gofer/internal/llm"
	"github.com/cristianadrielbraun/gofer/internal/policy"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func configuredJevClient() (*jev.Client, string, error) {
	apiKey := strings.TrimSpace(os.Getenv("GOFER_JEV_API_KEY"))
	if apiKey == "" {
		return nil, "", nil
	}
	endpoint := strings.TrimSpace(os.Getenv("GOFER_JEV_URL"))
	if endpoint == "" {
		endpoint = jev.DefaultEndpoint
	}
	client, err := jev.NewClient(jev.Config{
		Endpoint: endpoint,
		APIKey:   apiKey,
		Model:    strings.TrimSpace(os.Getenv("GOFER_JEV_MODEL")),
		Timeout:  20 * time.Second,
	})
	if err != nil {
		return nil, endpoint, fmt.Errorf("invalid Jev configuration: %w", err)
	}
	return client, endpoint, nil
}

func configuredLLMClient() (*llm.Client, string, error) {
	baseURL := strings.TrimSpace(os.Getenv("GOFER_LLM_BASE_URL"))
	if baseURL == "" {
		return nil, "", nil
	}
	model := strings.TrimSpace(os.Getenv("GOFER_LLM_MODEL"))
	if model == "" {
		return nil, baseURL, fmt.Errorf("GOFER_LLM_MODEL is required when GOFER_LLM_BASE_URL is set")
	}
	client, err := llm.NewClient(llm.Config{
		BaseURL: baseURL,
		APIKey:  strings.TrimSpace(os.Getenv("GOFER_LLM_API_KEY")),
		Model:   model,
		Timeout: 45 * time.Second,
	})
	if err != nil {
		return nil, baseURL, err
	}
	return client, baseURL, nil
}

func startAIWorker(ctx context.Context, db *storage.DB, events *aievents.Bus, secrets *config.AccountStore) (bool, error) {
	if db != nil {
		dispatcher := &airouting.Dispatcher{
			DB: db, WorkerID: fmt.Sprintf("gofer-routing-%d", os.Getpid()),
			Logf: log.Printf,
		}
		if secrets != nil {
			dispatcher.Resolve = func(_ context.Context, userID, connectorID, encrypted string) (string, error) {
				return secrets.DecryptAIConnectorSecret(userID, connectorID, encrypted)
			}
		}
		go dispatcher.Run(ctx)
	}
	client, endpoint, err := configuredJevClient()
	if err != nil {
		return false, err
	}
	if client == nil {
		log.Printf("boot: AI worker disabled; set GOFER_JEV_API_KEY to enable Jev analysis")
		return false, nil
	}
	analyzer := &jev.Analyzer{
		DB: db, Client: client,
		PostProcessor: &aipipeline.Processor{DB: db, Config: policy.DefaultConfig(), Logf: log.Printf},
		OnPostError:   func(err error) { log.Printf("ai pipeline: %v", err) },
	}
	runner := &aiworker.Runner{
		DB:         db,
		Classifier: analyzer,
		WorkerID:   fmt.Sprintf("gofer-%d", os.Getpid()),
		Lease:      2 * time.Minute,
		Events:     events,
	}
	go func() {
		log.Printf("boot: AI worker started (endpoint %s)", endpoint)
		if err := runner.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("boot: AI worker stopped: %v", err)
		}
	}()
	return true, nil
}

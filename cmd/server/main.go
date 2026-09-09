// cmd/server is the first real process wiring for EnvoyTrade: the
// Dashboard page's HTTP API (internal/httpapi) backed by Postgres
// (internal/store/postgres) and the fan-out engine (internal/engine).
//
// ponytail: every follower is registered with a kite/fake.Broker
// placeholder, not a real Kite adapter — internal/kite has no live
// broker implementation yet (see AGENTS.md). Swap this for a real
// adapter once one exists; nothing else in this file changes.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"

	"envoytrade/internal/engine"
	"envoytrade/internal/httpapi"
	"envoytrade/internal/kite/fake"
	"envoytrade/internal/store/postgres"
	"envoytrade/internal/worker"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "8080"
	}

	pool, err := postgres.NewPool(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	store := postgres.New(pool)
	if err := store.Migrate(ctx); err != nil {
		return err
	}

	workerPool := worker.NewPool()
	defer workerPool.Shutdown()
	if err := registerFollowers(ctx, store, workerPool); err != nil {
		return err
	}

	eng := engine.New(store, workerPool)
	router := httpapi.NewRouter(store, eng)

	log.Printf("listening on :%s", addr)
	return http.ListenAndServe(":"+addr, router)
}

// registerFollowers registers every existing follower account with the
// worker pool so Rebalance can dispatch to it. Real onboarding (PLAN.md's
// separate milestone) would register a follower as soon as it's created
// instead of walking every group at startup.
func registerFollowers(ctx context.Context, store *postgres.Store, pool *worker.Pool) error {
	groups, err := store.Groups(ctx)
	if err != nil {
		return err
	}
	for _, g := range groups {
		detail, err := store.GroupDetail(ctx, g.MasterID)
		if err != nil {
			return err
		}
		for _, f := range detail.Followers {
			pool.Register(ctx, f.AccountID, &fake.Broker{}, store, 16)
		}
	}
	return nil
}

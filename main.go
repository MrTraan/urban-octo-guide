package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	mongocli "github.com/MadBox-Games/backend-engineer-hiring/internal/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func main() {
	addr := getenv("HTTP_ADDR", ":8080")
	mongoURI := getenv("MONGO_URI", "mongodb://localhost:27017")

	connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongocli.Connect(connectCtx, mongoURI)
	if err != nil {
		log.Fatalf("mongo connect: %v", err)
	}
	// Paired with Connect. log.Fatal and signal-based termination both
	// skip defers, so this won't fire in the current shape — it's here
	// to document ownership and to be correct if main ever returns
	// normally (e.g., you add graceful shutdown).
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	}()

	log.Printf("server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, newMux(client)))
}

// newMux builds the HTTP handler tree with the Mongo client wired in.
// Candidate: register your handlers here. Extracting this from main()
// keeps the handler wiring testable — see main_test.go.
func newMux(client *mongo.Client) *http.ServeMux {
	mux := http.NewServeMux()

	// Example handler showing how to thread `client` into a handler.
	// Keep, replace, or delete — it's a reference, not a requirement.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := client.Ping(r.Context(), nil); err != nil {
			http.Error(w, "mongo unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	return mux
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

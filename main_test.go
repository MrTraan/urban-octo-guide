package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/mongo"

	mongocli "github.com/MadBox-Games/backend-engineer-hiring/internal/mongo"
)

// Shared state for integration tests. TestMain sets these up once; each
// test function reads them. Namespace your collections per test (or
// clean up in t.Cleanup) so tests don't see each other's writes.
var (
	testMongo  *mongo.Client
	testServer *httptest.Server
)

// TestMain boots a real MongoDB container via testcontainers-go, connects
// through the Mongo helper, wires the client into newMux, and exposes the
// client and an httptest server as package-level globals for your tests
// to use.
//
// One container is shared across all tests — startup is ~5-15s, so paying
// that per-test would hurt.
func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	mongoC, err := mongodb.Run(ctx, "mongo:7")
	if err != nil {
		log.Fatalf("start mongo container: %v", err)
	}

	uri, err := mongoC.ConnectionString(ctx)
	if err != nil {
		_ = mongoC.Terminate(context.Background())
		log.Fatalf("get connection string: %v", err)
	}

	testMongo, err = mongocli.Connect(ctx, uri)
	if err != nil {
		_ = mongoC.Terminate(context.Background())
		log.Fatalf("connect mongo: %v", err)
	}

	testServer = httptest.NewServer(newMux(testMongo))

	// os.Exit skips deferred cleanup, so tear down explicitly here.
	code := m.Run()
	testServer.Close()
	_ = testMongo.Disconnect(context.Background())
	_ = mongoC.Terminate(context.Background())
	os.Exit(code)
}

// TestHealthz is a minimal example test showing how to use testServer.
// Replace or delete it as you add tests for your own handlers.
func TestHealthz(t *testing.T) {
	resp, err := http.Get(testServer.URL + "/healthz")
	if err != nil {
		t.Fatalf("get healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

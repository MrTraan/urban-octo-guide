// Command server runs the Player Profile gRPC service.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	playerv1 "github.com/MadBox-Games/backend-engineer-hiring/gen/player/v1"
	mongocli "github.com/MadBox-Games/backend-engineer-hiring/internal/mongo"
	"github.com/MadBox-Games/backend-engineer-hiring/internal/player"
)

func main() {
	addr := getenv("GRPC_ADDR", ":8080")
	mongoURI := getenv("MONGO_URI", "mongodb://localhost:27017")
	mongoDB := getenv("MONGO_DB", "player")

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	if err := run(addr, mongoURI, mongoDB, log); err != nil {
		log.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func run(addr, mongoURI, mongoDB string, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client, err := mongocli.Connect(connectCtx, mongoURI)
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	}()

	store, err := player.NewStore(connectCtx, client.Database(mongoDB))
	if err != nil {
		return err
	}

	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(logging(log)))
	playerv1.RegisterPlayerServiceServer(srv, player.NewService(store))
	reflection.Register(srv) // lets grpcurl discover the API

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(srv, healthSrv)
	go watchMongo(ctx, client, healthSrv)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	// Serve until a signal arrives, then let in-flight RPCs finish.
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(lis) }()
	log.Info("server listening", "addr", addr, "mongo_db", mongoDB)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		srv.GracefulStop()
		return nil
	}
}

// watchMongo keeps the gRPC health status in sync with Mongo reachability.
func watchMongo(ctx context.Context, client *mongo.Client, h *health.Server) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		st := healthpb.HealthCheckResponse_SERVING
		if err := client.Ping(ctx, nil); err != nil {
			st = healthpb.HealthCheckResponse_NOT_SERVING
		}
		h.SetServingStatus("", st)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// logging is a unary interceptor that logs one line per RPC.
func logging(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := next(ctx, req)
		log.Info("rpc",
			"method", info.FullMethod,
			"code", status.Code(err).String(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return resp, err
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

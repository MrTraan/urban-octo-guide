// Command loadgen sends a realistic mix of traffic to the Player Profile
// service so the Grafana dashboard has something meaningful to show.
//
//	go run ./cmd/loadgen                       # 200 req/s for 60s
//	go run ./cmd/loadgen -rps 1000 -duration 5m
//
// -rps sets the total request rate, -workers the concurrency. Ctrl-C stops early.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	playerv1 "github.com/MrTraan/urban-octo-guide/gen/player/v1"
)

var (
	eventTypes = []string{"login", "level_start", "level_complete", "level_failed", "purchase", "ad_watched"}
	countries  = []string{"FR", "US", "DE", "JP", "BR", "GB"}
)

func main() {
	addr := flag.String("addr", envOr("PLAYER_ADDR", "localhost:8080"), "service address")
	rps := flag.Int("rps", 200, "target requests per second (total)")
	workers := flag.Int("workers", 20, "concurrent workers")
	players := flag.Int("players", 500, "size of the simulated player base")
	duration := flag.Duration("duration", 60*time.Second, "how long to run")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *duration)
	defer cancel()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	defer conn.Close()
	client := playerv1.NewPlayerServiceClient(conn)

	fmt.Printf("warming up %d profiles...\n", *players)
	warmUp(ctx, client, *players)

	fmt.Printf("sending ~%d req/s with %d workers for %s (Ctrl-C to stop)\n", *rps, *workers, *duration)
	var stats stats
	ticks := time.NewTicker(time.Second / time.Duration(*rps))
	defer ticks.Stop()

	var wg sync.WaitGroup
	for range *workers {
		wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticks.C: // one tick = one request, shared across workers
					stats.record(oneRequest(ctx, client, *players))
				}
			}
		})
	}
	go stats.report(ctx, 5*time.Second)
	wg.Wait()
	stats.summary()
}

// warmUp makes sure every simulated player has a profile, 50 at a time.
func warmUp(ctx context.Context, client playerv1.PlayerServiceClient, players int) {
	sem := make(chan struct{}, 50)
	var wg sync.WaitGroup
	for i := range players {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			_, _ = client.CreateProfile(ctx, &playerv1.CreateProfileRequest{
				PlayerId:    playerID(i),
				DisplayName: fmt.Sprintf("Player %d", i),
				Country:     pick(countries),
			})
		})
	}
	wg.Wait()
}

// oneRequest performs a single randomly chosen call, weighted like real
// client traffic. Most calls are events and session starts; a few are
// deliberate errors so the error panels are not flat.
func oneRequest(ctx context.Context, client playerv1.PlayerServiceClient, players int) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	id := playerID(rand.IntN(players))

	var err error
	switch r := rand.IntN(100); {
	case r < 45:
		payload, _ := structpb.NewStruct(map[string]any{"level": rand.IntN(50), "score": rand.IntN(10000)})
		_, err = client.RecordEvent(ctx, &playerv1.RecordEventRequest{PlayerId: id, Type: pick(eventTypes), Payload: payload})
	case r < 70:
		_, err = client.GetSessionSummary(ctx, &playerv1.GetSessionSummaryRequest{PlayerId: id})
	case r < 85:
		_, err = client.ListRecentEvents(ctx, &playerv1.ListRecentEventsRequest{PlayerId: id, Limit: 10})
	case r < 93:
		_, err = client.GetProfile(ctx, &playerv1.GetProfileRequest{PlayerId: id})
	case r < 97:
		_, err = client.UpdateProfile(ctx, &playerv1.UpdateProfileRequest{PlayerId: id, Country: proto.String(pick(countries))})
	case r < 99:
		_, err = client.GetProfile(ctx, &playerv1.GetProfileRequest{PlayerId: "ghost"}) // NotFound
	default:
		_, err = client.CreateProfile(ctx, &playerv1.CreateProfileRequest{PlayerId: id, DisplayName: "x", Country: "France"}) // InvalidArgument
	}
	return err
}

// stats counts requests and errors and prints a line every few seconds.
type stats struct {
	total, errors atomic.Int64
	start         time.Time
}

func (s *stats) record(err error) {
	s.total.Add(1)
	if err != nil {
		s.errors.Add(1)
	}
}

func (s *stats) report(ctx context.Context, every time.Duration) {
	s.start = time.Now()
	t := time.NewTicker(every)
	defer t.Stop()
	var last int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n := s.total.Load()
			fmt.Printf("%6.0fs  %8d req  %6d err  %6.0f req/s\n",
				time.Since(s.start).Seconds(), n, s.errors.Load(), float64(n-last)/every.Seconds())
			last = n
		}
	}
}

func (s *stats) summary() {
	elapsed := time.Since(s.start)
	n := s.total.Load()
	fmt.Printf("done: %d requests, %d errors, %.0f req/s over %s\n",
		n, s.errors.Load(), float64(n)/elapsed.Seconds(), elapsed.Round(time.Second))
}

func playerID(i int) string { return fmt.Sprintf("load-%04d", i) }

func pick(list []string) string { return list[rand.IntN(len(list))] }

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

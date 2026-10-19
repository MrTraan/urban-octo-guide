package player

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	playerv1 "github.com/MadBox-Games/backend-engineer-hiring/gen/player/v1"
	mongocli "github.com/MadBox-Games/backend-engineer-hiring/internal/mongo"
)

// client talks to a real Service backed by a real Mongo (testcontainers),
// over an in-memory gRPC connection. Set up once in TestMain.
var client playerv1.PlayerServiceClient

func TestMain(m *testing.M) {
	// -short skips the Docker-backed suite; validate_test.go still runs.
	flag.Parse()
	if testing.Short() {
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	mongoC, err := mongodb.Run(ctx, "mongo:7")
	if err != nil {
		log.Fatalf("start mongo container: %v", err)
	}
	uri, err := mongoC.ConnectionString(ctx)
	if err != nil {
		log.Fatalf("connection string: %v", err)
	}
	mc, err := mongocli.Connect(ctx, uri)
	if err != nil {
		log.Fatalf("connect mongo: %v", err)
	}
	store, err := NewStore(ctx, mc.Database("player_test"))
	if err != nil {
		log.Fatalf("new store: %v", err)
	}

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	playerv1.RegisterPlayerServiceServer(srv, NewService(store))
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	client = playerv1.NewPlayerServiceClient(conn)

	code := m.Run()
	_ = conn.Close()
	srv.Stop()
	_ = mc.Disconnect(context.Background())
	_ = mongoC.Terminate(context.Background())
	os.Exit(code)
}

func skipIfShort(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test needs Docker")
	}
}

func TestCreateProfileIsIdempotent(t *testing.T) {
	skipIfShort(t)
	ctx := context.Background()
	id := t.Name()

	first, err := client.CreateProfile(ctx, &playerv1.CreateProfileRequest{PlayerId: id, DisplayName: "Nathan", Country: "FR"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	second, err := client.CreateProfile(ctx, &playerv1.CreateProfileRequest{PlayerId: id, DisplayName: "Someone Else", Country: "US"})
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if !proto.Equal(first, second) {
		t.Errorf("second create changed the profile:\n first=%v\nsecond=%v", first, second)
	}
	if first.GetDisplayName() != "Nathan" || first.GetCountry() != "FR" {
		t.Errorf("unexpected profile: %v", first)
	}
}

func TestGetProfileNotFound(t *testing.T) {
	skipIfShort(t)
	_, err := client.GetProfile(context.Background(), &playerv1.GetProfileRequest{PlayerId: "nobody"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestUpdateProfilePartial(t *testing.T) {
	skipIfShort(t)
	ctx := context.Background()
	id := t.Name()
	if _, err := client.CreateProfile(ctx, &playerv1.CreateProfileRequest{PlayerId: id, DisplayName: "Nathan", Country: "FR"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := client.UpdateProfile(ctx, &playerv1.UpdateProfileRequest{PlayerId: id, Country: proto.String("DE")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.GetCountry() != "DE" || got.GetDisplayName() != "Nathan" {
		t.Errorf("partial update wrong: %v", got)
	}
	if got.GetUpdatedAt().AsTime().Before(got.GetCreatedAt().AsTime()) {
		t.Errorf("updated_at should not be before created_at: %v", got)
	}

	_, err = client.UpdateProfile(ctx, &playerv1.UpdateProfileRequest{PlayerId: id})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("empty update: want InvalidArgument, got %v", err)
	}
	_, err = client.UpdateProfile(ctx, &playerv1.UpdateProfileRequest{PlayerId: "nobody", Country: proto.String("DE")})
	if status.Code(err) != codes.NotFound {
		t.Errorf("unknown player: want NotFound, got %v", err)
	}
}

func TestValidationErrors(t *testing.T) {
	skipIfShort(t)
	ctx := context.Background()
	_, err := client.CreateProfile(ctx, &playerv1.CreateProfileRequest{PlayerId: "p", DisplayName: "x", Country: "France"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("bad country: want InvalidArgument, got %v", err)
	}
	_, err = client.RecordEvent(ctx, &playerv1.RecordEventRequest{PlayerId: "", Type: "x"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("missing player: want InvalidArgument, got %v", err)
	}
}

func TestRecordAndListEvents(t *testing.T) {
	skipIfShort(t)
	ctx := context.Background()
	id := t.Name()
	base := time.Now().UTC().Truncate(time.Millisecond)

	payload, _ := structpb.NewStruct(map[string]any{"level": 3, "nested": map[string]any{"ok": true}})
	for i := 0; i < 3; i++ {
		_, err := client.RecordEvent(ctx, &playerv1.RecordEventRequest{
			PlayerId:  id,
			Type:      "level_complete",
			Timestamp: timestamppb.New(base.Add(time.Duration(i) * time.Minute)),
			Payload:   payload,
		})
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	resp, err := client.ListRecentEvents(ctx, &playerv1.ListRecentEventsRequest{PlayerId: id, Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.GetEvents()) != 2 {
		t.Fatalf("want 2 events (limit), got %d", len(resp.GetEvents()))
	}
	newest := resp.GetEvents()[0]
	if !newest.GetTimestamp().AsTime().Equal(base.Add(2 * time.Minute)) {
		t.Errorf("events should be newest first, got %v", newest.GetTimestamp().AsTime())
	}
	if !proto.Equal(newest.GetPayload(), payload) {
		t.Errorf("payload did not round-trip:\n got=%v\nwant=%v", newest.GetPayload(), payload)
	}
	if newest.GetId() == "" {
		t.Error("event id should be set")
	}
}

func TestSessionSummary(t *testing.T) {
	skipIfShort(t)
	ctx := context.Background()
	id := t.Name()
	now := time.Now().UTC()

	if _, err := client.CreateProfile(ctx, &playerv1.CreateProfileRequest{PlayerId: id, DisplayName: "Nathan", Country: "FR"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	record := func(typ string, at time.Time) {
		t.Helper()
		if _, err := client.RecordEvent(ctx, &playerv1.RecordEventRequest{PlayerId: id, Type: typ, Timestamp: timestamppb.New(at)}); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	record("level_complete", now.Add(-1*time.Hour))
	record("level_complete", now.Add(-2*time.Hour))
	record("purchase", now.Add(-3*time.Hour))
	record("purchase", now.AddDate(0, 0, -8)) // outside the 7-day window

	sum, err := client.GetSessionSummary(ctx, &playerv1.GetSessionSummaryRequest{PlayerId: id, RecentLimit: 3})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if sum.GetProfile().GetDisplayName() != "Nathan" {
		t.Errorf("profile missing: %v", sum.GetProfile())
	}
	if len(sum.GetRecentEvents()) != 3 {
		t.Errorf("want 3 recent events, got %d", len(sum.GetRecentEvents()))
	}
	want := map[string]int64{"level_complete": 2, "purchase": 1}
	got := map[string]int64{}
	for _, c := range sum.GetCounts() {
		got[c.GetType()] = c.GetCount()
	}
	if len(got) != len(want) || got["level_complete"] != want["level_complete"] || got["purchase"] != want["purchase"] {
		t.Errorf("counts: got %v, want %v", got, want)
	}

	_, err = client.GetSessionSummary(ctx, &playerv1.GetSessionSummaryRequest{PlayerId: "nobody"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("unknown player: want NotFound, got %v", err)
	}
}

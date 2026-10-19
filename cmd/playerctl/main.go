// Command playerctl is a small CLI to poke the Player Profile service.
//
//	playerctl create  -player p1 -name Nathan -country FR
//	playerctl get     -player p1
//	playerctl update  -player p1 -name Nate [-country DE]
//	playerctl record  -player p1 -type level_complete [-payload '{"level":3}']
//	playerctl events  -player p1 [-limit 5]
//	playerctl summary -player p1 [-limit 5] [-days 7]
//
// Server address comes from PLAYER_ADDR (default localhost:8080).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	playerv1 "github.com/MadBox-Games/backend-engineer-hiring/gen/player/v1"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd, args := os.Args[1], os.Args[2:]

	addr := os.Getenv("PLAYER_ADDR")
	if addr == "" {
		addr = "localhost:8080"
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fail(err)
	}
	defer conn.Close()
	client := playerv1.NewPlayerServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	player := fs.String("player", "", "player id (required)")
	name := fs.String("name", "", "display name")
	country := fs.String("country", "", "ISO 3166-1 alpha-2 country, e.g. FR")
	typ := fs.String("type", "", "event type")
	payload := fs.String("payload", "", "event payload as a JSON object")
	limit := fs.Int("limit", 0, "max events to return (0 = server default)")
	days := fs.Int("days", 0, "summary window in days (0 = server default)")
	_ = fs.Parse(args)

	var resp proto.Message
	switch cmd {
	case "create":
		resp, err = client.CreateProfile(ctx, &playerv1.CreateProfileRequest{PlayerId: *player, DisplayName: *name, Country: *country})
	case "get":
		resp, err = client.GetProfile(ctx, &playerv1.GetProfileRequest{PlayerId: *player})
	case "update":
		req := &playerv1.UpdateProfileRequest{PlayerId: *player}
		if *name != "" {
			req.DisplayName = name
		}
		if *country != "" {
			req.Country = country
		}
		resp, err = client.UpdateProfile(ctx, req)
	case "record":
		var st *structpb.Struct
		if *payload != "" {
			st = &structpb.Struct{}
			if err := protojson.Unmarshal([]byte(*payload), st); err != nil {
				fail(fmt.Errorf("payload: %w", err))
			}
		}
		resp, err = client.RecordEvent(ctx, &playerv1.RecordEventRequest{PlayerId: *player, Type: *typ, Payload: st})
	case "events":
		resp, err = client.ListRecentEvents(ctx, &playerv1.ListRecentEventsRequest{PlayerId: *player, Limit: int32(*limit)})
	case "summary":
		resp, err = client.GetSessionSummary(ctx, &playerv1.GetSessionSummaryRequest{PlayerId: *player, RecentLimit: int32(*limit), WindowDays: int32(*days)})
	default:
		usage()
	}
	if err != nil {
		fail(err)
	}

	out, _ := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(resp)
	fmt.Println(string(out))
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: playerctl <create|get|update|record|events|summary> [flags]")
	fmt.Fprintln(os.Stderr, "       PLAYER_ADDR=host:port (default localhost:8080)")
	os.Exit(2)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

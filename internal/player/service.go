package player

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	playerv1 "github.com/MadBox-Games/backend-engineer-hiring/gen/player/v1"
)

// Service implements the PlayerService gRPC API on top of Store.
// Each handler follows the same shape: validate -> store -> convert to proto.
type Service struct {
	playerv1.UnimplementedPlayerServiceServer
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) CreateProfile(ctx context.Context, req *playerv1.CreateProfileRequest) (*playerv1.Profile, error) {
	if err := errors.Join(
		validatePlayerID(req.GetPlayerId()),
		validateDisplayName(req.GetDisplayName()),
		validateCountry(req.GetCountry()),
	); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	now := time.Now().UTC()
	doc, err := s.store.CreateProfile(ctx, profileDoc{
		PlayerID:    req.GetPlayerId(),
		DisplayName: req.GetDisplayName(),
		Country:     req.GetCountry(),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return toProfile(doc), nil
}

func (s *Service) GetProfile(ctx context.Context, req *playerv1.GetProfileRequest) (*playerv1.Profile, error) {
	if err := validatePlayerID(req.GetPlayerId()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	doc, err := s.store.GetProfile(ctx, req.GetPlayerId())
	if err != nil {
		return nil, toStatus(err)
	}
	return toProfile(doc), nil
}

func (s *Service) UpdateProfile(ctx context.Context, req *playerv1.UpdateProfileRequest) (*playerv1.Profile, error) {
	if err := validatePlayerID(req.GetPlayerId()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Only fields present in the request are changed.
	fields := bson.M{}
	if req.DisplayName != nil {
		if err := validateDisplayName(req.GetDisplayName()); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		fields["display_name"] = req.GetDisplayName()
	}
	if req.Country != nil {
		if err := validateCountry(req.GetCountry()); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		fields["country"] = req.GetCountry()
	}
	if len(fields) == 0 {
		return nil, status.Error(codes.InvalidArgument, "nothing to update")
	}

	doc, err := s.store.UpdateProfile(ctx, req.GetPlayerId(), fields)
	if err != nil {
		return nil, toStatus(err)
	}
	return toProfile(doc), nil
}

func (s *Service) RecordEvent(ctx context.Context, req *playerv1.RecordEventRequest) (*playerv1.Event, error) {
	if err := errors.Join(
		validatePlayerID(req.GetPlayerId()),
		validateEventType(req.GetType()),
	); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	ts := time.Now().UTC()
	if req.Timestamp != nil {
		ts = req.GetTimestamp().AsTime()
	}
	doc, err := s.store.InsertEvent(ctx, eventDoc{
		PlayerID:  req.GetPlayerId(),
		Type:      req.GetType(),
		Timestamp: ts,
		Payload:   req.GetPayload().AsMap(), // nil payload -> empty map
	})
	if err != nil {
		return nil, toStatus(err)
	}
	eventsRecorded.WithLabelValues(doc.Type).Inc()
	return toEvent(doc)
}

func (s *Service) ListRecentEvents(ctx context.Context, req *playerv1.ListRecentEventsRequest) (*playerv1.ListRecentEventsResponse, error) {
	if err := validatePlayerID(req.GetPlayerId()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	docs, err := s.store.ListRecentEvents(ctx, req.GetPlayerId(), clamp(req.GetLimit(), defaultLimit, maxLimit))
	if err != nil {
		return nil, toStatus(err)
	}
	events, err := toEvents(docs)
	if err != nil {
		return nil, err
	}
	return &playerv1.ListRecentEventsResponse{Events: events}, nil
}

// GetSessionSummary runs the three independent reads concurrently.
func (s *Service) GetSessionSummary(ctx context.Context, req *playerv1.GetSessionSummaryRequest) (*playerv1.GetSessionSummaryResponse, error) {
	if err := validatePlayerID(req.GetPlayerId()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	id := req.GetPlayerId()
	limit := clamp(req.GetRecentLimit(), defaultLimit, maxLimit)
	since := time.Now().UTC().AddDate(0, 0, -clamp(req.GetWindowDays(), defaultWindowDays, maxWindowDays))

	var (
		profile profileDoc
		recent  []eventDoc
		counts  []eventCount
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { profile, err = s.store.GetProfile(gctx, id); return err })
	g.Go(func() (err error) { recent, err = s.store.ListRecentEvents(gctx, id, limit); return err })
	g.Go(func() (err error) { counts, err = s.store.CountEventsByType(gctx, id, since); return err })
	if err := g.Wait(); err != nil {
		return nil, toStatus(err)
	}

	events, err := toEvents(recent)
	if err != nil {
		return nil, err
	}
	resp := &playerv1.GetSessionSummaryResponse{
		Profile:      toProfile(profile),
		RecentEvents: events,
	}
	for _, c := range counts {
		resp.Counts = append(resp.Counts, &playerv1.EventCount{Type: c.Type, Count: c.Count})
	}
	return resp, nil
}

// --- helpers ---------------------------------------------------------------

// toStatus maps store errors to gRPC status codes.
func toStatus(err error) error {
	if errors.Is(err, ErrNotFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}

func toProfile(d profileDoc) *playerv1.Profile {
	return &playerv1.Profile{
		PlayerId:    d.PlayerID,
		DisplayName: d.DisplayName,
		Country:     d.Country,
		CreatedAt:   timestamppb.New(d.CreatedAt),
		UpdatedAt:   timestamppb.New(d.UpdatedAt),
	}
}

func toEvent(d eventDoc) (*playerv1.Event, error) {
	payload, err := structpb.NewStruct(plain(d.Payload).(map[string]any))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "event %s payload: %v", d.ID.Hex(), err)
	}
	return &playerv1.Event{
		Id:        d.ID.Hex(),
		PlayerId:  d.PlayerID,
		Type:      d.Type,
		Timestamp: timestamppb.New(d.Timestamp),
		Payload:   payload,
	}, nil
}

// plain converts Mongo's named container types (bson.M, bson.A) back into the
// plain map[string]any / []any that structpb understands. Scalars pass through.
func plain(v any) any {
	switch t := v.(type) {
	case bson.M:
		return plain(map[string]any(t))
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = plain(val)
		}
		return out
	case bson.A:
		return plain([]any(t))
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = plain(val)
		}
		return out
	default:
		return v
	}
}

func toEvents(docs []eventDoc) ([]*playerv1.Event, error) {
	out := make([]*playerv1.Event, 0, len(docs))
	for _, d := range docs {
		e, err := toEvent(d)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

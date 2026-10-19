package player

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ErrNotFound is returned when a player has no profile.
var ErrNotFound = errors.New("profile not found")

// profileDoc is the Mongo document in the "profiles" collection.
// The player id is the document _id, so lookups need no extra index.
type profileDoc struct {
	PlayerID    string    `bson:"_id"`
	DisplayName string    `bson:"display_name"`
	Country     string    `bson:"country"`
	CreatedAt   time.Time `bson:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at"`
}

// eventDoc is the Mongo document in the "events" collection.
type eventDoc struct {
	ID        bson.ObjectID  `bson:"_id,omitempty"`
	PlayerID  string         `bson:"player_id"`
	Type      string         `bson:"type"`
	Timestamp time.Time      `bson:"timestamp"`
	Payload   map[string]any `bson:"payload"`
}

// eventCount is one row of the "events by type" aggregation.
type eventCount struct {
	Type  string `bson:"_id"`
	Count int64  `bson:"count"`
}

// Store is the Mongo persistence layer for profiles and events.
type Store struct {
	profiles *mongo.Collection
	events   *mongo.Collection
}

// NewStore wires the collections and ensures the one index we rely on.
func NewStore(ctx context.Context, db *mongo.Database) (*Store, error) {
	s := &Store{
		profiles: db.Collection("profiles"),
		events:   db.Collection("events"),
	}
	// Every event query is "this player, newest first".
	_, err := s.events.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "player_id", Value: 1}, {Key: "timestamp", Value: -1}},
	})
	if err != nil {
		return nil, fmt.Errorf("create events index: %w", err)
	}
	return s, nil
}

// CreateProfile inserts the profile if missing and returns the stored one.
// Calling it twice is safe: the second call returns the existing profile unchanged.
func (s *Store) CreateProfile(ctx context.Context, p profileDoc) (profileDoc, error) {
	filter := bson.M{"_id": p.PlayerID}
	update := bson.M{"$setOnInsert": p}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)

	var out profileDoc
	if err := s.profiles.FindOneAndUpdate(ctx, filter, update, opts).Decode(&out); err != nil {
		return profileDoc{}, fmt.Errorf("upsert profile: %w", err)
	}
	return out, nil
}

// GetProfile returns ErrNotFound when the player has no profile.
func (s *Store) GetProfile(ctx context.Context, playerID string) (profileDoc, error) {
	var out profileDoc
	err := s.profiles.FindOne(ctx, bson.M{"_id": playerID}).Decode(&out)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return profileDoc{}, ErrNotFound
	}
	if err != nil {
		return profileDoc{}, fmt.Errorf("find profile: %w", err)
	}
	return out, nil
}

// UpdateProfile applies the given fields ($set) and bumps updated_at.
// fields is a map of bson field name -> new value; only those are changed.
func (s *Store) UpdateProfile(ctx context.Context, playerID string, fields bson.M) (profileDoc, error) {
	fields["updated_at"] = time.Now().UTC()
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	var out profileDoc
	err := s.profiles.FindOneAndUpdate(ctx, bson.M{"_id": playerID}, bson.M{"$set": fields}, opts).Decode(&out)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return profileDoc{}, ErrNotFound
	}
	if err != nil {
		return profileDoc{}, fmt.Errorf("update profile: %w", err)
	}
	return out, nil
}

// InsertEvent stores one event and returns it with its generated id.
func (s *Store) InsertEvent(ctx context.Context, e eventDoc) (eventDoc, error) {
	e.ID = bson.NewObjectID()
	if _, err := s.events.InsertOne(ctx, e); err != nil {
		return eventDoc{}, fmt.Errorf("insert event: %w", err)
	}
	return e, nil
}

// ListRecentEvents returns up to limit events for the player, newest first.
func (s *Store) ListRecentEvents(ctx context.Context, playerID string, limit int) ([]eventDoc, error) {
	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}}).SetLimit(int64(limit))
	cur, err := s.events.Find(ctx, bson.M{"player_id": playerID}, opts)
	if err != nil {
		return nil, fmt.Errorf("find events: %w", err)
	}
	var out []eventDoc
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("decode events: %w", err)
	}
	return out, nil
}

// CountEventsByType counts the player's events since the given time, grouped by type.
func (s *Store) CountEventsByType(ctx context.Context, playerID string, since time.Time) ([]eventCount, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"player_id": playerID, "timestamp": bson.M{"$gte": since}}}},
		{{Key: "$group", Value: bson.M{"_id": "$type", "count": bson.M{"$sum": 1}}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
	}
	cur, err := s.events.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate events: %w", err)
	}
	var out []eventCount
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("decode counts: %w", err)
	}
	return out, nil
}

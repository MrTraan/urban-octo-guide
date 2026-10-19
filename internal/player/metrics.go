package player

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// eventsRecorded counts successful RecordEvent calls by event type.
var eventsRecorded = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "player_events_recorded_total",
	Help: "Gameplay events recorded, by type.",
}, []string{"type"})

// RegisterMetrics exposes the app-level metrics on reg: the events counter and
// two gauges with the current number of profiles and events in Mongo. The gauges
// use EstimatedDocumentCount (collection metadata), so scraping them is free.
func RegisterMetrics(reg prometheus.Registerer, store *Store) {
	reg.MustRegister(eventsRecorded)
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "player_profiles_total",
		Help: "Number of player profiles stored.",
	}, countOf(store.profiles.EstimatedDocumentCount)))
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "player_events_total",
		Help: "Number of gameplay events stored.",
	}, countOf(store.events.EstimatedDocumentCount)))
}

// countOf adapts a Mongo count method to the float64 getter a GaugeFunc expects.
// A failed count reports -1 so dashboards can tell "Mongo unreachable" from "empty".
func countOf[T any](count func(context.Context, ...T) (int64, error)) func() float64 {
	return func() float64 {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		n, err := count(ctx)
		if err != nil {
			return -1
		}
		return float64(n)
	}
}

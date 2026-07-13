package metrics_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/metrics"
	"silent-devops/internal/store"
)

func TestPersistCurrentAndMinuteAggregate(t *testing.T) {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB().Exec("INSERT INTO agents(id,created_unix_ms) VALUES('a',?)", time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	repo := metrics.NewRepository(s.DB())
	base := time.Unix(120, 0)
	for i, v := range []float64{10, 20} {
		snapshot := &devopsv1.MetricsSnapshot{SampledUnixMs: base.Add(time.Duration(i) * 15 * time.Second).UnixMilli(), Metrics: []*devopsv1.Metric{{Name: "cpu", Value: v}}}
		if err := repo.Store(context.Background(), "a", snapshot); err != nil {
			t.Fatal(err)
		}
	}
	current, err := repo.Current(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if current.Metrics[0].Value != 20 {
		t.Fatalf("current=%v", current.Metrics[0].Value)
	}
	var count int
	if err := s.DB().QueryRow("SELECT count(*) FROM metrics_minute WHERE agent_id='a'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("minute rows=%d", count)
	}
}
func TestAggregateCardinalityBound(t *testing.T) {
	snapshot := &devopsv1.MetricsSnapshot{}
	for i := 0; i < 5; i++ {
		snapshot.Metrics = append(snapshot.Metrics, &devopsv1.Metric{Name: string(rune('a' + i)), Value: 1})
	}
	got := metrics.Bound(snapshot, 3, 2)
	if len(got.Metrics) != 3 {
		t.Fatalf("metrics=%d", len(got.Metrics))
	}
}

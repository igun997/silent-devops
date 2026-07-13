package metrics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"google.golang.org/protobuf/encoding/protojson"
	devopsv1 "silent-devops/api/devops/v1"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Store(ctx context.Context, agentID string, snapshot *devopsv1.MetricsSnapshot) error {
	if agentID == "" || snapshot == nil {
		return errors.New("agent and snapshot required")
	}
	payload, err := protojson.Marshal(snapshot)
	if err != nil {
		return err
	}
	bucket := snapshot.SampledUnixMs - snapshot.SampledUnixMs%60000
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO metrics_current(agent_id,sampled_unix_ms,payload) VALUES(?,?,?) ON CONFLICT(agent_id) DO UPDATE SET sampled_unix_ms=excluded.sampled_unix_ms,payload=excluded.payload WHERE excluded.sampled_unix_ms>=metrics_current.sampled_unix_ms", agentID, snapshot.SampledUnixMs, payload); err != nil {
		return err
	}
	var existing []byte
	err = tx.QueryRowContext(ctx, "SELECT payload FROM metrics_minute WHERE agent_id=? AND bucket_unix_ms=?", agentID, bucket).Scan(&existing)
	aggregate := snapshot
	if err == nil {
		previous := &devopsv1.MetricsSnapshot{}
		if err := protojson.Unmarshal(existing, previous); err != nil {
			return err
		}
		aggregate = merge(previous, snapshot)
	} else if err != sql.ErrNoRows {
		return err
	}
	minute, err := protojson.Marshal(aggregate)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO metrics_minute(agent_id,bucket_unix_ms,payload) VALUES(?,?,?) ON CONFLICT(agent_id,bucket_unix_ms) DO UPDATE SET payload=excluded.payload", agentID, bucket, minute); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) Current(ctx context.Context, agentID string) (*devopsv1.MetricsSnapshot, error) {
	var payload []byte
	if err := r.db.QueryRowContext(ctx, "SELECT payload FROM metrics_current WHERE agent_id=?", agentID).Scan(&payload); err != nil {
		return nil, err
	}
	out := &devopsv1.MetricsSnapshot{}
	return out, protojson.Unmarshal(payload, out)
}
func merge(a, b *devopsv1.MetricsSnapshot) *devopsv1.MetricsSnapshot {
	values := map[string][]float64{}
	labels := map[string]map[string]string{}
	for _, snapshot := range []*devopsv1.MetricsSnapshot{a, b} {
		for _, metric := range snapshot.Metrics {
			key := metric.Name + labelKey(metric.Labels)
			values[key] = append(values[key], metric.Value)
			labels[key] = metric.Labels
		}
	}
	out := &devopsv1.MetricsSnapshot{SampledUnixMs: b.SampledUnixMs}
	for key, all := range values {
		sum := 0.0
		for _, v := range all {
			sum += v
		}
		name := key
		if i := indexByte(name, '{'); i >= 0 {
			name = name[:i]
		}
		out.Metrics = append(out.Metrics, &devopsv1.Metric{Name: name, Value: sum / float64(len(all)), Labels: labels[key]})
	}
	return out
}
func labelKey(labels map[string]string) string { data, _ := json.Marshal(labels); return string(data) }
func indexByte(s string, b byte) int {
	for i := range len(s) {
		if s[i] == b {
			return i
		}
	}
	return -1
}
func Bound(snapshot *devopsv1.MetricsSnapshot, maxMetrics, maxLabels int) *devopsv1.MetricsSnapshot {
	out := &devopsv1.MetricsSnapshot{SampledUnixMs: snapshot.SampledUnixMs}
	for _, metric := range snapshot.Metrics {
		if len(out.Metrics) >= maxMetrics {
			break
		}
		copy := &devopsv1.Metric{Name: metric.Name, Value: metric.Value, Labels: map[string]string{}}
		count := 0
		for k, v := range metric.Labels {
			if count >= maxLabels {
				break
			}
			copy.Labels[k] = v
			count++
		}
		out.Metrics = append(out.Metrics, copy)
	}
	return out
}

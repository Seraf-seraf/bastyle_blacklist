package observability

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
)

func TestStartMetricsServerWritesOutboxAndIndexMetrics(t *testing.T) {
	address := freeAddress(t)

	server, err := StartMetricsServer(config.Metrics{
		Enabled: true,
		Host:    strings.Split(address, ":")[0],
		Port:    mustPort(t, address),
		Path:    "/metrics",
	}, MetricsDependencies{
		Outbox: &fakeOutboxStore{stats: ports.OutboxStats{
			UnpublishedCount:  3,
			UnpublishedMaxAge: 2 * time.Minute,
		}},
		Checkpoints: &fakeMetricsCheckpointStore{stats: []ports.IndexCheckpointStat{
			{ConsumerID: "replica-a", IndexName: ports.IndexExact, Stale: true},
		}},
		ConsumerID: "replica-a",
		IndexNames: []string{ports.IndexExact},
		ReplicaID:  "replica-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	response, err := http.Get("http://" + address + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, expected := range []string{
		"bastyle_outbox_unpublished_total 3",
		"bastyle_outbox_unpublished_max_age_seconds 120",
		"bastyle_index_stale",
		"consumer_id=\"replica-a\"",
		"index_name=\"exact\"",
		"bastyle_build_info",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics не содержат %q: %s", expected, body)
		}
	}
}

func mustPort(t *testing.T, address string) int {
	t.Helper()
	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

type fakeOutboxStore struct {
	stats ports.OutboxStats
}

func (s *fakeOutboxStore) Stats(context.Context) (ports.OutboxStats, error) {
	return s.stats, nil
}

type fakeMetricsCheckpointStore struct {
	stats []ports.IndexCheckpointStat
}

func (s *fakeMetricsCheckpointStore) GetOrCreate(context.Context, string, string) (ports.IndexCheckpoint, error) {
	return ports.IndexCheckpoint{}, nil
}

func (s *fakeMetricsCheckpointStore) Update(context.Context, string, string, string, int64) error {
	return nil
}

func (s *fakeMetricsCheckpointStore) MarkStale(context.Context, string, string, string) error {
	return nil
}

func (s *fakeMetricsCheckpointStore) CheckFresh(context.Context, string, []string) error {
	return nil
}

func (s *fakeMetricsCheckpointStore) Stats(context.Context, string, []string) ([]ports.IndexCheckpointStat, error) {
	return s.stats, nil
}

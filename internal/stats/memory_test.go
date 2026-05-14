package stats

import (
	"context"
	"testing"

	"github.com/simp-frp/go-ginx-2/internal/store"
)

func TestMemoryRecordsTCPAndHTTPStats(t *testing.T) {
	memory := NewMemory()

	memory.RecordTCPStart("proxy-1")
	memory.RecordTCPEnd("proxy-1", 12, 34, false)
	memory.RecordUDP("proxy-1", 7, 11, false)
	memory.RecordUDP("proxy-1", 0, 3, true)
	memory.RecordHTTP("proxy-1", 201, 5, 9, false)
	memory.RecordHTTP("proxy-1", 502, 1, 2, true)

	snapshot := memory.Snapshot("proxy-1")
	if snapshot.TCPConnections != 1 || snapshot.TCPCurrentConnections != 0 || snapshot.TCPUploadBytes != 12 || snapshot.TCPDownloadBytes != 34 {
		t.Fatalf("unexpected TCP stats: %+v", snapshot)
	}
	if snapshot.HTTPRequests != 2 || snapshot.HTTPStatusCodes[201] != 1 || snapshot.HTTPStatusCodes[502] != 1 || snapshot.HTTPErrors != 1 {
		t.Fatalf("unexpected HTTP stats: %+v", snapshot)
	}
	if snapshot.UDPPackets != 1 || snapshot.UDPUploadBytes != 7 || snapshot.UDPDownloadBytes != 14 || snapshot.UDPErrors != 1 {
		t.Fatalf("unexpected UDP stats: %+v", snapshot)
	}
}

func TestPersistentLoadsAndFlushesStats(t *testing.T) {
	repo := &fakeStatsRepository{snapshots: []store.ProxyStats{{ProxyID: "proxy-1", TCPConnections: 2, TCPUploadBytes: 10, UDPPackets: 1, UDPUploadBytes: 4, HTTPRequests: 1, HTTPStatusCodes: map[int]int64{200: 1}}}}
	persistent, err := NewPersistent(context.Background(), repo, 0)
	if err != nil {
		t.Fatalf("new persistent stats: %v", err)
	}

	persistent.RecordTCPStart("proxy-1")
	persistent.RecordTCPEnd("proxy-1", 3, 4, true)
	persistent.RecordUDP("proxy-1", 6, 7, false)
	persistent.RecordHTTP("proxy-1", 502, 5, 6, true)
	if err := persistent.Close(context.Background()); err != nil {
		t.Fatalf("close persistent stats: %v", err)
	}

	found := repo.saved["proxy-1"]
	if found.TCPConnections != 3 || found.TCPCurrentConnections != 0 || found.TCPUploadBytes != 13 || found.TCPErrors != 1 {
		t.Fatalf("unexpected TCP stats: %+v", found)
	}
	if found.HTTPRequests != 2 || found.HTTPStatusCodes[200] != 1 || found.HTTPStatusCodes[502] != 1 || found.HTTPErrors != 1 {
		t.Fatalf("unexpected HTTP stats: %+v", found)
	}
	if found.UDPPackets != 2 || found.UDPUploadBytes != 10 || found.UDPDownloadBytes != 7 {
		t.Fatalf("unexpected UDP stats: %+v", found)
	}
}

type fakeStatsRepository struct {
	snapshots []store.ProxyStats
	saved     map[string]ProxyStats
}

func (repo *fakeStatsRepository) Save(ctx context.Context, snapshots []store.ProxyStats) error {
	repo.saved = make(map[string]ProxyStats, len(snapshots))
	for _, snapshot := range snapshots {
		repo.saved[snapshot.ProxyID] = ProxyStats{ProxyID: snapshot.ProxyID, TCPConnections: snapshot.TCPConnections, TCPUploadBytes: snapshot.TCPUploadBytes, TCPDownloadBytes: snapshot.TCPDownloadBytes, TCPErrors: snapshot.TCPErrors, UDPPackets: snapshot.UDPPackets, UDPUploadBytes: snapshot.UDPUploadBytes, UDPDownloadBytes: snapshot.UDPDownloadBytes, UDPErrors: snapshot.UDPErrors, HTTPRequests: snapshot.HTTPRequests, HTTPUploadBytes: snapshot.HTTPUploadBytes, HTTPDownloadBytes: snapshot.HTTPDownloadBytes, HTTPErrors: snapshot.HTTPErrors, HTTPStatusCodes: snapshot.HTTPStatusCodes}
	}
	return nil
}

func (repo *fakeStatsRepository) List(ctx context.Context) ([]store.ProxyStats, error) {
	return repo.snapshots, nil
}

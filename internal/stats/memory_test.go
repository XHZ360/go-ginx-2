package stats

import "testing"

func TestMemoryRecordsTCPAndHTTPStats(t *testing.T) {
	memory := NewMemory()

	memory.RecordTCPStart("proxy-1")
	memory.RecordTCPEnd("proxy-1", 12, 34, false)
	memory.RecordHTTP("proxy-1", 201, 5, 9, false)
	memory.RecordHTTP("proxy-1", 502, 1, 2, true)

	snapshot := memory.Snapshot("proxy-1")
	if snapshot.TCPConnections != 1 || snapshot.TCPCurrentConnections != 0 || snapshot.TCPUploadBytes != 12 || snapshot.TCPDownloadBytes != 34 {
		t.Fatalf("unexpected TCP stats: %+v", snapshot)
	}
	if snapshot.HTTPRequests != 2 || snapshot.HTTPStatusCodes[201] != 1 || snapshot.HTTPStatusCodes[502] != 1 || snapshot.HTTPErrors != 1 {
		t.Fatalf("unexpected HTTP stats: %+v", snapshot)
	}
}

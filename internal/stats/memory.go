package stats

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/store"
)

type Recorder interface {
	RecordTCPStart(proxyID string)
	RecordTCPEnd(proxyID string, uploadBytes int64, downloadBytes int64, failed bool)
	RecordUDP(proxyID string, uploadBytes int64, downloadBytes int64, failed bool)
	RecordHTTP(proxyID string, statusCode int, uploadBytes int64, downloadBytes int64, failed bool)
}

type Memory struct {
	mu      sync.RWMutex
	proxies map[string]*ProxyStats
}

type Persistent struct {
	memory *Memory
	repo   store.StatsRepository
	cancel context.CancelFunc
	done   chan struct{}
}

type ProxyStats struct {
	ProxyID               string
	TCPConnections        int64
	TCPCurrentConnections int64
	TCPUploadBytes        int64
	TCPDownloadBytes      int64
	TCPErrors             int64
	UDPPackets            int64
	UDPUploadBytes        int64
	UDPDownloadBytes      int64
	UDPErrors             int64
	HTTPRequests          int64
	HTTPUploadBytes       int64
	HTTPDownloadBytes     int64
	HTTPErrors            int64
	HTTPStatusCodes       map[int]int64
}

func NewMemory() *Memory {
	return &Memory{proxies: make(map[string]*ProxyStats)}
}

func NewPersistent(ctx context.Context, repo store.StatsRepository, interval time.Duration) (*Persistent, error) {
	memory := NewMemory()
	snapshots, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, snapshot := range snapshots {
		memory.Load(snapshot)
	}
	persistent := &Persistent{memory: memory, repo: repo, done: make(chan struct{})}
	if interval <= 0 {
		close(persistent.done)
		return persistent, nil
	}
	loopCtx, cancel := context.WithCancel(ctx)
	persistent.cancel = cancel
	go persistent.flushLoop(loopCtx, interval)
	return persistent, nil
}

func (persistent *Persistent) Memory() *Memory { return persistent.memory }

func (persistent *Persistent) RecordTCPStart(proxyID string) {
	persistent.memory.RecordTCPStart(proxyID)
}

func (persistent *Persistent) RecordTCPEnd(proxyID string, uploadBytes int64, downloadBytes int64, failed bool) {
	persistent.memory.RecordTCPEnd(proxyID, uploadBytes, downloadBytes, failed)
}

func (persistent *Persistent) RecordUDP(proxyID string, uploadBytes int64, downloadBytes int64, failed bool) {
	persistent.memory.RecordUDP(proxyID, uploadBytes, downloadBytes, failed)
}

func (persistent *Persistent) RecordHTTP(proxyID string, statusCode int, uploadBytes int64, downloadBytes int64, failed bool) {
	persistent.memory.RecordHTTP(proxyID, statusCode, uploadBytes, downloadBytes, failed)
}

func (persistent *Persistent) Flush(ctx context.Context) error {
	return persistent.repo.Save(ctx, persistent.memory.StoreSnapshots())
}

func (persistent *Persistent) Close(ctx context.Context) error {
	if persistent.cancel != nil {
		persistent.cancel()
		<-persistent.done
	}
	return persistent.Flush(ctx)
}

func (memory *Memory) RecordTCPStart(proxyID string) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	stats := memory.proxy(proxyID)
	stats.TCPConnections++
	stats.TCPCurrentConnections++
}

func (memory *Memory) RecordTCPEnd(proxyID string, uploadBytes int64, downloadBytes int64, failed bool) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	stats := memory.proxy(proxyID)
	if stats.TCPCurrentConnections > 0 {
		stats.TCPCurrentConnections--
	}
	stats.TCPUploadBytes += uploadBytes
	stats.TCPDownloadBytes += downloadBytes
	if failed {
		stats.TCPErrors++
	}
}

func (memory *Memory) RecordUDP(proxyID string, uploadBytes int64, downloadBytes int64, failed bool) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	stats := memory.proxy(proxyID)
	if uploadBytes > 0 {
		stats.UDPPackets++
	}
	stats.UDPUploadBytes += uploadBytes
	stats.UDPDownloadBytes += downloadBytes
	if failed {
		stats.UDPErrors++
	}
}

func (memory *Memory) RecordHTTP(proxyID string, statusCode int, uploadBytes int64, downloadBytes int64, failed bool) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	stats := memory.proxy(proxyID)
	stats.HTTPRequests++
	stats.HTTPUploadBytes += uploadBytes
	stats.HTTPDownloadBytes += downloadBytes
	stats.HTTPStatusCodes[statusCode]++
	if failed {
		stats.HTTPErrors++
	}
}

func (memory *Memory) Snapshot(proxyID string) ProxyStats {
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	stats := memory.proxyCopy(proxyID)
	return stats
}

func (memory *Memory) List() []ProxyStats {
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	stats := make([]ProxyStats, 0, len(memory.proxies))
	for proxyID := range memory.proxies {
		stats = append(stats, memory.proxyCopy(proxyID))
	}
	return stats
}

func (memory *Memory) Load(snapshot store.ProxyStats) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	memory.proxies[snapshot.ProxyID] = &ProxyStats{
		ProxyID:           snapshot.ProxyID,
		TCPConnections:    snapshot.TCPConnections,
		TCPUploadBytes:    snapshot.TCPUploadBytes,
		TCPDownloadBytes:  snapshot.TCPDownloadBytes,
		TCPErrors:         snapshot.TCPErrors,
		UDPPackets:        snapshot.UDPPackets,
		UDPUploadBytes:    snapshot.UDPUploadBytes,
		UDPDownloadBytes:  snapshot.UDPDownloadBytes,
		UDPErrors:         snapshot.UDPErrors,
		HTTPRequests:      snapshot.HTTPRequests,
		HTTPUploadBytes:   snapshot.HTTPUploadBytes,
		HTTPDownloadBytes: snapshot.HTTPDownloadBytes,
		HTTPErrors:        snapshot.HTTPErrors,
		HTTPStatusCodes:   maps.Clone(snapshot.HTTPStatusCodes),
	}
	if memory.proxies[snapshot.ProxyID].HTTPStatusCodes == nil {
		memory.proxies[snapshot.ProxyID].HTTPStatusCodes = make(map[int]int64)
	}
}

func (memory *Memory) StoreSnapshots() []store.ProxyStats {
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	snapshots := make([]store.ProxyStats, 0, len(memory.proxies))
	for _, stats := range memory.proxies {
		snapshots = append(snapshots, store.ProxyStats{
			ProxyID:           stats.ProxyID,
			TCPConnections:    stats.TCPConnections,
			TCPUploadBytes:    stats.TCPUploadBytes,
			TCPDownloadBytes:  stats.TCPDownloadBytes,
			TCPErrors:         stats.TCPErrors,
			UDPPackets:        stats.UDPPackets,
			UDPUploadBytes:    stats.UDPUploadBytes,
			UDPDownloadBytes:  stats.UDPDownloadBytes,
			UDPErrors:         stats.UDPErrors,
			HTTPRequests:      stats.HTTPRequests,
			HTTPUploadBytes:   stats.HTTPUploadBytes,
			HTTPDownloadBytes: stats.HTTPDownloadBytes,
			HTTPErrors:        stats.HTTPErrors,
			HTTPStatusCodes:   maps.Clone(stats.HTTPStatusCodes),
		})
	}
	return snapshots
}

func (persistent *Persistent) flushLoop(ctx context.Context, interval time.Duration) {
	defer close(persistent.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = persistent.Flush(context.Background())
		}
	}
}

func (memory *Memory) proxy(proxyID string) *ProxyStats {
	stats, ok := memory.proxies[proxyID]
	if !ok {
		stats = &ProxyStats{ProxyID: proxyID, HTTPStatusCodes: make(map[int]int64)}
		memory.proxies[proxyID] = stats
	}
	return stats
}

func (memory *Memory) proxyCopy(proxyID string) ProxyStats {
	stats, ok := memory.proxies[proxyID]
	if !ok {
		return ProxyStats{ProxyID: proxyID, HTTPStatusCodes: make(map[int]int64)}
	}
	copy := *stats
	copy.HTTPStatusCodes = make(map[int]int64, len(stats.HTTPStatusCodes))
	maps.Copy(copy.HTTPStatusCodes, stats.HTTPStatusCodes)
	return copy
}

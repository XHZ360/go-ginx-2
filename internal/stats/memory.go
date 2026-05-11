package stats

import (
	"maps"
	"sync"
)

type Recorder interface {
	RecordTCPStart(proxyID string)
	RecordTCPEnd(proxyID string, uploadBytes int64, downloadBytes int64, failed bool)
	RecordHTTP(proxyID string, statusCode int, uploadBytes int64, downloadBytes int64, failed bool)
}

type Memory struct {
	mu      sync.RWMutex
	proxies map[string]*ProxyStats
}

type ProxyStats struct {
	ProxyID               string
	TCPConnections        int64
	TCPCurrentConnections int64
	TCPUploadBytes        int64
	TCPDownloadBytes      int64
	TCPErrors             int64
	HTTPRequests          int64
	HTTPUploadBytes       int64
	HTTPDownloadBytes     int64
	HTTPErrors            int64
	HTTPStatusCodes       map[int]int64
}

func NewMemory() *Memory {
	return &Memory{proxies: make(map[string]*ProxyStats)}
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

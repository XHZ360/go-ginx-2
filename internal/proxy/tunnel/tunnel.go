package tunnel

import (
	"bufio"
	"io"
	"net/http"
	"strings"
	"sync"
)

type bufferedReadWriteCloser struct {
	io.ReadWriteCloser
	reader *bufio.Reader
}

func WithBufferedReader(conn io.ReadWriteCloser, reader *bufio.Reader) io.ReadWriteCloser {
	if reader == nil {
		return conn
	}
	return &bufferedReadWriteCloser{ReadWriteCloser: conn, reader: reader}
}

func (conn *bufferedReadWriteCloser) Read(p []byte) (int, error) {
	if conn.reader.Buffered() > 0 {
		return conn.reader.Read(p)
	}
	return conn.ReadWriteCloser.Read(p)
}

func IsWebSocketUpgrade(header http.Header) bool {
	if !strings.EqualFold(header.Get("Upgrade"), "websocket") {
		return false
	}
	for _, value := range header.Values("Connection") {
		for token := range strings.SplitSeq(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return true
			}
		}
	}
	return false
}

func NormalizeWebSocketRequest(request *http.Request) {
	request.Proto = "HTTP/1.1"
	request.ProtoMajor = 1
	request.ProtoMinor = 1
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Connection", "Upgrade")
}

func CopyBidirectional(left io.ReadWriteCloser, right io.ReadWriteCloser) (int64, int64) {
	type result struct {
		leftToRight bool
		bytes       int64
	}
	done := make(chan result, 2)
	var closeOnce sync.Once
	closeBoth := func() {
		_ = left.Close()
		_ = right.Close()
	}
	go func() {
		bytes, _ := io.Copy(right, left)
		closeOnce.Do(closeBoth)
		done <- result{leftToRight: true, bytes: bytes}
	}()
	go func() {
		bytes, _ := io.Copy(left, right)
		closeOnce.Do(closeBoth)
		done <- result{bytes: bytes}
	}()
	var leftToRight int64
	var rightToLeft int64
	for range 2 {
		result := <-done
		if result.leftToRight {
			leftToRight = result.bytes
		} else {
			rightToLeft = result.bytes
		}
	}
	return leftToRight, rightToLeft
}

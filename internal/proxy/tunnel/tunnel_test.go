package tunnel

import (
	"net/http"
	"testing"
)

func TestIsWebSocketUpgrade(t *testing.T) {
	tests := map[string]struct {
		header http.Header
		want   bool
	}{
		"connection multi token": {
			header: http.Header{"Upgrade": {"websocket"}, "Connection": {"keep-alive, Upgrade"}},
			want:   true,
		},
		"missing upgrade token": {
			header: http.Header{"Upgrade": {"websocket"}, "Connection": {"keep-alive"}},
		},
		"non websocket upgrade": {
			header: http.Header{"Upgrade": {"h2c"}, "Connection": {"upgrade"}},
		},
		"case insensitive": {
			header: http.Header{"Upgrade": {"WebSocket"}, "Connection": {"UPGRADE"}},
			want:   true,
		},
		"separate connection values": {
			header: http.Header{"Upgrade": {"websocket"}, "Connection": {"keep-alive", "Upgrade"}},
			want:   true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := IsWebSocketUpgrade(test.header); got != test.want {
				t.Fatalf("IsWebSocketUpgrade()=%t want=%t", got, test.want)
			}
		})
	}
}

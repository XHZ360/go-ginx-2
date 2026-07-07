// Package main demonstrates basic GoGinX SDK usage: connect, list proxies,
// dial a proxy, send/receive data, and close.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/simp-frp/go-ginx-2/sdk"
)

func main() {
	client := sdk.New(sdk.Config{
		ServerAddress: getEnv("GOGINX_SERVER_ADDR", "control.example.com:8443"),
		ServerName:    getEnv("GOGINX_SERVER_NAME", "go-ginx-control.local"),
		ServerCAFile:  getEnv("GOGINX_CA_FILE", "data/certs/server-ca.crt"),
		ClientID:      getEnv("GOGINX_CLIENT_ID", "sdk-client-1"),
		Credential:    getEnv("GOGINX_CREDENTIAL", "secret"),
	})

	ctx := context.Background()

	// Connect to the GoGinX control channel as a consumer.
	if err := client.Connect(ctx); err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer client.Close()

	// List available proxies.
	proxies, err := client.Proxies(ctx)
	if err != nil {
		log.Fatalf("proxies: %v", err)
	}
	fmt.Printf("Available proxies (%d):\n", len(proxies))
	for _, p := range proxies {
		fmt.Printf("  %s (%s) -> %s:%d\n", p.ID, p.Type, p.TargetHost, p.TargetPort)
	}

	if len(proxies) == 0 {
		fmt.Println("No proxies available.")
		return
	}

	// Dial the first proxy.
	proxyID := proxies[0].ID
	fmt.Printf("\nDialing proxy %s...\n", proxyID)

	conn, err := client.Dial(ctx, proxyID)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send data and read response.
	if _, err := conn.Write([]byte("Hello from SDK!")); err != nil {
		log.Fatalf("write: %v", err)
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		log.Fatalf("read: %v", err)
	}
	fmt.Printf("Received: %s\n", string(buf[:n]))

	// Example: use HTTPTransport for HTTP proxies.
	// transport := client.HTTPTransport(proxyID)
	// httpClient := &http.Client{Transport: transport}
	// resp, err := httpClient.Get("http://target.example.com/api")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/nats-io/nats.go"
	"log"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

// TimingData holds the duration of various phases of the request lifecycle
type TimingData struct {
	DNSDuration     time.Duration
	ConnectDuration time.Duration
	TLSDuration     time.Duration
	TimeToFirstByte time.Duration
	TotalDuration   time.Duration
}

// newClientTrace creates a new httptrace.ClientTrace to track request timings
func newClientTrace(timings *TimingData, start time.Time) *httptrace.ClientTrace {
	var dnsStart, connectStart, tlsStart time.Time

	return &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			timings.DNSDuration = time.Since(dnsStart)
		},
		ConnectStart: func(_, _ string) {
			connectStart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			timings.ConnectDuration = time.Since(connectStart)
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			timings.TLSDuration = time.Since(tlsStart)
		},
		GotFirstResponseByte: func() {
			timings.TimeToFirstByte = time.Since(start)
		},
	}
}

// TraceRequest sends an HTTP GET request and traces the request lifecycle
func TraceRequest(targetURL string) (*TimingData, error) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	start := time.Now()
	timings := &TimingData{}

	// Attach the trace to the request's context
	req = req.WithContext(httptrace.WithClientTrace(context.Background(), newClientTrace(timings, start)))

	// Perform the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Calculate total duration after receiving the response
	timings.TotalDuration = time.Since(start)

	return timings, nil
}

func main() {
	nc, _ := nats.Connect(nats.DefaultURL)

	// Use a WaitGroup to wait for a message to arrive
	wg := sync.WaitGroup{}
	wg.Add(1)

	// Subscribe
	if _, err := nc.Subscribe("updates", func(m *nats.Msg) {
		res, _ := TraceRequest(string(m.Data))
		println(res.DNSDuration.String())
	}); err != nil {
		log.Fatal(err)
	}

	// Wait for a message to come in
	wg.Wait()

}

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"io"
	"log"
	"net/http"
	"net/http/httptrace"
	"os"
	"os/signal"
	"time"
)

// Metrics holds the duration of various phases of the request lifecycle
type Metrics struct {
	DNSDuration     time.Duration `json:"dns_duration"`
	ConnectDuration time.Duration `json:"con_duration"`
	TLSDuration     time.Duration `json:"tls_duration"`
	TimeToFirstByte time.Duration `json:"tt_first_byte"`
	TotalDuration   time.Duration `json:"total_duration"`
}

type TraceResult struct {
	URL     string  `json:"url"`
	Status  int     `json:"status"`
	Metrics Metrics `json:"metrics"`
}

// newClientTrace creates a new httptrace.ClientTrace to track request timings
func newClientTrace(metrics *Metrics, start time.Time) *httptrace.ClientTrace {
	var dnsStart, connectStart, tlsStart time.Time

	return &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			metrics.DNSDuration = time.Since(dnsStart)
		},
		ConnectStart: func(_, _ string) {
			connectStart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			metrics.ConnectDuration = time.Since(connectStart)
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			metrics.TLSDuration = time.Since(tlsStart)
		},
		GotFirstResponseByte: func() {
			metrics.TimeToFirstByte = time.Since(start)
		},
	}
}

// TraceRequest sends an HTTP GET request and traces the request lifecycle
func TraceRequest(targetURL string) (TraceResult, error) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return TraceResult{}, fmt.Errorf("failed to create request: %w", err)
	}

	// Making sure the request won't be cached
	req.Header.Set("Expires", "0")

	start := time.Now()
	metrics := &Metrics{}

	// Attach the trace to the request's context
	req = req.WithContext(httptrace.WithClientTrace(context.Background(), newClientTrace(metrics, start)))

	// Perform the request
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return TraceResult{}, fmt.Errorf("request failed: %w", err)
	}

	// Reading the body
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return TraceResult{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Calculate total duration after receiving the response
	metrics.TotalDuration = time.Since(start)

	return TraceResult{
		Metrics: *metrics,
		URL:     targetURL,
		Status:  resp.StatusCode,
	}, nil
}

func main() {
	region := os.Args[1]
	consumerName := fmt.Sprintf("worker_%s", region)

	// Connecting to the NATS-Server
	nc, err := nats.Connect(nats.DefaultURL)

	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	// Configuring jetstream
	js, err := jetstream.New(nc)

	if err != nil {
		log.Fatal(err)
	}

	// Creating a consumer for all subjects
	ctx := context.Background()
	consumer, err := js.CreateOrUpdateConsumer(ctx, "jobs", jetstream.ConsumerConfig{
		Name:          consumerName,
		Durable:       consumerName,
		Description:   fmt.Sprintf("Worker for Tracing-Jobs in %s", region),
		FilterSubject: fmt.Sprintf("jobs.%s.*", region),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Subscribing to all messages
	c, err := consumer.Consume(func(msg jetstream.Msg) {
		target := string(msg.Data())
		result, err := TraceRequest(target)

		if err != nil {
			fmt.Printf("Message to %s failed\n", target)
			_ = msg.Nak()
		}

		j, err := json.Marshal(result)
		if err != nil {
			fmt.Printf("Message to %s failed to marshal\n", target)
			_ = msg.Nak()
		}

		if err := nc.Publish("jobs.results", j); err != nil {
			fmt.Printf("Message to %s failed to publish result\n", target)
			_ = msg.Nak()
		}

		_ = msg.Ack()
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Stop()

	println("Worker-Node is ready.")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
}

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"log"
	"net/http"
	"net/http/httptrace"
	"os"
	"os/signal"
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
func TraceRequest(targetURL string) (TimingData, error) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return TimingData{}, fmt.Errorf("failed to create request: %w", err)
	}

	// Making sure the request won't be cached
	req.Header.Set("Expires", "0")

	start := time.Now()
	timings := &TimingData{}

	// Attach the trace to the request's context
	req = req.WithContext(httptrace.WithClientTrace(context.Background(), newClientTrace(timings, start)))

	// Perform the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return TimingData{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Calculate total duration after receiving the response
	timings.TotalDuration = time.Since(start)

	return *timings, nil
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
		FilterSubject: fmt.Sprintf("jobs.%s.*", "eu"),
	})
	if err != nil {
		log.Fatal(err)
	}

	println("Registering the consumer.")

	// Subscribing to all messages
	c, err := consumer.Consume(func(msg jetstream.Msg) {
		target := string(msg.Data())
		res, err := TraceRequest(target)

		if err != nil {
			fmt.Printf("Message to %s failed\n", target)
			_ = msg.Nak()
		}

		println(res.TotalDuration.String())

		_ = msg.Ack()
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Stop()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
}

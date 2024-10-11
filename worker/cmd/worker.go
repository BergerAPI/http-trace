package main

import (
	"github.com/nats-io/nats.go"
	"io"
	"log"
	"net/http"
	"sync"
)

func sendRequest(requestURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	return string(resBody), nil
}

func main() {
	nc, _ := nats.Connect(nats.DefaultURL)

	// Use a WaitGroup to wait for a message to arrive
	wg := sync.WaitGroup{}
	wg.Add(1)

	// Subscribe
	if _, err := nc.Subscribe("updates", func(m *nats.Msg) {
		res, _ := sendRequest(string(m.Data))
		println(res)
	}); err != nil {
		log.Fatal(err)
	}

	// Wait for a message to come in
	wg.Wait()

}

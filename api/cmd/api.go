package main

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/nats-io/nats.go"
	"log"
	"net/url"
	"slices"
)

func main() {
	app := fiber.New()

	// Connecting to the NATS-Server
	nc, err := nats.Connect(nats.DefaultURL)

	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	// Create a JetStream context
	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("Error creating JetStream context: %v", err)
	}

	if _, err := js.Subscribe("jobs.results", func(msg *nats.Msg) {
		println(string(msg.Data))
		_ = msg.Ack()
	}, nats.Durable("API_RESULTS")); err != nil {
		log.Fatal(err)
	}

	// Fetch stream information to list all available subjects
	streamInfo, err := js.StreamInfo("jobs")
	if err != nil {
		log.Fatalf("Error fetching stream info: %v", err)
	}

	app.Get("/health", func(ctx *fiber.Ctx) error {
		return ctx.SendString("OK")
	})

	app.Post("/v1/probe", func(ctx *fiber.Ctx) error {
		payload := struct {
			Region string `json:"region"`
			Target string `json:"target"`
		}{}

		if err := ctx.BodyParser(&payload); err != nil {
			return err
		}

		// Checking whether the target is a valid url
		targetUrl, err := url.ParseRequestURI(payload.Target)
		if err != nil {
			return ctx.Status(401).SendString("Target is missing or invalid. Refer to the official documentation.")
		}

		region := payload.Region

		// Making sure that at least some region is provided
		if region == "" {
			region = "eu"
		}

		subject := fmt.Sprintf("jobs.%s.*", region)

		// Make sure a subjects with the provided region is available
		if !slices.Contains(streamInfo.Config.Subjects, subject) {
			return ctx.Status(401).SendString("Region is not available.")
		}

		if err := nc.Publish(subject, []byte(targetUrl.String())); err != nil {
			return err
		}

		return ctx.SendString("OK")
	})

	log.Fatal(app.Listen(":3000"))
}

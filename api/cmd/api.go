package main

import (
	"github.com/gofiber/fiber/v2"
	"github.com/nats-io/nats.go"
	"log"
	"net/url"
)

func main() {
	app := fiber.New()

	// Connecting to the NATS-Server
	nc, err := nats.Connect(nats.DefaultURL)

	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

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

		if err := nc.Publish("jobs.eu.trace", []byte(targetUrl.String())); err != nil {
			return err
		}

		return ctx.SendString("OK")
	})

	log.Fatal(app.Listen(":3000"))
}

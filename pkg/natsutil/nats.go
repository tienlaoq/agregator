package natsutil

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// DrainTimeout is the maximum time nc.Drain() will wait for in-flight
// message handlers to finish before forcibly closing subscriptions.
// It must fit within the service's overall shutdown budget (20 s) and
// leave room for grpcServer.GracefulStop() running concurrently.
const DrainTimeout = 10 * time.Second

func Connect(url string) (*nats.Conn, nats.JetStreamContext, error) {
	nc, err := nats.Connect(url,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(nats.DefaultReconnectWait),
		nats.DrainTimeout(DrainTimeout),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("connect nats: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("jetstream: %w", err)
	}

	return nc, js, nil
}

func EnsureStream(js nats.JetStreamContext, name string, subjects []string) error {
	_, err := js.StreamInfo(name)
	if err != nil {
		_, err = js.AddStream(&nats.StreamConfig{
			Name:     name,
			Subjects: subjects,
			Storage:  nats.FileStorage,
		})
		if err != nil {
			return fmt.Errorf("add stream %s: %w", name, err)
		}
	}
	return nil
}

// EnsureConsumer creates or updates a durable push consumer on the given stream.
// If a consumer with cfg.Durable already exists its configuration is updated;
// if it does not exist it is created.  Only push consumers (with a DeliverSubject)
// are supported — pull consumers have a different subscription model.
//
// Call this before js.Subscribe so the consumer's MaxDeliver / AckWait / BackOff
// policy is set server-side and not silently defaulted to unlimited retries.
func EnsureConsumer(js nats.JetStreamContext, stream string, cfg nats.ConsumerConfig) error {
	_, err := js.ConsumerInfo(stream, cfg.Durable)
	if err == nil {
		// Consumer exists — update to pick up any config changes (e.g. MaxDeliver bump).
		_, err = js.UpdateConsumer(stream, &cfg)
		if err != nil {
			return fmt.Errorf("update consumer %s/%s: %w", stream, cfg.Durable, err)
		}
		return nil
	}
	_, err = js.AddConsumer(stream, &cfg)
	if err != nil {
		return fmt.Errorf("add consumer %s/%s: %w", stream, cfg.Durable, err)
	}
	return nil
}

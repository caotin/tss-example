package main

import (
	"context"
	"strconv"
	"time"

	"encoding/binary"

	amqp "github.com/rabbitmq/amqp091-go"
)

func intToBytes(n int) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(n))
	return b
}

func bytesToInt(b []byte) int {
	return int(binary.LittleEndian.Uint64(b))
}

func stringToInt(s string) (int, error) {
	return strconv.Atoi(s)
}

func ReciveMessage(ch *amqp.Channel, queue string) <-chan amqp.Delivery {
	q, err := ch.QueueDeclare(
		queue, // name
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	failOnError(err, "Failed to declare a queue")

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Failed to register a consumer")
	return msgs
}

func SendMessage(ch *amqp.Channel, queue string, body []byte) {
	q, err := ch.QueueDeclare(
		queue, // name
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	failOnError(err, "Failed to declare a queue")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = ch.PublishWithContext(ctx,
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        body,
		})
	failOnError(err, "Failed to recive a consumer")
}

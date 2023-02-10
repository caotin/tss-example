package main

import (
	"fmt"
	"os"
	"strconv"

	amqp "github.com/rabbitmq/amqp091-go"
)

func failOnError(err error, msg string) {
	if err != nil {
		fmt.Printf("%s: %s\n", msg, err)
	}
}
func main() {
	if len(os.Args) < 1 {
		fmt.Println(nil, "Input party number")
	}
	arg := os.Args[1]
	partyInt, _ := strconv.Atoi(arg)

	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	NewKeygen(partyInt, 3, 4, conn)
}

package porthos

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/streadway/amqp"
)

type ResponseTest struct {
	Original float64 `json:"original"`
	Sum      float64 `json:"sum"`
}

func TestNewServer(t *testing.T) {
	broker, err := NewBroker(os.Getenv("AMQP_URL"))

	if err != nil {
		t.Fatal("NewBroker failed.", err)
	}

	userService, err := NewServer(broker, "UserService", Options{false})
	s := userService.(*server)
	defer userService.Close()

	if err != nil {
		t.Fatal("NewServer failed.", err)
	}

	if s.serviceName != "UserService" {
		t.Errorf("Wrong serviceName, expected: 'UserService', got: '%s'", s.serviceName)
	}

	if s.channel == nil {
		t.Error("Service channel is nil.")
	}

	if s.requestChannel == nil {
		t.Error("Service requestChannel is nil.")
	}

	if s.methods == nil {
		t.Error("Service methods is nil.")
	}
}

func TestServerProcessRequest(t *testing.T) {
	b, err := NewBroker(os.Getenv("AMQP_URL"))

	if err != nil {
		t.Fatal("NewBroker failed.", err)
	}

	userService, err := NewServer(b, "UserService", Options{false})
	s := userService.(*server)
	defer userService.Close()

	if err != nil {
		t.Fatal("NewServer failed.", err)
	}

	// register the method that we will test.
	userService.Register("doSomething", func(req Request, res Response) {
		form, _ := req.Form()
		x, _ := form.GetArg(0).AsFloat64()

		res.JSON(200, ResponseTest{x, x + 1})
	})

	go userService.ListenAndServe()

	// This code below is to simulate the client invoking the remote method.

	// create the request message body.
	body, _ := json.Marshal([]interface{}{10})

	// declare the response queue.
	q, err := s.channel.QueueDeclare("", false, false, true, false, nil)

	if err != nil {
		t.Fatal("Queue declare failed.", err)
	}

	// start consuming from the response queue.
	dc, err := s.channel.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)

	if err != nil {
		t.Fatal("Consume failed.", err)
	}

	// publish the request.
	err = s.channel.Publish(
		"",
		s.serviceName,
		false,
		false,
		amqp.Publishing{
			Headers: amqp.Table{
				"X-Method": "doSomething",
			},
			Expiration:    "3000",
			ContentType:   "application/json",
			CorrelationId: "1",
			ReplyTo:       q.Name,
			Body:          body,
		})

	if err != nil {
		t.Fatal("Publish failed.", err)
	}

	// wait the response or timeout.
	for {
		select {
		case response := <-dc:
			if response.Headers["statusCode"].(int16) != 200 {
				t.Errorf("Expected status code was 200, got: %d", response.Headers["statusCode"])
			}

			var responseTest ResponseTest
			err = json.Unmarshal(response.Body, &responseTest)

			if responseTest.Sum != 11 {
				t.Errorf("Response failed, expected: 11, got: %s", responseTest.Sum)
			}

			return
		case <-time.After(5 * time.Second):
			t.Fatal("No response receive. Timedout.")
		}
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"log"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type PublishRequest struct {
	Topic    string
	Qos      byte
	Retained bool
	Payload  interface{}
}

func publisherLoop(requests <-chan PublishRequest) error {
	broker := *mqttBroker
	if broker == "" {
		log.Printf("MQTT publishing disabled (empty -mqtt_broker)")
		for range requests {
		}
		return nil
	}
	log.Printf("Connecting to MQTT broker %q", broker)
	opts := mqtt.NewClientOptions().AddBroker(broker)
	opts.SetClientID("hmgo")
	opts.SetConnectRetry(true)
	mqttClient := mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("MQTT connection failed: %v", token.Error())
	}

	for r := range requests {
		// discard Token, MQTT publishing is best-effort
		_ = mqttClient.Publish(r.Topic, r.Qos, r.Retained, r.Payload)
	}
	return nil
}

func MQTT() chan<- PublishRequest {
	result := make(chan PublishRequest, 64)
	go func() {
		if err := publisherLoop(result); err != nil {
			log.Print(err)
		}
	}()
	return result
}

func publishMQTT(ch chan<- PublishRequest, hmtype, name, event string, payload interface{}) {
	b, err := json.Marshal(payload)
	if err != nil {
		log.Printf("marshaling MQTT payload for %s/%s/%s: %v", hmtype, name, event, err)
		return
	}
	req := PublishRequest{
		Topic:    fmt.Sprintf("github.com/stapelberg/hmgo/%s/%s/%s", hmtype, name, event),
		Qos:      0,
		Retained: true,
		Payload:  b,
	}
	select {
	case ch <- req:
	default:
		log.Printf("MQTT channel full, dropping message for %s", req.Topic)
	}
}

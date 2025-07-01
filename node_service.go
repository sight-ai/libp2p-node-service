package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	hostlibp2p "github.com/libp2p/go-libp2p/core/host"
)

type Libp2pNodeService struct {
	did        string
	keypair    Keypair
	tunnelAPI  string
	isGateway  bool
	node       hostlibp2p.Host
	pubsub     *pubsub.PubSub
	subscribed *pubsub.Subscription
	topic      *pubsub.Topic
	bootstrap  []string
	nodePort   int
}

func NewLibp2pNodeService(kp Keypair, port int, tunnelAPI string, isGateway bool, bootstrap []string) *Libp2pNodeService {
	did := "gateway"
	if !isGateway {
		did = ToSightDID(kp.PublicKey)
		log.Printf("[Libp2p Node with this] DID: %s", did)
	}
	return &Libp2pNodeService{
		keypair:   kp,
		did:       did,
		tunnelAPI: tunnelAPI,
		isGateway: isGateway,
		nodePort:  port,
		bootstrap: bootstrap,
	}
}

func (s *Libp2pNodeService) InitNode() {
	ctx := context.Background()

	// Create node and pubsub
	h, ps := CreateLibp2pNode(ctx, s.nodePort, s.bootstrap)
	s.node = h

	topic, err := ps.Join("sight-message")
	if err != nil {
		log.Fatalf("Failed to join topic: %v", err)
	}
	s.topic = topic

	sub, err := topic.Subscribe()
	if err != nil {
		log.Fatalf("Failed to subscribe to topic: %v", err)
	}
	s.subscribed = sub

	// Start message handler in a goroutine
	go s.handleIncomingMessages(ctx)
}

func (s *Libp2pNodeService) handleIncomingMessages(ctx context.Context) {
	for {
		msg, err := s.subscribed.Next(ctx)
		if err != nil {
			log.Printf("PubSub error: %v", err)
			return
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			log.Printf("Invalid message format: %v", err)
			continue
		}

		// Only process messages intended for this node
		if payload["to"] != s.did {
			continue
		}

		buf, err := json.Marshal(payload["payload"])
		if err != nil {
			log.Printf("Error marshalling payload: %v", err)
			continue
		}

		// Send the message to the tunnel API
		var resp *http.Response
		resp, err = http.Post(s.tunnelAPI, "application/json", bytes.NewBuffer(buf))
		if err != nil {
			log.Printf("Forward error: %v", err)
		} else {
			in, _ := json.MarshalIndent(payload, "", "  ")
			log.Printf("Received and forwarded message to tunnel: \n%s", in)
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
		}
	}
}

// HandleOutgoingMessage publishes outgoing messages to the topic
func (s *Libp2pNodeService) HandleOutgoingMessage(msg map[string]interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshalling outgoing message: %v", err)
		return
	}

	if err := s.topic.Publish(context.Background(), data); err != nil {
		log.Printf("Error publishing message: %v", err)
	} else {
		out, _ := json.MarshalIndent(msg, "", "  ")
		log.Printf("Published outgoing message: \n%s", out)
	}
}

// Stop gracefully stops the libp2p node
func (s *Libp2pNodeService) Stop() {
	if err := s.node.Close(); err != nil {
		log.Printf("Error stopping node: %v", err)
	}
}

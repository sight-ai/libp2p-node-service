package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/ed25519"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

var seeds = [][]byte{
	{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	{4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	{5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	{6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	{7, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	{8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	{9, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
}
var ports = []int{15001, 15002, 15003, 15004, 15005, 15006, 15007, 15008, 15009}

func randomNeighbors(n int, exclude int) []int {
	indices := make([]int, 0, len(ports)-1)
	for i := range ports {
		if i != exclude {
			indices = append(indices, i)
		}
	}
	rand.Shuffle(len(indices), func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})
	count := 4 + rand.Intn(2) // 4 or 5 neighbors
	if count > len(indices) {
		count = len(indices)
	}
	return indices[:count]
}

func main() {
	rand.Seed(time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts, topics, err := CreateBootstrapNodes(ctx, seeds, ports)
	if err != nil {
		log.Fatalf("Failed to create bootstrap nodes: %v", err)
	}

	// Connect neighbors
	for i, h := range hosts {
		neighbors := randomNeighbors(len(ports), i)
		for _, n := range neighbors {
			pi := peer.AddrInfo{
				ID:    hosts[n].ID(),
				Addrs: hosts[n].Addrs(),
			}
			if err := h.Connect(ctx, pi); err != nil {
				log.Printf("Node@%d failed to connect to Node@%d: %v", ports[i], ports[n], err)
			} else {
				log.Printf("Node@%d connected to Node@%d", ports[i], ports[n])
			}
		}
	}

	// Node 0 broadcasts a message after a short delay to ensure connections are established
	go func() {
		time.Sleep(3 * time.Second)
		msg := map[string]string{"text": "Hello from bootstrap node 0"}
		data, _ := json.Marshal(msg)
		if err := topics[0].Publish(ctx, data); err != nil {
			log.Printf("Node@%d failed to publish message: %v", ports[0], err)
		} else {
			log.Printf("Node@%d published a message", ports[0])
		}
	}()

	// Handle SIGINT for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	log.Println("Received interrupt signal, shutting down nodes...")

	var wg sync.WaitGroup
	for i, h := range hosts {
		wg.Add(1)
		go func(i int, h hostCloser) {
			defer wg.Done()
			log.Printf("Closing node@%d...", ports[i])
			if err := h.Close(); err != nil {
				log.Printf("Error closing node@%d: %v", ports[i], err)
			} else {
				log.Printf("Node@%d closed", ports[i])
			}
		}(i, h)
	}
	wg.Wait()
	log.Println("All nodes shut down. Exiting.")
}

type hostCloser interface {
	Close() error
	Addrs() []multiaddr.Multiaddr
	ID() peer.ID
	Connect(ctx context.Context, pi peer.AddrInfo) error
}

func CreateBootstrapNodes(ctx context.Context, seeds [][]byte, ports []int) ([]hostCloser, []*pubsub.Topic, error) {
	var hosts []hostCloser
	var topics []*pubsub.Topic

	for i, seed := range seeds {
		privKey := ed25519.NewKeyFromSeed(seed)
		priv, _, _ := crypto.KeyPairFromStdKey(&privKey)
		h, err := libp2p.New(
			libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", ports[i])),
			libp2p.Identity(priv),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create host %d: %w", i, err)
		}

		// DHT
		dhtNode, err := dht.New(ctx, h, dht.Mode(dht.ModeServer))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create dht for host %d: %w", i, err)
		}
		go func() {
			if err := dhtNode.Bootstrap(ctx); err != nil {
				log.Printf("[Node@%d] DHT bootstrap error: %v", ports[i], err)
			} else {
				log.Printf("[Node@%d] DHT bootstrap done", ports[i])
			}
		}()

		ps, err := pubsub.NewGossipSub(ctx, h)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create pubsub for host %d: %w", i, err)
		}

		topic, err := ps.Join("sight-message")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to join topic for host %d: %w", i, err)
		}

		sub, err := topic.Subscribe()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to subscribe topic for host %d: %w", i, err)
		}

		go func(i int, sub *pubsub.Subscription) {
			for {
				msg, err := sub.Next(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("[Node@%d] Error reading message: %v", ports[i], err)
					continue
				}
				log.Printf("[Node@%d] received from %s: %s", ports[i], msg.GetFrom().String(), string(msg.Data))
			}
		}(i, sub)

		for _, addr := range h.Addrs() {
			log.Printf("Bootstrap Node@%d at %s/p2p/%s", ports[i], addr, h.ID().String())
		}

		hosts = append(hosts, h)
		topics = append(topics, topic)
	}

	return hosts, topics, nil
}

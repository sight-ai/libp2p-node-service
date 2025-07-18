package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	hostlibp2p "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/crypto/ed25519"
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
	dht        *dht.IpfsDHT
}

func NewLibp2pNodeService(kp Keypair, port int, tunnelAPI string, isGateway bool, bootstrap []string) *Libp2pNodeService {
	did := "gateway"
	if !isGateway {
		did = ToSightDID(kp.PublicKey)
		log.Printf("[Libp2p Node with this] DID: %s", did)
	}
	// 给Gateway一个固定的keypair
	// TODO：Gateway切换到用did标识后，需要有自己的keypair
	if isGateway {
		seed := make([]byte, 32)
		seed[0] = 32
		priv := ed25519.NewKeyFromSeed(seed)
		pub := priv.Public().(ed25519.PublicKey)
		kp = Keypair{
			PrivateKey: priv,
			PublicKey:  pub,
		}
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
	h, ps, dht := CreateLibp2pNode(ctx, s.nodePort, s.bootstrap, s.keypair)
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

	s.dht = dht

	// Start message handler in a goroutine
	go s.handleIncomingMessages(ctx)

	// 暂时将libp2p直接消息协议设置为test/0.0.1
	s.node.SetStreamHandler("/test/0.0.1", s.handleDirectIncomingMessage)
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

func (s *Libp2pNodeService) handleDirectIncomingMessage(stream network.Stream) {
	go func() { // 并发处理
		defer stream.Close()
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(stream); err != nil {
			log.Printf("Failed to read p2p message: %v", err)
			return
		}
		// 解包
		var payload map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
			log.Printf("Invalid p2p message format: %v", err)
			return
		}

		// 直接不需要判断
		// 判断 to
		// if payload["to"] != s.did {
		// 	log.Printf("Direct message not for me, ignoring")
		// 	return
		// }
		// 发给 tunnel API
		data, _ := json.Marshal(payload["payload"])
		resp, err := http.Post(s.tunnelAPI, "application/json", bytes.NewBuffer(data))
		if err != nil {
			log.Printf("Direct message forward error: %v", err)
		} else {
			log.Printf("Direct message forwarded, payload: %v", payload)
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
		}
	}()
}

// Stop gracefully stops the libp2p node
func (s *Libp2pNodeService) Stop() {
	if err := s.node.Close(); err != nil {
		log.Printf("Error stopping node: %v", err)
	}
}

// GetPublicKeyByPeerId returns the public key of a peer by its peer ID
func (s *Libp2pNodeService) GetPublicKeyByPeerId(ctx context.Context, peerId string) ([]byte, error) {
	pk, err := DecodePublicKeyFromPeerId(peerId)
	if err == nil && pk != nil {
		// println(`decode from peerId`)
		return pk, nil
	}

	pid, err := peer.Decode(peerId)
	if err != nil {
		return nil, err
	}
	// 查找 peerstore
	pub := s.node.Peerstore().PubKey(pid)
	if pub != nil {
		// println(`find from peerstore`)
		return pub.Raw()
	}
	// 没有就在DHT找对方地址
	addrInfo, err := s.dht.FindPeer(ctx, pid)
	if err != nil {
		return nil, err
	}
	// 连接对端
	err = s.node.Connect(ctx, addrInfo)
	if err != nil {
		return nil, err
	}
	// 连接后，再次查
	pub = s.node.Peerstore().PubKey(pid)
	if pub == nil {
		// println(`find from DHT`)
		return nil, errors.New("no public key found")
	}
	return pub.Raw()
}

// ConnectByDIDOrMultiAddr connects to a peer by its DID or multiaddr
func (s *Libp2pNodeService) ConnectByDIDOrMultiAddr(ctx context.Context, did string) error {
	if strings.HasPrefix(did, "/") {
		maddr, err := ma.NewMultiaddr(did)
		if err != nil {
			return err
		}
		info, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			return err
		}
		return s.node.Connect(ctx, *info)
	}

	pub, err := DIDToPublicKey(did)
	if err != nil {
		return err
	}
	pid, err := PublicKeyToPeerId(pub)
	if err != nil {
		return err
	}
	addrInfo, err := s.dht.FindPeer(ctx, pid)
	if err != nil {
		return err
	}
	return s.node.Connect(ctx, addrInfo)
}

// GetNeighbors returns a list of currently connected neighbor peer IDs
func (s *Libp2pNodeService) GetNeighbors() []string {
	var neighbors []string
	for _, pid := range s.node.Network().Peers() {
		neighbors = append(neighbors, pid.String())
	}
	return neighbors
}

// PingPeer pings a peer by its DID or multiaddr
func (s *Libp2pNodeService) PingPeer(ctx context.Context, did string) (int64, error) {
	err := s.ConnectByDIDOrMultiAddr(ctx, did)
	if err != nil {
		return 0, err
	}
	var pid peer.ID
	if strings.HasPrefix(did, "/") {
		maddr, _ := ma.NewMultiaddr(did)
		info, _ := peer.AddrInfoFromP2pAddr(maddr)
		pid = info.ID
	} else {
		pub, _ := DIDToPublicKey(did)
		pid, _ = PublicKeyToPeerId(pub)
	}
	pinger := ping.NewPingService(s.node)
	ch := pinger.Ping(ctx, pid)
	// 只要第一个ping响应
	res := <-ch
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RTT.Milliseconds(), nil
}

// SendDirectMessage sends a direct message to a peer by its DID or multiaddr
func (s *Libp2pNodeService) SendDirectMessage(ctx context.Context, did string, payload []byte) error {
	err := s.ConnectByDIDOrMultiAddr(ctx, did)
	if err != nil {
		return err
	}
	var pid peer.ID
	if strings.HasPrefix(did, "/") {
		maddr, _ := ma.NewMultiaddr(did)
		info, _ := peer.AddrInfoFromP2pAddr(maddr)
		pid = info.ID
	} else {
		pub, _ := DIDToPublicKey(did)
		pid, _ = PublicKeyToPeerId(pub)
	}
	// 暂时采用 "/test/0.0.1" 的自定义 p2p 协议名
	stream, err := s.node.NewStream(ctx, pid, "/test/0.0.1")
	if err != nil {
		return err
	}
	defer stream.Close()
	_, err = stream.Write(payload)
	return err
}

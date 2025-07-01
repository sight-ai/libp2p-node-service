package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	hostlibp2p "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/mr-tron/base58"
	"golang.org/x/crypto/ed25519"
	"crypto/rand"
)

// Keypair struct to hold the keypair data
type Keypair struct {
	Seed      []byte `json:"seed"`
	CreatedAt string `json:"createdAt"`
	LastUsed  string `json:"lastUsed"`
	PublicKey []byte `json:"publicKey,omitempty"`
	PrivateKey []byte `json:"privateKey,omitempty"`
}

// LoadOrGenerateKeypair function for loading or generating a keypair
func LoadOrGenerateKeypair() Keypair {
	keyDir := getDataDir()
	keyFile := keyDir + "/device-keypair.json"

	// Check if the keypair file exists
	if _, err := os.Stat(keyFile); err == nil {
		// Read keypair from the file
		kpStr, err := os.ReadFile(keyFile)
		if err != nil {
			log.Fatal("Error reading keypair: ", err)
		}

		// Define a temporary struct to decode seed as []int and other fields
		var tmp struct {
			Seed      []int  `json:"seed"`
			CreatedAt string `json:"createdAt"`
			LastUsed  string `json:"lastUsed"`
		}
		err = json.Unmarshal(kpStr, &tmp)
		if err != nil {
			log.Fatal("Error unmarshalling keypair: ", err)
		}

		// Convert []int seed to []byte
		seed := make([]byte, len(tmp.Seed))
		for i, v := range tmp.Seed {
			seed[i] = byte(v)
		}

		// Generate ed25519 keypair from seed using golang.org/x/crypto/ed25519
		// This simulates JS nacl.sign.keyPair.fromSeed
		privKey := ed25519.NewKeyFromSeed(seed)
		pubKey := privKey.Public().(ed25519.PublicKey)

		kp := Keypair{
			Seed:      seed,
			CreatedAt: tmp.CreatedAt,
			LastUsed:  tmp.LastUsed,
			PublicKey: pubKey,
			PrivateKey: privKey,
		}

		log.Printf("[KeyPair] Loaded from %s", keyFile)
		return kp
	} else {
		// Generate a new random 32-byte seed
		seed := make([]byte, ed25519.SeedSize) // 32 bytes
		_, err := rand.Read(seed)
		if err != nil {
			log.Fatal("Error generating random seed: ", err)
		}

		// Generate ed25519 keypair from seed
		privKey := ed25519.NewKeyFromSeed(seed)
		pubKey := privKey.Public().(ed25519.PublicKey)

		now := time.Now().Format(time.RFC3339)

		// Convert seed []byte to []int for JSON decimal array
		seedInt := make([]int, len(seed))
		for i, b := range seed {
			seedInt[i] = int(b)
		}

		kp := Keypair{
			Seed:      seed,
			CreatedAt: now,
			LastUsed:  now,
			PublicKey: pubKey,
			PrivateKey: privKey,
		}

		// Prepare JSON with seed as decimal array
		type jsonKeypair struct {
			Seed      []int  `json:"seed"`
			CreatedAt string `json:"createdAt"`
			LastUsed  string `json:"lastUsed"`
		}

		jkp := jsonKeypair{
			Seed:      seedInt,
			CreatedAt: now,
			LastUsed:  now,
		}

		_ = os.MkdirAll(keyDir, os.ModePerm)
		kpStr, err := json.MarshalIndent(jkp, "", "  ")
		if err != nil {
			log.Fatal("Error marshalling keypair: ", err)
		}
		err = os.WriteFile(keyFile, kpStr, 0644)
		if err != nil {
			log.Fatal("Error writing keypair to file: ", err)
		}
		log.Printf("[KeyPair] Generated new and saved to %s", keyFile)
		return kp
	}
}

// getDataDir gets the directory where the configuration is stored
func getDataDir() string {
	if dir := os.Getenv("SIGHTAI_DATA_DIR"); dir != "" {
		return dir + "/config"
	}
	return os.Getenv("HOME") + "/.sightai/config"
}

// CreateLibp2pNode creates a libp2p node and returns the host and pubsub service
func CreateLibp2pNode(ctx context.Context, port int, bootstrapList []string) (hostlibp2p.Host, *pubsub.PubSub) {
	priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 0)
	if err != nil {
		log.Fatal("Failed to generate keypair: ", err)
	}
	h, err := libp2p.New(
		libp2p.DefaultMuxers,
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)),
		libp2p.Identity(priv),
	)
	if err != nil {
		log.Fatal("Failed to create libp2p host: ", err)
	}
	log.Printf("Libp2p Host created with peer ID: %s", h.ID())

	pubsubService, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		log.Fatal("Failed to create pubsub service: ", err)
	}

	// Optionally add bootstrap nodes
	if len(bootstrapList) > 0 {
		var peerAddrs []peer.AddrInfo
		for _, addr := range bootstrapList {
			info, err := peer.AddrInfoFromString(addr)
			if err != nil {
				log.Printf("Invalid bootstrap addr: %s (%v)", addr, err)
				continue
			}
			peerAddrs = append(peerAddrs, *info)
		}
		for _, info := range peerAddrs {
			if err := h.Connect(ctx, info); err != nil {
				log.Printf("Failed to connect to %s: %v", info.ID, err)
			} else {
				log.Printf("Connected to bootstrap peer: %s", info.ID)
			}
		}
	}
	return h, pubsubService
}

// ToSightDID generates a DID for the node from the public key
func ToSightDID(publicKey []byte) string {
	multicodec := append([]byte{0xed, 0x01}, publicKey...)
	return "did:sight:hoster:" + base58.Encode(multicodec)
}

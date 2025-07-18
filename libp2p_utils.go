package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"crypto/rand"

	"strings"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	hostlibp2p "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multihash"
	"golang.org/x/crypto/ed25519"
)

// Keypair struct to hold the keypair data
type Keypair struct {
	Seed       []byte `json:"seed"`
	CreatedAt  string `json:"createdAt"`
	LastUsed   string `json:"lastUsed"`
	PublicKey  []byte `json:"publicKey,omitempty"`
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
			Seed:       seed,
			CreatedAt:  tmp.CreatedAt,
			LastUsed:   tmp.LastUsed,
			PublicKey:  pubKey,
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
			Seed:       seed,
			CreatedAt:  now,
			LastUsed:   now,
			PublicKey:  pubKey,
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

// 和 local backend 逻辑一致：支持 Docker 和本地
func getDataDir() string {
	// 首先检查是否设置了 SIGHTAI_DATA_DIR（Docker 环境）
	if dataDir := os.Getenv("SIGHTAI_DATA_DIR"); dataDir != "" {
		return filepath.Join(dataDir, "config")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Warning: Could not get user home directory: %v", err)
	}

	return filepath.Join(homeDir, ".sightai", "config")
}

// CreateLibp2pNode creates a libp2p node and returns the host and pubsub service
func CreateLibp2pNode(ctx context.Context, port int, bootstrapList []string, kp Keypair) (hostlibp2p.Host, *pubsub.PubSub, *dht.IpfsDHT) {
	priv, err := crypto.UnmarshalEd25519PrivateKey(kp.PrivateKey)
	if err != nil {
		log.Fatal("Failed to unmarshal ed25519 private key: ", err)
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

	// DHT
	myDHT, err := dht.New(ctx, h, dht.Mode(dht.ModeServer))
	if err != nil {
		log.Fatal("Failed to create DHT: ", err)
	}
	go func() {
		if err := myDHT.Bootstrap(ctx); err != nil {
			log.Printf("[DHT] Bootstrap error: %v", err)
		} else {
			log.Printf("[DHT] Bootstrapped and ready")
		}
	}()

	return h, pubsubService, myDHT
}

// ToSightDID generates a DID for the node from the public key
func ToSightDID(publicKey []byte) string {
	multicodec := append([]byte{0xed, 0x01}, publicKey...)
	return "did:sight:hoster:" + base58.Encode(multicodec)
}

// peerId -> MultiAddr
func FindPeerAddr(ctx context.Context, dhtNode *dht.IpfsDHT, peerIdStr string) ([]string, error) {
	pid, err := peer.Decode(peerIdStr)
	if err != nil {
		return nil, err
	}
	info, err := dhtNode.FindPeer(ctx, pid)
	if err != nil {
		return nil, err
	}
	var addrs []string
	for _, addr := range info.Addrs {
		addrs = append(addrs, addr.String())
	}
	return addrs, nil
}

func DecodePublicKeyFromPeerId(peerId string) ([]byte, error) {
	// 解码peerId
	c, err := cid.Decode(peerId)
	if err != nil {
		// 如果不是cid格式，尝试base58解码
		decoded, _ := base58.Decode(peerId)
		if len(decoded) == 0 {
			return nil, fmt.Errorf("invalid PeerId format")
		}
		// 解码 multihash
		mh, err := multihash.Decode(decoded)
		if err != nil {
			return nil, fmt.Errorf("multihash decode failed: %v", err)
		}
		// 判断是否identity，有identity，可以反推出公钥
		if mh.Code == multihash.IDENTITY {
			return mh.Digest, nil
		}
		return nil, fmt.Errorf("peerid does not embed public key")
	}

	// 是cid格式，尝试identity反推公钥
	mh := c.Hash()
	decodedMh, err := multihash.Decode(mh)
	if err != nil {
		return nil, fmt.Errorf("multihash decode failed: %v", err)
	}

	if decodedMh.Code == multihash.IDENTITY {
		return decodedMh.Digest, nil
	}
	return nil, fmt.Errorf("peerid does not embed public key (not identity multihash)")
}

func DIDToPublicKey(did string) ([]byte, error) {
	const prefix = "did:sight:hoster:"
	if !strings.HasPrefix(did, prefix) {
		return nil, fmt.Errorf("not a valid sight DID")
	}
	encoded := did[len(prefix):]
	decoded, err := base58.Decode(encoded)
	if err != nil {
		return nil, err
	}
	if len(decoded) != 34 || decoded[0] != 0xed || decoded[1] != 0x01 {
		return nil, fmt.Errorf("not a valid ed25519 encoded key")
	}
	return decoded[2:], nil
}

func PublicKeyToPeerId(pub []byte) (peer.ID, error) {
	pk, err := crypto.UnmarshalEd25519PublicKey(pub)
	if err != nil {
		return "", err
	}
	return peer.IDFromPublicKey(pk)
}

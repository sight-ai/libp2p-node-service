package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/mr-tron/base58"
)

type Libp2pNodeController struct {
	service *Libp2pNodeService
}

func NewLibp2pNodeController(service *Libp2pNodeService) *Libp2pNodeController {
	return &Libp2pNodeController{service}
}

func (c *Libp2pNodeController) SendHandler(w http.ResponseWriter, r *http.Request) {
	var tunnelMsg map[string]interface{}
	err := json.NewDecoder(r.Body).Decode(&tunnelMsg)
	if err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}
	libp2pMsg := map[string]interface{}{
		"to":      tunnelMsg["to"],
		"payload": tunnelMsg,
	}
	c.service.HandleOutgoingMessage(libp2pMsg)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// PeerId -> MultiAddr
func (c *Libp2pNodeController) FindPeerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	peerIdStr := vars["peerId"]

	// println(`try to find `, peerIdStr)

	addrs, err := FindPeerAddr(r.Context(), c.service.dht, peerIdStr)
	if err != nil {
		http.Error(w, "Peer not found: "+err.Error(), 404)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"peerId": peerIdStr,
		"addrs":  addrs,
	})
}

// PeerId -> PublicKey(bs58)
func (c *Libp2pNodeController) GetPublicKeyHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	peerIdStr := vars["peerId"]

	// println(`try to find `, peerIdStr)

	pubKeyBytes, err := c.service.GetPublicKeyByPeerId(r.Context(), peerIdStr)
	if err != nil {
		http.Error(w, "Failed to get public key: "+err.Error(), 404)
		return
	}

	pubKeyStr := base58.Encode(pubKeyBytes)

	json.NewEncoder(w).Encode(map[string]string{
		"peerId":    peerIdStr,
		"publicKey": pubKeyStr,
	})
}

// ConnectHandler 支持 MultiAddr 或 DID 输入，进行连接
func (c *Libp2pNodeController) ConnectHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	did := vars["did"]

	err := c.service.ConnectByDIDOrMultiAddr(r.Context(), did)
	if err != nil {
		http.Error(w, "Failed to connect: "+err.Error(), 400)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"status":        "connected",
		"did/multiAddr": did,
	})
}

func (c *Libp2pNodeController) GetNeighborsHandler(w http.ResponseWriter, r *http.Request) {
	neighbors := c.service.GetNeighbors()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"neighbors": neighbors,
	})
}

func (c *Libp2pNodeController) PingHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	did := vars["did"]

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rtt, err := c.service.PingPeer(ctx, did)
	if err != nil {
		http.Error(w, "Ping failed: "+err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"did/multiAddr": did,
		"rtt_ms":        rtt,
	})
}

func (c *Libp2pNodeController) SendDirectHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	did := vars["did"]
	var msg map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}
	payload, _ := json.Marshal(msg)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	err := c.service.SendDirectMessage(ctx, did, payload)
	if err != nil {
		http.Error(w, "Send failed: "+err.Error(), 500)
		return
	}
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

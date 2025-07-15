package main

import (
	"encoding/json"
	"net/http"

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

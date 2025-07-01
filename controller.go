package main

import (
	"encoding/json"
	"net/http"
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

package main

import (
	"context"
	_ "embed"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"github.com/joho/godotenv"
)

//go:embed .env
var embeddedEnv string

func main() {
	// Try to load from embedded .env first, then from file system
	err := loadEnvVars()
    if err != nil {
        log.Println("Warning: Failed to load environment variables:", err)
    }
	// Load or generate keypair
	keypair := LoadOrGenerateKeypair()

	// Get environment variables (with defaults)
	isGateway := os.Getenv("IS_GATEWAY") == "1"
	nodePort := getEnvInt("NODE_PORT", 15050)
	libp2pPort := getEnvInt("LIBP2P_PORT", 4010)
	bootstrap := strings.Split(os.Getenv("BOOTSTRAP_ADDRS"), ",")
	// log.Println("bootstrap nodes:", bootstrap)
	tunnelAPI := "http://localhost:" + os.Getenv("API_PORT") + "/libp2p/message"

	// Create the Libp2p service
	service := NewLibp2pNodeService(keypair, nodePort, tunnelAPI, isGateway, bootstrap)
	service.InitNode()

	// Create the controller
	controller := NewLibp2pNodeController(service)

	// Set up router
	router := mux.NewRouter()
	router.HandleFunc("/libp2p/send", controller.SendHandler).Methods("POST")

	// Start the HTTP server
	srv := &http.Server{
		Handler: router,
		Addr:    ":" + strconv.Itoa(libp2pPort),
	}

	// Run server in a goroutine
	go func() {
		log.Printf("HTTP server started on :%d", libp2pPort)
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	log.Println("Shutting down...")
	service.Stop()
	srv.Shutdown(context.Background())
}

func loadEnvVars() error {
	// First try to load from file system (for development)
	if err := godotenv.Load(); err == nil {
		log.Println("Loaded environment variables from .env file")
		return nil
	}

	// If file doesn't exist, try to load from embedded content
	if embeddedEnv != "" {
		envMap, err := godotenv.Unmarshal(embeddedEnv)
		if err != nil {
			return err
		}

		// Set environment variables from embedded content
		for key, value := range envMap {
			if os.Getenv(key) == "" { // Only set if not already set
				os.Setenv(key, value)
			}
		}
		log.Println("Loaded environment variables from embedded .env")
		return nil
	}

	log.Println("Using system environment variables only")
	return nil
}

func getEnvInt(key string, defaultVal int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultVal
	}
	intVal, err := strconv.Atoi(value)
	if err != nil {
		return defaultVal
	}
	return intVal
}

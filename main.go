package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
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

// CLI flags
var (
	nodePort      = flag.String("node-port", "", "Node port (overrides NODE_PORT)")
	libp2pPort    = flag.String("libp2p-port", "", "Libp2p REST API port (overrides LIBP2P_REST_API)")
	apiPort       = flag.String("api-port", "", "API port (overrides API_PORT)")
	isGateway     = flag.String("is-gateway", "", "Is gateway (0 or 1, overrides IS_GATEWAY)")
	bootstrapAddrs = flag.String("bootstrap-addrs", "", "Bootstrap addresses (comma-separated, overrides BOOTSTRAP_ADDRS)")
	showHelp      = flag.Bool("help", false, "Show help message")
)

func main() {
	// Parse command line flags
	flag.Parse()

	// Show help if requested
	if *showHelp {
		showUsage()
		return
	}

	// Load environment variables (embedded .env or file system)
	err := loadEnvVars()
    if err != nil {
        log.Println("Warning: Failed to load environment variables:", err)
    }

    // Override with CLI flags if provided
    overrideWithCLIFlags()
	// Load or generate keypair
	keypair := LoadOrGenerateKeypair()

	// Get environment variables (with defaults)
	isGatewayFlag := os.Getenv("IS_GATEWAY") == "1"
	nodePortInt := getEnvInt("NODE_PORT", 15050)
	libp2pPortInt := getEnvInt("LIBP2P_REST_API", 4010)
	bootstrap := strings.Split(os.Getenv("BOOTSTRAP_ADDRS"), ",")
	// log.Println("bootstrap nodes:", bootstrap)
	tunnelAPI := "http://localhost:" + getEnvWithDefault("API_PORT", "8716") + "/libp2p/message"

	// Create the Libp2p service
	service := NewLibp2pNodeService(keypair, nodePortInt, tunnelAPI, isGatewayFlag, bootstrap)
	service.InitNode()

	// Create the controller
	controller := NewLibp2pNodeController(service)

	// Set up router
	router := mux.NewRouter()
	router.HandleFunc("/libp2p/send", controller.SendHandler).Methods("POST")

	// Start the HTTP server
	srv := &http.Server{
		Handler: router,
		Addr:    ":" + strconv.Itoa(libp2pPortInt),
	}

	// Run server in a goroutine
	go func() {
		log.Printf("HTTP server started on :%d", libp2pPortInt)
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

func showUsage() {
	fmt.Println("Sight Libp2p Node Service")
	fmt.Println("Usage: sight-libp2p-node [options]")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  --node-port <port>        Node port (default: 15050)")
	fmt.Println("  --libp2p-port <port>      Libp2p REST API port (default: 4010)")
	fmt.Println("  --api-port <port>         API port (default: 8716)")
	fmt.Println("  --is-gateway <0|1>        Is gateway mode (default: 0)")
	fmt.Println("  --bootstrap-addrs <addrs> Bootstrap addresses (comma-separated)")
	fmt.Println("  --help                    Show this help message")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  # Use default configuration")
	fmt.Println("  ./sight-libp2p-node")
	fmt.Println("")
	fmt.Println("  # Override specific settings")
	fmt.Println("  ./sight-libp2p-node --node-port 15051 --is-gateway 1")
	fmt.Println("")
	fmt.Println("  # Use benchmark environment")
	fmt.Println("  ./sight-libp2p-node --node-port 25050 --libp2p-port 5010 --api-port 9716")
}

func overrideWithCLIFlags() {
	if *nodePort != "" {
		os.Setenv("NODE_PORT", *nodePort)
		log.Printf("CLI override: NODE_PORT = %s", *nodePort)
	}
	if *libp2pPort != "" {
		os.Setenv("LIBP2P_REST_API", *libp2pPort)
		log.Printf("CLI override: LIBP2P_REST_API = %s", *libp2pPort)
	}
	if *apiPort != "" {
		os.Setenv("API_PORT", *apiPort)
		log.Printf("CLI override: API_PORT = %s", *apiPort)
	}
	if *isGateway != "" {
		os.Setenv("IS_GATEWAY", *isGateway)
		log.Printf("CLI override: IS_GATEWAY = %s", *isGateway)
	}
	if *bootstrapAddrs != "" {
		os.Setenv("BOOTSTRAP_ADDRS", *bootstrapAddrs)
		log.Printf("CLI override: BOOTSTRAP_ADDRS = %s", *bootstrapAddrs)
	}
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

	log.Println("Using system environment variables with defaults")
	return nil
}

func getEnvWithDefault(key, defaultVal string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultVal
	}
	return value
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

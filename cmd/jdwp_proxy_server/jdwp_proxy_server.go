package main

import (
	log "github.com/sirupsen/logrus"
	"jdwp-http/server"
	"os"
)

var (
	serverHTTPPort         = 8080
	jdwpPort               = 8000
	serverTCPClientAddress = "127.0.0.1"
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.TextFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.DebugLevel)
}

func main() {
	startServer()
}

func startServer() {
	log.Info("start server...")

	httpServer := server.NewHTTPServer(serverHTTPPort, jdwpPort, server.NewTCPClient(serverTCPClientAddress))
	err := httpServer.Start()
	if err != nil {
		log.Errorf("start server failed: %v", err)
	}
}

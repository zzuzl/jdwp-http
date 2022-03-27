package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"jdwp-http/client"
	"os"
)

var (
	clientTCPPort     = 6666
	serverHTTPPort    = 8080
	serverHTTPAddress = fmt.Sprintf("http://127.0.0.1:%d/", serverHTTPPort)
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
	startClient()
}

func startClient() {
	log.Info("start client...")

	tcpServer := client.NewTCPServer(clientTCPPort, client.NewHTTPClient(serverHTTPAddress))
	err := tcpServer.Start()
	if err != nil {
		log.Errorf("start client failed: %v", err)
	}
}

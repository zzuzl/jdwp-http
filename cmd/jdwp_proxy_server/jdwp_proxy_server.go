package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	"jdwp-http/server"
	"os"
)

var (
	serverHTTPPort = flag.Int("listen-http-port", 8080, "http port to listen")
	jdwpAddress    = flag.String("jdwp-addr", "127.0.0.1:8000", "jdwp address")
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetReportCaller(false)
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp: false,
		TimestampFormat:  "2006-01-02 15:04:05.000000",
		FullTimestamp:    true,
	})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.InfoLevel)
}

func main() {
	startServer()
}

func startServer() {
	log.Info("start server...")

	httpServer := server.NewHTTPServer(*serverHTTPPort, server.NewTCPClient(*jdwpAddress))
	err := httpServer.Start()
	if err != nil {
		log.Errorf("start server failed: %v", err)
	}
}

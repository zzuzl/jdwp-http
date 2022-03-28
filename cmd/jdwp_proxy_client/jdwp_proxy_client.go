package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	"jdwp-http/client"
	"os"
)

var (
	clientTCPPort     = flag.Int("tcp-port", 6666, "client local tcp port to listen")
	serverHTTPAddress = flag.String("http-addr", "http://127.0.0.1:8080/", "remote http server addr to request")
)

func init() {
	// init command args
	flag.Parse()

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
	startClient()
}

func startClient() {
	log.Info("start client...")

	tcpServer := client.NewTCPServer(*clientTCPPort, client.NewHTTPClient(*serverHTTPAddress))
	err := tcpServer.Start()
	if err != nil {
		log.Errorf("start client failed: %v", err)
	}
}

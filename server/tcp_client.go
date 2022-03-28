package server

import (
	"fmt"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"net"
)

type TCPClient struct {
	server string
}

func NewTCPClient(jdwpAddress string) *TCPClient {
	return &TCPClient{
		server: jdwpAddress,
	}
}

func (client *TCPClient) Connect() (*net.TCPConn, error) {
	addr := client.server
	log.Infof("connecting tcp server: %s", addr)

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, errors.Wrap(err, "can't resolve addr:"+addr)
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("can't connect %s", client.server))
	}
	log.Infof("tcp server connected: %s", addr)
	return conn, nil
}

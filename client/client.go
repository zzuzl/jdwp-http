package client

import "jdwp-http/connection"

type HTTPClient struct {
	serverAddr string
	clientID   string
}

func NewHTTPClient(serverAddr string) *HTTPClient {
	return &HTTPClient{
		serverAddr: serverAddr,
	}
}

func (h *HTTPClient) Connect(clientID string) (*connection.HTTPConn, error) {
	return connection.NewHTTPConn(h.serverAddr, clientID), nil
}

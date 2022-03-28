package server

import (
	"fmt"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"jdwp-http/connection"
	"jdwp-http/protocol"
	"net"
	"net/http"
	"sync"
	"time"
)

type remoteClient struct {
	clientConn *net.TCPConn
	onceRecv   sync.Once
	stopCh     <-chan struct{}
	packets    chan *protocol.WrappedPacket
}

type HTTPServer struct {
	port     int
	client   *TCPClient
	clients  map[string]*remoteClient
	clientMu sync.RWMutex
}

func NewHTTPServer(port int, client *TCPClient) *HTTPServer {
	return &HTTPServer{
		port:    port,
		client:  client,
		clients: make(map[string]*remoteClient),
	}
}

func (s *HTTPServer) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	log.Infof("starting http server: %s", addr)
	err := s.listen(addr)
	return errors.Wrap(err, "listen server")
}

func (s *HTTPServer) listen(addr string) error {
	http.HandleFunc("/", s.onMessage)
	http.HandleFunc("/handshark", s.onNewClient)
	if err := http.ListenAndServe(addr, nil); err != nil {
		return errors.Wrap(err, "http listen and serve")
	}
	return nil
}

func (s *HTTPServer) onNewClient(w http.ResponseWriter, request *http.Request) {
	var clientConn *net.TCPConn
	var err error

	defer closeOnFail(clientConn, err)

	clientID := request.Header.Get("client")
	clientConn, err = s.client.Connect()
	if err != nil {
		log.Errorf("can't connect to jvm:%v", err)
		return
	}

	connection.SetTCPConnOptions(clientConn)
	clientConn.SetWriteDeadline(time.Now().Add(connection.HandSharkDeadlineDuration))
	if err = protocol.SendHandShark(clientConn); err != nil {
		log.Errorf("send handshark fialed: %s", err.Error())
		return
	}
	log.Infof("sent handshark to jdwp server, client:%s", clientID)

	clientConn.SetReadDeadline(time.Now().Add(connection.HandSharkDeadlineDuration))
	if err = protocol.ReadCheckHandShark(clientConn); err != nil {
		log.Errorf("read handshark fialed: %s", err.Error())
		return
	}
	log.Infof("got and checked handshark for:%s", clientID)

	writeResp(w, http.StatusOK, protocol.Handshaking)
	client := s.addRemoteClient(clientID, clientConn)

	client.onceRecv.Do(func() {
		go receive(clientConn, client.stopCh, client.packets)
	})
}

func (s *HTTPServer) onMessage(w http.ResponseWriter, request *http.Request) {
	var clientConn *net.TCPConn
	var err error

	defer closeOnFail(clientConn, err)

	clientID := request.Header.Get("client")
	c := s.getRemoteClient(clientID)
	if c == nil {
		log.Errorf("not found client for:%s", clientID)
		writeResp(w, http.StatusForbidden, "not found client")
		return
	}
	clientConn = c.clientConn
	connection.SetTCPConnOptions(clientConn)
	defer request.Body.Close()

	var body []byte
	if body, err = ioutil.ReadAll(request.Body); err != nil {
		log.Errorf("read body :%s, err:%v", clientID, err)
		writeResp(w, http.StatusInternalServerError, "read body err")
		return
	}

	var packets []*protocol.WrappedPacket
	if packets, err = connection.ReadPackets(body); err != nil {
		log.Errorf("deserialize packet :%s, err:%v", clientID, err)
		writeResp(w, http.StatusInternalServerError, "deserialize packet err")
		return
	}

	if len(packets) > 0 {
		log.Infof("receive packets: %d", len(packets))
	}

	for _, p := range packets {
		clientConn.SetWriteDeadline(time.Now().Add(connection.WriteDeadlineDuration))
		if err = protocol.WritePacket(clientConn, p); err != nil {
			log.Errorf("write packet :%s, err:%v", clientID, err)
			writeResp(w, http.StatusInternalServerError, "write packet err")
			return
		}
		log.Infof("write packet to jdwp server:%s", p)
	}

	var respStr string
	returnPackets := findSomePacketsOfClient(c)
	if len(returnPackets) > 0 {
		if respStr, err = connection.ConvertToBase64String(returnPackets); err != nil {
			log.Errorf("convert packets to base64 string :%s, err:%v", clientID, err)
			writeResp(w, http.StatusInternalServerError, "convert packets to base64 string err")
			return
		}
	}
	writeResp(w, http.StatusOK, respStr)
}

func findSomePacketsOfClient(c *remoteClient) []*protocol.WrappedPacket {
	packets := make([]*protocol.WrappedPacket, 0)

	length := len(c.packets)
	if length == 0 {
		return packets
	}

	for len(packets) < length {
		p, ok := <-c.packets
		if !ok || p == nil {
			break
		}
		if len(packets) >= 10 {
			break
		}
		packets = append(packets, p)
	}
	log.Infof("find %d packets of client", len(packets))
	return packets
}

func (s *HTTPServer) addRemoteClient(clientID string, clientConn *net.TCPConn) *remoteClient {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()

	client := s.clients[clientID]
	if client != nil {
		log.Warnf("client had exists: %s", clientID)
		return client
	}

	client = &remoteClient{
		clientConn: clientConn,
		packets:    make(chan *protocol.WrappedPacket, 100),
		stopCh:     make(chan struct{}),
	}
	s.clients[clientID] = client
	log.Infof("add remote client: %s", clientID)
	return client
}

func (s *HTTPServer) getRemoteClient(clientID string) *remoteClient {
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()

	client := s.clients[clientID]
	if client == nil {
		log.Warnf("client not found for: %s", clientID)
		return nil
	}
	return client
}

func (s *HTTPServer) removeRemoteClient(conn *connection.HTTPConn, clientConn *net.TCPConn) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	// todo
}

func receive(clientConn *net.TCPConn, stopCh <-chan struct{}, packets chan *protocol.WrappedPacket) {
	defer closeConn(clientConn)

	var err error
	for {
		if clientConn == nil {
			log.Warnf("clientConn is:%v", clientConn)
			return
		}

		select {
		case <-stopCh:
			log.Info("received stop sig")
			return
		default:
			clientConn.SetReadDeadline(time.Now().Add(connection.DeadlineDuration))
			var packet *protocol.WrappedPacket
			if packet, err = protocol.ReadPacket(clientConn); err != nil {
				log.Errorf("read packet failed:%v", err)
				return
			}
			log.Infof("read packet from jdwp server:%s", packet)
			packets <- packet
		}
	}
}

func writeResp(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	fmt.Fprintf(w, msg)
}

func closeConn(clientConn *net.TCPConn) {
	log.Info("connection was disconnected")

	if err := recover(); err != nil {
		log.Errorf("recover from:%s", err)
	}
	if clientConn != nil {
		clientConn.Close()
	}
}

func closeOnFail(clientConn *net.TCPConn, err error) {
	if e := recover(); e != nil {
		log.Errorf("got err in handle:%s", e)
		if err == nil {
			err = fmt.Errorf("%v", e)
		}
	}
	if err != nil {
		if clientConn != nil {
			clientConn.Close()
		}
	}
}

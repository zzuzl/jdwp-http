package client

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"jdwp-http/connection"
	"jdwp-http/protocol"
	"net"
	"time"
)

type TCPServer struct {
	client *HTTPClient
	port   int
}

func NewTCPServer(port int, client *HTTPClient) *TCPServer {
	return &TCPServer{
		client: client,
		port:   port,
	}
}

func (server *TCPServer) Start() error {
	addr := fmt.Sprintf(":%d", server.port)
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return errors.Wrap(err, "can't resolve addr:%s"+addr)
	}

	err = server.listen(tcpAddr)
	if err != nil {
		return errors.Wrap(err, "listen error")
	}
	return nil
}

func (server *TCPServer) listen(addr *net.TCPAddr) error {
	log.Infof("starting tcp server: %s", addr.String())

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return errors.Wrap(err, "listen tcp")
	}
	return server.accept(listener)
}

func (server *TCPServer) accept(listener *net.TCPListener) error {
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			return errors.Wrap(err, "accept tcp")
		}
		server.handle(conn)
	}
}

func (server *TCPServer) handle(conn *net.TCPConn) {
	var clientConn *connection.HTTPConn
	var err error
	defer closeOnFail(conn, clientConn)

	clientID := genClientID()
	log.Infof("got tcp connection from %s, use clientID: %s", conn.RemoteAddr(), clientID)

	clientConn, err = server.client.Connect(clientID)
	if err != nil {
		log.Errorf("can't connect to server %s", err)
		return
	}

	connection.SetTCPConnOptions(conn)

	go send(conn, clientConn)
	go loopFetch(conn, clientConn)
}

func send(conn *net.TCPConn, clientConn *connection.HTTPConn) {
	defer closeConn(conn, clientConn)

	var err error
	// check handshark first
	if clientConn == nil || conn == nil {
		log.Warnf("conn is:%v, clientConn is:%v", conn, clientConn)
		return
	}
	conn.SetReadDeadline(time.Now().Add(connection.HandSharkDeadlineDuration))
	if err = protocol.ReadCheckHandShark(conn); err != nil {
		log.Errorf("read handshark fialed: %s", err.Error())
		return
	}
	log.Info("got and checked handshark")

	if err = clientConn.SendHandShark(); err != nil {
		log.Errorf("send handshark fialed: %s", err.Error())
		return
	}
	log.Info("sent handshark to remote")

	conn.SetWriteDeadline(time.Now().Add(connection.HandSharkDeadlineDuration))
	if err = protocol.SendHandShark(conn); err != nil {
		log.Errorf("send handshark fialed: %s", err.Error())
		return
	}
	log.Info("sent handshark to client")

	var packet *protocol.WrappedPacket
	for {
		conn.SetReadDeadline(time.Now().Add(connection.DeadlineDuration))
		if packet, err = protocol.ReadPacket(conn); err != nil {
			log.Errorf("read tcp fialed: %s", err)
			return
		}
		log.Infof("read packet from local client: %v", packet)

		var resp []byte
		if resp, err = clientConn.SendData(packet.Serialize()); err != nil {
			log.Errorf("send data fialed: %s", err)
			return
		}

		var packets []*protocol.WrappedPacket
		if packets, err = clientConn.ParsePacketsFromResp(resp); err != nil {
			log.Errorf("parse packets fialed: %s", err)
			return
		}

		for _, p := range packets {
			conn.SetWriteDeadline(time.Now().Add(connection.WriteDeadlineDuration))
			if err = protocol.WritePacket(conn, p); err != nil {
				log.Errorf("write packet fialed: %s", err)
				return
			}
		}
	}
}

func loopFetch(conn *net.TCPConn, clientConn *connection.HTTPConn) {
	defer closeConn(conn, clientConn)

	interval := time.Millisecond * 100
	ticker := time.NewTicker(interval)
	var err error
	defer ticker.Stop()

	log.Infof("start loop fetch, interval:%s", interval)
	for range ticker.C {
		if clientConn == nil || conn == nil {
			log.Warnf("conn is:%v, clientConn is:%v", conn, clientConn)
			return
		}

		var resp []byte
		if resp, err = clientConn.FetchPackets(); err != nil {
			log.Errorf("fetch packets fialed: %s", err)
			return
		}

		var packets []*protocol.WrappedPacket
		if packets, err = clientConn.ParsePacketsFromResp(resp); err != nil {
			log.Errorf("parse packets fialed: %s", err)
			return
		}

		for _, p := range packets {
			conn.SetWriteDeadline(time.Now().Add(connection.WriteDeadlineDuration))
			if err = protocol.WritePacket(conn, p); err != nil {
				log.Errorf("write packet fialed: %s", err)
				return
			}
		}
	}
}

func genClientID() string {
	id, _ := uuid.NewUUID()
	return id.String()
}

func closeConn(conn *net.TCPConn, clientConn *connection.HTTPConn) {
	log.Info("connection was disconnected")

	if err := recover(); err != nil {
		log.Errorf("recover from:%s", err)
	}

	if conn != nil {
		conn.Close()
	}

	if clientConn != nil {
		clientConn.Close()
	}
}

func closeOnFail(conn *net.TCPConn, clientConn *connection.HTTPConn) {
	if err := recover(); err != nil {
		log.Errorf("got err in handle:%s", err)
		if conn != nil {
			conn.Close()
		}
		if clientConn != nil {
			clientConn.Close()
		}
	}
}

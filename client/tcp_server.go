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

type localClient struct {
	conn       *net.TCPConn
	clientConn *connection.HTTPConn
	packets    chan *protocol.WrappedPacket
}

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
	var err error

	lc := &localClient{
		conn:    conn,
		packets: make(chan *protocol.WrappedPacket, 100),
	}
	defer closeOnFail(lc, err)

	clientID := genClientID()
	log.Infof("got tcp connection from %s, use clientID: %s", conn.RemoteAddr(), clientID)

	lc.clientConn, err = server.client.Connect(clientID)
	if err != nil {
		log.Errorf("can't connect to server %s", err)
		return
	}

	connection.SetTCPConnOptions(conn)

	go readLocal(lc)
	go loopSendAndFetch(lc)
}

func readLocal(lc *localClient) {
	conn := lc.conn
	clientConn := lc.clientConn
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
		lc.packets <- packet
	}
}

func loopSendAndFetch(lc *localClient) {
	conn := lc.conn
	clientConn := lc.clientConn
	defer closeConn(conn, clientConn)

	interval := time.Millisecond * 100
	ticker := time.NewTicker(interval)
	var err error
	defer ticker.Stop()

	log.Infof("start loop send and fetch, interval:%s", interval)
	for range ticker.C {
		if clientConn == nil || conn == nil {
			log.Warnf("conn is:%v, clientConn is:%v", conn, clientConn)
			return
		}

		var packetStr = ""
		packetsOfClient := findSomePacketsOfClient(lc)
		if len(packetsOfClient) > 0 {
			if packetStr, err = connection.ConvertToBase64String(packetsOfClient); err != nil {
				log.Errorf("convert base64 fialed: %s", err)
				return
			}
		}

		var resp []byte
		if resp, err = clientConn.SendAndFetchPackets([]byte(packetStr)); err != nil {
			log.Errorf("send data fialed: %s", err)
			return
		}

		var packets []*protocol.WrappedPacket
		if packets, err = connection.ReadPackets(resp); err != nil {
			log.Errorf("parse packets fialed: %s", err)
			return
		}
		if len(packets) > 0 {
			log.Infof("read packets size:%d", len(packets))
		}

		for _, p := range packets {
			conn.SetWriteDeadline(time.Now().Add(connection.WriteDeadlineDuration))
			if err = protocol.WritePacket(conn, p); err != nil {
				log.Errorf("write packet fialed: %s", err)
				return
			}
			log.Infof("write packet from local client: %v", p)
		}
	}
}

func findSomePacketsOfClient(lc *localClient) []*protocol.WrappedPacket {
	packets := make([]*protocol.WrappedPacket, 0)

	length := len(lc.packets)
	if length == 0 {
		return packets
	}

	for len(packets) < length {
		p, ok := <-lc.packets
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

func closeOnFail(lc *localClient, err error) {
	if e := recover(); e != nil {
		log.Errorf("got err in handle:%s", e)
		if err == nil {
			err = fmt.Errorf("%v", e)
		}
	}
	if err != nil {
		if lc.conn != nil {
			lc.conn.Close()
		}
		if lc.clientConn != nil {
			lc.clientConn.Close()
		}
	}
}

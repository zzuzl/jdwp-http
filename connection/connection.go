package connection

import (
	"bytes"
	"encoding/base64"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"jdwp-http/proto/pb_gen"
	"jdwp-http/protocol"
	"net"
	"net/http"
	"time"
)

const (
	HandSharkDeadlineDuration = time.Millisecond * 500
	WriteDeadlineDuration     = time.Millisecond * 500
	DeadlineDuration          = time.Second * 60 * 30

	ReqTypePacket = "packet"
	ReqTypeFetch  = "fetch"
)

var (
	hClient = &http.Client{Timeout: 2 * time.Second}
)

type HTTPConn struct {
	address  string
	clientID string
}

func NewHTTPConn(addr, clientID string) *HTTPConn {
	return &HTTPConn{
		address:  addr,
		clientID: clientID,
	}
}

func (c *HTTPConn) SendHandShark() error {
	request, err := http.NewRequest(http.MethodPost, c.address+"handshark", bytes.NewBuffer([]byte(protocol.Handshaking)))
	if err != nil {
		return errors.Wrap(err, "new request")
	}
	if request.Header == nil {
		request.Header = http.Header{}
	}
	request.Header.Set("client", c.clientID)

	resp, err := hClient.Do(request)
	if err != nil {
		return errors.Wrap(err, "send request")
	}
	defer resp.Body.Close()
	return nil
}

func (c *HTTPConn) SendData(data []byte) ([]byte, error) {
	request, err := http.NewRequest(http.MethodPost, c.address, bytes.NewBuffer(data))
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}
	if request.Header == nil {
		request.Header = http.Header{}
	}
	request.Header.Set("type", ReqTypePacket)
	request.Header.Set("client", c.clientID)

	resp, err := hClient.Do(request)
	if err != nil {
		return nil, errors.Wrap(err, "send request")
	}
	defer resp.Body.Close()

	var body []byte
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read from http resp")
	}
	return body, nil
}

func (c *HTTPConn) FetchPackets() ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, c.address, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}
	if request.Header == nil {
		request.Header = http.Header{}
	}
	request.Header.Set("type", ReqTypeFetch)
	request.Header.Set("client", c.clientID)

	resp, err := hClient.Do(request)
	if err != nil {
		return nil, errors.Wrap(err, "send request")
	}
	defer resp.Body.Close()

	var body []byte
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read from http resp")
	}
	return body, nil
}

func ReadPackets(data []byte) ([]*protocol.WrappedPacket, error) {
	var err error
	packets := make([]*protocol.WrappedPacket, 0)

	if len(data) == 0 {
		return packets, nil
	}

	var decodeData []byte
	if decodeData, err = base64.StdEncoding.DecodeString(string(data)); err != nil {
		return nil, errors.Wrap(err, "decode base64")
	}

	ps := &pb_gen.Packets{}
	if err = proto.Unmarshal(decodeData, ps); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	for _, p := range ps.Packets {
		packet := &protocol.WrappedPacket{}
		if err = packet.FromPb(p); err != nil {
			return nil, errors.Wrap(err, "parse")
		}
		packets = append(packets, packet)
	}
	log.Infof("read packets size:%d", len(packets))
	return packets, nil
}

func ConvertToBase64String(packets []*protocol.WrappedPacket) (string, error) {
	if len(packets) == 0 {
		return "", nil
	}

	var err error
	ps := &pb_gen.Packets{
		Packets: make([]*pb_gen.Packet, 0, len(packets)),
	}
	for _, p := range packets {
		ps.Packets = append(ps.Packets, p.ToPb())
	}

	log.Infof("convert packets size:%d", len(packets))
	var data []byte
	if data, err = proto.Marshal(ps); err != nil {
		return "", errors.Wrap(err, "marshal packets")
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (c *HTTPConn) Close() error {
	return nil
}

//SetTCPConnOptions initialize tcp connection to keep alive
func SetTCPConnOptions(conn *net.TCPConn) {
	if conn == nil {
		return
	}
	conn.SetKeepAlive(true)
	conn.SetNoDelay(true)
	conn.SetLinger(3)
	conn.SetDeadline(time.Now().Add(DeadlineDuration))
}

package connection

import (
	"bytes"
	"github.com/pkg/errors"
	"io/ioutil"
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

func (c *HTTPConn) ParsePacketsFromResp(data []byte) ([]*protocol.WrappedPacket, error) {
	packets := make([]*protocol.WrappedPacket, 0)
	if len(data) > 0 {
		p := &protocol.WrappedPacket{}
		err := p.DeserializeFrom(data)
		if err != nil {
			return nil, errors.Wrap(err, "deserialize from resp")
		}
		packets = append(packets, p)
	}
	return packets, nil
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

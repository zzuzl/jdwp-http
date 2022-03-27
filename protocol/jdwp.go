package protocol

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"io"
	"jdwp-http/proto/pb_gen"
)

const (
	headerBytes      = 11
	Handshaking      = "JDWP-Handshake"
	flagsReplyPacket = 0x80
)

// WrappedPacket represents a command or reply packet
type WrappedPacket struct {
	id            uint32
	flags         byte
	commandPacket *CommandPacket
	replyPacket   *ReplyPacket
}

func (w *WrappedPacket) ID() uint32 {
	return w.id
}

func (w *WrappedPacket) Serialize() []byte {
	packet := pb_gen.Packet{}
	packet.Id = w.id
	packet.Flags = int32(w.flags)
	if w.commandPacket != nil {
		packet.CommandPacket = &pb_gen.CommandPacket{
			Commandset: int32(w.commandPacket.Commandset),
			Command:    int32(w.commandPacket.Command),
			Data:       w.commandPacket.Data,
		}
	}
	if w.replyPacket != nil {
		packet.ReplyPacket = &pb_gen.ReplyPacket{
			Errorcode: uint32(w.replyPacket.Errorcode),
			Data:      w.replyPacket.Data,
		}
	}
	data, _ := proto.Marshal(&packet)
	return []byte(base64.StdEncoding.EncodeToString(data))
}

func (w *WrappedPacket) DeserializeFrom(data []byte) error {
	decodeData, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return errors.Wrap(err, "base64 decode")
	}

	packet := pb_gen.Packet{}
	if err = proto.Unmarshal(decodeData, &packet); err != nil {
		return errors.Wrap(err, "Unmarshal proto")
	}
	w.id = packet.Id
	w.flags = byte(packet.Flags)
	if packet.CommandPacket != nil {
		w.commandPacket = &CommandPacket{
			Commandset: byte(packet.CommandPacket.Commandset),
			Command:    byte(packet.CommandPacket.Command),
			Data:       packet.CommandPacket.Data,
		}
	}
	if packet.ReplyPacket != nil {
		w.replyPacket = &ReplyPacket{
			Errorcode: uint16(packet.ReplyPacket.Errorcode),
			Data:      packet.ReplyPacket.Data,
		}
	}
	return nil
}

func (w *WrappedPacket) String() string {
	if w.commandPacket != nil {
		return fmt.Sprintf("{id=%v flags=%x commandpacket=%v", w.id, w.flags, w.commandPacket)
	}
	return fmt.Sprintf("{id=%v flags=%x replypacket=%v", w.id, w.flags, w.replyPacket)
}

// CommandPacket represents a command packet
type CommandPacket struct {
	Commandset byte
	Command    byte
	Data       []byte
}

func (c *CommandPacket) String() string {
	return fmt.Sprintf("{commandset=%v[TODO] command=%v[TODO] length=%v",
		c.Commandset, c.Command, len(c.Data))
}

// ReplyPacket represents a reply packet
type ReplyPacket struct {
	Errorcode uint16
	Data      []byte
}

func (r *ReplyPacket) String() string {
	return fmt.Sprintf("{errorcode=%v length=%v",
		r.Errorcode, len(r.Data))
}

func ReadCheckHandShark(conn io.Reader) error {
	buf := make([]byte, len(Handshaking))
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		return errors.Wrap(err, "read from conn")
	}
	if Handshaking != string(buf) {
		return fmt.Errorf("invalid data:%s", buf)
	}
	return nil
}

func SendHandShark(conn io.Writer) error {
	_, err := conn.Write([]byte(Handshaking))
	return err
}

func ReadPacket(conn io.Reader) (*WrappedPacket, error) {
	var wrappedPacket WrappedPacket
	var size uint32
	err := binary.Read(conn, binary.BigEndian, &size)
	if err != nil {
		return nil, err
	}
	if size < headerBytes {
		return nil, fmt.Errorf("packet too small: %d", size)
	}
	dataSize := size - headerBytes
	err = binary.Read(conn, binary.BigEndian, &wrappedPacket.id)
	if err != nil {
		return nil, errors.Wrap(err, "read id from conn")
	}
	err = binary.Read(conn, binary.BigEndian, &wrappedPacket.flags)
	if err != nil {
		return nil, errors.Wrap(err, "read flags from conn")
	}

	var dataSlice *[]byte
	if wrappedPacket.flags&flagsReplyPacket == flagsReplyPacket {
		var replyPacket ReplyPacket
		wrappedPacket.replyPacket = &replyPacket
		err = binary.Read(conn, binary.BigEndian, &replyPacket.Errorcode)
		if err != nil {
			return nil, errors.Wrap(err, "read Errorcode from conn")
		}
		dataSlice = &replyPacket.Data
	} else {
		var commandPacket CommandPacket
		wrappedPacket.commandPacket = &commandPacket
		err = binary.Read(conn, binary.BigEndian, &commandPacket.Commandset)
		if err != nil {
			return nil, errors.Wrap(err, "read Commandset from conn")
		}
		err = binary.Read(conn, binary.BigEndian, &commandPacket.Command)
		if err != nil {
			return nil, errors.Wrap(err, "read Command from conn")
		}
		dataSlice = &commandPacket.Data
	}

	*dataSlice = make([]byte, dataSize)
	_, err = io.ReadFull(conn, *dataSlice)
	if err != nil {
		return nil, errors.Wrap(err, "read packet data from conn")
	}
	return &wrappedPacket, nil
}

func WritePacket(conn io.Writer, p *WrappedPacket) error {
	var totalSize uint32 = 11
	if p.commandPacket != nil {
		totalSize += (uint32)(len(p.commandPacket.Data))
	} else if p.replyPacket != nil {
		totalSize += (uint32)(len(p.replyPacket.Data))
	} else {
		return fmt.Errorf("illegal packet")
	}

	err := binary.Write(conn, binary.BigEndian, totalSize)
	if err != nil {
		return errors.Wrap(err, "write size")
	}
	err = binary.Write(conn, binary.BigEndian, p.id)
	if err != nil {
		return errors.Wrap(err, "write id")
	}
	err = binary.Write(conn, binary.BigEndian, p.flags)
	if err != nil {
		return errors.Wrap(err, "write flags")
	}

	if p.commandPacket != nil {
		err = binary.Write(conn, binary.BigEndian, p.commandPacket.Commandset)
		if err != nil {
			return errors.Wrap(err, "write Commandset")
		}
		err = binary.Write(conn, binary.BigEndian, p.commandPacket.Command)
		if err != nil {
			return errors.Wrap(err, "write Command")
		}
		n, err := conn.Write(p.commandPacket.Data)
		if err != nil {
			return errors.Wrap(err, "write Data")
		}
		if n != len(p.commandPacket.Data) {
			return fmt.Errorf("did not write all bytes, got %v expect %v", n, len(p.commandPacket.Data))
		}
		return nil
	}

	err = binary.Write(conn, binary.BigEndian, p.replyPacket.Errorcode)
	if err != nil {
		return errors.Wrap(err, "write Errorcode")
	}
	n, err := conn.Write(p.replyPacket.Data)
	if err != nil {
		return errors.Wrap(err, "write Data")
	}
	if n != len(p.replyPacket.Data) {
		return fmt.Errorf("did not write all bytes, got %v expect %v", n, len(p.replyPacket.Data))
	}
	return nil
}

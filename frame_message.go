package rdialer

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"
)

type MessageType uint8

const (
	Connect MessageType = iota + 1
	KeepAlive
)

var (
	legacyDeadline = (15 * time.Second).Milliseconds()
	// 添加字节缓冲区对象池
	lenBufPool = sync.Pool{
		New: func() any {
			return make([]byte, 4)
		},
	}
)

// DecodeBuffer 结构体及其方法
type DecodeBuffer struct {
	MessageLength int
	MessageType   MessageType
	Buffer        []byte
}

// NewDecodeBuffer 创建一个新的解码缓冲区
func NewDecodeBuffer() *DecodeBuffer {
	return &DecodeBuffer{}
}

// ReadFrom 从io.Reader读取一个完整的消息
func (d *DecodeBuffer) ReadFrom(r io.Reader) (int64, error) {
	// 1. 首先读取消息长度 (4字节)
	lenBuf := lenBufPool.Get().([]byte)
	defer lenBufPool.Put(lenBuf)

	n1, err := io.ReadFull(r, lenBuf)
	if err != nil {
		return int64(n1), err
	}

	d.MessageLength = int(binary.BigEndian.Uint32(lenBuf))

	// 3. 先读取消息类型 (1字节)
	typeBuf := make([]byte, 1)
	n2, err := io.ReadFull(r, typeBuf)
	if err != nil {
		return int64(n1 + n2), err
	}
	d.MessageType = MessageType(typeBuf[0])

	// 4. 分配新的缓冲区并读取消息内容
	msgBuf := make([]byte, d.MessageLength-1)
	n3, err := io.ReadFull(r, msgBuf)
	if err != nil {
		return int64(n1 + n2 + n3), err
	}
	d.Buffer = msgBuf
	return int64(n3), nil
}

type EncodeBuffer struct {
	MessageLength int
	Buffer        []byte
}

// NewEncodeBuffer 创建一个新的编码缓冲区
func NewEncodeBuffer(messageType MessageType, data []byte) *EncodeBuffer {
	dataLen := len(data)
	totalLen := 4 + 1 + dataLen // 4字节长度 + 1字节消息类型 + 数据长度
	// 分配足够的空间：4字节长度 + 1字节消息类型 + 数据长度
	buffer := make([]byte, totalLen)

	// 先写入消息长度（4字节）
	binary.BigEndian.PutUint32(buffer[:4], uint32(1+dataLen)) // 消息类型(1字节) + 数据长度
	// 写入消息类型（第5个字节）
	buffer[4] = byte(messageType)
	// 写入数据（从第6个字节开始）
	if dataLen > 0 {
		copy(buffer[5:], data)
	}
	return &EncodeBuffer{
		MessageLength: totalLen,
		Buffer:        buffer,
	}
}

// Size 返回编码后的总大小
func (e *EncodeBuffer) Size() int {
	if e.Buffer == nil {
		return 0
	}
	return e.MessageLength
}

func (e *EncodeBuffer) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(e.Buffer)
	return int64(n), err
}

func SendConnectMessage(w io.Writer, proto, address string) (int64, error) {
	eb := NewEncodeBuffer(Connect, []byte(fmt.Sprintf("%s/%s", proto, address)))
	return eb.WriteTo(w)
}

func SendKeepAliveMessage(w io.Writer) (int64, error) {
	eb := NewEncodeBuffer(KeepAlive, nil)
	return eb.WriteTo(w)
}

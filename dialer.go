package rdialer

import (
	"context"
	"github.com/quic-go/webtransport-go"
	"net"
)

type Dialer func(ctx context.Context, network, address string) (net.Conn, error)

func toDialer(session *webtransport.Session, prefix string) Dialer {
	return func(ctx context.Context, proto, address string) (net.Conn, error) {
		stream, err := session.OpenStream()
		if err != nil {
			return nil, err
		}
		if prefix == "" {
			_, err := SendConnectMessage(stream, proto, address)
			if err != nil {
				return nil, err
			}
			return newConnection(stream, proto, address)
		}
		_, err = SendConnectMessage(stream, prefix+"::"+proto, address)
		if err != nil {
			return nil, err
		}
		return newConnection(stream, prefix+"::"+proto, address)
	}
}

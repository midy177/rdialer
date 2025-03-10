package rdialer

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/midy177/webtransport-go"
	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog/log"
	"net/http"
	"net/url"
	"time"
)

// Client 表示一个 WebTransport 客户端
type Client struct {
	serverURL *url.URL
	session   *webtransport.Session
}

// NewClient 创建一个新的 WebTransport 客户端
func NewClient(serverAddr string) (*Client, error) {
	serverURL, err := url.Parse(serverAddr)
	if err != nil {
		return nil, fmt.Errorf("FailedTo resolve server address: %w", err)
	}

	// 确保使用 HTTPS 协议
	if serverURL.Scheme != "https" {
		return nil, fmt.Errorf("WebTransport Require HTTPS protocol")
	}

	return &Client{
		serverURL: serverURL,
	}, nil
}

// Connect 连接到 WebTransport 服务器
func (c *Client) Connect(ctx context.Context, header http.Header) error {
	if c.session != nil {
		return nil // 已经连接
	}
	// 配置 TLS
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // 注意：生产环境应该使用有效证书
		NextProtos:         []string{"h3"},
	}

	// 配置 QUIC
	quicConfig := &quic.Config{
		MaxIncomingStreams: 100000, // 允许最多10000个并发双向流
		EnableDatagrams:    true,   // 启用数据报支持
	}

	// 创建 WebTransport 拨号器
	dialer := &webtransport.Dialer{
		TLSClientConfig: tlsConfig,
		QUICConfig:      quicConfig,
	}

	// 使用拨号器创建 WebTransport 会话
	resp, session, err := dialer.Dial(ctx, c.serverURL.String(), header)
	if err != nil {
		return fmt.Errorf("WebTransport Connection failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ConnectionRefused, status code: %d", resp.StatusCode)
	}
	go handleSession("local", session)
	stream, err := session.OpenStream()
	if err != nil {
		return fmt.Errorf("OpenStream failed: %w", err)
	}
	_, err = SendKeepAliveMessage(stream)
	if err != nil {
		return fmt.Errorf("SendKeepAliveMessage failed: %w", err)
	}
	c.session = session
	return c.keepalive(stream)
}

// Close 关闭客户端连接
func (c *Client) Close() error {
	var err error
	if c.session != nil {
		err = c.session.CloseWithError(0, "Client actively closes")
		c.session = nil
	}
	return err
}

// GetDialer 打开一个OpenDialer
func (c *Client) GetDialer() (Dialer, error) {
	if c.session == nil {
		return nil, fmt.Errorf("Session is nil")
	}
	return toDialer(c.session, ""), nil
}

func (c *Client) GetPrefixDialer(prefix string) (Dialer, error) {
	return toDialer(c.session, prefix), nil
}

func (c *Client) keepalive(stream webtransport.Stream) error {
	remoteAddr := c.session.RemoteAddr()
	localAddr := c.session.LocalAddr()
	streamID := stream.StreamID()
	log.Info().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).Msg("Successfully connected to rdialer")
	keepMsg := make([]byte, 1)
	for {
		time.Sleep(time.Second * 3)
		_, err := stream.Write(keepMsg)
		if err != nil {
			log.Error().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).Err(err).Msg("write keepalive")
			return err
		}
		log.Debug().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).
			Int64("StreamID", int64(streamID)).Str("Role", "client").Msg("send keepalive")
		_, err = stream.Read(keepMsg)
		if err != nil {
			log.Error().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).Err(err).Msg("read keepalive")
			return err
		}
		log.Debug().Str("LocalAddr", localAddr.String()).Str("RemoteAddr", remoteAddr.String()).
			Int64("StreamID", int64(streamID)).Str("Role", "client").Msg("read keepalive")
	}
}

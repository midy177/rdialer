package rdialer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

// Client 表示一个 WebTransport 客户端
type Client struct {
	serverURL    *url.URL
	session      *webtransport.Session
	roundTripper *http3.RoundTripper
	closed       bool
}

// NewClient 创建一个新的 WebTransport 客户端
func NewClient(serverAddr string) (*Client, error) {
	serverURL, err := url.Parse(serverAddr)
	if err != nil {
		return nil, fmt.Errorf("解析服务器地址失败: %w", err)
	}

	// 确保使用 HTTPS 协议
	if serverURL.Scheme != "https" {
		return nil, fmt.Errorf("WebTransport 需要 HTTPS 协议")
	}

	return &Client{
		serverURL: serverURL,
	}, nil
}

// Connect 连接到 WebTransport 服务器
func (c *Client) Connect(ctx context.Context, header http.Header) error {
	if c.closed {
		return fmt.Errorf("客户端已关闭")
	}

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
		return fmt.Errorf("WebTransport 连接失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("连接被拒绝，状态码: %d", resp.StatusCode)
	}
	go handleSession("local", session)
	c.session = session
	return nil
}

// Close 关闭客户端连接
func (c *Client) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	var err error
	if c.session != nil {
		err = c.session.CloseWithError(0, "客户端主动关闭")
		c.session = nil
	}
	if c.roundTripper != nil {
		c.roundTripper.Close()
		c.roundTripper = nil
	}

	return err
}

// GetDialer 打开一个OpenDialer
func (c *Client) GetDialer() (Dialer, error) {
	return toDialer(c.session, ""), nil
}

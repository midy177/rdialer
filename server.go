package rdialer

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/midy177/webtransport-go"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/rs/zerolog/log"
)

var (
	errFailedAuth = errors.New("failed authentication")
)

type Authorizer func(req *http.Request) (clientKey string, authed bool, err error)

func DefaultAuthorizer(req *http.Request) (clientKey string, authed bool, err error) {
	id := req.Header.Get("tunnel-id")
	return id, id != "", nil
}

type ErrorWriter func(rw http.ResponseWriter, req *http.Request, code int, err error)

func DefaultErrorWriter(rw http.ResponseWriter, req *http.Request, code int, err error) {
	rw.WriteHeader(code)
	_, _ = rw.Write([]byte(err.Error()))
}

// Server 表示一个 WebTransport 服务器
type Server struct {
	addr           string
	authorizer     Authorizer
	errorWriter    ErrorWriter
	sessions       *sessionManager
	wtServer       *webtransport.Server // WebTransport 服务器
	mu             *sync.Mutex
	certificate    string
	certificateKey string
	closed         bool
}

// ServerOption 定义服务器配置选项
type ServerOption func(*Server)

// WithTLSConfig 设置 TLS 配置
func WithTLSConfig(config *tls.Config) ServerOption {
	return func(s *Server) {
		s.wtServer.H3.TLSConfig = config
	}
}

// WithQUICConfig 设置 QUIC 配置
func WithQUICConfig(config *quic.Config) ServerOption {
	return func(s *Server) {
		s.wtServer.H3.QUICConfig = config
	}
}

func WithAuthorizer(authorizer Authorizer) ServerOption {
	return func(s *Server) {
		s.authorizer = authorizer
	}
}

func WithErrorWriter(errorWriter ErrorWriter) ServerOption {
	return func(s *Server) {
		s.errorWriter = errorWriter
	}
}

func WithHandleFuncPattern(pattern string) ServerOption {
	return func(s *Server) {
		s.SetHandleFuncPattern(pattern)
	}
}

func WithCertificate(certFile, keyFile string) ServerOption {
	return func(s *Server) {
		s.certificate = certFile
		s.certificateKey = keyFile
	}
}

// NewServer 创建一个新的 WebTransport 服务器
func NewServer(addr string, options ...ServerOption) *Server {
	s := &Server{
		addr:     addr,
		sessions: newSessionManager(),
		wtServer: &webtransport.Server{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			H3: http3.Server{
				Addr: addr,
			},
		},
		mu: &sync.Mutex{},
	}

	for _, opt := range options {
		opt(s)
	}

	if s.authorizer == nil {
		s.authorizer = DefaultAuthorizer
	}

	if s.errorWriter == nil {
		s.errorWriter = DefaultErrorWriter
	}

	if s.certificate == "" || s.certificateKey == "" {
		s.certificate = "cert.pem"
		s.certificateKey = "key.pem"
	}

	// 设置默认 TLS 配置
	if s.wtServer.H3.TLSConfig == nil {
		s.wtServer.H3.TLSConfig = &tls.Config{
			NextProtos: []string{"h3"},
		}
	}

	// 设置默认 QUIC 配置
	if s.wtServer.H3.QUICConfig == nil {
		s.wtServer.H3.QUICConfig = &quic.Config{
			MaxIncomingStreams: 100000,
		}
	}

	return s
}

// Start 启动服务器
func (s *Server) Start() error {
	// 如果证书文件不存在，生成新证书
	_, _certErr := os.Stat(s.certificate)
	_, _keyErr := os.Stat(s.certificateKey)
	if os.IsNotExist(_certErr) || os.IsNotExist(_keyErr) {
		if err := generateCertificate(s.certificate, s.certificateKey); err != nil {
			log.Fatal().Err(err).Msg("failed to generate certificate")
		}
		log.Info().Msg("Generated new certificate files")
	}
	log.Info().Msgf("Listening on %s", s.addr)
	return s.wtServer.ListenAndServeTLS(s.certificate, s.certificateKey)
}

// Close 关闭服务器
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	// 关闭所有会话
	s.sessions.removeAll()

	// 关闭服务器
	if s.wtServer != nil {
		return s.wtServer.Close()
	}

	return nil
}

// SetHandleFuncPattern 设置 WebTransport 会话处理函数
func (s *Server) SetHandleFuncPattern(pattern string) {
	http.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		clientKey, authed, err := s.authorizer(r)
		if err != nil {
			s.errorWriter(w, r, 400, err)
			return
		}
		if !authed {
			s.errorWriter(w, r, 401, errFailedAuth)
			return
		}
		session, err := s.wtServer.Upgrade(w, r)
		if err != nil {
			log.Err(err).Msg("Upgrade failed")
			return
		}
		s.sessions.add(clientKey, session)
		defer s.sessions.remove(clientKey, session)
		handleSession(clientKey, session)
		log.Info().Str("ClientKey", clientKey).Msg("Session remove")
	})
}

func (s *Server) GetDialer(clientKey string) (Dialer, error) {
	return s.sessions.getDialer(clientKey)
}

func generateCertificate(cert, key string) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"WT Org"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return err
	}

	certOut, err := os.Create(cert)
	if err != nil {
		return err
	}
	defer certOut.Close()
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyOut, err := os.Create(key)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	return nil
}

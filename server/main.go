package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/time/rate"
	"log"
	"math/big"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/quic-go/webtransport-go"
)

func generateCertificate() error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
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

	certOut, err := os.Create("cert.pem")
	if err != nil {
		return err
	}
	defer certOut.Close()
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyOut, err := os.Create("key.pem")
	if err != nil {
		return err
	}
	defer keyOut.Close()
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	return nil
}

type streamStats struct {
	activeStreams int64
	totalStreams  int64
}

var stats streamStats

func main() {
	// 添加监控
	go monitorStreams()

	// 如果证书文件不存在，生成新证书
	if _, err := os.Stat("cert.pem"); os.IsNotExist(err) {
		if err := generateCertificate(); err != nil {
			log.Fatal("Failed to generate certificate:", err)
		}
		log.Println("Generated new certificate files")
	}

	wtServer := webtransport.Server{}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		session, err := wtServer.Upgrade(w, r)
		if err != nil {
			log.Println("Upgrade failed:", err)
			return
		}
		go handleSession(session)
	})

	server := http3.Server{
		Addr: ":4433",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := wtServer.Upgrade(w, r)
			if err != nil {
				log.Println("Upgrade failed:", err)
				return
			}
			go handleSession(session)
		}),
		QUICConfig: &quic.Config{
			MaxIncomingStreams:    10000, // Allow up to 10000 concurrent bidirectional streams
			MaxIncomingUniStreams: 10000, // Allow up to 10000 concurrent unidirectional streams
		},
	}

	log.Println("WebTransport server running on :4433 (HTTP/3)")
	log.Fatal(server.ListenAndServeTLS("cert.pem", "key.pem"))
}

func handleSession(session *webtransport.Session) {
	// 添加连接级别的限流器
	limiter := rate.NewLimiter(rate.Limit(1000), 100) // 每秒1000个请求，突发100

	for {
		// 限流控制
		if err := limiter.Wait(context.Background()); err != nil {
			log.Println("Rate limit exceeded:", err)
			continue
		}

		stream, err := session.AcceptStream(context.Background())
		if err != nil {
			log.Println("Stream accept failed:", err)
			return
		}

		atomic.AddInt64(&stats.activeStreams, 1)
		atomic.AddInt64(&stats.totalStreams, 1)

		go func() {
			handleStream(stream)
			atomic.AddInt64(&stats.activeStreams, -1)
		}()
	}
}

func monitorStreams() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		active := atomic.LoadInt64(&stats.activeStreams)
		total := atomic.LoadInt64(&stats.totalStreams)
		log.Printf("活跃流数量: %d, 总流数量: %d", active, total)

		// 如果活跃流数量过高，可以触发告警
		if active > 5000 {
			log.Printf("警告: 活跃流数量过高: %d", active)
		}
	}
}

func handleStream(stream webtransport.Stream) {
	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil {
		log.Println("Read failed:", err)
		return
	}
	log.Println("Received:", string(buf[:n]))

	// 并行发送数据
	stream.Write([]byte("Hello from server!"))
}

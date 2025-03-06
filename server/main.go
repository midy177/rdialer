package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"github.com/quic-go/quic-go/http3"
	"log"
	"math/big"
	"net/http"
	"os"
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

func main() {
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
	}

	log.Println("WebTransport server running on :4433 (HTTP/3)")
	log.Fatal(server.ListenAndServeTLS("cert.pem", "key.pem"))
}

func handleSession(session *webtransport.Session) {
	for {
		// 接收多个并行流
		stream, err := session.AcceptStream(context.Background())
		if err != nil {
			log.Println("Stream accept failed:", err)
			return
		}
		go handleStream(stream)
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

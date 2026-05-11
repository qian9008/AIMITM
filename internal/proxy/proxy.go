package proxy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"airoxy-linux/internal/config"

	"github.com/elazarl/goproxy"
)

var (
	ProxyInstance *goproxy.ProxyHttpServer
	caCert        []byte
	caKey         []byte
)

func Init() error {
	ProxyInstance = goproxy.NewProxyHttpServer()
	ProxyInstance.Verbose = false

	if err := loadOrCreateCA(); err != nil {
		return err
	}

	setCA()

	ProxyInstance.OnRequest().HandleConnectFunc(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		if shouldMITM(host) {
			return goproxy.MitmConnect, host
		}
		// 名单外域名：直接透传 (TCP Forward)，不解密，不触发证书警告
		return goproxy.OkConnect, host
	})

	return nil
}

func loadOrCreateCA() error {
	certPath := filepath.Join("certs", "rootCA.crt")
	keyPath := filepath.Join("certs", "rootCA.key")

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		fmt.Println("Generating Root CA...")
		if err := generateCA(certPath, keyPath); err != nil {
			return err
		}
	}

	var err error
	caCert, err = os.ReadFile(certPath)
	if err != nil {
		return err
	}
	caKey, err = os.ReadFile(keyPath)
	return err
}

func generateCA(certPath, keyPath string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"AIRoxy Core"},
			CommonName:   "AIRoxy Root CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return err
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()

	keyOut, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()

	return nil
}

func setCA() {
	ca, err := tls.X509KeyPair(caCert, caKey)
	if err != nil {
		panic(err)
	}
	if ca.Leaf, err = x509.ParseCertificate(ca.Certificate[0]); err != nil {
		panic(err)
	}

	goproxy.GoproxyCa = ca
	goproxy.OkConnect = &goproxy.ConnectAction{Action: goproxy.ConnectAccept, TLSConfig: goproxy.TLSConfigFromCA(&ca)}
	goproxy.MitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(&ca)}
	goproxy.HTTPMitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectHTTPMitm, TLSConfig: goproxy.TLSConfigFromCA(&ca)}
	goproxy.RejectConnect = &goproxy.ConnectAction{Action: goproxy.ConnectReject, TLSConfig: goproxy.TLSConfigFromCA(&ca)}
}

func shouldMITM(host string) bool {
	cfg := config.Get()
	if cfg.RedirectAll {
		return true
	}
	hostOnly := strings.Split(host, ":")[0]
	for _, pattern := range cfg.MITMHosts {
		if matchHost(pattern, hostOnly) {
			return true
		}
	}
	return false
}

func matchHost(pattern, host string) bool {
	if pattern == host {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:]
		return strings.HasSuffix(host, suffix)
	}
	return false
}

func GetCACert() []byte {
	return caCert
}

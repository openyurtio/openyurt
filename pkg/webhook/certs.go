/*
Copyright 2022 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"time"

	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
)

const (
	CAKeyName       = "ca-key.pem"
	CACertName      = "ca-cert.pem"
	ServerKeyName   = "key.pem"
	ServerKeyName2  = "tls.key"
	ServerCertName  = "cert.pem"
	ServerCertName2 = "tls.crt"
)

type Certs struct {
	// PEM encoded private key
	Key []byte
	// PEM encoded serving certificate
	Cert []byte
	// PEM encoded CA private key
	CAKey []byte
	// PEM encoded CA certificate
	CACert []byte
	// Resource version of the certs
	ResourceVersion string
}

const (
	rsaKeySize = 2048
)

// GenerateCerts generate a suite of self signed CA and server cert
func GenerateCerts(serviceNamespace, serviceName string) *Certs {
	certs := &Certs{}
	certs.generate(serviceNamespace, serviceName)

	return certs
}

func (c *Certs) generate(serviceNamespace, serviceName string) error {
	caKey, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		return fmt.Errorf("failed to create CA private key: %v", err)
	}
	caCert, err := cert.NewSelfSignedCACert(cert.Config{CommonName: "yurt-webhooks-cert-ca"}, caKey)
	if err != nil {
		return fmt.Errorf("failed to create CA cert: %v", err)
	}

	key, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		return fmt.Errorf("failed to create private key: %v", err)
	}

	commonName := ServiceToCommonName(serviceNamespace, serviceName)
	hostIP := net.ParseIP(commonName)
	var altIPs []net.IP
	if hostIP.To4() != nil {
		altIPs = append(altIPs, hostIP.To4())
	}
	dnsNames := []string{serviceName, fmt.Sprintf("%s.%s", serviceName, serviceNamespace), commonName}
	cert, err := NewSignedCert(
		cert.Config{
			CommonName: commonName,
			Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			AltNames:   cert.AltNames{IPs: altIPs, DNSNames: dnsNames},
		},
		key, caCert, caKey,
	)
	if err != nil {
		return fmt.Errorf("failed to create cert: %v", err)
	}

	c.Key = EncodePrivateKeyPEM(key)
	c.Cert = EncodeCertPEM(cert)
	c.CAKey = EncodePrivateKeyPEM(caKey)
	c.CACert = EncodeCertPEM(caCert)

	return nil
}

func NewSignedCert(cfg cert.Config, key crypto.Signer, caCert *x509.Certificate, caKey crypto.Signer) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, err
	}
	if len(cfg.CommonName) == 0 {
		return nil, errors.New("must specify a CommonName")
	}
	if len(cfg.Usages) == 0 {
		return nil, errors.New("must specify at least one ExtKeyUsage")
	}

	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		DNSNames:     cfg.AltNames.DNSNames,
		IPAddresses:  cfg.AltNames.IPs,
		SerialNumber: serial,
		NotBefore:    caCert.NotBefore,
		NotAfter:     time.Now().Add(time.Hour * 24 * 365 * 100).UTC(),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  cfg.Usages,
	}
	certDERBytes, err := x509.CreateCertificate(rand.Reader, &certTmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}

// EncodePrivateKeyPEM returns PEM-encoded private key data
func EncodePrivateKeyPEM(key *rsa.PrivateKey) []byte {
	block := pem.Block{
		Type:  keyutil.RSAPrivateKeyBlockType,
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return pem.EncodeToMemory(&block)
}

// EncodeCertPEM returns PEM-endcoded certificate data
func EncodeCertPEM(ct *x509.Certificate) []byte {
	block := pem.Block{
		Type:  cert.CertificateBlockType,
		Bytes: ct.Raw,
	}
	return pem.EncodeToMemory(&block)
}

// serviceToCommonName generates the CommonName for the certificate when using a k8s service.
func ServiceToCommonName(serviceNamespace, serviceName string) string {
	return fmt.Sprintf("%s.%s.svc", serviceName, serviceNamespace)
}

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

package yurtcoordinatorcert

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math"
	"math/big"
	"net"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/certificate"
	"k8s.io/client-go/util/keyutil"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/util/kubeconfig"
)

const (
	certificateBlockType = "CERTIFICATE"
	publicKeyBlockType   = "PUBLIC KEY"
	rsaKeySize           = 2048
	certDuration         = time.Hour * 24 * 365 * 100 // certificate validity time
)

// NewPrivateKey creates an RSA private key
func NewPrivateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(cryptorand.Reader, rsaKeySize)
}

func NewSelfSignedCA() (*x509.Certificate, crypto.Signer, error) {
	key, err := NewPrivateKey()
	if err != nil {
		return nil, nil, errors.Wrap(err, "Create CA private key fail")
	}

	cert, err := certutil.NewSelfSignedCACert(certutil.Config{
		CommonName: YurtCoordinatorOrg,
	}, key)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Create CA cert fail")
	}

	return cert, key, nil
}

// NewSignedCert creates a signed certificate using the given CA certificate and key
func NewSignedCert(client client.Interface, cfg *CertConfig, key crypto.Signer, caCert *x509.Certificate, caKey crypto.Signer, stopCh <-chan struct{}) (cert *x509.Certificate, err error) {

	// check cert constraints
	if len(cfg.CommonName) == 0 {
		return nil, errors.New("must specify a CommonName")
	}
	if len(cfg.ExtKeyUsage) == 0 {
		return nil, errors.New("must specify at least one ExtKeyUsage")
	}

	// initialize cert if necessary
	if cfg.certInit != nil {
		if cfg.IPs == nil {
			cfg.IPs = []net.IP{}
		}
		if cfg.DNSNames == nil {
			cfg.DNSNames = []string{}
		}

		ips, dnsNames, err := cfg.certInit(client, stopCh)
		if err != nil {
			return nil, errors.Wrapf(err, "init cert %s fail", cfg.CertName)
		}

		cfg.IPs = append(cfg.IPs, ips...)
		cfg.DNSNames = append(cfg.DNSNames, dnsNames...)
	}

	// prepare cert serial number
	serial, err := cryptorand.Int(cryptorand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		DNSNames:     cfg.DNSNames,
		IPAddresses:  cfg.IPs,
		SerialNumber: serial,
		NotBefore:    caCert.NotBefore,
		NotAfter:     time.Now().Add(certDuration).UTC(),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  cfg.ExtKeyUsage,
	}
	certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &certTmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}

func loadCertAndKeyFromSecret(clientSet client.Interface, certConf CertConfig) (*x509.Certificate, crypto.Signer, error) {

	secretName := certConf.SecretName
	certName := certConf.CertName

	// get secret
	secret, err := clientSet.CoreV1().Secrets(YurtCoordinatorNS).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	var certBytes, keyBytes []byte
	var ok bool

	if certConf.IsKubeConfig {
		kubeConfigBytes, ok := secret.Data[certName]
		if !ok {
			return nil, nil, errors.Errorf("%s not exist in %s secret", certName, secretName)
		}
		kubeConfig, err := clientcmd.Load(kubeConfigBytes)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "couldn't parse the kubeconfig file in the %s secret", secretName)
		}
		authInfo := kubeconfig.GetAuthInfoFromKubeConfig(kubeConfig)
		if authInfo == nil {
			return nil, nil, errors.Errorf("auth info is not found in secret(%s)", secretName)
		}
		certBytes = authInfo.ClientCertificateData
		keyBytes = authInfo.ClientKeyData
	} else {
		certBytes, ok = secret.Data[fmt.Sprintf("%s.crt", certName)]
		if !ok {
			return nil, nil, errors.Errorf("%s.crt not exist in %s secret", certName, secretName)
		}
		keyBytes, ok = secret.Data[fmt.Sprintf("%s.key", certName)]
		if !ok {
			return nil, nil, errors.Wrapf(err, "%s.key not exist in %s secret", certName, secretName)
		}
	}

	// parse cert
	certs, err := certutil.ParseCertsPEM(certBytes)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "couldn't parse cert from %s.crt in secret %s", certName, secretName)
	}

	// parse private key
	privKey, err := keyutil.ParsePrivateKeyPEM(keyBytes)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "couldn't parse key from %s.key in secret %s", certName, secretName)
	}
	var key crypto.Signer
	switch k := privKey.(type) {
	case *rsa.PrivateKey:
		key = k
	case *ecdsa.PrivateKey:
		key = k
	default:
		return nil, nil, errors.New("the private key is neither in RSA nor ECDSA format")
	}

	return certs[0], key, nil
}

func IsCertFromCA(cert *x509.Certificate, caCert *x509.Certificate) bool {
	rootPool := x509.NewCertPool()
	rootPool.AddCert(caCert)

	verifyOptions := x509.VerifyOptions{
		Roots:     rootPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	if _, err := cert.Verify(verifyOptions); err != nil {
		klog.Info(Format("cert not authorized by current CA: %v", err))
		return false
	}

	return true
}

func initYurtCoordinatorCert(client client.Interface, cfg CertConfig, caCert *x509.Certificate, caKey crypto.Signer, stopCh <-chan struct{}) error {
	key, err := NewPrivateKey()
	if err != nil {
		return errors.Wrapf(err, "init yurtcoordinator cert: create %s key fail", cfg.CertName)
	}

	cert, err := NewSignedCert(client, &cfg, key, caCert, caKey, stopCh)
	if err != nil {
		return errors.Wrapf(err, "init yurtcoordinator cert: create %s cert fail", cfg.CertName)
	}

	if !cfg.IsKubeConfig {
		err = WriteCertAndKeyIntoSecret(client, cfg.CertName, cfg.SecretName, cert, key)
		if err != nil {
			return errors.Wrapf(err, "init yurtcoordinator cert: write %s into secret %s fail", cfg.CertName, cfg.SecretName)
		}
	} else {
		apiServerURL, err := getAPIServerSVCURL(client)
		if err != nil {
			return errors.Wrapf(err, "couldn't get YurtCoordinator APIServer service url")
		}

		keyBytes, _ := keyutil.MarshalPrivateKeyToPEM(key)
		certBytes, _ := EncodeCertPEM(cert)
		caBytes, _ := EncodeCertPEM(caCert)
		kubeConfig := kubeconfig.CreateWithCerts(apiServerURL, "cluster", cfg.CommonName, caBytes, keyBytes, certBytes)
		kubeConfigByte, err := clientcmd.Write(*kubeConfig)
		if err != nil {
			return err
		}

		if err := WriteKubeConfigIntoSecret(client, cfg.SecretName, cfg.CertName, kubeConfigByte); err != nil {
			return errors.Wrapf(err, "couldn't write kubeconfig into secret %s", cfg.SecretName)
		}
	}

	return nil
}

// EncodeCertPEM returns PEM-encoded certificate data
func EncodeCertPEM(c *x509.Certificate) ([]byte, error) {

	if c == nil {
		return nil, nil
	}

	block := pem.Block{
		Type:  certificateBlockType,
		Bytes: c.Raw,
	}
	return pem.EncodeToMemory(&block), nil
}

// EncodePublicKeyPEM returns PEM-encoded public data
func EncodePublicKeyPEM(key crypto.PublicKey) ([]byte, error) {
	if key == nil {
		return nil, nil
	}

	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return []byte{}, err
	}
	block := pem.Block{
		Type:  publicKeyBlockType,
		Bytes: der,
	}
	return pem.EncodeToMemory(&block), nil
}

func GetCertFromTLSCert(cert *tls.Certificate) (certPEM []byte, err error) {
	if cert == nil {
		return nil, errors.New("tls certificate cannot be nil")
	}

	return EncodeCertPEM(cert.Leaf)
}

func GetPrivateKeyFromTLSCert(cert *tls.Certificate) (keyPEM []byte, err error) {
	if cert == nil {
		return nil, errors.New("tls certificate cannot be nil")
	}

	return keyutil.MarshalPrivateKeyToPEM(cert.PrivateKey)
}

// GetCertAndKeyFromCertMgr will get certificate & private key (in PEM format) from certmanager
func GetCertAndKeyFromCertMgr(certManager certificate.Manager, stopCh <-chan struct{}) (key []byte, cert []byte, err error) {
	// waiting for the certificate is generated
	certManager.Start()

	err = wait.PollUntilContextCancel(context.Background(), 5*time.Second, true, func(ctx context.Context) (bool, error) {
		// keep polling until the certificate is signed
		if certManager.Current() != nil {
			klog.Info(Format("%s certificate signed successfully", ComponentName))
			return true, nil
		}
		klog.Info(Format("waiting for the master to sign the %s certificate", ComponentName))
		return false, nil
	})

	if err != nil {
		return nil, nil, err
	}

	// When CSR is issued and approved
	// get key from certificate
	key, err = GetPrivateKeyFromTLSCert(certManager.Current())
	if err != nil {
		return
	}
	// get certificate from certificate
	cert, err = GetCertFromTLSCert(certManager.Current())
	if err != nil {
		return
	}

	return
}

// WriteCertIntoSecret will write cert&key pair generated from certManager into a secret
func WriteCertIntoSecret(clientSet client.Interface, certName, secretName string, certManager certificate.Manager, stopCh <-chan struct{}) error {

	keyPEM, certPEM, err := GetCertAndKeyFromCertMgr(certManager, stopCh)
	if err != nil {
		return errors.Wrapf(err, "write cert %s fail", certName)
	}

	// write certificate data into secret
	secretClient, err := NewSecretClient(clientSet, YurtCoordinatorNS, secretName)
	if err != nil {
		return err
	}
	err = secretClient.AddData(fmt.Sprintf("%s.key", certName), keyPEM)
	if err != nil {
		return err
	}
	err = secretClient.AddData(fmt.Sprintf("%s.crt", certName), certPEM)
	if err != nil {
		return err
	}

	klog.Info(Format("successfully write %s cert/key pair into %s", certName, secretName))

	return nil
}

// WriteCertAndKeyIntoSecret is used for writing cert&key into secret
// Notice: if cert OR key is nil, it will be ignored
func WriteCertAndKeyIntoSecret(clientSet client.Interface, certName, secretName string, cert *x509.Certificate, key crypto.Signer) error {
	// write certificate data into secret
	secretClient, err := NewSecretClient(clientSet, YurtCoordinatorNS, secretName)
	if err != nil {
		return err
	}

	if key != nil {
		keyPEM, err := keyutil.MarshalPrivateKeyToPEM(key)
		if err != nil {
			return errors.Wrapf(err, "could not write %s.key into secret %s", certName, secretName)
		}
		err = secretClient.AddData(fmt.Sprintf("%s.key", certName), keyPEM)
		if err != nil {
			return errors.Wrapf(err, "could not write %s.key into secret %s", certName, secretName)
		}
	}

	if cert != nil {
		certPEM, err := EncodeCertPEM(cert)
		if err != nil {
			return errors.Wrapf(err, "could not write %s.cert into secret %s", certName, secretName)
		}
		err = secretClient.AddData(fmt.Sprintf("%s.crt", certName), certPEM)
		if err != nil {
			return errors.Wrapf(err, "could not write %s.cert into secret %s", certName, secretName)
		}
	}

	klog.Info(Format("successfully write %s cert/key into %s", certName, secretName))

	return nil
}

func WriteKubeConfigIntoSecret(clientSet client.Interface, secretName, kubeConfigName string, kubeConfigByte []byte) error {
	secretClient, err := NewSecretClient(clientSet, YurtCoordinatorNS, secretName)
	if err != nil {
		return err
	}
	err = secretClient.AddData(kubeConfigName, kubeConfigByte)
	if err != nil {
		return err
	}

	klog.Info(Format("successfully write kubeconfig into secret %s", secretName))

	return nil
}

func WriteKeyPairIntoSecret(clientSet client.Interface, secretName, keyName string, key crypto.Signer) error {
	secretClient, err := NewSecretClient(clientSet, YurtCoordinatorNS, secretName)
	if err != nil {
		return err
	}

	privateKeyPEM, err := keyutil.MarshalPrivateKeyToPEM(key)
	if err != nil {
		return errors.Wrapf(err, "could not marshal private key into PEM format %s", keyName)
	}
	err = secretClient.AddData(fmt.Sprintf("%s.key", keyName), privateKeyPEM)
	if err != nil {
		return errors.Wrapf(err, "could not write %s.key into secret %s", keyName, secretName)
	}

	publicKey := key.Public()
	publicKeyPEM, err := EncodePublicKeyPEM(publicKey)
	if err != nil {
		return errors.Wrapf(err, "could not marshal public key into PEM format %s", keyName)
	}
	err = secretClient.AddData(fmt.Sprintf("%s.pub", keyName), publicKeyPEM)
	if err != nil {
		return errors.Wrapf(err, "could not write %s.pub into secret %s", keyName, secretName)
	}

	klog.Info(Format("successfully write key pair into secret %s", secretName))

	return nil
}

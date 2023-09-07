/*
Copyright 2020 The OpenYurt Authors.

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

package testdata

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	certificatesv1 "k8s.io/api/certificates/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/server/dynamiccertificates"
	clientset "k8s.io/client-go/kubernetes"
	fakeclient "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	tokenjws "k8s.io/cluster-bootstrap/token/jws"

	kubeconfigutil "github.com/openyurtio/openyurt/pkg/util/kubeconfig"
)

const (
	caCert = `-----BEGIN CERTIFICATE-----
MIICyDCCAbCgAwIBAgIBADANBgkqhkiG9w0BAQsFADAVMRMwEQYDVQQDEwprdWJl
cm5ldGVzMB4XDTE5MTEyMDAwNDk0MloXDTI5MTExNzAwNDk0MlowFTETMBEGA1UE
AxMKa3ViZXJuZXRlczCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAMqQ
ctECzA8yFSuVYupOUYgrTmfQeKe/9BaDWagaq7ow9+I2IvsfWFvlrD8QQr8sea6q
xjq7TV67Vb4RxBaoYDA+yI5vIcujWUxULun64lu3Q6iC1sj2UnmUpIdgazRXXEkZ
vxA6EbAnoxA0+lBOn1CZWl23IQ4s70o2hZ7wIp/vevB88RRRjqtvgc5elsjsbmDF
LS7L1Zuye8c6gS93bR+VjVmSIfr1IEq0748tIIyXjAVCWPVCvuP41MlfPc/JVpZD
uD2+pO6ZYREcdAnOf2eD4/eLOMKko4L1dSFy9JKM5PLnOC0Zk0AYOd1vS8DTAfxj
XPEIY8OBYFhlsxf4TE8CAwEAAaMjMCEwDgYDVR0PAQH/BAQDAgKkMA8GA1UdEwEB
/wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAH/OYq8zyl1+zSTmuow3yI/15PL1
dl8hB7IKnZNWmC/LTdm/+noh3Sb1IdRv6HkKg/GUn0UMuRUngLhju3EO4ozJPQcX
quaxzgmTKNWJ6ErDvRvWhGX0ZcbdBfZv+dowyRqzd5nlJ49hC+NrtFFQq6P05BYn
7SemguqeXmXwIj2Sa+1DeR6lRm9o8shAYjnyThUFqaMn18kI3SANJ5vk/3DFrPEO
CKC9EzFku2kuxg2dM12PbRGZQ2o0K6HEZgrrIKTPOy3ocb8r9M0aSFhjOV/NqGA4
SaupXSW6XfvIi/UHoIbU3pNcsnUJGnQfQvip95XKk/gqcUr+m50vxgumxtA=
-----END CERTIFICATE-----`
	tokenID     = "123456"
	tokenSecret = "abcdef1234567890"
)

func CreateCertFakeClient(CaDir string) (clientset.Interface, error) {
	// Create a fake client
	client := fakeclient.NewSimpleClientset()

	// prepare cluster-info configmap
	kubeconfig := buildSecureBootstrapKubeConfig("127.0.0.1", []byte(caCert), "somecluster")
	kubeconfigBytes, err := clientcmd.Write(*kubeconfig)
	if err != nil {
		return client, err
	}

	// Generate signature of the insecure kubeconfig
	sig, err := tokenjws.ComputeDetachedSignature(string(kubeconfigBytes), tokenID, tokenSecret)
	if err != nil {
		return client, err
	}

	cm := &fakeConfigMap{
		name: bootstrapapi.ConfigMapClusterInfo,
		data: map[string]string{
			bootstrapapi.KubeConfigKey:                   string(kubeconfigBytes),
			bootstrapapi.JWSSignatureKeyPrefix + tokenID: sig,
		},
	}

	if err = cm.createOrUpdate(client); err != nil {
		return client, err
	}

	// prepare certificates
	client = prepareCsrAndCert(client, CaDir)

	return client, nil
}

type fakeConfigMap struct {
	name string
	data map[string]string
}

func (c *fakeConfigMap) createOrUpdate(client clientset.Interface) error {
	return createOrUpdateConfigMap(client, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.name,
			Namespace: metav1.NamespacePublic,
		},
		Data: c.data,
	})
}

// createOrUpdateConfigMap creates a ConfigMap if the target resource doesn't exist. If the resource exists already, this function will update the resource instead.
func createOrUpdateConfigMap(client clientset.Interface, cm *v1.ConfigMap) error {
	if _, err := client.CoreV1().ConfigMaps(cm.ObjectMeta.Namespace).Create(context.TODO(), cm, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "unable to create ConfigMap")
		}

		if _, err := client.CoreV1().ConfigMaps(cm.ObjectMeta.Namespace).Update(context.TODO(), cm, metav1.UpdateOptions{}); err != nil {
			return errors.Wrap(err, "unable to update ConfigMap")
		}
	}
	return nil
}

var (
	csr *certificatesv1.CertificateSigningRequest
)

func prepareCsrAndCert(f *fakeclient.Clientset, testDir string) *fakeclient.Clientset {
	f.PrependReactor("create", "certificatesigningrequests", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		switch action.GetResource().Version {
		case "v1":
			cAction, ok := action.(clienttesting.CreateAction)
			if ok {
				csr = cAction.GetObject().(*certificatesv1.CertificateSigningRequest)
			}
			return true, &certificatesv1.CertificateSigningRequest{ObjectMeta: metav1.ObjectMeta{UID: "fake-uid"}}, nil
		default:
			return false, nil, nil
		}
	})
	f.PrependReactor("list", "certificatesigningrequests", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		switch action.GetResource().Version {
		case "v1":
			return true, &certificatesv1.CertificateSigningRequestList{Items: []certificatesv1.CertificateSigningRequest{{ObjectMeta: metav1.ObjectMeta{UID: "fake-uid"}}}}, nil
		default:
			return false, nil, nil
		}
	})
	f.PrependWatchReactor("certificatesigningrequests", func(action clienttesting.Action) (handled bool, ret watch.Interface, err error) {
		switch action.GetResource().Version {
		case "v1":
			certData, err := signCertificate(csr, testDir)
			if err != nil {
				return true, nil, err
			}
			return true, &fakeWatch{
				version:        action.GetResource().Version,
				certificatePEM: certData,
			}, nil
		default:
			return false, nil, nil
		}
	})
	return f
}

type fakeWatch struct {
	version        string
	certificatePEM []byte
}

func (w *fakeWatch) Stop() {
}

func (w *fakeWatch) ResultChan() <-chan watch.Event {
	var csr runtime.Object

	switch w.version {
	case "v1":
		condition := certificatesv1.CertificateSigningRequestCondition{
			Type: certificatesv1.CertificateApproved,
		}

		csr = &certificatesv1.CertificateSigningRequest{
			ObjectMeta: metav1.ObjectMeta{UID: "fake-uid"},
			Status: certificatesv1.CertificateSigningRequestStatus{
				Conditions: []certificatesv1.CertificateSigningRequestCondition{
					condition,
				},
				Certificate: []byte(w.certificatePEM),
			},
		}
	}

	c := make(chan watch.Event, 1)
	c <- watch.Event{
		Type:   watch.Added,
		Object: csr,
	}
	return c
}

// buildSecureBootstrapKubeConfig makes a kubeconfig object that connects securely to the API Server for bootstrapping purposes (validating with the specified CA)
func buildSecureBootstrapKubeConfig(endpoint string, caCert []byte, clustername string) *clientcmdapi.Config {
	controlPlaneEndpoint := fmt.Sprintf("https://%s", endpoint)
	bootstrapConfig := kubeconfigutil.CreateBasic(controlPlaneEndpoint, clustername, "token-bootstrap-client", caCert)
	return bootstrapConfig
}

func signCertificate(csr *certificatesv1.CertificateSigningRequest, testDir string) ([]byte, error) {
	caFile := filepath.Join(testDir, "ca.crt")
	caKeyFile := filepath.Join(testDir, "ca.key")
	caLoader, err := dynamiccertificates.NewDynamicServingContentFromFiles("csr-controller", caFile, caKeyFile)
	if err != nil {
		return nil, fmt.Errorf("error reading CA cert file %q: %v", caFile, err)
	}

	cap := &caProvider{
		caLoader: caLoader,
	}
	if err := cap.setCA(); err != nil {
		return nil, err
	}

	x509cr, err := parseCSR(csr.Spec.Request)
	if err != nil {
		return nil, fmt.Errorf("unable to parse csr %q: %v", csr.Name, err)
	}

	currCA, err := cap.currentCA()
	if err != nil {
		return nil, err
	}

	der, err := currCA.Sign(x509cr.Raw, PermissiveSigningPolicy{
		TTL:      365 * 24 * 3600 * time.Second,
		Usages:   csr.Spec.Usages,
		Backdate: 5 * time.Minute,
		Short:    8 * time.Hour,
		Now:      nil,
	})
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

var serialNumberLimit = new(big.Int).Lsh(big.NewInt(1), 128)

// CertificateAuthority implements a certificate authority that supports policy
// based signing. It's used by the signing controller.
type CertificateAuthority struct {
	// RawCert is an optional field to determine if signing cert/key pairs have changed
	RawCert []byte
	// RawKey is an optional field to determine if signing cert/key pairs have changed
	RawKey []byte

	Certificate *x509.Certificate
	PrivateKey  crypto.Signer
}

// Sign signs a certificate request, applying a SigningPolicy and returns a DER
// encoded x509 certificate.
func (ca *CertificateAuthority) Sign(crDER []byte, policy SigningPolicy) ([]byte, error) {
	cr, err := x509.ParseCertificateRequest(crDER)
	if err != nil {
		return nil, fmt.Errorf("unable to parse certificate request: %v", err)
	}
	if err := cr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("unable to verify certificate request signature: %v", err)
	}

	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("unable to generate a serial number for %s: %v", cr.Subject.CommonName, err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:       serialNumber,
		Subject:            cr.Subject,
		DNSNames:           cr.DNSNames,
		IPAddresses:        cr.IPAddresses,
		EmailAddresses:     cr.EmailAddresses,
		URIs:               cr.URIs,
		PublicKeyAlgorithm: cr.PublicKeyAlgorithm,
		PublicKey:          cr.PublicKey,
		Extensions:         cr.Extensions,
		ExtraExtensions:    cr.ExtraExtensions,
	}
	if err := policy.apply(tmpl, ca.Certificate.NotAfter); err != nil {
		return nil, err
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Certificate, cr.PublicKey, ca.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign certificate: %v", err)
	}
	return der, nil
}

type caProvider struct {
	caValue  atomic.Value
	caLoader *dynamiccertificates.DynamicCertKeyPairContent
}

// setCA unconditionally stores the current cert/key content
func (p *caProvider) setCA() error {
	certPEM, keyPEM := p.caLoader.CurrentCertKeyContent()

	certs, err := cert.ParseCertsPEM(certPEM)
	if err != nil {
		return fmt.Errorf("error reading CA cert file %q: %v", p.caLoader.Name(), err)
	}
	if len(certs) != 1 {
		return fmt.Errorf("error reading CA cert file %q: expected 1 certificate, found %d", p.caLoader.Name(), len(certs))
	}

	key, err := keyutil.ParsePrivateKeyPEM(keyPEM)
	if err != nil {
		return fmt.Errorf("error reading CA key file %q: %v", p.caLoader.Name(), err)
	}
	priv, ok := key.(crypto.Signer)
	if !ok {
		return fmt.Errorf("error reading CA key file %q: key did not implement crypto.Signer", p.caLoader.Name())
	}

	ca := &CertificateAuthority{
		RawCert: certPEM,
		RawKey:  keyPEM,

		Certificate: certs[0],
		PrivateKey:  priv,
	}
	p.caValue.Store(ca)

	return nil
}

// currentCA provides the current value of the CA.
// It always check for a stale value.  This is cheap because it's all an in memory cache of small slices.
func (p *caProvider) currentCA() (*CertificateAuthority, error) {
	certPEM, keyPEM := p.caLoader.CurrentCertKeyContent()
	currCA := p.caValue.Load().(*CertificateAuthority)
	if bytes.Equal(currCA.RawCert, certPEM) && bytes.Equal(currCA.RawKey, keyPEM) {
		return currCA, nil
	}

	// the bytes weren't equal, so we have to set and then load
	if err := p.setCA(); err != nil {
		return currCA, err
	}
	return p.caValue.Load().(*CertificateAuthority), nil
}

// ParseCSR extracts the CSR from the bytes and decodes it.
func parseCSR(pemBytes []byte) (*x509.CertificateRequest, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, errors.New("PEM block type must be CERTIFICATE REQUEST")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, err
	}
	return csr, nil
}

// SigningPolicy validates a CertificateRequest before it's signed by the
// CertificateAuthority. It may default or otherwise mutate a certificate
// template.
type SigningPolicy interface {
	// not-exporting apply forces signing policy implementations to be internal
	// to this package.
	apply(template *x509.Certificate, signerNotAfter time.Time) error
}

// PermissiveSigningPolicy is the signing policy historically used by the local
// signer.
//
//   - It forwards all SANs from the original signing request.
//   - It sets allowed usages as configured in the policy.
//   - It zeros all extensions.
//   - It sets BasicConstraints to true.
//   - It sets IsCA to false.
//   - It validates that the signer has not expired.
//   - It sets NotBefore and NotAfter:
//     All certificates set NotBefore = Now() - Backdate.
//     Long-lived certificates set NotAfter = Now() + TTL - Backdate.
//     Short-lived certificates set NotAfter = Now() + TTL.
//     All certificates truncate NotAfter to the expiration date of the signer.
type PermissiveSigningPolicy struct {
	// TTL is used in certificate NotAfter calculation as described above.
	TTL time.Duration

	// Usages are the allowed usages of a certificate.
	Usages []certificatesv1.KeyUsage

	// Backdate is used in certificate NotBefore calculation as described above.
	Backdate time.Duration

	// Short is the duration used to determine if the lifetime of a certificate should be considered short.
	Short time.Duration

	// Now defaults to time.Now but can be stubbed for testing
	Now func() time.Time
}

func (p PermissiveSigningPolicy) apply(tmpl *x509.Certificate, signerNotAfter time.Time) error {
	var now time.Time
	if p.Now != nil {
		now = p.Now()
	} else {
		now = time.Now()
	}

	ttl := p.TTL

	usage, extUsages, err := keyUsagesFromStrings(p.Usages)
	if err != nil {
		return err
	}
	tmpl.KeyUsage = usage
	tmpl.ExtKeyUsage = extUsages

	tmpl.ExtraExtensions = nil
	tmpl.Extensions = nil
	tmpl.BasicConstraintsValid = true
	tmpl.IsCA = false

	tmpl.NotBefore = now.Add(-p.Backdate)

	if ttl < p.Short {
		// do not backdate the end time if we consider this to be a short lived certificate
		tmpl.NotAfter = now.Add(ttl)
	} else {
		tmpl.NotAfter = now.Add(ttl - p.Backdate)
	}

	if !tmpl.NotAfter.Before(signerNotAfter) {
		tmpl.NotAfter = signerNotAfter
	}

	if !tmpl.NotBefore.Before(signerNotAfter) {
		return fmt.Errorf("the signer has expired: NotAfter=%v", signerNotAfter)
	}

	if !now.Before(signerNotAfter) {
		return fmt.Errorf("refusing to sign a certificate that expired in the past: NotAfter=%v", signerNotAfter)
	}

	return nil
}

var keyUsageDict = map[certificatesv1.KeyUsage]x509.KeyUsage{
	certificatesv1.UsageSigning:           x509.KeyUsageDigitalSignature,
	certificatesv1.UsageDigitalSignature:  x509.KeyUsageDigitalSignature,
	certificatesv1.UsageContentCommitment: x509.KeyUsageContentCommitment,
	certificatesv1.UsageKeyEncipherment:   x509.KeyUsageKeyEncipherment,
	certificatesv1.UsageKeyAgreement:      x509.KeyUsageKeyAgreement,
	certificatesv1.UsageDataEncipherment:  x509.KeyUsageDataEncipherment,
	certificatesv1.UsageCertSign:          x509.KeyUsageCertSign,
	certificatesv1.UsageCRLSign:           x509.KeyUsageCRLSign,
	certificatesv1.UsageEncipherOnly:      x509.KeyUsageEncipherOnly,
	certificatesv1.UsageDecipherOnly:      x509.KeyUsageDecipherOnly,
}

var extKeyUsageDict = map[certificatesv1.KeyUsage]x509.ExtKeyUsage{
	certificatesv1.UsageAny:             x509.ExtKeyUsageAny,
	certificatesv1.UsageServerAuth:      x509.ExtKeyUsageServerAuth,
	certificatesv1.UsageClientAuth:      x509.ExtKeyUsageClientAuth,
	certificatesv1.UsageCodeSigning:     x509.ExtKeyUsageCodeSigning,
	certificatesv1.UsageEmailProtection: x509.ExtKeyUsageEmailProtection,
	certificatesv1.UsageSMIME:           x509.ExtKeyUsageEmailProtection,
	certificatesv1.UsageIPsecEndSystem:  x509.ExtKeyUsageIPSECEndSystem,
	certificatesv1.UsageIPsecTunnel:     x509.ExtKeyUsageIPSECTunnel,
	certificatesv1.UsageIPsecUser:       x509.ExtKeyUsageIPSECUser,
	certificatesv1.UsageTimestamping:    x509.ExtKeyUsageTimeStamping,
	certificatesv1.UsageOCSPSigning:     x509.ExtKeyUsageOCSPSigning,
	certificatesv1.UsageMicrosoftSGC:    x509.ExtKeyUsageMicrosoftServerGatedCrypto,
	certificatesv1.UsageNetscapeSGC:     x509.ExtKeyUsageNetscapeServerGatedCrypto,
}

// keyUsagesFromStrings will translate a slice of usage strings from the
// certificates API ("pkg/apis/certificates".KeyUsage) to x509.KeyUsage and
// x509.ExtKeyUsage types.
func keyUsagesFromStrings(usages []certificatesv1.KeyUsage) (x509.KeyUsage, []x509.ExtKeyUsage, error) {
	var keyUsage x509.KeyUsage
	var unrecognized []certificatesv1.KeyUsage
	extKeyUsages := make(map[x509.ExtKeyUsage]struct{})
	for _, usage := range usages {
		if val, ok := keyUsageDict[usage]; ok {
			keyUsage |= val
		} else if val, ok := extKeyUsageDict[usage]; ok {
			extKeyUsages[val] = struct{}{}
		} else {
			unrecognized = append(unrecognized, usage)
		}
	}

	var sorted sortedExtKeyUsage
	for eku := range extKeyUsages {
		sorted = append(sorted, eku)
	}
	sort.Sort(sorted)

	if len(unrecognized) > 0 {
		return 0, nil, fmt.Errorf("unrecognized usage values: %q", unrecognized)
	}

	return keyUsage, sorted, nil
}

type sortedExtKeyUsage []x509.ExtKeyUsage

func (s sortedExtKeyUsage) Len() int {
	return len(s)
}

func (s sortedExtKeyUsage) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s sortedExtKeyUsage) Less(i, j int) bool {
	return s[i] < s[j]
}

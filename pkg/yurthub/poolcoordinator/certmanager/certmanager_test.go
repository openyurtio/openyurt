/*
Copyright 2023 The OpenYurt Authors.

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

package certmanager

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/yurthub/poolcoordinator/constants"
	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
)

const (
	testPKIDir = "/tmp/pool-coordinator-pki"
	caByte     = `-----BEGIN CERTIFICATE-----
MIIC/jCCAeagAwIBAgIBADANBgkqhkiG9w0BAQsFADAVMRMwEQYDVQQDEwprdWJl
cm5ldGVzMB4XDTIyMTIyODAzMzgyM1oXDTMyMTIyNTAzMzgyM1owFTETMBEGA1UE
AxMKa3ViZXJuZXRlczCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAKUI
4IgEu/xH2orH1uLx1ad+eBy8WcqwOaJKZMZqEgEorWRXUvsM/UAE447V/eGkvwT/
rFlFuyhVzpsecE4n2zK13lf7/cHD6raS4XR2vvbgX/KRkNPHPK38326zCu+rvZVU
9zq5rxXGHKytL+2uVuCnjP8xOtgEy9iB8kML2wWBMuO8Seyh4/F/jJ5Zrhi/zgHp
swfgvmEYz0BGFBqnVYYx7CST2ek95LVXnc3xS8wlmo+X4foiJG9mVSTGtfQoBQ2H
hg3vZV3+fsXNNYT4xigZ5kU97npaZk/nfZGyaHuEeiNWQOimQYCvJWFHJ6G/Vuyt
gpujDjMpH9nYwZkKb8UCAwEAAaNZMFcwDgYDVR0PAQH/BAQDAgKkMA8GA1UdEwEB
/wQFMAMBAf8wHQYDVR0OBBYEFKKux0rxaMSl/ks3ndmrOeu8PN4mMBUGA1UdEQQO
MAyCCmt1YmVybmV0ZXMwDQYJKoZIhvcNAQELBQADggEBAGN4uO2xB10zcrbCYjeG
hM3v3rfYaV1vbZlVk/EHf/rtaP+GPIOhv0cdeARKS9VaUXnf4j5a3/nHGDLKVvEv
+ExJqLzgMLTcCKzkSRR+vIETzAmrfsp6xDILMn3yKxTcKRjFGJGVRfuyFH9rMKhQ
M+H4VUQcFGYRPhU+2bxRCxuEHe2tDXBGp7N36SPFJLNSvpf7RYdHPu00n8rKJ69D
XI0fjWnZMbOV7tUWVd/6rW4mhez3xgxW8H8h0IWHY6cdAjO3q0J9PHyaCFB1yZ0A
WOkCYynzE8EVrosIUIko+6IopX5wheTJ0IcU4yCQNo+avzYKMFztVh6eQLoe7afq
GFQ=
-----END CERTIFICATE-----
`
	certByte = `-----BEGIN CERTIFICATE-----
MIIDLjCCAhagAwIBAgIIDOMcH2sIQDowDQYJKoZIhvcNAQELBQAwFTETMBEGA1UE
AxMKa3ViZXJuZXRlczAeFw0yMjEyMjgwMzM4MjNaFw0yMzEyMjgwMzM4MjNaMEEx
FzAVBgNVBAoTDnN5c3RlbTptYXN0ZXJzMSYwJAYDVQQDEx1rdWJlLWFwaXNlcnZl
ci1rdWJlbGV0LWNsaWVudDCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB
AMUtJEadOe43qPTAzphJ+efJXmkTgbsdSHGI7BigqCXOgQ8kEeTQSIVqTLpvpkJ1
fCmv6CbNNQqrABSIvH9oPo1ATY04EreAW5krHdSFaOPO1T/TrySyG7NW5ikEZoji
IBFEQ1B2JbpJWCHsDspaB7BMI/yKgrs2RunTqgLd8VPoGz+QFrXe1DEZ93q7qHqs
U3dW2UD+h8igVLVefXx6NM4e3c1wE2u4IzeUbVVJ/72CpeFmmz3QGiofrvk0NXWY
D9xGmajI1vj5hs+IuN/2lSahZIDfv9Lf2TUDG0faRfnhPluS8X5klicwCOnZQAzD
w3X89RkaRhH3R05ky5wXjYECAwEAAaNWMFQwDgYDVR0PAQH/BAQDAgWgMBMGA1Ud
JQQMMAoGCCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAwHwYDVR0jBBgwFoAUoq7HSvFo
xKX+Szed2as567w83iYwDQYJKoZIhvcNAQELBQADggEBAHV63tYhK7fGSOfL7PDZ
Ox3DySyMy2HYVFXmzFo/netFqARNZB58AqY1iCib1Fg4BJYUpPqfcg7fswft9dY/
1SSekPEfZRz46yPN9JZlMEqtVEoqsNA0qUDWOotPjwb2+vgEroh4+rMk0gqgzx5m
dXqJMpWGIYWNH2Sa8yvHo2qGsShl5/uRNHycBVu2fGHCcLOCfPTslPzZYYJxQ33O
mNW/2WySzy7YL9wLyBRbYPoZK1ATt8ZtmUv/R03a4J8iSKBZwVrn5Yvr5gS+7JNC
ip2++hBi1NIyUYAhdktGas6FZPORtn+kvVs5A/V88EacqkWqVWRW0582gcyL8uJD
QXo=
-----END CERTIFICATE-----
`
	keyByte = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAxS0kRp057jeo9MDOmEn558leaROBux1IcYjsGKCoJc6BDyQR
5NBIhWpMum+mQnV8Ka/oJs01CqsAFIi8f2g+jUBNjTgSt4BbmSsd1IVo487VP9Ov
JLIbs1bmKQRmiOIgEURDUHYluklYIewOyloHsEwj/IqCuzZG6dOqAt3xU+gbP5AW
td7UMRn3eruoeqxTd1bZQP6HyKBUtV59fHo0zh7dzXATa7gjN5RtVUn/vYKl4Wab
PdAaKh+u+TQ1dZgP3EaZqMjW+PmGz4i43/aVJqFkgN+/0t/ZNQMbR9pF+eE+W5Lx
fmSWJzAI6dlADMPDdfz1GRpGEfdHTmTLnBeNgQIDAQABAoIBAQCzW/fWoCjVMB5p
3YVQdGJ2XO+bh5oH+oAufs29LU8nbOxrOHVqfaiqa+K16OAFLleumAwGV757IMfm
5ecJwmq8FJU2853a/FDWSKlO67hZGYlUERwNtlKKVW7yOsWGmKNw8XaGF6MEDLm1
ycQ+f5zk2q4ViG2ZHKtvAhJxnzBqEGtVssHZya4j3E0WJjv1TRlLYxzgIQHgk49p
ysxD23O5EJ/nCexCnZizAKLLNmDDhC4KVVUts3sQVVG5I4wRHfg61w7KiEpLinMA
mYhhomRJKSz46QI/i4Clrsi3et2DjiZdyNmGTSi2TpNL/1pci9qmhh8sUdV6Cqjz
hgAF9OCtAoGBAMzlzGlBJAOnbm8OpItiN+R10AgYEjHe1WBjJicfnAAIXEIY1tpH
KhSN0RplhNZcXZJn45fIP5YRMeRUKp1fRWtluoQp210hkyroScRz1ELFBYoBXnx3
d++KfODcCiGjgFys1VYYWiUT9wgNFJzFMinUcddUtGZWKC37N0OTZlbTAoGBAPZa
W0heH2gz+zQtH2wpqD/ac6aaY9BN/OwHC2Xoya57PJ2jqbHO33hWSUL4qap0pA3G
Ji3Ibnsj81ObXaB3d28Pmtp3BHAZOiBNuI3n3mVqSiwsfTefdAWKAswsqf36yL3w
EVWc0J/OnfDUX9nUWX2w8qE5alqMhCFkmYdY2T3bAoGAdMAwNH1gpxBdVbyzN5TU
okIbMrF8lJwTW2PDlqFlQ4OABk2fBytrp+CTGIZmJbrluoml3pPE356WnjLzQU7L
AIIrwCkVjMCX2egYOG+DsDQRjuxuyV9NoNl5hKr8vuQqPSRiPzeLDfuNVDIX36hh
iAI8h+UFEhbfuCuf9spjku8CgYBzjC/ygosyoeb6Mwvg/Kz4viqugw27/0hZIHi9
JPGr0Au/WKtYRdLVK4uTSPSziaAFAeKYaMFBKryPg3jnsgEn62bTfy1qsrprumiM
zqumX7NIgtl8hGKz0ma7g1t8T+tmAzruL+4+dnfoJISMtCgBZ0R2UGrM68lxrDDC
pe7HLwKBgF9lHHhy76nDW8VMlcEtYIZf329VoqeTMUmvDWFyHAWuY4ZQ4ugAoBUK
9izEbjs0oFHJtF1waWhD9MXJ0BGJK7Zcxy0413CK4dwJT8euSnY81Td7goKTM4Ud
otCqT57JeYWq2hEFromJoSiBgai7weO/E2lAR2Qs99uEPp45q9JQ
-----END RSA PRIVATE KEY-----
`

	newCertByte = `-----BEGIN CERTIFICATE-----
MIIDKDCCAhCgAwIBAgIIYxZk3ye/TxMwDQYJKoZIhvcNAQELBQAwEjEQMA4GA1UE
AxMHZXRjZC1jYTAeFw0yMjEyMjgwMzM4MjRaFw0yMzEyMjgwMzM4MjRaMD4xFzAV
BgNVBAoTDnN5c3RlbTptYXN0ZXJzMSMwIQYDVQQDExprdWJlLWFwaXNlcnZlci1l
dGNkLWNsaWVudDCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAKwHHoOt
iwe3aPgqCcKjwdVpu02UuGQO+tjfQNayPeLWwz9QbHRyVOVOeTnMgc9lHmE6XFcn
99CYsqrasUS6k4MJGpbLLzVU/7uja7mj5cO6LcRu3gCtxYanEBFCC6KHx1tWZuUA
UWN+r9UWpBAf1tByhZKLmRHJh/Zca332OOhD79oAQwDmmNt+jSW2f+bGHji1+k8j
OugCV6lDo2K/ywCklL4nnRbdJ0tWDT3J30AotZVlgzt9QDPKLiw+4LxRaFgQQjgP
Da/TZ/A5g2YVXjvUP/tpX3kppJ43Fd2NlXmDlEmKeqq8KH+HAmoG4hnU3g9N2heE
c90oChRfHE2iquMCAwEAAaNWMFQwDgYDVR0PAQH/BAQDAgWgMBMGA1UdJQQMMAoG
CCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAwHwYDVR0jBBgwFoAUn/K9YUtK7mBi+FRD
AiRmCuf3DFMwDQYJKoZIhvcNAQELBQADggEBADFJE6DUF6FRPLECCxfl4fvtoewp
Q1OPTKM3m50GDU8xM8ir6DBHq9UkO4CEXhMnRTezu+39dQ7McJsfp0Ttgq+ImLVF
uH5wsrgwMk24AGpGbVkh5WHaHPChyBFezdSvO8vi+1hxIA8un4caUXzEj/ptKstU
R9glF1lbzAsjxmL80ZOdWsltX5ZxduyDEIkSyqSwAIZaQp+deJdrBUx3UpVKznd7
/kPv/J2zCjZt8Vp1A+6ikwnFyiIe46Mk/MHCkAvuv5tEh7DFSCtd7ndfT8jlSChz
hO5Jx+cUDzD4du+hY8IwWmTIqBm6hLw31B/qTfd0HMCMf1yDl3ctFwsBKDI=
-----END CERTIFICATE-----
`
	newKeyByte = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEArAceg62LB7do+CoJwqPB1Wm7TZS4ZA762N9A1rI94tbDP1Bs
dHJU5U55OcyBz2UeYTpcVyf30JiyqtqxRLqTgwkalssvNVT/u6NruaPlw7otxG7e
AK3FhqcQEUILoofHW1Zm5QBRY36v1RakEB/W0HKFkouZEcmH9lxrffY46EPv2gBD
AOaY236NJbZ/5sYeOLX6TyM66AJXqUOjYr/LAKSUviedFt0nS1YNPcnfQCi1lWWD
O31AM8ouLD7gvFFoWBBCOA8Nr9Nn8DmDZhVeO9Q/+2lfeSmknjcV3Y2VeYOUSYp6
qrwof4cCagbiGdTeD03aF4Rz3SgKFF8cTaKq4wIDAQABAoIBAHBVxctfDCbh0h4b
9Xuwy+a8wJ8Musw8K/pq70BD7L2wWJeDwQ7Zii6ja+4eabYw5gG/xoTziJQi4qlH
XfLvk1xCGabWz+EXvFefg70aFfQWI8TeUQJId3BSr99VLZvY5onyhgaMiplaJSAV
RNVytSgxYKAtoKtI2ww5lcgPfWHNyQJaJ1WnFclImzbEcFirJHBX+u7ATLPNJs1v
rylPiayVB6zQwKTolPchvgJsCdPGP9iopEAhY0ccduKvqNPcDakGJJYUli0l+b+X
cBp+K8pG8UeWF4NxVNWKlMtfIDg0RkJ3/fI+0M9fyCVU5eSPTP7YMfv3fSIfz4Vx
A/N6ikECgYEAyQqaPNv1Qk54II1SrXf4h8uIM+/eQtZDZpBr4gEuYLCLww3mHnae
V/KJbcoqohEpsQ56n0ndWg3Sw3nvLpomwdg8YJqgY2tlEl0pl3LvXicP7aXWyuj/
FS8oJKQfFkiIH3Env81+TCpEH4HIQGCgjE8vV5eUy00Vqqo4fUvPz7kCgYEA2w4R
0CpDmqVw06F15H3PBEGzOQQof6RKYCEC2F6VunH8amgQaSEQQoLxfP6/feJzpHb6
mvXft5Uccc7dkJDr7Wn1iaHgMwze2Qvpqdm/bvt1jhcHqa6SsOQjk+VBWSByBrby
DZFvUwxNiXWsdqUxoVIFkoe6SyoKFX7F7AC1RXsCgYBxaMO9VS+zqeRmKJLdPHI8
2HoLImM1PP1knE/ffF8XOEB/VhXcVXnZjv4rqwIFzrzAHrTZqqdtp6KfludwWJFI
hJz6uf+EVg78HwXZY4LYkBySKR1T9b//yUxR7yuCPIRdiE2uC1QVzzoCtAmtF1U6
EWlZdi7/yIpSbhfTxrKCMQKBgQCQNC/n0JrWiEjBGM5qT6PjUnjwdNtQQ9AufizI
UWPR7E3VopIDEyAIGPluZqma7mNghm6tamUPDps+FIdpLu4RSaq5IxZbpQJi8eOt
y8mo/uLBWknSGzk4N8dwCgC98oz9/JtV8ULO8g9tCUkyhccpQrymXLF338Hpqp4S
odizVwKBgQCImXprzRCsIJvjsz7pbqj6fvfev/9xxnmlZhHBQq8PRdBubA2wRRLn
lrVcO/z7xgv9knoKvSQ5lZRtACA4/u3ZOzBRr56ZtkvbWH0Ch1QafJ7suomsMHAx
KAGM4g6DY68asv37ATNrYjLZ0MGsArWhKXsbxiR9CrzrNFVVtVIc6g==
-----END RSA PRIVATE KEY-----`
)

var (
	fileStore             = fs.FileSystemOperator{}
	secretGVR             = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	poolCoordinatorSecret = &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      constants.PoolCoordinatorClientSecretName,
			Namespace: constants.PoolCoordinatorClientSecretNamespace,
		},
		TypeMeta: v1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		Data: map[string][]byte{
			"ca.crt":                              []byte(caByte),
			"pool-coordinator-yurthub-client.crt": []byte(certByte),
			"pool-coordinator-yurthub-client.key": []byte(keyByte),
		},
	}

	// Used to test FieldSelector
	otherSecret = &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "default-token",
			Namespace: "default",
		},
		TypeMeta: v1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		Data: map[string][]byte{
			"token": []byte("token"),
		},
	}
)

func TestSecretAdd(t *testing.T) {
	t.Run("CertManager should not react for secret that is not pool-coordinator-yurthub-certs", func(t *testing.T) {
		fakeClient, certMgr, cancel, err := initFakeClientAndCertManager()
		if err != nil {
			t.Errorf("failed to initialize, %v", err)
		}
		defer cancel()

		if err := fakeClient.Tracker().Add(otherSecret); err != nil {
			t.Errorf("failed to add secret %s, %v", otherSecret.Name, err)
		}

		// Expect to timeout which indicates the CertManager does not save the cert
		// that is not pool-coordinator-yurthub-certs.
		err = wait.PollImmediate(50*time.Millisecond, 10*time.Second, func() (done bool, err error) {
			if certMgr.cert != nil {
				return false, fmt.Errorf("unexpect cert initialization")
			}

			if _, err := fileStore.Read(certMgr.GetCaFile()); err == nil {
				return false, fs.ErrExists
			} else if err != fs.ErrNotExists {
				return false, err
			}

			return false, nil
		})

		if err != wait.ErrWaitTimeout {
			t.Errorf("CertManager should not react for add event of secret that is not pool-coordinator-yurthub-certs, %v", err)
		}

		if err := fileStore.DeleteDir(testPKIDir); err != nil {
			t.Errorf("failed to clean test dir %s, %v", testPKIDir, err)
		}
	})

	t.Run("CertManager should react for pool-coordinator-yurthub-certs", func(t *testing.T) {
		fakeClient, certMgr, cancel, err := initFakeClientAndCertManager()
		if err != nil {
			t.Errorf("failed to initialize, %v", err)
		}
		defer cancel()

		if err := fakeClient.Tracker().Add(poolCoordinatorSecret); err != nil {
			t.Errorf("failed to add secret %s, %v", poolCoordinatorSecret.Name, err)
		}

		err = wait.PollImmediate(50*time.Millisecond, 10*time.Second, func() (done bool, err error) {
			return checkSecret(certMgr, poolCoordinatorSecret)
		})

		if err != nil {
			t.Errorf("failed to check poolcoordinator cert, %v", err)
		}

		if err := fileStore.DeleteDir(testPKIDir); err != nil {
			t.Errorf("failed to clean test dir %s, %v", testPKIDir, err)
		}
	})
}

func TestSecretUpdate(t *testing.T) {
	t.Run("CertManager should update cert files when secret is updated", func(t *testing.T) {
		fakeClient, certMgr, cancel, err := initFakeClientAndCertManager()
		if err != nil {
			t.Errorf("failed to initialize, %v", err)
		}
		defer cancel()

		if err := fakeClient.Tracker().Add(poolCoordinatorSecret); err != nil {
			t.Errorf("failed to add secret %s, %v", poolCoordinatorSecret.Name, err)
		}

		err = wait.Poll(50*time.Millisecond, 10*time.Second, func() (done bool, err error) {
			return checkSecret(certMgr, poolCoordinatorSecret)
		})
		if err != nil {
			t.Errorf("failed to wait cert manager to be initialized, %v", err)
		}

		newSecret := poolCoordinatorSecret.DeepCopy()
		newSecret.Data["pool-coordinator-yurthub-client.key"] = []byte(newKeyByte)
		newSecret.Data["pool-coordinator-yurthub-client.crt"] = []byte(newCertByte)
		if err := fakeClient.Tracker().Update(secretGVR, newSecret, newSecret.Namespace); err != nil {
			t.Errorf("failed to update secret, %v", err)
		}

		err = wait.PollImmediate(50*time.Millisecond, 10*time.Second, func() (done bool, err error) {
			return checkSecret(certMgr, newSecret)
		})
		if err != nil {
			t.Errorf("failed to wait cert manager to be updated, %v", err)
		}

		if err := fileStore.DeleteDir(testPKIDir); err != nil {
			t.Errorf("failed to clean test dir %s, %v", testPKIDir, err)
		}
	})
}

func TestSecretDelete(t *testing.T) {
	t.Run("Cert manager should clean cert when secret has been deleted", func(t *testing.T) {
		fakeClient, certMgr, cancel, err := initFakeClientAndCertManager()
		if err != nil {
			t.Errorf("failed to initialize, %v", err)
		}
		defer cancel()

		if err := fakeClient.Tracker().Add(poolCoordinatorSecret); err != nil {
			t.Errorf("failed to add secret %s, %v", poolCoordinatorSecret.Name, err)
		}

		err = wait.PollImmediate(50*time.Millisecond, 10*time.Second, func() (done bool, err error) {
			return checkSecret(certMgr, poolCoordinatorSecret)
		})
		if err != nil {
			t.Errorf("failed to wait cert manager to be initialized, %v", err)
		}

		if err := fakeClient.Tracker().Delete(secretGVR, poolCoordinatorSecret.Namespace, poolCoordinatorSecret.Name); err != nil {
			t.Errorf("failed to delete secret, %v", err)
		}

		err = wait.PollImmediate(50*time.Millisecond, 10*time.Second, func() (done bool, err error) {
			if certMgr.cert == nil {
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			t.Errorf("failed to clean cert, %v", err)
		}

		if err := fileStore.DeleteDir(testPKIDir); err != nil {
			t.Errorf("failed to clean test dir %s, %v", testPKIDir, err)
		}
	})
}

func initFakeClientAndCertManager() (*fake.Clientset, *CertManager, func(), error) {
	fakeClientSet := fake.NewSimpleClientset()
	fakeInformerFactory := informers.NewSharedInformerFactory(fakeClientSet, 0)
	certMgr, err := NewCertManager(testPKIDir, fakeClientSet, fakeInformerFactory)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create cert manager, %v", err)
	}
	stopCh := make(chan struct{})
	fakeInformerFactory.Start(stopCh)

	return fakeClientSet, certMgr, func() { close(stopCh) }, nil
}

func checkSecret(certMgr *CertManager, secret *corev1.Secret) (bool, error) {
	if certMgr.secret == nil {
		return false, nil
	}
	if !reflect.DeepEqual(certMgr.secret, secret) {
		return false, nil
	}

	files := []struct {
		path   string
		expect []byte
	}{
		{
			path:   certMgr.GetFilePath(RootCA),
			expect: secret.Data["ca.crt"],
		},
		{
			path:   certMgr.GetFilePath(YurthubClientCert),
			expect: secret.Data["pool-coordinator-yurthub-client.crt"],
		},
		{
			path:   certMgr.GetFilePath(YurthubClientKey),
			expect: secret.Data["pool-coordinator-yurthub-client.key"],
		},
	}

	for _, f := range files {
		buf, err := fileStore.Read(f.path)
		if err != nil {
			return false, fmt.Errorf("failed to read file at %s, %v", f.path, err)
		}
		if string(buf) != string(f.expect) {
			return false, fmt.Errorf("unexpected value of file %s", f.path)
		}
	}

	return true, nil
}

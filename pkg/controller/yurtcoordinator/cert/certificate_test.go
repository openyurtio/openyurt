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
	"crypto/x509"
	"net"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/keyutil"
)

func TestNewSignedCert(t *testing.T) {
	key, _ := NewPrivateKey()
	caCert, caKey, _ := NewSelfSignedCA()
	client := fake.NewSimpleClientset()
	tests := []struct {
		name string
		cfg  *CertConfig
		err  error
	}{
		{
			"normal config",
			&CertConfig{
				CommonName: "test",
				ExtKeyUsage: []x509.ExtKeyUsage{
					x509.ExtKeyUsageClientAuth,
				},
			},
			nil,
		},
		{
			"config missing commonName",
			&CertConfig{
				CommonName: "",
				ExtKeyUsage: []x509.ExtKeyUsage{
					x509.ExtKeyUsageClientAuth,
				},
			},
			errors.New("must specify a CommonName"),
		},
		{
			"config missing commonName",
			&CertConfig{
				CommonName:  "test",
				ExtKeyUsage: []x509.ExtKeyUsage{},
			},
			errors.New("must specify at least one ExtKeyUsage"),
		},
		{
			"config with empty IP&DNS init",
			&CertConfig{
				CommonName: "test",
				ExtKeyUsage: []x509.ExtKeyUsage{
					x509.ExtKeyUsageClientAuth,
				},
				certInit: func(i kubernetes.Interface, c <-chan struct{}) ([]net.IP, []string, error) {
					return []net.IP{
						net.ParseIP("127.0.0.1"),
					}, []string{"test"}, nil
				},
			},
			nil,
		},
		{
			"config with existing IP&DNS init",
			&CertConfig{
				CommonName: "test",
				ExtKeyUsage: []x509.ExtKeyUsage{
					x509.ExtKeyUsageClientAuth,
				},
				DNSNames: []string{
					"test",
				},
				certInit: func(i kubernetes.Interface, c <-chan struct{}) ([]net.IP, []string, error) {
					return []net.IP{
						net.ParseIP("127.0.0.1"),
					}, []string{"test"}, nil
				},
			},
			nil,
		},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				_, err := NewSignedCert(client, st.cfg, key, caCert, caKey, nil)
				if err != nil && errors.Is(err, st.err) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.err, err)
				}
			}
		}
		t.Run(st.name, tf)
	}
}

func TestLoadCertAndKeyFromSecret(t *testing.T) {

	kubeconfigCert := `-----BEGIN CERTIFICATE-----
MIIEBTCCAu2gAwIBAgIICkYN5aHg5wowDQYJKoZIhvcNAQELBQAwJDEiMCAGA1UE
AxMZb3Blbnl1cnQ6cG9vbC1jb29yZGluYXRvcjAgFw0yMjEyMjkxMzU5MTNaGA8y
MTIyMTIwNTE3MjU0OVowUzEiMCAGA1UEChMZb3Blbnl1cnQ6cG9vbC1jb29yZGlu
YXRvcjEtMCsGA1UEAxMkb3Blbnl1cnQ6cG9vbC1jb29yZGluYXRvcjptb25pdG9y
aW5nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA8hUB10ZUbw5sawnZ
9/R6Nzn2OLbfrKa5rLdqcZ5sGGvT9D7LTgQEuBRUFA3EqpEAhfji5w18hYuSUuDv
cI8+QebBd3i0KFWXR6jI1femHgDZJAze+EJ9Trrvj633wX67ywRtbYCchV9ULKnv
6GM1hximW/nHdpA7XQvuESZLlddp3YmtMbNdeEtcZTVWpfXPHR5UdbdCgq2rlCh6
n5GJo08Lk/uTyvx/JYeqVEgA++QHxRnefxduj6PVmSIkS8RMaiE0/NJUC76VOL8m
UykcoRezHy3ISFPPPa2UYRnyK/XpLN8VaYHJWEqIK/XLf9hhecfWqzn88ASFCrmY
TRRTEQIDAQABo4IBCDCCAQQwDgYDVR0PAQH/BAQDAgWgMBMGA1UdJQQMMAoGCCsG
AQUFBwMCMB8GA1UdIwQYMBaAFFapMgOk/JFaE5GwA82b8Kb26EefMIG7BgNVHREE
gbMwgbCCGnBvb2wtY29vcmRpbmF0b3ItYXBpc2VydmVygiZwb29sLWNvb3JkaW5h
dG9yLWFwaXNlcnZlci5rdWJlLXN5c3RlbYIqcG9vbC1jb29yZGluYXRvci1hcGlz
ZXJ2ZXIua3ViZS1zeXN0ZW0uc3Zjgjhwb29sLWNvb3JkaW5hdG9yLWFwaXNlcnZl
ci5rdWJlLXN5c3RlbS5zdmMuY2x1c3Rlci5sb2NhbIcECmCrqzANBgkqhkiG9w0B
AQsFAAOCAQEAg5QUUrxKVjJuo/FpRBraspkINOYRcMWgsijp1J1mlYvA1Yc0z+UB
vrd8hRlkjEtd0c9tR6OaQ93XOAO3+YZVFQqCwyNg3h5swFC8nCuarTGgi1p4qU4Q
oWndTu2jx5fqJ0k5stybym+cfgNJl3rrcjAzmOFa/mALH1XTV0db2dZAj/VWMb+B
HYfsyrogZVzg9rUe3D0MJdW0spqmvEbUlZHG/1mxUoA+ow8hT8ave2zqRgyMLHGO
64Y8iv7wM77Svukr9gdTTVAxUFHLp0mk58+VhIOlFWrVpisp8NUBdr+OisuxrgAV
BYNXzx6BovA/8xH7UfXz8UbsH0siStdr7A==
-----END CERTIFICATE-----
`
	kubeconfigKey := `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA8hUB10ZUbw5sawnZ9/R6Nzn2OLbfrKa5rLdqcZ5sGGvT9D7L
TgQEuBRUFA3EqpEAhfji5w18hYuSUuDvcI8+QebBd3i0KFWXR6jI1femHgDZJAze
+EJ9Trrvj633wX67ywRtbYCchV9ULKnv6GM1hximW/nHdpA7XQvuESZLlddp3Ymt
MbNdeEtcZTVWpfXPHR5UdbdCgq2rlCh6n5GJo08Lk/uTyvx/JYeqVEgA++QHxRne
fxduj6PVmSIkS8RMaiE0/NJUC76VOL8mUykcoRezHy3ISFPPPa2UYRnyK/XpLN8V
aYHJWEqIK/XLf9hhecfWqzn88ASFCrmYTRRTEQIDAQABAoIBADNc06wqRuXdSJGZ
YH7khz3KdXxpCKIoKcMEk3gR5dt0nV74J8igv6OS5Jfwp+aMp3DFctcVHHN1PpGJ
GiRmsA3peOjxWkAokNVqcVo8lilNgsTMWk6QROf8b7GrdqK+UffsM4+FNzBxHnnv
gHBtBEFqsHlZUMHOLlo6msNWvbjHtxATjKRP7jFkL5gJBzT1xT7kGgNXd63YsW80
x0Q2sJSEmjM+vdG/2FaWewG+YFf5sIWlantVeh2T86caceZmNyUvmW9XUT4azLLW
9egJQV0X62dF2SDiHP798aocIsJMqJ63XltoR3ZRdaXT1XnONnsE89e6ZXeqKLy2
7WqAVUECgYEA+/f1ASCA6kgA75iE7JsX8svRK0zTTzAhI2GgX+fl83GpyXjM1Yv6
18ShmPJywZLtb2GC9LO2Umu3Jz+3q5oltde+bx+21dFQVrqFAXqQ3MjHVu48NAr8
Ai7YgNLzZAu0vadnElZB+CNW26CGRmH3Dd4ljmZ4oDwDhU6vhbCWbdkCgYEA9fSO
RTpVlim2DcvsPo6FhcZ/RrzK7haKtEY6JR6fljVhKZYE/JNVWNSrDQPEFl3AJVWw
y9hN6vIyNrv8FMg5wu8f/G8XBuauCmXIuHsoYQO4ktlN62HUEzgOC5UdDsWE7DND
9rnMdWHIgG1vkdOiww/MDv+6uw57DduZ8xDjc/kCgYAUv+2wQxH6uSVClefUaE1H
lFtMWo5IRilkdYS0gS9hpemaitUrfNSSckHwi37BzCy7cGdNaYNJNE+n7spcWlxi
pjqrggwXfZ5FFiUf4w0M8Yfg88uHaaQpNdxkd3rNsV0YBTIqw2m5WoernIOSRj0H
KlUjbfLfFzIfB0TTGKC6uQKBgDVPbarpqvViUxiIc8tXXu+RB7NQZnfWoPfUJPQ4
wARxy36VCr2oPZ6EchLfFxh195jgCvMUDkd3eZTNiCUFBSgQZpFzjr0rMNwGFcyO
vUDR6qbBvRbg3HPR+ZFfH64898OulPOccAmdSTU1AzLLeYLoIKW7nkC/McLeL280
4OgZAoGBAMvICZ+wkmWYEvL+ESXMLM7ookRt32/ZbMu16f6SMIFxn8kHa0H4Fsjj
AY3yaAbPpTk9/vtsYuSbXTJjDDzVkENMBh7k7BSPkajU5Iy487icHgzAJK9yXaop
kfxJOAEW+ycilk1fntDAXblqMA5qbnIGEB6OYRQH1nGhBkvnXbYj
-----END RSA PRIVATE KEY-----
`
	cert := `-----BEGIN CERTIFICATE-----
MIICRzCCAfGgAwIBAgIJALMb7ecMIk3MMA0GCSqGSIb3DQEBCwUAMH4xCzAJBgNV
BAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNVBAcMBkxvbmRvbjEYMBYGA1UE
CgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1JVCBEZXBhcnRtZW50MRswGQYD
VQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTAwIBcNMTcwNDI2MjMyNjUyWhgPMjExNzA0
MDIyMzI2NTJaMH4xCzAJBgNVBAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNV
BAcMBkxvbmRvbjEYMBYGA1UECgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1J
VCBEZXBhcnRtZW50MRswGQYDVQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTAwXDANBgkq
hkiG9w0BAQEFAANLADBIAkEAtBMa7NWpv3BVlKTCPGO/LEsguKqWHBtKzweMY2CV
tAL1rQm913huhxF9w+ai76KQ3MHK5IVnLJjYYA5MzP2H5QIDAQABo1AwTjAdBgNV
HQ4EFgQU22iy8aWkNSxv0nBxFxerfsvnZVMwHwYDVR0jBBgwFoAU22iy8aWkNSxv
0nBxFxerfsvnZVMwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAANBAEOefGbV
NcHxklaW06w6OBYJPwpIhCVozC1qdxGX1dg8VkEKzjOzjgqVD30m59OFmSlBmHsl
nkVA6wyOSDYBf3o=
-----END CERTIFICATE-----
`
	key := `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAtW5yBzPbwx/TUUkYyJBPp5s1LHCwOfLkuXx9jzttLQU+noNp
nHR+rBP74dMwSUFSI3zE9btT7m/02UNCQVQ0CEJA2GsIB5bQahaQ+AoPqEqjxy2y
YKtTwGGFyQxhGmbQSGyxwUOZtEDy2gO3LqfDsfY36p9YOxodBgjuUHU7n2Mgm2Zg
CR5w/y55VDcDljnyUeFnNR8PiQrAS5rt9Zlh1GUYLckFg+hjq4IYeHUMVrUlRwKB
buA8/3CGnTI8BQDrIidmtRahY6BSSmF/kbLI+UNRqY89V6+ar4W14mlh7AR0C+Ez
ZQpv4a45b7tYHvMdDkGXHPCiC22aDolVPFE2NwIDAQABAoIBAQChTietRar+aV48
p8uUDdH0BycYcrwLWWuHiior3T7sxvvsfJO6GUzB+yZ3mYcVqjC4AmcuvrUZiYpc
W9MdOBrmuNjkUQybireTiqqrDG7tRickn/k2vzxAD7SdkhcHHR/TNGg5lruiKhiA
QstrfJads3X1cGRGb6ocQnZ3LDOtOToZh4ZoAaAtMctzxhud1MqtTeM5Vv7tS4mr
krmnViE8T2ExRiA67lLqGsbNo+ixzaJD8QVM1V+WoMt2Mw2QHVzHLIHCE3ClgAC6
AY5kS2a+QrgE+ppjhmnLqxl+EKgLmFpfZGGL2pvntkwHswfMnKhq23v5U5GmrzWz
M26lSMvZAoGBAOFuQaBTpD/9h01IGpzKGhoQ42gI8LHqWRE83CYxgUGms+4hdEvP
ze4pincD7RlqMEU8aFyD0MmQjOU7rjEt3HYRkZdgGE29yJZuVvnbcf9MJkTf25ue
wzU01aMGw0jXllt8L6oOmuLzLgGxl1V0H27w/NiWuMqBFVmtl7opEvaVAoGBAM4I
xVajh4nNHod+Fh8V+D0n9+RLzzepn4wddSJlb49AFdB0wGD3Za6VJ8aa+d0mkL8M
LrvNgHpcn5mZlZCfsQeqv/Uqtppc0HqjEYc+9bgZWxO9Zr9APwU6oCnyCsxNFVd0
0tKJr5962RwKk0Z7y86F/nlo7+AbVCRNTu9B1cKbAoGAORYGoGcN7PZy0Os1cgbr
3TXxoGLDMQq7S1YyGannpYxlfCQUoy4YY/s5CTKBVDJDzwShGOx4btKgG1ylm+aV
MYD5cW/wN5+bsBx5AgTENXY/KqnVnu7xWAPtJb+MrGGLvdcQ6uuP5XDXca5bOFST
sTBtlxtz6DQQCAmhpo7IMpECgYAXN4PNPIY0cAnFqN6jSB19/rf/YM+L7TBOYK9n
XdjRYp5SrCVVh+tMXgBqb+JCGmtrK9tETGby4ucVLupcrrILNCGHZfXHtTfE6gU6
oUydHzZVJh2i5YF0fGO59k1jMjh6b26mTN+ecABxGXv5EFAqCI1hbwLA1TOJF7ES
Yu/MiwKBgDU6WWgaB8CZbLLDbQ8uD/inEwqZU5ilr2ZpwfNtccKRns+dAyaL79tn
KyWIktEBSjwQRabtVcGHvhMhC09iJft8MPcrwvkUhw/BWtgCiouD0+FLsDOqWbCV
hWftZ5Ow8AEBf65jYMvy5lp+xTGbntGAmtKUopSjRcD0H+ilg5KQ
-----END RSA PRIVATE KEY-----
`

	kubeConfigStr := `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURLekNDQWhPZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFrTVNJd0lBWURWUVFERXhsdmNHVnUKZVhWeWREcHdiMjlzTFdOdmIzSmthVzVoZEc5eU1CNFhEVEl5TVRJeU9URXpOVGt4TTFvWERUTXlNVEl5TmpFegpOVGt4TTFvd0pERWlNQ0FHQTFVRUF4TVpiM0JsYm5sMWNuUTZjRzl2YkMxamIyOXlaR2x1WVhSdmNqQ0NBU0l3CkRRWUpLb1pJaHZjTkFRRUJCUUFEZ2dFUEFEQ0NBUW9DZ2dFQkFLKzdTeDFDKzVzN1FOUkwycHp6TW1PZTU5VFUKY0NEMC9QSkI0eGFEOFlDNTBiWjFXMDEvS24wR3RGK3VEK2hvOWxnMTloRWNyWXNpKzI5K2VtMFAvZXozMGNxRAppYkpmV3Rnc2dMS2s2cFMwWEkxdngzQkZWczcyMk85VTkzUTFFSHFXM0VoTytjdHBHRXhXVmVscytHVXJjcHRhCjJMK3dUczl1R3BHZmRBOXIxdVA0TU1qTGs4U0c0STJVN21YdHV2Y0ZyVHRuVDVTTk5GNU0rTHlDY2ZjT3o1bjEKK2JCdjdyMG0vd200VkV5a2xQK2JSR3pXcDdVdG1EYVRvKzVnOXluKzJ0bjF0SW16cTIzOXZkQzh6cnArMkNHTQoxNkVoNnZTb2pFZGNqN2crNDFnTG9SLzV0aEkyckRUMnZiRFRULzAxWFlxdHVHb09tNFdZUVlJQjhDOENBd0VBCkFhTm9NR1l3RGdZRFZSMFBBUUgvQkFRREFnS2tNQThHQTFVZEV3RUIvd1FGTUFNQkFmOHdIUVlEVlIwT0JCWUUKRkZhcE1nT2svSkZhRTVHd0E4MmI4S2IyNkVlZk1DUUdBMVVkRVFRZE1CdUNHVzl3Wlc1NWRYSjBPbkJ2YjJ3dApZMjl2Y21ScGJtRjBiM0l3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQUd3ck5yK1NuNzFYWE5OMjZmejR1eHV1CjJmZndNZlNZbzMyTEcrSHB0WEtJOHVvSjZPWEhBKysrd2hNT1pORzB1S2FsMnNxUjk5NG5sRXNKNnpwWmZSazgKQVhWcWk5dHlFM0hIQXVyOHA0TDBZcUxGY2lNWTJxWjhYVjVxNVowdWd6MWFXcjZ0U2VrK25aRnpqT0tyZjJ1TwpxWWJ0K1ZTQU81aWU0b01FM0pZZ3VmdWdNUWh5RDVPbHEwTVMwUWt5T29YSmVQanBhZm9PSlIyaGNNS1NhTnorCkJHOGkvS04rTmdBVVgyNW50UnI5THhBUGhwOHRvZDZ3VWl6QjZLSm1VMk9WdkhGYVY2S1h1T3VORGoyNHFDZnoKbXZ2cDVGU2w0YmdEbzJmUElaVSs0S3YvSiticzU1TXVJQy9VcFNqV2RMdjZQdGNMRnRKQjRBNCtBV3Q1ZFBrPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==
    server: https://xx.xx.xx.xx:443
    name: cluster
contexts:
- context:
    cluster: cluster
    user: openyurt:yurt-coordinator:monitoring
  name: openyurt:yurt-coordinator:monitoring@cluster
current-context: openyurt:yurt-coordinator:monitoring@cluster
kind: Config
users:
- name: openyurt:yurt-coordinator:monitoring
  user:
    client-certificate-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUVCVENDQXUyZ0F3SUJBZ0lJQ2tZTjVhSGc1d293RFFZSktvWklodmNOQVFFTEJRQXdKREVpTUNBR0ExVUUKQXhNWmIzQmxibmwxY25RNmNHOXZiQzFqYjI5eVpHbHVZWFJ2Y2pBZ0Z3MHlNakV5TWpreE16VTVNVE5hR0E4eQpNVEl5TVRJd05URTNNalUwT1Zvd1V6RWlNQ0FHQTFVRUNoTVpiM0JsYm5sMWNuUTZjRzl2YkMxamIyOXlaR2x1CllYUnZjakV0TUNzR0ExVUVBeE1rYjNCbGJubDFjblE2Y0c5dmJDMWpiMjl5WkdsdVlYUnZjanB0YjI1cGRHOXkKYVc1bk1JSUJJakFOQmdrcWhraUc5dzBCQVFFRkFBT0NBUThBTUlJQkNnS0NBUUVBOGhVQjEwWlVidzVzYXduWgo5L1I2TnpuMk9MYmZyS2E1ckxkcWNaNXNHR3ZUOUQ3TFRnUUV1QlJVRkEzRXFwRUFoZmppNXcxOGhZdVNVdUR2CmNJOCtRZWJCZDNpMEtGV1hSNmpJMWZlbUhnRFpKQXplK0VKOVRycnZqNjMzd1g2N3l3UnRiWUNjaFY5VUxLbnYKNkdNMWh4aW1XL25IZHBBN1hRdnVFU1pMbGRkcDNZbXRNYk5kZUV0Y1pUVldwZlhQSFI1VWRiZENncTJybENoNgpuNUdKbzA4TGsvdVR5dngvSlllcVZFZ0ErK1FIeFJuZWZ4ZHVqNlBWbVNJa1M4Uk1haUUwL05KVUM3NlZPTDhtClV5a2NvUmV6SHkzSVNGUFBQYTJVWVJueUsvWHBMTjhWYVlISldFcUlLL1hMZjloaGVjZldxem44OEFTRkNybVkKVFJSVEVRSURBUUFCbzRJQkNEQ0NBUVF3RGdZRFZSMFBBUUgvQkFRREFnV2dNQk1HQTFVZEpRUU1NQW9HQ0NzRwpBUVVGQndNQ01COEdBMVVkSXdRWU1CYUFGRmFwTWdPay9KRmFFNUd3QTgyYjhLYjI2RWVmTUlHN0JnTlZIUkVFCmdiTXdnYkNDR25CdmIyd3RZMjl2Y21ScGJtRjBiM0l0WVhCcGMyVnlkbVZ5Z2lad2IyOXNMV052YjNKa2FXNWgKZEc5eUxXRndhWE5sY25abGNpNXJkV0psTFhONWMzUmxiWUlxY0c5dmJDMWpiMjl5WkdsdVlYUnZjaTFoY0dsegpaWEoyWlhJdWEzVmlaUzF6ZVhOMFpXMHVjM1pqZ2pod2IyOXNMV052YjNKa2FXNWhkRzl5TFdGd2FYTmxjblpsCmNpNXJkV0psTFhONWMzUmxiUzV6ZG1NdVkyeDFjM1JsY2k1c2IyTmhiSWNFQ21DcnF6QU5CZ2txaGtpRzl3MEIKQVFzRkFBT0NBUUVBZzVRVVVyeEtWakp1by9GcFJCcmFzcGtJTk9ZUmNNV2dzaWpwMUoxbWxZdkExWWMweitVQgp2cmQ4aFJsa2pFdGQwYzl0UjZPYVE5M1hPQU8zK1laVkZRcUN3eU5nM2g1c3dGQzhuQ3VhclRHZ2kxcDRxVTRRCm9XbmRUdTJqeDVmcUowazVzdHlieW0rY2ZnTkpsM3JyY2pBem1PRmEvbUFMSDFYVFYwZGIyZFpBai9WV01iK0IKSFlmc3lyb2daVnpnOXJVZTNEME1KZFcwc3BxbXZFYlVsWkhHLzFteFVvQStvdzhoVDhhdmUyenFSZ3lNTEhHTwo2NFk4aXY3d003N1N2dWtyOWdkVFRWQXhVRkhMcDBtazU4K1ZoSU9sRldyVnBpc3A4TlVCZHIrT2lzdXhyZ0FWCkJZTlh6eDZCb3ZBLzh4SDdVZlh6OFVic0gwc2lTdGRyN0E9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==
    client-key-data: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFb3dJQkFBS0NBUUVBOGhVQjEwWlVidzVzYXduWjkvUjZOem4yT0xiZnJLYTVyTGRxY1o1c0dHdlQ5RDdMClRnUUV1QlJVRkEzRXFwRUFoZmppNXcxOGhZdVNVdUR2Y0k4K1FlYkJkM2kwS0ZXWFI2akkxZmVtSGdEWkpBemUKK0VKOVRycnZqNjMzd1g2N3l3UnRiWUNjaFY5VUxLbnY2R00xaHhpbVcvbkhkcEE3WFF2dUVTWkxsZGRwM1ltdApNYk5kZUV0Y1pUVldwZlhQSFI1VWRiZENncTJybENoNm41R0pvMDhMay91VHl2eC9KWWVxVkVnQSsrUUh4Um5lCmZ4ZHVqNlBWbVNJa1M4Uk1haUUwL05KVUM3NlZPTDhtVXlrY29SZXpIeTNJU0ZQUFBhMlVZUm55Sy9YcExOOFYKYVlISldFcUlLL1hMZjloaGVjZldxem44OEFTRkNybVlUUlJURVFJREFRQUJBb0lCQUROYzA2d3FSdVhkU0pHWgpZSDdraHozS2RYeHBDS0lvS2NNRWszZ1I1ZHQwblY3NEo4aWd2Nk9TNUpmd3ArYU1wM0RGY3RjVkhITjFQcEdKCkdpUm1zQTNwZU9qeFdrQW9rTlZxY1ZvOGxpbE5nc1RNV2s2UVJPZjhiN0dyZHFLK1VmZnNNNCtGTnpCeEhubnYKZ0hCdEJFRnFzSGxaVU1IT0xsbzZtc05XdmJqSHR4QVRqS1JQN2pGa0w1Z0pCelQxeFQ3a0dnTlhkNjNZc1c4MAp4MFEyc0pTRW1qTSt2ZEcvMkZhV2V3RytZRmY1c0lXbGFudFZlaDJUODZjYWNlWm1OeVV2bVc5WFVUNGF6TExXCjllZ0pRVjBYNjJkRjJTRGlIUDc5OGFvY0lzSk1xSjYzWGx0b1IzWlJkYVhUMVhuT05uc0U4OWU2WlhlcUtMeTIKN1dxQVZVRUNnWUVBKy9mMUFTQ0E2a2dBNzVpRTdKc1g4c3ZSSzB6VFR6QWhJMkdnWCtmbDgzR3B5WGpNMVl2NgoxOFNobVBKeXdaTHRiMkdDOUxPMlVtdTNKeiszcTVvbHRkZStieCsyMWRGUVZycUZBWHFRM01qSFZ1NDhOQXI4CkFpN1lnTkx6WkF1MHZhZG5FbFpCK0NOVzI2Q0dSbUgzRGQ0bGptWjRvRHdEaFU2dmhiQ1diZGtDZ1lFQTlmU08KUlRwVmxpbTJEY3ZzUG82RmhjWi9ScnpLN2hhS3RFWTZKUjZmbGpWaEtaWUUvSk5WV05TckRRUEVGbDNBSlZXdwp5OWhONnZJeU5ydjhGTWc1d3U4Zi9HOFhCdWF1Q21YSXVIc29ZUU80a3RsTjYySFVFemdPQzVVZERzV0U3RE5ECjlybk1kV0hJZ0cxdmtkT2l3dy9NRHYrNnV3NTdEZHVaOHhEamMva0NnWUFVdisyd1F4SDZ1U1ZDbGVmVWFFMUgKbEZ0TVdvNUlSaWxrZFlTMGdTOWhwZW1haXRVcmZOU1Nja0h3aTM3QnpDeTdjR2ROYVlOSk5FK243c3BjV2x4aQpwanFyZ2d3WGZaNUZGaVVmNHcwTThZZmc4OHVIYWFRcE5keGtkM3JOc1YwWUJUSXF3Mm01V29lcm5JT1NSajBICktsVWpiZkxmRnpJZkIwVFRHS0M2dVFLQmdEVlBiYXJwcXZWaVV4aUljOHRYWHUrUkI3TlFabmZXb1BmVUpQUTQKd0FSeHkzNlZDcjJvUFo2RWNoTGZGeGgxOTVqZ0N2TVVEa2QzZVpUTmlDVUZCU2dRWnBGempyMHJNTndHRmN5Twp2VURSNnFiQnZSYmczSFBSK1pGZkg2NDg5OE91bFBPY2NBbWRTVFUxQXpMTGVZTG9JS1c3bmtDL01jTGVMMjgwCjRPZ1pBb0dCQU12SUNaK3drbVdZRXZMK0VTWE1MTTdvb2tSdDMyL1piTXUxNmY2U01JRnhuOGtIYTBINEZzamoKQVkzeWFBYlBwVGs5L3Z0c1l1U2JYVEpqRER6VmtFTk1CaDdrN0JTUGthalU1SXk0ODdpY0hnekFKSzl5WGFvcAprZnhKT0FFVyt5Y2lsazFmbnREQVhibHFNQTVxYm5JR0VCNk9ZUlFIMW5HaEJrdm5YYllqCi0tLS0tRU5EIFJTQSBQUklWQVRFIEtFWS0tLS0tCg==`

	emptySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: YurtCoordinatorNS,
		},
		Data: make(map[string][]byte),
	}
	kubeConfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: YurtCoordinatorNS,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(kubeConfigStr),
		},
	}
	certSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: YurtCoordinatorNS,
		},
		Data: map[string][]byte{
			"test.crt": []byte(cert),
			"test.key": []byte(key),
		},
	}

	tests := []struct {
		name   string
		client kubernetes.Interface
		cfg    CertConfig
		cert   []byte
		key    []byte
	}{
		{
			name:   "no secret",
			client: fake.NewSimpleClientset(),
			cfg: CertConfig{
				IsKubeConfig: false,
				SecretName:   "test",
			},
			cert: nil,
			key:  nil,
		},
		{
			name:   "empty secret",
			client: fake.NewSimpleClientset(emptySecret),
			cfg: CertConfig{
				IsKubeConfig: false,
				SecretName:   "test",
			},
			cert: nil,
			key:  nil,
		},
		{
			name:   "kubeconfig secret",
			client: fake.NewSimpleClientset(kubeConfigSecret),
			cfg: CertConfig{
				IsKubeConfig: true,
				CertName:     "kubeconfig",
				SecretName:   "test",
				CommonName:   "openyurt:yurt-coordinator:monitoring",
			},
			cert: []byte(kubeconfigCert),
			key:  []byte(kubeconfigKey),
		},
		{
			name:   "cert secret",
			client: fake.NewSimpleClientset(certSecret),
			cfg: CertConfig{
				IsKubeConfig: false,
				SecretName:   "test",
				CertName:     "test",
			},
			cert: []byte(cert),
			key:  []byte(key),
		},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				cert, key, _ := loadCertAndKeyFromSecret(st.client, st.cfg)

				certBytes, _ := EncodeCertPEM(cert)
				keyBytes, _ := keyutil.MarshalPrivateKeyToPEM(key)

				assert.Equal(t, st.cert, certBytes)
				assert.Equal(t, st.key, keyBytes)
			}
		}
		t.Run(st.name, tf)
	}
}

// Create a fake client which have an YurtCoordinatorAPIServer SVC
func newClientWithYurtCoordinatorAPIServerSVC(objects ...runtime.Object) *fake.Clientset {
	objects = append(objects, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: YurtCoordinatorNS,
			Name:      YurtCoordinatorAPIServerSVC,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "xxxx",
			Ports: []corev1.ServicePort{
				{
					Port: 644,
				},
			},
		}})
	return fake.NewSimpleClientset(objects...)
}

func TestInitYurtCoordinatorCert(t *testing.T) {
	caCert, caKey, _ := NewSelfSignedCA()

	tests := []struct {
		name   string
		client kubernetes.Interface
		cfg    CertConfig
	}{
		{
			"normal cert init",
			fake.NewSimpleClientset(),
			CertConfig{
				CertName:   "test",
				SecretName: "test",
				CommonName: "test",
				ExtKeyUsage: []x509.ExtKeyUsage{
					x509.ExtKeyUsageClientAuth,
				},
			},
		},
		{
			"cert init with existing secret ",
			fake.NewSimpleClientset(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: YurtCoordinatorNS,
				},
				Data: map[string][]byte{
					"test.crt": nil,
					"test.key": nil,
				},
			}),
			CertConfig{
				CertName:   "test",
				SecretName: "test",
				CommonName: "test",
				ExtKeyUsage: []x509.ExtKeyUsage{
					x509.ExtKeyUsageClientAuth,
				},
			},
		},
		{
			"normal kubeconfig init",
			newClientWithYurtCoordinatorAPIServerSVC(),
			CertConfig{
				IsKubeConfig: true,
				CertName:     "test",
				SecretName:   "test",
				CommonName:   "test",
				ExtKeyUsage: []x509.ExtKeyUsage{
					x509.ExtKeyUsageClientAuth,
				},
			},
		},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				err := initYurtCoordinatorCert(st.client, st.cfg, caCert, caKey, nil)
				assert.Nil(t, err)
			}
		}
		t.Run(st.name, tf)
	}
}

func TestIsCertFromCA(t *testing.T) {
	client := fake.NewSimpleClientset()

	caCert1, caKey1, _ := NewSelfSignedCA()
	caCert2, _, _ := NewSelfSignedCA()
	key, _ := NewPrivateKey()

	ca1Cert1, _ := NewSignedCert(client, &CertConfig{
		CommonName: "test",
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
	}, key, caCert1, caKey1, nil)

	assert.Equal(t, true, IsCertFromCA(ca1Cert1, caCert1))
	assert.Equal(t, false, IsCertFromCA(ca1Cert1, caCert2))
}

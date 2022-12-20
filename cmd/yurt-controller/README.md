
kubebuiler

os=$(go env GOOS)
arch=$(go env GOARCH)

https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.2.0/kubebuilder_${os}_${arch}


kubebuilder init --domain openyurt.io

kubebuilder create api --group apps --version v1beta1 --kind Sample --force --plural samples

kubebuilder create webhook --group apps --version v1beta1 --kind Sample --defaulting --programmatic-validation --conversion --plural samples


# script namespace

//+genclient

package kubernetes

import (
	appsv1 "k8s.io/api/apps/v1"
	"testing"
)

const testDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:latest
        ports:
        - containerPort: 80
`

func TestYamlToObject(t *testing.T) {
	obj, err := YamlToObject([]byte(testDeployment))
	if err != nil {
		t.Fatalf("YamlToObj failed: %s", err)
	}

	nd, ok := obj.(*appsv1.Deployment)
	if !ok {
		t.Fatalf("Fail to assert deployment: %s", err)
	}

	if nd.GetName() != "nginx-deployment" {
		t.Fatalf("YamlToObj failed: want \"nginx-deployment\" get \"%s\"", nd.GetName())
	}

	val, exist := nd.GetLabels()["app"]
	if !exist {
		t.Fatal("YamlToObj failed: label \"app\" doesnot exist")
	}
	if val != "nginx" {
		t.Fatalf("YamlToObj failed: want \"nginx\" get %s", val)
	}

	if *nd.Spec.Replicas != 3 {
		t.Fatalf("YamlToObj failed: want 3 get %d", *nd.Spec.Replicas)
	}
}

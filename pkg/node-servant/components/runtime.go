/*
Copyright 2021 The OpenYurt Authors.

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

package components

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"
	utilsexec "k8s.io/utils/exec"
)

const (
	dockerSocket     = "/var/run/docker.sock" // The Docker socket is not CRI compatible
	containerdSocket = "/run/containerd/containerd.sock"
	// DefaultDockerCRISocket defines the default Docker CRI socket
	DefaultDockerCRISocket = "/var/run/dockershim.sock"
	defaultKubeletConfig   = "/var/lib/kubelet/config.yaml"
	defaultKubeletEnvFile  = "/etc/default/kubelet"
	defaultSysconfigFile   = "/etc/sysconfig/kubelet"

	// PullImageRetry specifies how many times ContainerRuntime retries when pulling image failed
	PullImageRetry = 5
)

// ContainerRuntime is an interface for working with container runtimes
type ContainerRuntimeForImage interface {
	IsDocker() bool
	PullImage(image string) error
	ImageExists(image string) (bool, error)
	ListKubeContainers() ([]KubeContainer, error)
	StopContainer(containerID string) error
}

// KubeContainer describes one Kubernetes-managed container discovered from the runtime.
type KubeContainer struct {
	ID            string
	Namespace     string
	PodName       string
	ContainerName string
}

// CRIRuntime is a struct that interfaces with the CRI
type CRIRuntime struct {
	exec      utilsexec.Interface
	criSocket string
}

// DockerRuntime is a struct that interfaces with the Docker daemon
type DockerRuntime struct {
	exec utilsexec.Interface
}

// NewContainerRuntime sets up and returns a ContainerRuntime struct
func NewContainerRuntimeForImage(execer utilsexec.Interface, criSocket string) (ContainerRuntimeForImage, error) {
	var toolName string
	var runtime ContainerRuntimeForImage

	if criSocket != DefaultDockerCRISocket {
		toolName = "crictl"
		// !!! temporary work around crictl warning:
		// Using "/var/run/crio/crio.sock" as endpoint is deprecated,
		// please consider using full url format "unix:///var/run/crio/crio.sock"
		if filepath.IsAbs(criSocket) && goruntime.GOOS != "windows" {
			criSocket = "unix://" + criSocket
		}
		runtime = &CRIRuntime{execer, criSocket}
	} else {
		toolName = "docker"
		runtime = &DockerRuntime{execer}
	}

	if _, err := execer.LookPath(toolName); err != nil {
		return nil, errors.Wrapf(err, "%s is required for container runtime", toolName)
	}
	return runtime, nil
}

// IsDocker returns true if the runtime is docker
func (runtime *CRIRuntime) IsDocker() bool {
	return false
}

// IsDocker returns true if the runtime is docker
func (runtime *DockerRuntime) IsDocker() bool {
	return true
}

// PullImage pulls the image
func (runtime *CRIRuntime) PullImage(image string) error {
	var err error
	var out []byte
	for i := 0; i < PullImageRetry; i++ {
		out, err = runtime.exec.Command("crictl", "-r", runtime.criSocket, "pull", image).CombinedOutput()
		if err == nil {
			return nil
		}
	}
	return errors.Wrapf(err, "output: %s, error", out)
}

// PullImage pulls the image
func (runtime *DockerRuntime) PullImage(image string) error {
	var err error
	var out []byte
	for i := 0; i < PullImageRetry; i++ {
		out, err = runtime.exec.Command("docker", "pull", image).CombinedOutput()
		if err == nil {
			return nil
		}
	}
	return errors.Wrapf(err, "output: %s, error", out)
}

// ImageExists checks to see if the image exists on the system
func (runtime *CRIRuntime) ImageExists(image string) (bool, error) {
	err := runtime.exec.Command("crictl", "-r", runtime.criSocket, "inspecti", image).Run()
	return err == nil, nil
}

// ImageExists checks to see if the image exists on the system
func (runtime *DockerRuntime) ImageExists(image string) (bool, error) {
	err := runtime.exec.Command("docker", "inspect", image).Run()
	return err == nil, nil
}

func (runtime *CRIRuntime) ListKubeContainers() ([]KubeContainer, error) {
	out, err := runtime.exec.Command("crictl", "-r", runtime.criSocket, "ps", "-o", "json").CombinedOutput()
	if err != nil {
		return nil, errors.Wrapf(err, "output: %s, error", out)
	}
	return parseCRIContainerListOutput(out)
}

func (runtime *DockerRuntime) ListKubeContainers() ([]KubeContainer, error) {
	const format = `{{.ID}}	{{.Label "io.kubernetes.pod.namespace"}}	{{.Label "io.kubernetes.pod.name"}}	{{.Label "io.kubernetes.container.name"}}`

	out, err := runtime.exec.Command("docker", "ps", "--format", format).CombinedOutput()
	if err != nil {
		return nil, errors.Wrapf(err, "output: %s, error", out)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}

	lines := strings.Split(trimmed, "\n")
	containers := make([]KubeContainer, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("unexpected docker ps output line %q", line)
		}

		containers = append(containers, KubeContainer{
			ID:            parts[0],
			Namespace:     parts[1],
			PodName:       parts[2],
			ContainerName: parts[3],
		})
	}

	return containers, nil
}

func (runtime *CRIRuntime) StopContainer(containerID string) error {
	out, err := runtime.exec.Command("crictl", "-r", runtime.criSocket, "stop", containerID).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "output: %s, error", out)
	}
	return nil
}

func (runtime *DockerRuntime) StopContainer(containerID string) error {
	out, err := runtime.exec.Command("docker", "stop", containerID).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "output: %s, error", out)
	}
	return nil
}

var (
	systemdQuotedValueRegexp = regexp.MustCompile(`"([^"]*)"|'([^']*)'`)
	kubeletConfigFieldRegexp = regexp.MustCompile(`(?m)^\s*containerRuntimeEndpoint:\s*["']?([^"'\s#]+)`)
)

// detectCRISocketImpl is separated out only for test purposes, DON'T call it directly, use DetectCRISocket instead
func detectCRISocketImpl(isSocket func(string) bool) (string, error) {
	foundCRISockets := []string{}
	knownCRISockets := []string{
		// Docker and containerd sockets are special cased below, hence not to be included here
		"/var/run/crio/crio.sock",
	}

	if isSocket(dockerSocket) {
		// the path in dockerSocket is not CRI compatible, hence we should replace it with a CRI compatible socket
		foundCRISockets = append(foundCRISockets, DefaultDockerCRISocket)
	} else if isSocket(containerdSocket) {
		// Docker 18.09 gets bundled together with containerd, thus having both dockerSocket and containerdSocket present.
		// For compatibility reasons, we use the containerd socket only if Docker is not detected.
		foundCRISockets = append(foundCRISockets, containerdSocket)
	}

	for _, socket := range knownCRISockets {
		if isSocket(socket) {
			foundCRISockets = append(foundCRISockets, socket)
		}
	}

	switch len(foundCRISockets) {
	case 0:
		// Fall back to Docker if no CRI is detected, we can error out later on if we need it
		return DefaultDockerCRISocket, nil
	case 1:
		// Precisely one CRI found, use that
		return foundCRISockets[0], nil
	default:
		// Multiple CRIs installed?
		return "", errors.Errorf("Found multiple CRI sockets, please use --cri-socket to select one: %s", strings.Join(foundCRISockets, ", "))
	}

}

// isExistingSocket checks if path exists and is domain socket
func isExistingSocket(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}

	return fileInfo.Mode()&os.ModeSocket != 0
}

// DetectCRISocket uses a list of known CRI sockets to detect one. If more than one or none is discovered, an error is returned.
func DetectCRISocket() (string, error) {
	if socket := detectCRISocketFromKubelet(os.ReadFile, isExistingSocket, GetDefaultKubeadmConfPath()); socket != "" {
		klog.Infof("detectCRISocket: use kubelet runtime endpoint %s", socket)
		return socket, nil
	}
	return detectCRISocketImpl(isExistingSocket)
}

func detectCRISocketFromKubelet(readFile func(string) ([]byte, error), isSocket func(string) bool, kubeletServiceConfigPaths []string) string {
	argSources := make([]string, 0, 8)
	envFiles := append([]string{}, kubeAdmFlagsEnvFile, defaultKubeletEnvFile, defaultSysconfigFile)
	configFiles := []string{defaultKubeletConfig}

	for _, path := range kubeletServiceConfigPaths {
		content, err := readFile(path)
		if err != nil {
			continue
		}

		text := string(content)
		argSources = append(argSources, extractEnvironmentArgs(text)...)
		argSources = append(argSources, extractExecStartArgs(text)...)
		envFiles = append(envFiles, extractEnvironmentFiles(text)...)
	}

	for _, path := range uniqueStrings(envFiles) {
		content, err := readFile(path)
		if err != nil {
			continue
		}
		argSources = append(argSources, extractEnvironmentArgs(string(content))...)
	}

	if socket := normalizeDetectedCRISocket(extractLastFlagValue(argSources, "container-runtime-endpoint")); socket != "" {
		if isReachableCRISocket(socket, isSocket) {
			return socket
		}
		klog.Warningf("detectCRISocket: kubelet runtime endpoint %s does not exist on host, falling back to socket probing", socket)
	}

	if configPath := strings.TrimSpace(extractLastFlagValue(argSources, "config")); configPath != "" {
		configFiles = append([]string{configPath}, configFiles...)
	}

	for _, path := range uniqueStrings(configFiles) {
		content, err := readFile(path)
		if err != nil {
			continue
		}

		matches := kubeletConfigFieldRegexp.FindStringSubmatch(string(content))
		if len(matches) != 2 {
			continue
		}

		if socket := normalizeDetectedCRISocket(matches[1]); socket != "" {
			if isReachableCRISocket(socket, isSocket) {
				return socket
			}
			klog.Warningf("detectCRISocket: kubelet config runtime endpoint %s does not exist on host, falling back to socket probing", socket)
		}
	}

	return ""
}

func extractEnvironmentFiles(content string) []string {
	var files []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, "EnvironmentFile=") {
			continue
		}

		for _, token := range extractLineTokens(strings.TrimPrefix(line, "EnvironmentFile=")) {
			token = strings.TrimSpace(strings.Trim(token, `"'`))
			token = strings.TrimPrefix(token, "-")
			if token != "" {
				files = append(files, token)
			}
		}
	}
	return files
}

func extractEnvironmentArgs(content string) []string {
	var args []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "Environment=") {
			line = strings.TrimPrefix(line, "Environment=")
			for _, token := range extractLineTokens(line) {
				if idx := strings.Index(token, "="); idx > 0 {
					value := strings.TrimSpace(strings.Trim(token[idx+1:], `"'`))
					if strings.Contains(value, "--") {
						args = append(args, value)
					}
				}
			}
			continue
		}

		if idx := strings.Index(line, "="); idx > 0 {
			value := strings.TrimSpace(strings.Trim(line[idx+1:], `"'`))
			if strings.Contains(value, "--") {
				args = append(args, value)
			}
		}
	}
	return args
}

func extractExecStartArgs(content string) []string {
	var args []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, "ExecStart=") {
			continue
		}

		command := strings.TrimSpace(strings.TrimPrefix(line, "ExecStart="))
		if strings.Contains(command, "--") {
			args = append(args, command)
		}
	}
	return args
}

func extractLineTokens(line string) []string {
	tokens := make([]string, 0, 4)
	seenQuoted := false
	for _, match := range systemdQuotedValueRegexp.FindAllStringSubmatch(line, -1) {
		value := match[1]
		if value == "" {
			value = match[2]
		}
		if value != "" {
			tokens = append(tokens, value)
			seenQuoted = true
		}
	}
	if seenQuoted {
		return tokens
	}
	return strings.Fields(line)
}

func extractLastFlagValue(argSources []string, flag string) string {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?:^|[\s"])--%s(?:=|\s+)([^"'\s]+)`, regexp.QuoteMeta(flag)))
	var value string
	for _, source := range argSources {
		for _, match := range pattern.FindAllStringSubmatch(source, -1) {
			if len(match) == 2 {
				value = match[1]
			}
		}
	}
	return strings.TrimSpace(strings.Trim(value, `"'`))
}

func normalizeDetectedCRISocket(socket string) string {
	socket = strings.TrimSpace(strings.Trim(socket, `"'`))
	socket = strings.TrimPrefix(socket, "unix://")
	if socket == "" {
		return ""
	}
	if socket == dockerSocket {
		return DefaultDockerCRISocket
	}
	if filepath.IsAbs(socket) {
		return filepath.Clean(socket)
	}
	return socket
}

func isReachableCRISocket(socket string, isSocket func(string) bool) bool {
	if socket == DefaultDockerCRISocket {
		return isSocket(dockerSocket) || isSocket(DefaultDockerCRISocket)
	}
	return isSocket(socket)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func parseCRIContainerListOutput(out []byte) ([]KubeContainer, error) {
	type criList struct {
		Containers []struct {
			ID       string `json:"id"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Labels map[string]string `json:"labels"`
		} `json:"containers"`
	}

	jsonPayload, err := extractJSONPayload(out)
	if err != nil {
		return nil, errors.Wrapf(err, "parse crictl ps output: %s", out)
	}

	var list criList
	if err := json.Unmarshal(jsonPayload, &list); err != nil {
		return nil, errors.Wrapf(err, "parse crictl ps output: %s", out)
	}

	containers := make([]KubeContainer, 0, len(list.Containers))
	for _, container := range list.Containers {
		podName := container.Labels["io.kubernetes.pod.name"]
		containerName := container.Labels["io.kubernetes.container.name"]
		if containerName == "" {
			containerName = container.Metadata.Name
		}
		containers = append(containers, KubeContainer{
			ID:            container.ID,
			Namespace:     container.Labels["io.kubernetes.pod.namespace"],
			PodName:       podName,
			ContainerName: containerName,
		})
	}

	return containers, nil
}

func extractJSONPayload(out []byte) ([]byte, error) {
	start := bytes.IndexByte(out, '{')
	if start == -1 {
		return nil, fmt.Errorf("no json object found in output")
	}

	end := bytes.LastIndexByte(out, '}')
	if end == -1 || end < start {
		return nil, fmt.Errorf("no complete json object found in output")
	}

	payload := bytes.TrimSpace(out[start : end+1])
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty json payload")
	}

	return payload, nil
}

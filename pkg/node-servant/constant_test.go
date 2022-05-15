package node_servant

import (
	"fmt"
	tmplutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/templates"
	"testing"
)

func Test(t *testing.T) {
	tmplCtx := make(map[string]string)
	tmplCtx["jobName"] = "testJobName"
	tmplCtx["nodeName"] = "testNodeName"
	tmplCtx["node_servant_image"] = "testImg"
	tmplCtx["working_mode"] = "testWorkingMode"
	tmplCtx["yurthub_image"] = "testYurthubImg"
	tmplCtx["joinToken"] = "testToken"

	// Deactivate these 2 lines below to see render result.
	// Inline '{{if}}{{end}}' template will not add any line break
	tmplCtx["yurthub_healthcheck_timeout"] = "10"
	tmplCtx["kubeadm_conf_path"] = "testKubeadmConfigPath"

	jobYaml, err := tmplutil.SubsituteTemplate(ConvertServantJobTemplate, tmplCtx)
	if err != nil {
		panic(err)
	}
	fmt.Println(jobYaml)
}

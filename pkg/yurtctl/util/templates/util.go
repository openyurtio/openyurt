package templates

import (
	"bytes"
	"text/template"
)

// SubsituteTemplate fills out the kubeconfig templates based on the context
func SubsituteTemplate(tmpl string, context interface{}) (string, error) {
	t, tmplPrsErr := template.New("test").Option("missingkey=zero").Parse(tmpl)
	if tmplPrsErr != nil {
		return "", tmplPrsErr
	}
	writer := bytes.NewBuffer([]byte{})
	if err := t.Execute(writer, context); nil != err {
		return "", err
	}

	return writer.String(), nil
}

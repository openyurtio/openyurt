{{/* vim: set filetype=mustache: */}}

{{- define "yurt-controller-manager.fullname" -}}
yurt-controller-manager
{{- end -}}

{{- define "yurt-controller-manager.name" -}}
yurt-controller-manager
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "yurt-controller-manager.selectorLabels" -}}
app.kubernetes.io/name: {{ include "yurt-controller-manager.name" . }}
app.kubernetes.io/instance: {{ printf "yurt-controller-manager-%s" .Release.Name }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "yurt-controller-manager.labels" -}}
{{ include "yurt-controller-manager.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
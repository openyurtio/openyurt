{{/*
Expand the name of the chart.
*/}}
{{- define "yurt-manager.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "yurt-manager.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "yurt-manager.labels" -}}
helm.sh/chart: {{ include "yurt-manager.chart" . }}
{{ include "yurt-manager.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "yurt-manager.selectorLabels" -}}
app.kubernetes.io/name: {{ include "yurt-manager.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
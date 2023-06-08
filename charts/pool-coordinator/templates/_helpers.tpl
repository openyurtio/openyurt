{{/*
Expand the name of the chart.
*/}}
{{- define "yurt-coordinator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "yurt-coordinator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "yurt-coordinator.labels" -}}
helm.sh/chart: {{ include "yurt-coordinator.chart" . }}
{{ include "yurt-coordinator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "yurt-coordinator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "yurt-coordinator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
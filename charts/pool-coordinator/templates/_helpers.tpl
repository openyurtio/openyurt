{{/*
Expand the name of the chart.
*/}}
{{- define "pool-coordinator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "pool-coordinator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "pool-coordinator.labels" -}}
helm.sh/chart: {{ include "pool-coordinator.chart" . }}
{{ include "pool-coordinator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "pool-coordinator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "pool-coordinator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
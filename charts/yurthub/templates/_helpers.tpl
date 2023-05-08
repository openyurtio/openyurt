{{/*
Expand the name of the chart.
*/}}
{{- define "yurthub.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "yurthub.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "yurthub.labels" -}}
helm.sh/chart: {{ include "yurthub.chart" . }}
{{ include "yurthub.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "yurthub.selectorLabels" -}}
app.kubernetes.io/name: {{ include "yurthub.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
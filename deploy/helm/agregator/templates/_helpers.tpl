{{- define "agregator.name" -}}
{{- .Chart.Name -}}
{{- end -}}

{{- define "agregator.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "agregator.labels" -}}
app.kubernetes.io/name: {{ include "agregator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end -}}
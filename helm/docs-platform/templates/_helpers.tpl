{{- define "docs-platform.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "docs-platform.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "docs-platform.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "docs-platform.labels" -}}
app.kubernetes.io/name: {{ include "docs-platform.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end -}}

{{- define "docs-platform.selectorLabels" -}}
app.kubernetes.io/name: {{ include "docs-platform.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "docs-platform.secretName" -}}
{{- if .Values.webhook.existingSecret -}}
{{- .Values.webhook.existingSecret -}}
{{- else -}}
{{- include "docs-platform.fullname" . -}}
{{- end -}}
{{- end -}}

{{- define "docs-platform.pvcName" -}}
{{- if .Values.persistence.existingClaim -}}
{{- .Values.persistence.existingClaim -}}
{{- else -}}
{{- printf "%s-data" (include "docs-platform.fullname" .) -}}
{{- end -}}
{{- end -}}

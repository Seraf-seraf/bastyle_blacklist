{{/*
Expand the name of the chart.
*/}}
{{- define "blacklist.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "blacklist.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "blacklist.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "blacklist.labels" -}}
helm.sh/chart: {{ include "blacklist.chart" . }}
{{ include "blacklist.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "blacklist.selectorLabels" -}}
app.kubernetes.io/name: {{ include "blacklist.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "blacklist.serviceAccountName" -}}
{{- $serviceAccount := default dict .Values.serviceAccount -}}
{{- default (include "blacklist.fullname" .) $serviceAccount.name }}
{{- end }}

{{/*
Return the Kubernetes ConfigMap name that contains bot config.yaml.
*/}}
{{- define "blacklist.botConfigMapName" -}}
{{- printf "%s-bot-config" (include "blacklist.fullname" .) -}}
{{- end -}}

{{/*
Return "true" when bot uses runtime secrets from Vault/VSO.
*/}}
{{- define "blacklist.botRuntimeSecretsEnabled" -}}
{{- $bot := default dict .Values.bot -}}
{{- $runtimeSecrets := default dict $bot.runtimeSecrets -}}
{{- if (default false $runtimeSecrets.enabled) -}}true{{- end -}}
{{- end -}}

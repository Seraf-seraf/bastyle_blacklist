{{/*
Expand the name of the chart.
*/}}
{{- define "bastyle.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "bastyle.fullname" -}}
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
{{- define "bastyle.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "bastyle.labels" -}}
helm.sh/chart: {{ include "bastyle.chart" . }}
{{ include "bastyle.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "bastyle.selectorLabels" -}}
app.kubernetes.io/name: {{ include "bastyle.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "bastyle.serviceAccountName" -}}
{{- $serviceAccount := default dict .Values.serviceAccount -}}
{{- default (include "bastyle.fullname" .) $serviceAccount.name }}
{{- end }}

{{/*
Return the Kubernetes ConfigMap name that contains bot config.yaml.
*/}}
{{- define "bastyle.botConfigMapName" -}}
{{- printf "%s-bot-config" (include "bastyle.fullname" .) -}}
{{- end -}}

{{/*
Return "true" when bot uses runtime secrets from Vault/VSO.
*/}}
{{- define "bastyle.botRuntimeSecretsEnabled" -}}
{{- $bot := default dict .Values.bot -}}
{{- $runtimeSecrets := default dict $bot.runtimeSecrets -}}
{{- if (default false $runtimeSecrets.enabled) -}}true{{- end -}}
{{- end -}}

{{/*
Return "true" when legacy ExternalSecret config is configured in current release values.
*/}}
{{- define "bastyle.legacyExternalConfigEnabled" -}}
{{- $externalSecrets := default dict .Values.externalSecrets -}}
{{- $fake := default dict $externalSecrets.fake -}}
{{- if and (not (include "bastyle.botRuntimeSecretsEnabled" .)) (default "" $fake.configYaml) -}}true{{- end -}}
{{- end -}}

{{/*
Return the Kubernetes Secret name that contains full config.yaml for legacy components.
*/}}
{{- define "bastyle.configSecretName" -}}
{{- if .Values.configSecret.existingSecret -}}
{{- .Values.configSecret.existingSecret -}}
{{- else -}}
{{- printf "%s-config" (include "bastyle.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/*
Return the Kubernetes Secret name that contains Grafana admin password.
*/}}
{{- define "bastyle.grafanaAdminSecretName" -}}
{{- if .Values.monitoring.grafana.existingSecret -}}
{{- .Values.monitoring.grafana.existingSecret -}}
{{- else -}}
{{- printf "%s-grafana-admin" (include "bastyle.fullname" .) -}}
{{- end -}}
{{- end -}}

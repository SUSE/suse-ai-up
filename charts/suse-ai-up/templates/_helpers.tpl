{{/*
Expand the name of the chart.
*/}}
{{- define "suse-ai-up.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "suse-ai-up.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "suse-ai-up.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "suse-ai-up.labels" -}}
helm.sh/chart: {{ include "suse-ai-up.chart" . }}
{{ include "suse-ai-up.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "suse-ai-up.selectorLabels" -}}
app.kubernetes.io/name: {{ include "suse-ai-up.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "suse-ai-up.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "suse-ai-up.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the proper image name
*/}}
{{- define "suse-ai-up.image" -}}
{{- $registry := .Values.image.registry }}
{{- if .Values.global.imageRegistry }}
{{- $registry = .Values.global.imageRegistry }}
{{- end }}
{{- $repository := .Values.image.repository }}
{{- $tag := default .Chart.AppVersion .Values.image.tag }}
{{- if $registry }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- else }}
{{- printf "%s:%s" $repository $tag }}
{{- end }}
{{- end }}

{{/*
Return the proper image pull policy
*/}}
{{- define "suse-ai-up.imagePullPolicy" -}}
{{- .Values.image.pullPolicy | default "IfNotPresent" }}
{{- end }}

{{/*
Create a default fully qualified configmap name.
*/}}
{{- define "suse-ai-up.configmapName" -}}
{{- printf "%s-config" (include "suse-ai-up.fullname" .) }}
{{- end }}

{{/*
Create a default fully qualified secret name.
*/}}
{{- define "suse-ai-up.secretName" -}}
{{- printf "%s-secret" (include "suse-ai-up.fullname" .) }}
{{- end }}

{{/*
Create a default fully qualified service name.
*/}}
{{- define "suse-ai-up.serviceName" -}}
{{- printf "%s-service" (include "suse-ai-up.fullname" .) }}
{{- end }}

{{/*
Common annotations
*/}}
{{- define "suse-ai-up.annotations" -}}
{{- with .Values.commonAnnotations }}
{{- toYaml . }}
{{- end }}
{{- end }}

{{/*
Return the proper Docker Image Registry Secret Names
*/}}
{{- define "suse-ai-up.imagePullSecrets" -}}
{{- $pullSecrets := list }}
{{- $defaultSecretName := include "suse-ai-up.fullname" . }}
{{- $defaultSecretName = printf "%s-registry" $defaultSecretName }}
{{- $pullSecrets = append $pullSecrets $defaultSecretName }}
{{- range .Values.imagePullSecrets }}
{{- $pullSecrets = append $pullSecrets . }}
{{- end }}
{{- range .Values.global.imagePullSecrets }}
{{- $pullSecrets = append $pullSecrets . }}
{{- end }}
{{- $pullSecrets | uniq | toYaml }}
{{- end }}

{{/*
Registry environment variables
*/}}
{{- define "suse-ai-up.registryEnvVars" -}}
{{- if .Values.registry.url }}
- name: MCP_REGISTRY_URL
  value: "{{ .Values.registry.url }}"
- name: REGISTRY_TIMEOUT
  value: "{{ .Values.registry.timeout }}"
{{- end }}
{{- end }}
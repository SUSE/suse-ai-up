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
OTEL Collector ConfigMap name
*/}}
{{- define "suse-ai-up.otelConfigName" -}}
{{- printf "%s-otel-config" (include "suse-ai-up.fullname" .) }}
{{- end }}

{{/*
OTEL resource attributes as comma-separated string
*/}}
{{- define "suse-ai-up.otelResourceAttributes" -}}
{{- $attrs := list -}}
{{- range .Values.otel.resourceAttributes -}}
{{- $attr := printf "%s=%s" .key .value -}}
{{- $attrs = append $attrs $attr -}}
{{- end -}}
{{- join "," $attrs -}}
{{- end }}

{{/*
OTEL environment variables
*/}}
{{- define "suse-ai-up.otelEnv" -}}
- name: OTEL_SERVICE_NAME
  value: {{ .Values.otel.serviceName | quote }}
- name: OTEL_SERVICE_VERSION
  value: {{ .Values.otel.serviceVersion | quote }}
- name: OTEL_TRACES_EXPORTER
  value: {{ .Values.otel.core.tracesExporter | quote }}
- name: OTEL_METRICS_EXPORTER
  value: {{ .Values.otel.core.metricsExporter | quote }}
- name: OTEL_LOGS_EXPORTER
  value: {{ .Values.otel.core.logsExporter | quote }}
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: {{ .Values.otel.core.otlpEndpoint | quote }}
- name: OTEL_EXPORTER_OTLP_PROTOCOL
  value: {{ .Values.otel.core.otlpProtocol | quote }}
- name: OTEL_EXPORTER_OTLP_INSECURE
  value: {{ .Values.otel.core.insecure | quote }}
- name: OTEL_RESOURCE_ATTRIBUTES
  value: {{ include "suse-ai-up.otelResourceAttributes" . | quote }}
- name: OTEL_TRACES_SAMPLER
  value: {{ .Values.otel.sampling.traces.sampler | quote }}
- name: OTEL_TRACES_SAMPLER_ARG
  value: {{ .Values.otel.sampling.traces.ratio | quote }}
{{- if .Values.otel.exporters.jaeger.enabled }}
- name: OTEL_EXPORTER_JAEGER_ENDPOINT
  value: {{ .Values.otel.exporters.jaeger.endpoint | quote }}
{{- end }}
{{- end }}

{{/*
Main application environment variables
*/}}
{{- define "suse-ai-up.appEnv" -}}
- name: PORT
  value: {{ .Values.env.port | quote }}
- name: HOST
  value: {{ .Values.env.host | quote }}
- name: AUTH_MODE
  value: {{ .Values.env.authMode | quote }}
- name: REGISTRY_ENABLED
  value: {{ .Values.env.registryEnabled | quote }}
- name: SMARTAGENTS_ENABLED
  value: {{ .Values.env.smartAgentsEnabled | quote }}
# MCP server spawning configuration
- name: SPAWNING_RETRY_ATTEMPTS
  value: {{ .Values.env.spawning.retryAttempts | quote }}
- name: SPAWNING_RETRY_BACKOFF_MS
  value: {{ .Values.env.spawning.retryBackoffMs | quote }}
- name: SPAWNING_DEFAULT_CPU
  value: {{ .Values.env.spawning.defaultCpu | quote }}
- name: SPAWNING_DEFAULT_MEMORY
  value: {{ .Values.env.spawning.defaultMemory | quote }}
- name: SPAWNING_MAX_CPU
  value: {{ .Values.env.spawning.maxCpu | quote }}
- name: SPAWNING_MAX_MEMORY
  value: {{ .Values.env.spawning.maxMemory | quote }}
- name: SPAWNING_LOG_LEVEL
  value: {{ .Values.env.spawning.logLevel | quote }}
- name: SPAWNING_INCLUDE_CONTEXT
  value: {{ .Values.env.spawning.includeContext | quote }}
# OpenTelemetry configuration
- name: OTEL_ENABLED
  value: {{ .Values.otel.app.enabled | quote }}
- name: OTEL_ENDPOINT
  value: {{ .Values.otel.app.endpoint | quote }}
- name: OTEL_PROTOCOL
  value: {{ .Values.otel.app.protocol | quote }}
{{- end }}

{{/*
Image pull policy
*/}}
{{- define "suse-ai-up.imagePullPolicy" -}}
{{- .Values.image.pullPolicy | default "IfNotPresent" }}
{{- end }}

{{/*
Main container image
*/}}
{{- define "suse-ai-up.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- if .Values.image.architectureTagSuffix }}
{{- $tag = printf "%s%s" $tag .Values.image.architectureTagSuffix }}
{{- end }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{/*
OTEL collector image
*/}}
{{- define "suse-ai-up.otelImage" -}}
{{- .Values.otel.collector.image }}
{{- end }}
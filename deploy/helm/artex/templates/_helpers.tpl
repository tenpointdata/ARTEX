{{- define "artex.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "artex.fullname" -}}
{{- if .Values.fullnameOverride }}{{ .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}{{- $n := default .Chart.Name .Values.nameOverride }}
{{- if contains $n .Release.Name }}{{ .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}{{ printf "%s-%s" .Release.Name $n | trunc 63 | trimSuffix "-" }}{{ end }}
{{- end }}
{{- end }}

{{- define "artex.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "artex.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/* selector labels: call with (dict "ctx" . "component" "control") */}}
{{- define "artex.selector" -}}
app.kubernetes.io/name: {{ include "artex.name" .ctx }}
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "artex.image" -}}
{{ .Values.image.repository }}:{{ default (printf "v%s" .Chart.AppVersion) .Values.image.tag }}
{{- end }}

{{- define "artex.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}{{ default (include "artex.fullname" .) .Values.serviceAccount.name }}
{{- else }}{{ default "default" .Values.serviceAccount.name }}{{ end }}
{{- end }}

{{/* Chart-managed Secret name. */}}
{{- define "artex.secretName" -}}{{ include "artex.fullname" . }}{{- end }}

{{- define "artex.secretKeyName" -}}
{{ default (include "artex.secretName" .) .Values.secretKey.existingSecret }}
{{- end }}
{{- define "artex.secretKeyKey" -}}
{{ if .Values.secretKey.existingSecret }}{{ .Values.secretKey.existingSecretKey }}{{ else }}secret-key{{ end }}
{{- end }}

{{/* PostgreSQL connection. */}}
{{- define "artex.pgHost" -}}
{{- if .Values.postgresql.bundled.enabled }}{{ include "artex.fullname" . }}-postgresql
{{- else }}{{ required "postgresql.external.host is required when postgresql.bundled.enabled=false" .Values.postgresql.external.host }}{{ end }}
{{- end }}
{{- define "artex.pgPort" -}}
{{ if .Values.postgresql.bundled.enabled }}5432{{ else }}{{ .Values.postgresql.external.port }}{{ end }}
{{- end }}
{{- define "artex.pgUser" -}}
{{ if .Values.postgresql.bundled.enabled }}{{ .Values.postgresql.bundled.user }}{{ else }}{{ .Values.postgresql.external.user }}{{ end }}
{{- end }}
{{- define "artex.pgDb" -}}
{{ if .Values.postgresql.bundled.enabled }}{{ .Values.postgresql.bundled.database }}{{ else }}{{ .Values.postgresql.external.database }}{{ end }}
{{- end }}
{{- define "artex.pgSslmode" -}}
{{ default (ternary "disable" "require" .Values.postgresql.bundled.enabled) .Values.postgresql.sslmode }}
{{- end }}
{{- define "artex.pgSecretName" -}}
{{- $s := ternary .Values.postgresql.bundled.existingSecret .Values.postgresql.external.existingSecret .Values.postgresql.bundled.enabled -}}
{{ default (include "artex.secretName" .) $s }}
{{- end }}
{{- define "artex.pgSecretKey" -}}
{{- if .Values.postgresql.bundled.enabled -}}
{{ if .Values.postgresql.bundled.existingSecret }}{{ .Values.postgresql.bundled.existingSecretKey }}{{ else }}pg-password{{ end }}
{{- else -}}
{{ if .Values.postgresql.external.existingSecret }}{{ .Values.postgresql.external.existingSecretKey }}{{ else }}pg-password{{ end }}
{{- end -}}
{{- end }}

{{/* Env shared by every ARTEX node. Call with (dict "ctx" . "extra" <map>) */}}
{{- define "artex.sharedEnv" -}}
- name: ARTEX_PG_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "artex.pgSecretName" .ctx }}
      key: {{ include "artex.pgSecretKey" .ctx }}
- name: ARTEX_SECRET_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "artex.secretKeyName" .ctx }}
      key: {{ include "artex.secretKeyKey" .ctx }}
{{- if and (not .ctx.Values.llm.existingSecret) .ctx.Values.llm.anthropicApiKey }}
- name: ANTHROPIC_API_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "artex.secretName" .ctx }}
      key: anthropic-api-key
{{- end }}
{{- if and (not .ctx.Values.llm.existingSecret) .ctx.Values.llm.openaiApiKey }}
- name: OPENAI_API_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "artex.secretName" .ctx }}
      key: openai-api-key
{{- end }}
{{- range $k, $v := .extra }}
- name: {{ $k }}
  value: {{ $v | quote }}
{{- end }}
{{- end }}

{{- define "artex.envFrom" -}}
- configMapRef:
    name: {{ include "artex.fullname" . }}
{{- if .Values.llm.existingSecret }}
- secretRef:
    name: {{ .Values.llm.existingSecret }}
{{- end }}
{{- with .Values.extraEnvFrom }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{- define "artex.containerSecurityContext" -}}
runAsNonRoot: {{ .Values.securityContext.runAsNonRoot }}
allowPrivilegeEscalation: {{ .Values.securityContext.allowPrivilegeEscalation }}
seccompProfile:
  type: {{ .Values.securityContext.seccompProfile.type }}
capabilities:
  drop: {{ toYaml .Values.securityContext.capabilities.drop | nindent 4 }}
  add:
    {{- if .Values.egress.enabled }}
    - NET_ADMIN
    {{- end }}
    {{- range .Values.securityContext.capabilities.add }}
    - {{ . }}
    {{- end }}
{{- end }}

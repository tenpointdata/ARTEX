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
{{/* Agent egress filter lists (egress.sh); see egress.* in values.yaml. */}}
{{- define "artex.egressDeny" -}}
{{- $l := list -}}
{{- with .Values.egress.deny }}{{ $l = append $l . }}{{ end -}}
{{- range .Values.egress.clusterCIDRs }}{{ $l = append $l . }}{{ end -}}
{{- join " " $l -}}
{{- end }}
{{- define "artex.egressDenyHosts" -}}
{{- $l := list -}}
{{- if .Values.egress.denyClusterServices }}
{{- $l = append $l (include "artex.pgHost" .) -}}
{{- $l = append $l (printf "%s-control" (include "artex.fullname" .)) -}}
{{- $l = append $l (printf "%s-control-pods:8787" (include "artex.fullname" .)) -}}
{{- end }}
{{- with .Values.egress.denyHosts }}{{ $l = append $l . }}{{ end -}}
{{- join " " $l -}}
{{- end }}
{{- define "artex.egressAllowHosts" -}}
{{- $l := list -}}
{{- if .Values.egress.clusterCIDRs }}{{ $l = append $l (printf "%s-control-proxy" (include "artex.fullname" .)) }}{{ end -}}
{{- with .Values.egress.allowHosts }}{{ $l = append $l . }}{{ end -}}
{{- join " " $l -}}
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

{{/* ---- Control API TLS (controlTLS.*, docs/DISTRIBUTED.md "Transport") ---- */}}
{{- define "artex.controlTLSSecret" -}}
{{- if eq .Values.controlTLS.source "secret" -}}
{{ required "controlTLS.existingSecret is required when controlTLS.source=secret" .Values.controlTLS.existingSecret }}
{{- else -}}
{{ include "artex.fullname" . }}-control-tls
{{- end -}}
{{- end }}

{{/* Key of the CA certificate in the TLS Secret. */}}
{{- define "artex.controlTLSCAKey" -}}
{{ if eq .Values.controlTLS.source "secret" }}{{ .Values.controlTLS.caKey }}{{ else }}ca.crt{{ end }}
{{- end }}

{{/* DNS names the control API certificate covers: the Service as workers and pods address it, plus extras. */}}
{{- define "artex.controlTLSDNSNames" -}}
{{- $svc := printf "%s-control" (include "artex.fullname" .) -}}
{{- $ns := .Release.Namespace -}}
{{- $names := list $svc (printf "%s.%s" $svc $ns) (printf "%s.%s.svc" $svc $ns) (printf "%s.%s.svc.cluster.local" $svc $ns) "localhost" -}}
{{- toJson (concat $names .Values.controlTLS.extraDnsNames) -}}
{{- end }}

{{/* URL scheme of the control API. */}}
{{- define "artex.controlScheme" -}}
{{ ternary "https" "http" .Values.controlTLS.enabled }}
{{- end }}

{{- define "artex.validateControlTLS" -}}
{{- if .Values.controlTLS.enabled }}
{{- if not (has .Values.controlTLS.source (list "generate" "secret" "certManager")) }}
{{- fail (printf "controlTLS.source %q: want generate, secret or certManager" .Values.controlTLS.source) }}
{{- end }}
{{- if and (eq .Values.controlTLS.source "certManager") (not .Values.controlTLS.certManager.issuerRef.name) }}
{{- fail "controlTLS.certManager.issuerRef.name is required when controlTLS.source=certManager" }}
{{- end }}
{{- end }}
{{- end }}

{{/* ---- Recording proxy hop (docs/DISTRIBUTED.md "Transport", c-677166) ----
With controlTLS.enabled the plain proxy binds loopback only (the control node's
own agents) and worker nodes reach its TLS front on control.proxyTLSPort through the
<release>-control-proxy Service, checking the certificate against
<release>-control (ARTEX_CONTROL_PROXY_TLS_NAME), the name every controlTLS
source already covers. */}}
{{- define "artex.controlProxyBind" -}}
{{- if .Values.control.proxyAddr -}}
{{ .Values.control.proxyAddr }}
{{- else if .Values.controlTLS.enabled -}}
127.0.0.1:8788
{{- else -}}
0.0.0.0:8788
{{- end -}}
{{- end }}

{{/* ARTEX_CONTROL_PROXY for worker nodes. */}}
{{- define "artex.controlProxyURL" -}}
{{- if .Values.controlTLS.enabled -}}
{{ printf "https://%s-control-proxy:%v" (include "artex.fullname" .) .Values.control.proxyTLSPort }}
{{- else -}}
{{ printf "http://%s-control-proxy:8788" (include "artex.fullname" .) }}
{{- end -}}
{{- end }}

{{/* Common name + labels */}}
{{- define "neon.fullname" -}}
{{- printf "%s" .Release.Name -}}
{{- end -}}

{{- define "neon.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: neon
{{- end -}}

{{/* storageClassName line for a volumeClaimTemplate spec; empty -> omitted */}}
{{- define "neon.storageClass" -}}
{{- if .Values.storageClass }}
storageClassName: {{ .Values.storageClass | quote }}
{{- end }}
{{- end -}}

{{/* S3 endpoint: in-chart SeaweedFS service, or external override */}}
{{- define "neon.s3.endpoint" -}}
{{- if .Values.seaweedfs.enabled -}}
http://{{ include "neon.fullname" . }}-seaweedfs:8333
{{- else -}}
{{ .Values.s3.endpoint }}
{{- end -}}
{{- end -}}

{{- define "neon.s3.bucket" -}}
{{- if .Values.seaweedfs.enabled -}}{{ .Values.seaweedfs.bucket }}{{- else -}}{{ .Values.s3.bucket }}{{- end -}}
{{- end -}}

{{/* Pageserver service host (CLI --pageserver-host) */}}
{{- define "neon.pageserver.host" -}}
{{ include "neon.fullname" . }}-pageserver.{{ .Release.Namespace }}.svc.cluster.local
{{- end -}}

{{/* Comma-separated safekeeper list for neon.safekeepers (--safekeepers) */}}
{{- define "neon.safekeepers" -}}
{{- $name := include "neon.fullname" . -}}
{{- $port := .Values.safekeeper.pgPort -}}
{{- $ns := .Release.Namespace -}}
{{- $list := list -}}
{{- range $i := until (int .Values.safekeeper.replicas) -}}
{{- $list = append $list (printf "%s-safekeeper-%d.%s-safekeeper.%s.svc.cluster.local:%d" $name $i $name $ns (int $port)) -}}
{{- end -}}
{{- join "," $list -}}
{{- end -}}

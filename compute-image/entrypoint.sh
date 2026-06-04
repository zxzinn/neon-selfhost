#!/usr/bin/env bash
# Self-contained Neon compute entrypoint.
#
# Unlike the upstream compose wrapper (which bind-mounts this script and a
# config), everything here is baked into the image so it runs unchanged under
# both docker and kubernetes. All storage endpoints are read from env vars with
# docker-compatible defaults.
set -eux

# --- configurable storage endpoints (env, with compose-compatible defaults) ---
PAGESERVER_HOST=${PAGESERVER_HOST:-pageserver}
PAGESERVER_HTTP_PORT=${PAGESERVER_HTTP_PORT:-9898}
PAGESERVER_PG_PORT=${PAGESERVER_PG_PORT:-6400}
SAFEKEEPERS=${SAFEKEEPERS:-safekeeper1:5454,safekeeper2:5454,safekeeper3:5454}
PG_VERSION=${PG_VERSION:-16}

PS_API="http://${PAGESERVER_HOST}:${PAGESERVER_HTTP_PORT}"

readonly CONFIG_FILE_ORG=/etc/neon/config.json
readonly CONFIG_FILE=/tmp/config.json

generate_id() {
    local -n resvar=${1}
    printf -v resvar '%08x%08x%08x%08x' ${SRANDOM} ${SRANDOM} ${SRANDOM} ${SRANDOM}
}

echo "Waiting for pageserver ${PAGESERVER_HOST}:${PAGESERVER_PG_PORT} ..."
while ! nc -z "${PAGESERVER_HOST}" "${PAGESERVER_PG_PORT}"; do
    sleep 1
done
echo "Pageserver is ready."

cp "${CONFIG_FILE_ORG}" "${CONFIG_FILE}"

# Resolve tenant + timeline: use provided ids, else discover/create on the
# pageserver. In k8s the CLI always passes TENANT_ID + TIMELINE_ID, so the
# create paths are mainly the docker bootstrap case.
if [[ -n "${TENANT_ID:-}" && -n "${TIMELINE_ID:-}" ]]; then
    tenant_id=${TENANT_ID}
    timeline_id=${TIMELINE_ID}
else
    tenant_id=$(curl -s -H "Content-Type: application/json" "${PS_API}/v1/tenant" | jq -r '.[0].id')
    if [[ -z "${tenant_id}" || "${tenant_id}" = null ]]; then
        generate_id tenant_id
        curl -sf -X PUT -H "Content-Type: application/json" \
            -d '{"mode": "AttachedSingle", "generation": 1, "tenant_conf": {}}' \
            "${PS_API}/v1/tenant/${tenant_id}/location_config" | jq .
    fi
    timeline_id=$(curl -s -H "Content-Type: application/json" "${PS_API}/v1/tenant/${tenant_id}/timeline" | jq -r '.[0].timeline_id')
    if [[ -z "${timeline_id}" || "${timeline_id}" = null ]]; then
        generate_id timeline_id
        curl -sbf -X POST -H "Content-Type: application/json" \
            -d "{\"new_timeline_id\": \"${timeline_id}\", \"pg_version\": ${PG_VERSION}}" \
            "${PS_API}/v1/tenant/${tenant_id}/timeline/" | jq .
    fi
fi

# Substitute placeholders in the spec.
sed -i "s|__SAFEKEEPERS__|${SAFEKEEPERS}|" "${CONFIG_FILE}"
sed -i "s|__PAGESERVER_HOST__|${PAGESERVER_HOST}|" "${CONFIG_FILE}"
sed -i "s|__PAGESERVER_PG_PORT__|${PAGESERVER_PG_PORT}|" "${CONFIG_FILE}"
sed -i "s|TENANT_ID|${tenant_id}|" "${CONFIG_FILE}"
sed -i "s|TIMELINE_ID|${timeline_id}|" "${CONFIG_FILE}"

cat "${CONFIG_FILE}"

echo "Starting compute node for tenant=${tenant_id} timeline=${timeline_id}"
exec /usr/local/bin/compute_ctl \
    --pgdata /var/db/postgres/compute \
    -C "postgresql://cloud_admin@localhost:55433/postgres" \
    -b /usr/local/bin/postgres \
    --compute-id "compute-${HOSTNAME:-$RANDOM}" \
    --config "${CONFIG_FILE}"

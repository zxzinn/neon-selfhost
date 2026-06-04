# Self-contained Neon compute image: the upstream compute-node plus a baked-in
# entrypoint and spec, so it runs under both docker and kubernetes without any
# bind-mounted files.
ARG REPOSITORY=ghcr.io/neondatabase
ARG COMPUTE_IMAGE=compute-node-v16
ARG TAG=latest

FROM ${REPOSITORY}/${COMPUTE_IMAGE}:${TAG}

USER root
RUN echo 'Acquire::Retries "5";' > /etc/apt/apt.conf.d/80-retries && \
    apt-get update && \
    apt-get install -y curl jq netcat-openbsd && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p /etc/neon
COPY config.json /etc/neon/config.json
COPY entrypoint.sh /usr/local/bin/neon-compute-entrypoint.sh
RUN chmod +x /usr/local/bin/neon-compute-entrypoint.sh && \
    chown -R postgres /etc/neon

USER postgres
ENTRYPOINT ["/usr/local/bin/neon-compute-entrypoint.sh"]

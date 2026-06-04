# neon-selfhost

A small CLI for **database branching on a self-hosted [Neon](https://github.com/neondatabase/neon)** stack, for local development.

Neon gives you copy-on-write Postgres branches by separating compute from
storage. The managed service wraps that in a control plane; `neon-selfhost` is a
minimal, self-hostable replacement for the one thing developers actually want:
**create / list / delete branches**, each backed by its own Postgres compute.

```
neon-selfhost branch create feature-x      # instant copy-on-write branch + a compute
neon-selfhost branch list
neon-selfhost branch delete feature-x
```

A branch is a pageserver *timeline*; creating one is a single API call and is
O(1) regardless of database size. `neon-selfhost` makes that call and runs a compute
for the branch.

## Status

Early/experimental. Built for dev environments, not production. No HA, no
billing, no autoscaling. The storage stack is upstream Neon; this project only
adds the branch orchestration and packaging.

## How it works

```
neon-selfhost ──HTTP──> pageserver API        (create/list/delete timelines)
         └──────-> compute backend       (one Postgres compute per branch)
                     ├─ docker   (local containers)
                     └─ k8s      (Deployment + Service via client-go)
```

- `internal/pageserver` — thin REST client for the pageserver timeline API.
- `internal/compute` — `Backend` interface; `docker` and `k8s` implementations.
- `internal/state` — local name → timeline → port mapping (`~/.neon-selfhost`).

## Quick start (docker)

Bring up a local Neon storage stack with the upstream docker-compose, then:

```sh
go build -o neon-selfhost .

neon-selfhost branch create dev
psql -h localhost -p 55434 -U cloud_admin -d postgres
```

By default `neon-selfhost` talks to `http://localhost:9898` (pageserver) and runs
computes as docker containers on the compose network.

## Kubernetes

Full walkthrough deploying the storage stack to a cluster (tested on k3s) and
creating a branch. Pick a `storageClass` that exists in your cluster.

### 1. Compute image

The k8s backend runs each branch as a compute pod, using a self-contained
compute image (storage endpoints read from env, no bind-mounts). Prebuilt
images are published to GHCR, one per Postgres major version:

```
ghcr.io/zxzinn/neon-compute:16
ghcr.io/zxzinn/neon-compute:17
```

Use those directly (step 5). To build your own instead:

```sh
docker build --platform linux/amd64 \
  --build-arg COMPUTE_IMAGE=compute-node-v16 \
  -t <registry>/neon-compute:16 compute-image
docker push <registry>/neon-compute:16
```

### 2. Install the storage stack

```sh
helm install neon ./charts/neon \
  --namespace neon --create-namespace \
  --set storageClass=longhorn      # set to a class that exists in your cluster

# wait until all storage pods are Ready (pageserver, 3x safekeeper, broker, seaweedfs)
kubectl -n neon get pods -w
```

The in-chart SeaweedFS provides S3; a post-install Job creates the bucket
automatically. To use an external S3 instead:

```sh
helm install neon ./charts/neon --namespace neon --create-namespace \
  --set storageClass=longhorn \
  --set seaweedfs.enabled=false \
  --set s3.endpoint=https://s3.example.com \
  --set s3.bucket=neon --set s3.accessKey=KEY --set s3.secretKey=SECRET
```

> **On `storageClass`** — the chart's PVCs (pageserver, safekeepers, SeaweedFS)
> need a storageClass. Set it explicitly rather than relying on the cluster
> default:
>
> - If your cluster has **more than one** class marked default (e.g. both
>   `local-path` and `longhorn` on k3s with Longhorn installed), PVC binding is
>   ambiguous and may pick the wrong backend.
> - Leaving it empty (`storageClass: ""`) omits the field, so Kubernetes uses
>   the single default class — fine only if there is exactly one.
>
> List your classes (and spot duplicate defaults) with `kubectl get storageclass`.

### 3. Port-forward the pageserver API

The CLI talks to the pageserver HTTP API; expose it locally:

```sh
kubectl -n neon port-forward svc/neon-pageserver 9898:9898
```

### 4. Bootstrap the first tenant + timeline

Run once per fresh stack:

```sh
neon-selfhost init --pageserver http://localhost:9898
```

### 5. Create a branch

```sh
neon-selfhost branch create dev \
  --backend k8s \
  --namespace neon \
  --pageserver http://localhost:9898 \
  --pageserver-host neon-pageserver.neon.svc.cluster.local \
  --safekeepers 'neon-safekeeper-0.neon-safekeeper.neon.svc.cluster.local:5454,neon-safekeeper-1.neon-safekeeper.neon.svc.cluster.local:5454,neon-safekeeper-2.neon-safekeeper.neon.svc.cluster.local:5454' \
  --image ghcr.io/zxzinn/neon-compute:16
```

For Postgres 17, use `--image ghcr.io/zxzinn/neon-compute:17 --pg-version 17`.

`helm install` prints the exact `init`, `--pageserver-host`, and `--safekeepers`
values for your release name and namespace. Set `storageClass` explicitly if
your cluster has more than one default class.

### 6. Connect to the branch

The branch compute is a ClusterIP Service; port-forward it to connect:

```sh
SVC=$(kubectl get svc -n neon -l neon-selfhost/branch -o jsonpath='{.items[0].metadata.name}')
kubectl -n neon port-forward svc/$SVC 15432:5432
psql -h 127.0.0.1 -p 15432 -U cloud_admin -d postgres
```

## Configuration

| Flag | Default | Notes |
|------|---------|-------|
| `--pageserver` | `http://localhost:9898` | pageserver HTTP API |
| `--backend` | `docker` | `docker` or `k8s` |
| `--tenant` | first tenant | tenant (project) id |
| `--pg-version` | `16` | Postgres major version (16 or 17; match the compute image tag) |
| `--namespace` | `default` | k8s namespace |
| `--pageserver-host` | `pageserver.neon.svc.cluster.local` | in-cluster pageserver (k8s) |
| `--safekeepers` | — | comma-separated `host:port` list (k8s) |

## Development

```sh
go test ./...
helm lint charts/neon
```

## License

Apache 2.0. See [LICENSE](LICENSE).

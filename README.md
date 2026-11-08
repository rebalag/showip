# showip

A small Go web server that sends back an HTML page with the caller's details:
client IP, User-Agent, operating system, browser, connection info and all request headers.
It uses only the Go standard library, with no third-party dependencies.

| Path       | Purpose                        |
|------------|--------------------------------|
| `/`        | HTML client information page   |
| `/healthz` | Liveness/readiness probe       |

## Run locally

```bash
make test          # unit tests (race detector)
make local         # builds bin/showip
./bin/showip       # listens on :8080 (override: -addr :9000 or LISTEN_ADDR)
open http://localhost:8080
```

## Build the container image

```bash
# Single-arch, local daemon
make image IMAGE=myregistry.example.com/team/showip TAG=v1.0.0

# Multi-arch (amd64 + arm64) and push. Log in first.
docker login myregistry.example.com
make images IMAGE=myregistry.example.com/team/showip TAG=v1.0.0
```

The runtime image is `distroless/static:nonroot` (no shell, runs as a non-root user).

## Private registry pull secret

The Deployment points at `imagePullSecrets: regcred` in the `showip` namespace.

### 1. Create the namespace

```bash
kubectl create namespace showip
```

### 2. Create the `docker-registry` secret (pick ONE option)

**Option A: from credentials (Docker Hub, Harbor, GHCR, Nexus, etc.)**

Use an access token, not your account password.

```bash
kubectl create secret docker-registry regcred \
  --namespace showip \
  --docker-server=<registry-host>      `# docker.io: https://index.docker.io/v1/ ; GHCR: ghcr.io` \
  --docker-username=<username> \
  --docker-password=<access-token> \
  --docker-email=<email>               `# optional`
```

To keep the token out of shell history, read it from stdin or a file:

```bash
read -s REG_TOKEN && kubectl create secret docker-registry regcred -n showip \
  --docker-server=ghcr.io --docker-username=<user> --docker-password="$REG_TOKEN"
unset REG_TOKEN
```

**Option B: reuse an existing `docker login`**

```bash
docker login <registry-host>
kubectl create secret generic regcred \
  --namespace showip \
  --from-file=.dockerconfigjson=$HOME/.docker/config.json \
  --type=kubernetes.io/dockerconfigjson
```

> On macOS, Docker Desktop usually keeps credentials in the keychain (`"credsStore": "desktop"`),
> so `config.json` may hold no actual auth. If so, use Option A.

**Option C: cloud registries**

```bash
# Google Artifact Registry / GCR (service-account JSON key)
kubectl create secret docker-registry regcred -n showip \
  --docker-server=us-docker.pkg.dev \
  --docker-username=_json_key \
  --docker-password="$(cat sa-key.json)"

# AWS ECR (token expires after 12h, so refresh it with a CronJob or use node IAM roles instead)
kubectl create secret docker-registry regcred -n showip \
  --docker-server=<acct>.dkr.ecr.<region>.amazonaws.com \
  --docker-username=AWS \
  --docker-password="$(aws ecr get-login-password --region <region>)"

# Azure ACR (service principal)
kubectl create secret docker-registry regcred -n showip \
  --docker-server=<name>.azurecr.io \
  --docker-username=<sp-app-id> --docker-password=<sp-password>
```

On GKE, EKS and AKS you can skip pull secrets entirely by giving the node or workload identity
pull access to the registry.

### 3. Verify the secret

```bash
kubectl get secret regcred -n showip
kubectl get secret regcred -n showip -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d
```

### 4. (Optional) Attach it to the namespace's default ServiceAccount

This applies the secret to every pod in the namespace, so you don't have to list it in each manifest:

```bash
kubectl patch serviceaccount default -n showip \
  -p '{"imagePullSecrets":[{"name":"regcred"}]}'
```

## Deploy

```bash
# Set your image in k8s/deployment.yaml (spec.template.spec.containers[0].image), then:
kubectl apply -f k8s/deployment.yaml
kubectl -n showip rollout status deploy/showip
kubectl -n showip get svc showip        # EXTERNAL-IP of the LoadBalancer
```

No LoadBalancer available (kind, minikube, bare metal)?

```bash
kubectl -n showip port-forward svc/showip 8080:80
```

### Troubleshooting image pulls

```bash
kubectl -n showip get pods
kubectl -n showip describe pod <pod>   # look for ErrImagePull / ImagePullBackOff events
```

Common causes: a wrong `--docker-server` (it must match the image's registry host exactly),
an expired token, or a secret created in a different namespace than the pod.

## Notes on client IP accuracy

- The Service uses `externalTrafficPolicy: Local`, which keeps the real source IP for
  LoadBalancer/NodePort traffic. Without it, kube-proxy SNATs the traffic and the app sees a node IP.
- Behind an Ingress controller, the app reads `X-Forwarded-For` / `X-Real-IP`. The client
  supplies these headers, so treat them as trustworthy only when a proxy you control sets them.
- OS and browser detection is a heuristic based on User-Agent. Browsers that send reduced UAs
  (for example Chrome on macOS) can report a frozen OS version.

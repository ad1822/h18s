# Argo CD restructure: migration guide

This guide moves the repo from the current folder-scanning `ApplicationSet`
(`argocd/app.yaml`) to an **app-of-apps** layout where every app is listed by
hand.

- One Argo CD `Application` per real app (radarr, jellyfin, ...), not one per
  resource kind (deployment, service, ...).
- Every folder has a `kustomization.yaml` with an explicit `resources:` list.
  Argo CD deploys only what is listed. New folders, stray files and `*.example`
  files are never picked up.
- Ordering comes from sync waves. Projects go first (`-1`), then infra (`0`–`1`),
  then apps (`10`).

---

## 0. Target layout

```
h18s/
├── bootstrap/
│   └── root.yaml                    # only manifest applied by hand
├── argocd/
│   ├── kustomization.yaml           # -> projects/, apps/
│   ├── projects/
│   │   ├── kustomization.yaml
│   │   ├── infra.yaml
│   │   └── apps.yaml
│   └── apps/
│       ├── kustomization.yaml       # THE app registry
│       ├── sealed-secrets.yaml
│       ├── traefik-config.yaml
│       ├── argocd-config.yaml
│       ├── homepage.yaml
│       ├── jellyfin.yaml
│       ├── prowlarr.yaml
│       ├── qbittorrent.yaml
│       ├── radarr.yaml
│       └── seerr.yaml
├── infra/
│   ├── sealed-secrets/
│   │   ├── kustomization.yaml
│   │   └── controller.yaml
│   ├── traefik/
│   │   ├── kustomization.yaml
│   │   ├── dashboard.yaml
│   │   ├── tls-store.yaml
│   │   └── h18s-tls.sealed.yaml     # created in step 6
│   └── argocd/
│       ├── kustomization.yaml
│       └── ingress.yaml
├── apps/
│   ├── ingress.yaml.example
│   ├── homepage/    {kustomization, rbac, deployment, service, ingress}.yaml
│   ├── jellyfin/    {kustomization, deployment, service, ingress}.yaml
│   ├── prowlarr/    {kustomization, deployment, service, ingress}.yaml
│   ├── qbittorrent/ {kustomization, deployment, service, ingress}.yaml
│   ├── radarr/      {kustomization, deployment, service, ingress}.yaml
│   └── seerr/       {kustomization, deployment, service, ingress}.yaml
└── pki/                             # certs, CSRs, keys. No Application points here
```

### Argo CD Applications after migration

| Application      | Project | Path                  | Namespace     | Wave |
|------------------|---------|-----------------------|---------------|------|
| `root`           | default | `argocd`              | `argocd`      | -    |
| `sealed-secrets` | infra   | `infra/sealed-secrets`| `kube-system` | 0    |
| `traefik-config` | infra   | `infra/traefik`       | `kube-system` | 1    |
| `argocd-config`  | infra   | `infra/argocd`        | `argocd`      | 1    |
| `homepage`       | apps    | `apps/homepage`       | `default`     | 10   |
| `jellyfin`       | apps    | `apps/jellyfin`       | `default`     | 10   |
| `prowlarr`       | apps    | `apps/prowlarr`       | `default`     | 10   |
| `qbittorrent`    | apps    | `apps/qbittorrent`    | `default`     | 10   |
| `radarr`         | apps    | `apps/radarr`         | `default`     | 10   |
| `seerr`          | apps    | `apps/seerr`          | `default`     | 10   |

### Old file → new file map

| Old                                      | New                                   |
|------------------------------------------|---------------------------------------|
| `deployment/<app>.yaml`                  | `apps/<app>/deployment.yaml`          |
| `service/<app>.yaml`                     | `apps/<app>/service.yaml`             |
| `serviceaccount/homepage.yaml`           | `apps/homepage/rbac.yaml`             |
<!-- | # TODO: `ingress/ingress.yaml` (one big Ingress) | `apps/<app>/ingress.yaml` (split)     | -->
| `ingress/traefik-dashboard.yaml`         | `infra/traefik/dashboard.yaml`        |
| `ingress/homepage-discovery.yaml.example`| `apps/ingress.yaml.example`           |
| `tls/tls-store.yaml`                     | `infra/traefik/tls-store.yaml`        |
| `tls/sealed-secrets/controller.yaml`     | `infra/sealed-secrets/controller.yaml`|
| `tls/certificates/*`                     | `pki/*`                               |
| `tls/secret.yaml`                        | `pki/h18s-tls.secret.yaml` (gitignored; sealed copy goes to `infra/traefik/`) |
| `argocd/ingress.yaml`                    | `infra/argocd/ingress.yaml`           |
| `argocd/app.yaml`                        | **delete** (replaced by `bootstrap/root.yaml`) |
| `argocd/argocd-application.yaml`         | **delete** (broken: `YOUR_USER/YOUR_REPO`) |
| `argocd/namespace.yaml`                  | **delete** (invalid apiVersion/kind; ns already exists) |

---

## 1. Move existing files

Run from the repo root (fish and bash both work):

```sh
mkdir -p bootstrap argocd/projects argocd/apps infra/argocd infra/sealed-secrets infra/traefik pki

for a in homepage jellyfin prowlarr qbittorrent radarr seerr
    mkdir -p apps/$a
    git mv deployment/$a.yaml apps/$a/deployment.yaml
    git mv service/$a.yaml    apps/$a/service.yaml
end
# bash: use `for a in ...; do ...; done` instead

git mv serviceaccount/homepage.yaml        apps/homepage/rbac.yaml
git mv ingress/traefik-dashboard.yaml      infra/traefik/dashboard.yaml
git mv tls/sealed-secrets/controller.yaml  infra/sealed-secrets/controller.yaml
git mv argocd/ingress.yaml                 infra/argocd/ingress.yaml
git rm argocd/app.yaml argocd/argocd-application.yaml argocd/namespace.yaml ingress/ingress.yaml

# untracked files: plain mv
mv tls/tls-store.yaml                      infra/traefik/tls-store.yaml
mv ingress/homepage-discovery.yaml.example apps/ingress.yaml.example
mv tls/certificates/*                      pki/
mv tls/secret.yaml                         pki/h18s-tls.secret.yaml

rmdir tls/certificates tls/sealed-secrets tls deployment service serviceaccount ingress
```

### `.gitignore` (replace)

```gitignore
# private keys and the unsealed TLS secret never go to git
pki/*.key
pki/*.secret.yaml
```

---

## 2. Fix bugs in moved manifests

### `apps/radarr/deployment.yaml` and `apps/qbittorrent/deployment.yaml`

`type: Directory` sits at the volume level, so Kubernetes ignores it. Move it
under `hostPath`:

```yaml
# before
      volumes:
        - name: data
          type: Directory
          hostPath:
            path: /home/ad/homelab/data

# after
      volumes:
        - name: data
          hostPath:
            path: /home/ad/homelab/data
            type: Directory
```

Do the same for the `config` volume in both files.

### `apps/homepage/service.yaml`

Delete the empty `  type:` line under `spec:`.

### `apps/ingress.yaml.example`

Change the comment line
`# Copy to <app>.yaml (Argo CD ignores *.example), ...`
to
`# Copy to apps/<app>/ingress.yaml and add it to that app's kustomization.yaml, ...`

---

## 3. Split the big Ingress into one per app

Create one `ingress.yaml` per app. Template (replace `NAME`, `HOST`, `PORT`):

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: NAME
  namespace: default
spec:
  ingressClassName: traefik
  rules:
    - host: HOST
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: NAME
                port:
                  number: PORT
```

| File                              | NAME          | HOST                    | PORT |
|-----------------------------------|---------------|-------------------------|------|
| `apps/homepage/ingress.yaml`      | `homepage`    | `h18s`                  | 3000 |
| `apps/jellyfin/ingress.yaml`      | `jellyfin`    | `jellyfin.h18s.lab`     | 8096 |
| `apps/prowlarr/ingress.yaml`      | `prowlarr`    | `prowlarr.h18s.lab`     | 9696 |
| `apps/qbittorrent/ingress.yaml`   | `qbittorrent` | `qbittorrent.h18s.lab`  | 8080 |
| `apps/radarr/ingress.yaml`        | `radarr`      | `radarr.h18s.lab`       | 7878 |
| `apps/seerr/ingress.yaml`         | `seerr`       | `seerr.h18s.lab`        | 5055 |

Optional: add the `gethomepage.dev/*` annotations from `apps/ingress.yaml.example`
so Homepage discovers each app automatically.

---

## 4. Kustomizations (explicit file lists)

### `apps/homepage/kustomization.yaml`

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - rbac.yaml
  - deployment.yaml
  - service.yaml
  - ingress.yaml
```

### `apps/{jellyfin,prowlarr,qbittorrent,radarr,seerr}/kustomization.yaml`

Use the same file in each of the five folders:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - deployment.yaml
  - service.yaml
  - ingress.yaml
```

### `infra/sealed-secrets/kustomization.yaml`

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - controller.yaml
```

### `infra/traefik/kustomization.yaml`

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - dashboard.yaml
  - tls-store.yaml
  # - h18s-tls.sealed.yaml   # uncomment after step 6
```

### `infra/argocd/kustomization.yaml`

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ingress.yaml
```

### `argocd/kustomization.yaml`

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - projects
  - apps
```

### `argocd/projects/kustomization.yaml`

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - infra.yaml
  - apps.yaml
```

### `argocd/apps/kustomization.yaml` (the app registry)

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  # infra
  - sealed-secrets.yaml
  - traefik-config.yaml
  - argocd-config.yaml
  # apps
  - homepage.yaml
  - jellyfin.yaml
  - prowlarr.yaml
  - qbittorrent.yaml
  - radarr.yaml
  - seerr.yaml
```

---

## 5. Argo CD manifests

### `bootstrap/root.yaml`

```yaml
# The only manifest applied by hand:
#   kubectl apply -f bootstrap/root.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: default
  source:
    repoURL: https://github.com/ad1822/h18s.git
    targetRevision: dev
    path: argocd
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

### `argocd/projects/infra.yaml`

```yaml
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: infra
  namespace: argocd
  annotations:
    argocd.argoproj.io/sync-wave: "-1"
spec:
  description: Cluster plumbing (sealed-secrets, traefik config, argocd config)
  sourceRepos:
    - https://github.com/ad1822/h18s.git
  destinations:
    - server: https://kubernetes.default.svc
      namespace: "*"
  clusterResourceWhitelist:
    - group: "*"
      kind: "*"
```

### `argocd/projects/apps.yaml`

```yaml
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: apps
  namespace: argocd
  annotations:
    argocd.argoproj.io/sync-wave: "-1"
spec:
  description: Homelab workloads
  sourceRepos:
    - https://github.com/ad1822/h18s.git
  destinations:
    - server: https://kubernetes.default.svc
      namespace: default
  # homepage needs a ClusterRole to read pods/ingresses for discovery
  clusterResourceWhitelist:
    - group: rbac.authorization.k8s.io
      kind: ClusterRole
    - group: rbac.authorization.k8s.io
      kind: ClusterRoleBinding
```

### Child Application template

Every file in `argocd/apps/` uses this shape. Only the five values in the table
below change.

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: NAME
  namespace: argocd
  annotations:
    argocd.argoproj.io/sync-wave: "WAVE"
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: PROJECT
  source:
    repoURL: https://github.com/ad1822/h18s.git
    targetRevision: dev
    path: PATH
  destination:
    server: https://kubernetes.default.svc
    namespace: NAMESPACE
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

| File                                 | NAME             | PROJECT | PATH                   | NAMESPACE     | WAVE |
|--------------------------------------|------------------|---------|------------------------|---------------|------|
| `argocd/apps/sealed-secrets.yaml`    | `sealed-secrets` | infra   | `infra/sealed-secrets` | `kube-system` | 0    |
| `argocd/apps/traefik-config.yaml`    | `traefik-config` | infra   | `infra/traefik`        | `kube-system` | 1    |
| `argocd/apps/argocd-config.yaml`     | `argocd-config`  | infra   | `infra/argocd`         | `argocd`      | 1    |
| `argocd/apps/homepage.yaml`          | `homepage`       | apps    | `apps/homepage`        | `default`     | 10   |
| `argocd/apps/jellyfin.yaml`          | `jellyfin`       | apps    | `apps/jellyfin`        | `default`     | 10   |
| `argocd/apps/prowlarr.yaml`          | `prowlarr`       | apps    | `apps/prowlarr`        | `default`     | 10   |
| `argocd/apps/qbittorrent.yaml`       | `qbittorrent`    | apps    | `apps/qbittorrent`     | `default`     | 10   |
| `argocd/apps/radarr.yaml`            | `radarr`         | apps    | `apps/radarr`          | `default`     | 10   |
| `argocd/apps/seerr.yaml`             | `seerr`          | apps    | `apps/seerr`           | `default`     | 10   |

Full example, `argocd/apps/radarr.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: radarr
  namespace: argocd
  annotations:
    argocd.argoproj.io/sync-wave: "10"
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: apps
  source:
    repoURL: https://github.com/ad1822/h18s.git
    targetRevision: dev
    path: apps/radarr
  destination:
    server: https://kubernetes.default.svc
    namespace: default
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

> The `resources-finalizer` means that **deleting an Application also deletes
> its resources**. To remove an app but keep its workloads, delete it with
> `--cascade=orphan`.

---

## 6. Seal the TLS secret (after sealed-secrets is running)

`infra/traefik/tls-store.yaml` expects the secret `h18s-tls` in `kube-system`.
Today it only exists as an unsealed, gitignored file. To manage it from git:

```sh
kubeseal --controller-namespace kube-system --controller-name sealed-secrets-controller \
  -f pki/h18s-tls.secret.yaml -o yaml > infra/traefik/h18s-tls.sealed.yaml
```

Then uncomment `h18s-tls.sealed.yaml` in `infra/traefik/kustomization.yaml`.

---

## 7. Validate locally before pushing

```sh
for d in argocd argocd/apps argocd/projects infra/* apps/*/
    test -f $d/kustomization.yaml; and kubectl kustomize $d > /dev/null; and echo "ok $d"
end
```

Every line should print `ok`.

---

## 8. Cut over the cluster (order matters)

1. **Remove the old ApplicationSet but keep the workloads.** If you skip this,
   it will turn `apps/`, `infra/` and `bootstrap/` into apps of their own.
   ```sh
   kubectl -n argocd delete applicationset homelab-apps --cascade=orphan
   ```
2. **Remove the Applications it generated, also keeping the workloads.**
   Ignore "not found" errors.
   ```sh
   kubectl -n argocd get applications
   kubectl -n argocd delete application deployment service ingress tls serviceaccount argocd --cascade=orphan
   ```
   If any of them still has a finalizer and hangs, patch it off:
   ```sh
   kubectl -n argocd patch application <name> --type merge -p '{"metadata":{"finalizers":null}}'
   ```
3. **Commit and push** the new layout to `dev`.
4. **Bootstrap:**
   ```sh
   kubectl apply -f bootstrap/root.yaml
   ```
   Names and namespaces haven't changed, so the new Applications take over the
   existing Deployments and Services without recreating them.
5. **Delete the old combined Ingress.** Its hosts now clash with the per-app
   Ingresses:
   ```sh
   kubectl -n default delete ingress homelab
   ```
6. **Verify:**
   ```sh
   kubectl -n argocd get applications   # all Synced / Healthy
   kubectl -n default get ingress       # one per app
   ```

---

## 9. Day-to-day: adding a new app `foo`

1. Create `apps/foo/` with `deployment.yaml`, `service.yaml`, `ingress.yaml`
   (copy from `apps/ingress.yaml.example`) and a `kustomization.yaml` that lists them.
2. Create `argocd/apps/foo.yaml` from the child Application template
   (project `apps`, path `apps/foo`, wave `10`).
3. Add `- foo.yaml` to `argocd/apps/kustomization.yaml`.
4. Push. `root` picks it up.

To remove an app, delete its line from `argocd/apps/kustomization.yaml` and push.
The finalizer removes the app's resources.

---

## 10. Optional follow-ups

- **Namespaces:** move the media apps into a `media` namespace. Update
  `destination.namespace`, `metadata.namespace` in the manifests, the
  ClusterRoleBinding subject, and the `apps` project's `destinations`. Check
  first whether any app config calls another service by short name (e.g.
  `http://radarr:7878`), because short names only resolve within the same namespace.
- **Branch:** all Applications track `dev`. To switch to `main`, change
  `targetRevision` in `bootstrap/root.yaml` and every `argocd/apps/*.yaml`.
- **Image tags:** most images use `:latest`. Pinning versions makes Argo CD
  diffs and rollbacks meaningful.
- **Argo CD managing itself:** later, add an `argocd` Application (Helm chart or
  upstream install manifest) under `infra/` so Argo CD's own install is in git too.

FROM ghcr.io/actions/actions-runner:2.337.0

ARG KUSTOMIZE_VERSION=v5.8.2
ARG KUBECONFORM_VERSION=v0.8.0

USER root
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates curl \
  && curl -sSL "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F${KUSTOMIZE_VERSION}/kustomize_${KUSTOMIZE_VERSION}_linux_amd64.tar.gz" \
  | tar xz -C /usr/local/bin kustomize \
  && curl -sSL "https://github.com/yannh/kubeconform/releases/download/${KUBECONFORM_VERSION}/kubeconform-linux-amd64.tar.gz" \
  | tar xz -C /usr/local/bin kubeconform \
  # schemas: sparse clones, only the folders we use
  && git clone --depth 1 --filter=blob:none --sparse https://github.com/yannh/kubernetes-json-schema /opt/schemas/k8s \
  && git -C /opt/schemas/k8s sparse-checkout set master-standalone-strict \
  && git clone --depth 1 --filter=blob:none --sparse https://github.com/datreeio/CRDs-catalog /opt/schemas/crds \
  && git -C /opt/schemas/crds sparse-checkout set bitnami.com argoproj.io traefik.io \
  && rm -rf /opt/schemas/*/.git /var/lib/apt/lists/*
USER runner

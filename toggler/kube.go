package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const saDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// kubeClient is a minimal Kubernetes API client: just enough to read a
// Deployment and patch its scale subresource, without pulling in client-go.
type kubeClient struct {
	base      string
	tokenFile string // empty when talking to `kubectl proxy`
	http      *http.Client
}

type deployment struct {
	Spec struct {
		Replicas *int32 `json:"replicas"`
	} `json:"spec"`
	Status struct {
		Replicas      int32 `json:"replicas"`
		ReadyReplicas int32 `json:"readyReplicas"`
	} `json:"status"`
}

// newKubeClient uses KUBE_API when set (local dev via `kubectl proxy`),
// otherwise the in-cluster service account.
func newKubeClient() (*kubeClient, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}

	if base := os.Getenv("KUBE_API"); base != "" {
		return &kubeClient{base: strings.TrimRight(base, "/"), http: httpClient}, nil
	}

	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("not running in a cluster; set KUBE_API (e.g. http://127.0.0.1:8001 from `kubectl proxy`)")
	}

	ca, err := os.ReadFile(saDir + "/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("read service account CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("service account CA contains no certificates")
	}
	httpClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}

	return &kubeClient{
		base:      "https://" + net.JoinHostPort(host, port),
		tokenFile: saDir + "/token",
		http:      httpClient,
	}, nil
}

func (k *kubeClient) do(ctx context.Context, method, path, contentType string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, k.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if k.tokenFile != "" {
		// Read on every request: projected service account tokens rotate.
		token, err := os.ReadFile(k.tokenFile)
		if err != nil {
			return fmt.Errorf("read service account token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	}

	resp, err := k.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func deploymentPath(namespace, name string) string {
	return "/apis/apps/v1/namespaces/" + namespace + "/deployments/" + name
}

func (k *kubeClient) getDeployment(ctx context.Context, namespace, name string) (*deployment, error) {
	var d deployment
	if err := k.do(ctx, http.MethodGet, deploymentPath(namespace, name), "", nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (k *kubeClient) scale(ctx context.Context, namespace, name string, replicas int32) error {
	body := fmt.Appendf(nil, `{"spec":{"replicas":%d}}`, replicas)
	return k.do(ctx, http.MethodPatch, deploymentPath(namespace, name)+"/scale",
		"application/merge-patch+json", body, nil)
}

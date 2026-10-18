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

type cronJob struct {
	Metadata struct {
		UID string `json:"uid"`
	} `json:"metadata"`
	Spec struct {
		JobTemplate struct {
			Metadata struct {
				Labels      map[string]string `json:"labels"`
				Annotations map[string]string `json:"annotations"`
			} `json:"metadata"`
			Spec map[string]any `json:"spec"`
		} `json:"jobTemplate"`
	} `json:"spec"`
}

type jobList struct {
	Items []struct {
		Metadata struct {
			OwnerReferences []struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"ownerReferences"`
		} `json:"metadata"`
		Status struct {
			Active int32 `json:"active"`
		} `json:"status"`
	} `json:"items"`
}

// jobActive reports whether a Job owned by the named CronJob is still running.
func (k *kubeClient) jobActive(ctx context.Context, namespace, cronJobName string) (bool, error) {
	var l jobList
	if err := k.do(ctx, http.MethodGet, "/apis/batch/v1/namespaces/"+namespace+"/jobs", "", nil, &l); err != nil {
		return false, err
	}
	for _, j := range l.Items {
		for _, o := range j.Metadata.OwnerReferences {
			if o.Kind == "CronJob" && o.Name == cronJobName && j.Status.Active > 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

// runCronJob creates a Job from the CronJob's template, like
// `kubectl create job --from=cronjob/<name>`. It returns the new Job's name.
func (k *kubeClient) runCronJob(ctx context.Context, namespace, name string) (string, error) {
	var cj cronJob
	if err := k.do(ctx, http.MethodGet, "/apis/batch/v1/namespaces/"+namespace+"/cronjobs/"+name, "", nil, &cj); err != nil {
		return "", err
	}

	tmpl := cj.Spec.JobTemplate
	spec := tmpl.Spec
	if spec == nil {
		spec = map[string]any{}
	}
	if _, ok := spec["ttlSecondsAfterFinished"]; !ok {
		spec["ttlSecondsAfterFinished"] = 3600 // clean up manual runs
	}
	annotations := map[string]string{"cronjob.kubernetes.io/instantiate": "manual"}
	for key, v := range tmpl.Metadata.Annotations {
		annotations[key] = v
	}

	jobName := fmt.Sprintf("%s-manual-%d", name, time.Now().Unix())
	body, err := json.Marshal(map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name":        jobName,
			"labels":      tmpl.Metadata.Labels,
			"annotations": annotations,
			"ownerReferences": []map[string]any{{
				"apiVersion": "batch/v1",
				"kind":       "CronJob",
				"name":       name,
				"uid":        cj.Metadata.UID,
				"controller": true,
			}},
		},
		"spec": spec,
	})
	if err != nil {
		return "", err
	}
	err = k.do(ctx, http.MethodPost, "/apis/batch/v1/namespaces/"+namespace+"/jobs", "application/json", body, nil)
	return jobName, err
}

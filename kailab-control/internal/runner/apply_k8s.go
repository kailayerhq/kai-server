package runner

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
)

// actionApplyK8s implements kailab/apply-k8s — reads kustomize manifests from
// the workspace, applies image overrides, applies to the cluster, and waits
// for deployments to roll out.
//
// Usage in workflow:
//
//	- uses: kailab/apply-k8s
//	  with:
//	    path: deploy/k8s/overlays/production
//	    images: |
//	      KAILAB_IMAGE=registry/app:sha
//	      KAILAB_CONTROL_IMAGE=registry/control:sha
//	    timeout: "600"    # optional, seconds, default 300
//	    namespace: kailab # optional fallback namespace
func (jp *JobPod) actionApplyK8s(ctx context.Context, stepDef *StepDefinition, logWriter io.Writer) (*ExecutionResult, error) {
	kustomizePath := stepDef.With["path"]
	imagesInput := stepDef.With["images"]
	timeoutStr := stepDef.With["timeout"]
	fallbackNS := stepDef.With["namespace"]

	if kustomizePath == "" {
		return &ExecutionResult{ExitCode: 1, Output: "missing required input: path"}, nil
	}

	timeout := 300 * time.Second
	if timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr + "s"); err == nil {
			timeout = d
		}
	}

	// Step 1: Read the kustomize directory tree from the workspace pod via tar
	// Tar the top-level directory (first path component) to capture sibling dirs
	// that kustomization.yaml may reference via relative paths (e.g. ../../base)
	fmt.Fprintf(logWriter, "Reading manifests from /workspace/%s\n", kustomizePath)

	// Find the root directory to tar — walk up to capture relative references
	tarRoot := kustomizePath
	if parts := strings.SplitN(kustomizePath, "/", 2); len(parts) > 0 {
		// Use the top two path components (e.g. "deploy/k8s" from "deploy/k8s/overlays/production")
		rootParts := strings.Split(kustomizePath, "/")
		if len(rootParts) > 2 {
			tarRoot = strings.Join(rootParts[:2], "/")
		}
	}

	tarCmd := fmt.Sprintf("cd /workspace && tar czf - %s | base64", tarRoot)
	result, err := jp.executeCommand(ctx, tarCmd, "bash", nil, "", io.Discard)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifests from pod: %w", err)
	}
	if result.ExitCode != 0 {
		fmt.Fprintf(logWriter, "Error: failed to read manifests at path %q\n", kustomizePath)
		return &ExecutionResult{ExitCode: 1, Output: "failed to read kustomize path"}, nil
	}

	// Extract tar to a temp directory
	tmpDir, err := extractBase64Tar(result.Output)
	if err != nil {
		fmt.Fprintf(logWriter, "Error: failed to extract manifests: %v\n", err)
		return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
	}
	defer os.RemoveAll(tmpDir)

	// Step 2: Resolve kustomize manifests
	fullPath := filepath.Join(tmpDir, kustomizePath)
	manifests, k, err := resolveKustomize(fullPath)
	if err != nil {
		fmt.Fprintf(logWriter, "Error: %v\n", err)
		return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
	}

	// Step 3: Collect image overrides from kustomization.yaml and action input
	var allImages []KustomizeImage
	if k != nil {
		allImages = append(allImages, k.Images...)
	}
	if imagesInput != "" {
		allImages = append(allImages, parseImageLines(imagesInput)...)
	}

	// Step 4: Apply image overrides
	if len(allImages) > 0 {
		for _, img := range allImages {
			newRef := img.NewName
			if img.Digest != "" {
				newRef += "@" + img.Digest
			} else if img.NewTag != "" {
				newRef += ":" + img.NewTag
			}
			fmt.Fprintf(logWriter, "Image: %s → %s\n", img.Name, newRef)
		}
		manifests = applyImageOverrides(manifests, allImages)
	}

	// Step 5: Parse into unstructured objects
	objects, err := decodeManifests(manifests)
	if err != nil {
		fmt.Fprintf(logWriter, "Error parsing manifests: %v\n", err)
		return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
	}

	fmt.Fprintf(logWriter, "Applying %d resources to cluster\n", len(objects))

	// Step 6: Apply each object via the K8s API
	dynClient, err := dynamic.NewForConfig(jp.executor.config)
	if err != nil {
		fmt.Fprintf(logWriter, "Error creating dynamic client: %v\n", err)
		return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
	}

	groupResources, err := restmapper.GetAPIGroupResources(jp.executor.client.Discovery())
	if err != nil {
		fmt.Fprintf(logWriter, "Error discovering API resources: %v\n", err)
		return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	var deployments []deploymentRef
	for _, obj := range objects {
		gvk := obj.GroupVersionKind()

		// Skip Namespace resources — they should already exist and require cluster-level permissions
		if gvk.Kind == "Namespace" {
			fmt.Fprintf(logWriter, "Skipping %s %s (already exists)\n", gvk.Kind, obj.GetName())
			continue
		}

		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			fmt.Fprintf(logWriter, "Error: unknown resource type %s: %v\n", gvk.String(), err)
			return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
		}

		ns := obj.GetNamespace()
		if ns == "" {
			ns = fallbackNS
		}

		name := obj.GetName()
		data, err := json.Marshal(obj.Object)
		if err != nil {
			return nil, err
		}

		var resource dynamic.ResourceInterface
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			resource = dynClient.Resource(mapping.Resource).Namespace(ns)
		} else {
			resource = dynClient.Resource(mapping.Resource)
		}

		force := true
		_, err = resource.Patch(ctx, name, types.ApplyPatchType, data, metav1.PatchOptions{
			FieldManager: "kailab-apply-k8s",
			Force:        &force,
		})
		if err != nil {
			fmt.Fprintf(logWriter, "Error applying %s %s/%s: %v\n", gvk.Kind, ns, name, err)
			return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
		}

		fmt.Fprintf(logWriter, "Applied %s %s/%s\n", gvk.Kind, ns, name)

		if gvk.Kind == "Deployment" {
			deployments = append(deployments, deploymentRef{name: name, namespace: ns})
		}
	}

	// Step 7: Wait for all deployments to roll out
	if len(deployments) > 0 {
		fmt.Fprintf(logWriter, "Waiting for %d deployment(s) to roll out (timeout %s)\n", len(deployments), timeout)
		if err := waitForDeployments(ctx, jp.executor.client.AppsV1(), deployments, timeout, logWriter); err != nil {
			return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
		}
	}

	fmt.Fprintf(logWriter, "All resources applied successfully\n")
	return &ExecutionResult{ExitCode: 0}, nil
}

type deploymentRef struct {
	name      string
	namespace string
}

func waitForDeployments(ctx context.Context, client appsv1client.AppsV1Interface, refs []deploymentRef, timeout time.Duration, logWriter io.Writer) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	pending := make(map[string]deploymentRef)
	for _, ref := range refs {
		pending[ref.namespace+"/"+ref.name] = ref
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancelled: %w", ctx.Err())
		case <-timer.C:
			var names []string
			for k := range pending {
				names = append(names, k)
			}
			return fmt.Errorf("rollout timed out, still pending: %s", strings.Join(names, ", "))
		case <-ticker.C:
			for key, ref := range pending {
				d, err := client.Deployments(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
				if err != nil {
					fmt.Fprintf(logWriter, "Warning: failed to check %s: %v\n", key, err)
					continue
				}
				if rolloutComplete(d) {
					fmt.Fprintf(logWriter, "Rollout complete: %s (%d/%d replicas ready)\n",
						key, d.Status.ReadyReplicas, replicaCount(d))
					delete(pending, key)
				} else {
					fmt.Fprintf(logWriter, "Waiting: %s %d/%d replicas ready\n",
						key, d.Status.ReadyReplicas, replicaCount(d))
				}
			}
			if len(pending) == 0 {
				return nil
			}
		}
	}
}

// decodeManifests splits multi-document YAML and decodes each into an Unstructured object.
func decodeManifests(data []byte) ([]*unstructured.Unstructured, error) {
	var objects []*unstructured.Unstructured
	decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(string(data)), 4096)
	serializer := k8syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decoding manifest: %w", err)
		}
		if raw == nil || string(raw) == "null" {
			continue
		}

		obj := &unstructured.Unstructured{}
		_, _, err := serializer.Decode(raw, nil, obj)
		if err != nil {
			return nil, fmt.Errorf("deserializing resource: %w", err)
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

// extractBase64Tar decodes base64-encoded tar.gz data and extracts to a temp directory.
func extractBase64Tar(b64 string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return "", fmt.Errorf("decoding base64: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "kailab-apply-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	gr, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("decompressing: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("reading tar: %w", err)
		}

		target := filepath.Join(tmpDir, hdr.Name)
		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), tmpDir) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.Create(target)
			if err != nil {
				os.RemoveAll(tmpDir)
				return "", err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				os.RemoveAll(tmpDir)
				return "", err
			}
			f.Close()
		}
	}

	return tmpDir, nil
}

// Ensure appsv1.Deployment is used to avoid unused import.
var _ *appsv1.Deployment

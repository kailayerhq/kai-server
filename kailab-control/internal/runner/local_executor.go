package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"kailab-control/internal/store"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// LocalExecutor runs jobs directly on the host machine (no Kubernetes).
// Designed for macOS runners and other environments without container orchestration.
type LocalExecutor struct {
	baseDir string // Base directory for job workspaces
	store   store.Store
}

// NewLocalExecutor creates a new local executor.
func NewLocalExecutor(storePath string, ciStore store.Store) (*LocalExecutor, error) {
	baseDir := filepath.Join(os.TempDir(), "kailab-ci-jobs")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base dir: %w", err)
	}
	return &LocalExecutor{
		baseDir: baseDir,
		store:   ciStore,
	}, nil
}

// CreateJob creates a local workspace directory for the job.
func (e *LocalExecutor) CreateJob(ctx context.Context, jobID, jobName string, jobContext map[string]interface{}) (Job, error) {
	workDir := filepath.Join(e.baseDir, "job-"+sanitizeName(jobID))
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Create GITHUB_OUTPUT, GITHUB_ENV, etc.
	for _, f := range []string{"github_output", "github_env", "github_state", "github_path", "github_step_summary"} {
		path := filepath.Join(workDir, f)
		os.WriteFile(path, []byte{}, 0644)
	}

	return &LocalJob{
		workDir:  workDir,
		executor: e,
	}, nil
}

// GCStalePods removes old workspace directories.
func (e *LocalExecutor) GCStalePods(ctx context.Context) {
	entries, err := os.ReadDir(e.baseDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-30 * time.Minute)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "job-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(e.baseDir, entry.Name())
			os.RemoveAll(path)
		}
	}
}

// LocalJob represents a job running directly on the host.
type LocalJob struct {
	workDir  string
	executor *LocalExecutor
}

// ExecuteStep runs a single workflow step locally.
func (j *LocalJob) ExecuteStep(ctx context.Context, stepDef *StepDefinition, jobContext map[string]interface{}, logWriter io.Writer) (*ExecutionResult, error) {
	if stepDef.Uses != "" {
		return j.executeAction(ctx, stepDef, jobContext, logWriter)
	}
	if stepDef.Run != "" {
		return j.executeCommand(ctx, stepDef.Run, stepDef.Shell, stepDef.Env, stepDef.WorkingDir, logWriter)
	}
	return &ExecutionResult{ExitCode: 0}, nil
}

// ExecuteCommandWithTimeout runs a command with a maximum duration.
func (j *LocalJob) ExecuteCommandWithTimeout(ctx context.Context, timeout time.Duration, script, shell string, logWriter io.Writer) (*ExecutionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return j.executeCommand(ctx, script, shell, nil, "", logWriter)
}

// Cleanup removes the job workspace.
func (j *LocalJob) Cleanup(ctx context.Context) error {
	return os.RemoveAll(j.workDir)
}

// executeCommand runs a shell command on the host.
func (j *LocalJob) executeCommand(ctx context.Context, script, shell string, env map[string]string, workingDir string, logWriter io.Writer) (*ExecutionResult, error) {
	if shell == "" {
		shell = "bash"
	}

	// Build the full script with env setup
	var cmdBuilder strings.Builder

	// Set up GITHUB_* file paths
	cmdBuilder.WriteString(fmt.Sprintf("export GITHUB_OUTPUT=%q\n", filepath.Join(j.workDir, "github_output")))
	cmdBuilder.WriteString(fmt.Sprintf("export GITHUB_ENV=%q\n", filepath.Join(j.workDir, "github_env")))
	cmdBuilder.WriteString(fmt.Sprintf("export GITHUB_STATE=%q\n", filepath.Join(j.workDir, "github_state")))
	cmdBuilder.WriteString(fmt.Sprintf("export GITHUB_PATH=%q\n", filepath.Join(j.workDir, "github_path")))
	cmdBuilder.WriteString(fmt.Sprintf("export GITHUB_STEP_SUMMARY=%q\n", filepath.Join(j.workDir, "github_step_summary")))
	cmdBuilder.WriteString(fmt.Sprintf("export GITHUB_WORKSPACE=%q\n", j.workDir))

	// Source any env vars set by previous steps via GITHUB_ENV
	cmdBuilder.WriteString(fmt.Sprintf("if [ -s %q ]; then set -a; . %q 2>/dev/null || true; set +a; fi\n",
		filepath.Join(j.workDir, "github_env"), filepath.Join(j.workDir, "github_env")))

	// Add PATH entries from GITHUB_PATH
	cmdBuilder.WriteString(fmt.Sprintf("if [ -s %q ]; then while IFS= read -r p; do export PATH=\"$p:$PATH\"; done < %q; fi\n",
		filepath.Join(j.workDir, "github_path"), filepath.Join(j.workDir, "github_path")))

	for name, value := range env {
		cmdBuilder.WriteString(fmt.Sprintf("export %s=%q\n", name, value))
	}

	effectiveDir := j.workDir
	if workingDir != "" {
		if filepath.IsAbs(workingDir) {
			effectiveDir = workingDir
		} else {
			effectiveDir = filepath.Join(j.workDir, workingDir)
		}
	}
	cmdBuilder.WriteString(fmt.Sprintf("cd %q\n", effectiveDir))
	cmdBuilder.WriteString(script)

	cmd := exec.CommandContext(ctx, shell, "-e", "-c", cmdBuilder.String())
	cmd.Dir = effectiveDir

	// Inherit host environment + overlay
	cmd.Env = os.Environ()
	for name, value := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", name, value))
	}

	var stdout bytes.Buffer
	cmd.Stdout = io.MultiWriter(&stdout, logWriter)
	cmd.Stderr = logWriter

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return &ExecutionResult{
		ExitCode: exitCode,
		Output:   stdout.String(),
	}, nil
}

// executeAction handles "uses:" steps locally.
func (j *LocalJob) executeAction(ctx context.Context, stepDef *StepDefinition, jobContext map[string]interface{}, logWriter io.Writer) (*ExecutionResult, error) {
	action := stepDef.Uses

	switch {
	case strings.HasPrefix(action, "actions/checkout"):
		return j.actionCheckout(ctx, stepDef, jobContext, logWriter)
	case strings.HasPrefix(action, "actions/setup-go"):
		return j.actionSetupGo(ctx, stepDef, logWriter)
	case strings.HasPrefix(action, "actions/setup-node"):
		return j.actionSetupNode(ctx, stepDef, logWriter)
	case strings.HasPrefix(action, "actions/upload-artifact"):
		return j.actionUploadArtifact(ctx, stepDef, jobContext, logWriter)
	case strings.HasPrefix(action, "actions/download-artifact"):
		return j.actionDownloadArtifact(ctx, stepDef, jobContext, logWriter)
	case strings.HasPrefix(action, "actions/cache"):
		// Cache is a no-op locally for now — local disk is the cache
		fmt.Fprintf(logWriter, "Cache action skipped (local executor uses host filesystem)\n")
		return &ExecutionResult{ExitCode: 0}, nil
	case strings.HasPrefix(action, "kailab/apply-k8s"):
		return j.actionApplyK8s(ctx, stepDef, logWriter)
	default:
		fmt.Fprintf(logWriter, "Unsupported action %q on local executor, skipping\n", action)
		return &ExecutionResult{ExitCode: 0}, nil
	}
}

// actionCheckout clones the repo into the workspace.
func (j *LocalJob) actionCheckout(ctx context.Context, stepDef *StepDefinition, jobContext map[string]interface{}, logWriter io.Writer) (*ExecutionResult, error) {
	cloneURL, _ := jobContext["clone_url"].(string)
	repo, _ := jobContext["repo"].(string)
	ref, _ := jobContext["ref"].(string)
	sha, _ := jobContext["sha"].(string)

	if overrideRef, ok := stepDef.With["ref"]; ok && overrideRef != "" {
		ref = overrideRef
	}
	if cloneURL == "" && repo != "" {
		cloneURL = repo
	}

	fmt.Fprintf(logWriter, "Checking out %s @ %s\n", repo, sha)

	// Try Kai API checkout first (archive download)
	snapRef := "snap.latest"
	if strings.HasPrefix(ref, "refs/heads/") {
		branch := strings.TrimPrefix(ref, "refs/heads/")
		snapRef = "snap." + branch
	}

	archiveURL := fmt.Sprintf("%s/v1/archive/%s", cloneURL, snapRef)
	fmt.Fprintf(logWriter, "Fetching archive from %s\n", snapRef)

	httpClient := &http.Client{Timeout: 120 * time.Second}
	resp, err := httpClient.Get(archiveURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		// Download archive to temp file, then extract
		tmpFile, tmpErr := os.CreateTemp("", "kai-archive-*.tar.gz")
		if tmpErr == nil {
			_, copyErr := io.Copy(tmpFile, resp.Body)
			resp.Body.Close()
			tmpFile.Close()
			if copyErr == nil {
				script := fmt.Sprintf("tar xzf %q -C %q", tmpFile.Name(), j.workDir)
				result, err := j.executeCommand(ctx, script, "bash", nil, "", logWriter)
				os.Remove(tmpFile.Name())
				if err == nil {
					fmt.Fprintf(logWriter, "Checkout complete\n")
					return result, nil
				}
			}
			os.Remove(tmpFile.Name())
		}
		if resp.Body != nil {
			resp.Body.Close()
		}
	} else if resp != nil {
		resp.Body.Close()
	}

	// Fallback: git clone into workspace
	fmt.Fprintf(logWriter, "Archive unavailable, falling back to git clone\n")
	script := fmt.Sprintf(`
git init
git remote add origin %q
git fetch --depth 1 origin %s
git checkout FETCH_HEAD
`, cloneURL, ref)
	return j.executeCommand(ctx, script, "bash", nil, "", logWriter)
}

// actionSetupGo verifies Go is available on the host.
func (j *LocalJob) actionSetupGo(ctx context.Context, stepDef *StepDefinition, logWriter io.Writer) (*ExecutionResult, error) {
	goVersion := "1.22"
	if v, ok := stepDef.With["go-version"]; ok {
		goVersion = v
	}

	fmt.Fprintf(logWriter, "Setting up Go %s\n", goVersion)

	script := fmt.Sprintf(`
if command -v go &> /dev/null; then
    echo "Go already installed: $(go version)"
else
    echo "Go not found. Installing Go %s..."
    if command -v brew &> /dev/null; then
        brew install go@%s 2>&1 || brew install go 2>&1
    elif command -v apt-get &> /dev/null; then
        sudo apt-get update -qq && sudo apt-get install -y -qq golang 2>&1
    else
        echo "Please install Go %s manually"
        exit 1
    fi
fi
go version
`, goVersion, goVersion, goVersion)

	return j.executeCommand(ctx, script, "bash", nil, "", logWriter)
}

// actionSetupNode verifies Node.js is available on the host.
func (j *LocalJob) actionSetupNode(ctx context.Context, stepDef *StepDefinition, logWriter io.Writer) (*ExecutionResult, error) {
	nodeVersion := "20"
	if v, ok := stepDef.With["node-version"]; ok {
		nodeVersion = v
	}

	fmt.Fprintf(logWriter, "Setting up Node.js %s\n", nodeVersion)

	script := fmt.Sprintf(`
if command -v node &> /dev/null; then
    echo "Node.js already installed: $(node --version)"
else
    echo "Node.js not found. Installing Node.js %s..."
    if command -v brew &> /dev/null; then
        brew install node@%s 2>&1 || brew install node 2>&1
    elif command -v apt-get &> /dev/null; then
        curl -fsSL https://deb.nodesource.com/setup_%s.x | sudo bash - > /dev/null 2>&1
        sudo apt-get install -y -qq nodejs > /dev/null
    else
        echo "Please install Node.js %s manually"
        exit 1
    fi
fi
node --version
npm --version
`, nodeVersion, nodeVersion, nodeVersion, nodeVersion)

	return j.executeCommand(ctx, script, "bash", nil, "", logWriter)
}

// actionUploadArtifact stores artifacts in the local store.
func (j *LocalJob) actionUploadArtifact(ctx context.Context, stepDef *StepDefinition, jobContext map[string]interface{}, logWriter io.Writer) (*ExecutionResult, error) {
	name := stepDef.With["name"]
	path := stepDef.With["path"]
	if name == "" || path == "" {
		return &ExecutionResult{ExitCode: 0}, nil
	}

	runID, _ := jobContext["run_id"].(string)
	fmt.Fprintf(logWriter, "Uploading artifact %s from %s\n", name, path)

	// Create a tar of the artifact path
	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(j.workDir, absPath)
	}

	script := fmt.Sprintf("cd %q && tar czf /dev/stdout .", filepath.Dir(absPath))
	result, err := j.executeCommand(ctx, script, "bash", nil, "", io.Discard)
	if err != nil {
		return result, err
	}

	if j.executor.store != nil {
		key := fmt.Sprintf("artifacts/%s/%s.tar.gz", runID, name)
		f, err := os.Open(absPath)
		if err == nil {
			info, _ := f.Stat()
			j.executor.store.Put(ctx, key, f, info.Size())
			f.Close()
			fmt.Fprintf(logWriter, "Artifact %s stored\n", name)
		}
	}

	return &ExecutionResult{ExitCode: 0}, nil
}

// actionDownloadArtifact retrieves artifacts from the local store.
func (j *LocalJob) actionDownloadArtifact(ctx context.Context, stepDef *StepDefinition, jobContext map[string]interface{}, logWriter io.Writer) (*ExecutionResult, error) {
	name := stepDef.With["name"]
	destPath := stepDef.With["path"]
	if destPath == "" {
		destPath = "."
	}

	runID, _ := jobContext["run_id"].(string)
	fmt.Fprintf(logWriter, "Downloading artifact %s\n", name)

	if j.executor.store == nil {
		fmt.Fprintf(logWriter, "No artifact store configured\n")
		return &ExecutionResult{ExitCode: 1}, fmt.Errorf("no artifact store")
	}

	key := fmt.Sprintf("artifacts/%s/%s.tar.gz", runID, name)
	reader, err := j.executor.store.Get(ctx, key)
	if err != nil {
		return &ExecutionResult{ExitCode: 1}, fmt.Errorf("artifact %s not found: %w", name, err)
	}
	defer reader.Close()

	absPath := destPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(j.workDir, absPath)
	}
	os.MkdirAll(absPath, 0755)

	script := fmt.Sprintf("cd %q && tar xzf -", absPath)
	return j.executeCommand(ctx, script, "bash", nil, "", logWriter)
}

// actionApplyK8s reads kustomize manifests from the local workspace,
// applies image overrides, and applies to the cluster via the in-cluster config.
func (j *LocalJob) actionApplyK8s(ctx context.Context, stepDef *StepDefinition, logWriter io.Writer) (*ExecutionResult, error) {
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

	fullPath := filepath.Join(j.workDir, kustomizePath)
	fmt.Fprintf(logWriter, "Reading manifests from %s\n", fullPath)

	manifests, k, err := resolveKustomize(fullPath)
	if err != nil {
		fmt.Fprintf(logWriter, "Error: %v\n", err)
		return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
	}

	// Collect image overrides from kustomization.yaml and action input
	var allImages []KustomizeImage
	if k != nil {
		allImages = append(allImages, k.Images...)
	}
	if imagesInput != "" {
		allImages = append(allImages, parseImageLines(imagesInput)...)
	}

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

	// Parse into unstructured objects
	objects, err := decodeManifests(manifests)
	if err != nil {
		fmt.Fprintf(logWriter, "Error parsing manifests: %v\n", err)
		return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
	}

	fmt.Fprintf(logWriter, "Applying %d resources to cluster\n", len(objects))

	// Get k8s config — try in-cluster first, fall back to default kubeconfig
	config, err := rest.InClusterConfig()
	if err != nil {
		config, err = clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
		if err != nil {
			fmt.Fprintf(logWriter, "Error: no kubernetes config available: %v\n", err)
			return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(logWriter, "Error creating kubernetes client: %v\n", err)
		return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(logWriter, "Error creating dynamic client: %v\n", err)
		return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
	}

	groupResources, err := restmapper.GetAPIGroupResources(clientset.Discovery())
	if err != nil {
		fmt.Fprintf(logWriter, "Error discovering API resources: %v\n", err)
		return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	var deployments []deploymentRef
	for _, obj := range objects {
		gvk := obj.GroupVersionKind()

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

	if len(deployments) > 0 {
		fmt.Fprintf(logWriter, "Waiting for %d deployment(s) to roll out (timeout %s)\n", len(deployments), timeout)
		if err := waitForDeployments(ctx, clientset.AppsV1(), deployments, timeout, logWriter); err != nil {
			return &ExecutionResult{ExitCode: 1, Output: err.Error()}, nil
		}
	}

	fmt.Fprintf(logWriter, "All resources applied successfully\n")
	return &ExecutionResult{ExitCode: 0}, nil
}

// hashFilesLocal computes hashFiles() on the local filesystem.
func hashFilesLocal(workDir string, patterns []string) string {
	h := sha256.New()
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(workDir, pattern))
		if err != nil {
			continue
		}
		for _, m := range matches {
			data, err := os.ReadFile(m)
			if err == nil {
				h.Write(data)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}


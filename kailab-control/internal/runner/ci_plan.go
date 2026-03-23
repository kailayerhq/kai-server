package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// actionCIPlan implements kailab/ci-plan — queries the data plane for changed
// files and determines which modules are affected. Sets job outputs that
// downstream jobs can use to skip unaffected targets.
//
// Usage in workflow:
//
//	- uses: kailab/ci-plan
//	  id: plan
//	  with:
//	    modules: |
//	      kailab=kailab/
//	      kailab-control=kailab-control/
//	      kai-core=kai-core/
//	      docs=docs-site/
//
// Downstream jobs can check outputs:
//
//	needs: [plan]
//	if: needs.plan.outputs.kailab == 'true'
func (jp *JobPod) actionCIPlan(ctx context.Context, stepDef *StepDefinition, jobContext map[string]interface{}, logWriter io.Writer) (*ExecutionResult, error) {
	modulesInput := stepDef.With["modules"]

	// Parse module mappings: name=path/prefix
	modules := parseModuleLines(modulesInput)
	if len(modules) == 0 {
		fmt.Fprintf(logWriter, "No modules configured, all targets will run\n")
		return &ExecutionResult{ExitCode: 0}, nil
	}

	// Get the changeset from the data plane
	cloneURL, _ := jobContext["clone_url"].(string)
	if cloneURL == "" {
		fmt.Fprintf(logWriter, "No clone URL available, skipping plan\n")
		return &ExecutionResult{ExitCode: 0}, nil
	}

	// Fetch changeset data
	changedFiles, err := fetchChangedFiles(ctx, cloneURL, logWriter)
	if err != nil {
		fmt.Fprintf(logWriter, "Warning: could not fetch changeset: %v\n", err)
		fmt.Fprintf(logWriter, "All targets will run (no plan available)\n")
		// Set all modules as affected
		result := &ExecutionResult{ExitCode: 0, Outputs: make(map[string]string)}
		for name := range modules {
			result.Outputs[name] = "true"
		}
		return result, nil
	}

	fmt.Fprintf(logWriter, "Changed files: %d\n", len(changedFiles))

	// Determine which modules are affected
	result := &ExecutionResult{ExitCode: 0, Outputs: make(map[string]string)}
	anyAffected := false

	for name, prefix := range modules {
		affected := false
		for _, file := range changedFiles {
			if strings.HasPrefix(file, prefix) {
				affected = true
				break
			}
		}
		result.Outputs[name] = fmt.Sprintf("%t", affected)
		if affected {
			anyAffected = true
			fmt.Fprintf(logWriter, "  %s: affected (prefix %s)\n", name, prefix)
		} else {
			fmt.Fprintf(logWriter, "  %s: not affected\n", name)
		}
	}

	// If nothing is affected (e.g. only config files changed), run everything
	if !anyAffected && len(changedFiles) > 0 {
		fmt.Fprintf(logWriter, "No modules matched changed files — running all targets\n")
		for name := range modules {
			result.Outputs[name] = "true"
		}
	}

	// Also output the changed files list
	filesJSON, _ := json.Marshal(changedFiles)
	result.Outputs["changed_files"] = string(filesJSON)

	return result, nil
}

// fetchChangedFiles queries the data plane for files changed in the latest changeset.
func fetchChangedFiles(ctx context.Context, baseURL string, logWriter io.Writer) ([]string, error) {
	// Try the changeset endpoint
	url := baseURL + "/v1/changeset/latest"

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching changeset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("changeset returned %d", resp.StatusCode)
	}

	var csData struct {
		ChangedFiles []string `json:"changedFiles"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&csData); err != nil {
		return nil, fmt.Errorf("decoding changeset: %w", err)
	}

	return csData.ChangedFiles, nil
}

// parseModuleLines parses "name=path/prefix" lines.
func parseModuleLines(input string) map[string]string {
	modules := make(map[string]string)
	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		modules[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return modules
}

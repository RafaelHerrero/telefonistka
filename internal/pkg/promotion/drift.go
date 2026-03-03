package promotion

import (
	"context"
	"fmt"
	"strings"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	log "github.com/sirupsen/logrus"
)

// CompareRepoDirectories compares two directories on a given ref by
// building flat maps of relative-path→SHA, then fetching content for any differences.
func CompareRepoDirectories(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo, sourcePath, targetPath, ref, repoURL string,
	prLogger *log.Entry,
) (bool, string, error) {
	sourceFiles, err := ListFilesRecursive(ctx, provider, owner, repo, sourcePath, ref)
	if err != nil {
		prLogger.Debugf("Source directory %s not accessible: %v", sourcePath, err)
		return true, fmt.Sprintf("Source directory `%s` not found\n", sourcePath), nil
	}

	targetFiles, err := ListFilesRecursive(ctx, provider, owner, repo, targetPath, ref)
	if err != nil {
		prLogger.Debugf("Target directory %s not accessible: %v", targetPath, err)
		return true, fmt.Sprintf("Target directory `%s` not found\n", targetPath), nil
	}

	sourceSHAs := make(map[string]string)
	for _, f := range sourceFiles {
		rel := strings.TrimPrefix(f.Path, sourcePath)
		rel = strings.TrimPrefix(rel, "/")
		sourceSHAs[rel] = f.SHA
	}

	targetSHAs := make(map[string]string)
	for _, f := range targetFiles {
		rel := strings.TrimPrefix(f.Path, targetPath)
		rel = strings.TrimPrefix(rel, "/")
		targetSHAs[rel] = f.SHA
	}

	if len(sourceSHAs) == len(targetSHAs) {
		allMatch := true
		for k, v := range sourceSHAs {
			if tv, ok := targetSHAs[k]; !ok || tv != v {
				allMatch = false
				break
			}
		}
		if allMatch {
			return false, "", nil
		}
	}

	var hasDiff bool
	var diffOutput strings.Builder
	var filesWithDiff []string
	diffOutput.WriteString("\n```diff\n")

	for filename, sha := range sourceSHAs {
		if targetSHA, found := targetSHAs[filename]; found {
			if sha != targetSHA {
				hasDiff = true
				sourceContent, err := provider.GetFileContent(ctx, owner, repo, sourcePath+"/"+filename, ref)
				if err != nil {
					prLogger.Warnf("Failed to get source content for %s: %v", filename, err)
					diffOutput.WriteString(fmt.Sprintf("--- %s/%s\n+++ %s/%s\n(content differs, could not fetch)\n", sourcePath, filename, targetPath, filename))
					continue
				}
				targetContent, err := provider.GetFileContent(ctx, owner, repo, targetPath+"/"+filename, ref)
				if err != nil {
					prLogger.Warnf("Failed to get target content for %s: %v", filename, err)
					diffOutput.WriteString(fmt.Sprintf("--- %s/%s\n+++ %s/%s\n(content differs, could not fetch)\n", sourcePath, filename, targetPath, filename))
					continue
				}
				edits := myers.ComputeEdits(span.URIFromPath(filename), string(sourceContent), string(targetContent))
				diffOutput.WriteString(fmt.Sprint(gotextdiff.ToUnified(sourcePath+"/"+filename, targetPath+"/"+filename, string(sourceContent), edits)))
				filesWithDiff = append(filesWithDiff, sourcePath+"/"+filename)
			}
		} else {
			hasDiff = true
			diffOutput.WriteString(fmt.Sprintf("--- %s/%s (missing from target dir %s)\n", sourcePath, filename, targetPath))
		}
	}

	for filename := range targetSHAs {
		if _, found := sourceSHAs[filename]; !found {
			hasDiff = true
			diffOutput.WriteString(fmt.Sprintf("+++ %s/%s (missing from source dir %s)\n", targetPath, filename, sourcePath))
		}
	}

	diffOutput.WriteString("\n```\n")

	if len(filesWithDiff) > 0 && repoURL != "" {
		diffOutput.WriteString("\n### Blame Links:\n")
		for _, f := range filesWithDiff {
			blameURL := fmt.Sprintf("%s/-/blame/HEAD/%s", repoURL, f)
			diffOutput.WriteString(fmt.Sprintf("- [%s](%s)\n", f, blameURL))
		}
	}

	return hasDiff, diffOutput.String(), nil
}

// GenerateDriftComment builds the drift warning comment body.
func GenerateDriftComment(diffOutputMap map[string]string) string {
	var body strings.Builder
	body.WriteString("# ⚠️  Found drift between environments ⚠️\n\n")
	body.WriteString("## Intro\n")
	body.WriteString("Drift detection runs on the files in the main branch irrespective of the changes of the MR.\n\n")
	body.WriteString("This could happen in two scenarios:\n")
	body.WriteString("1. A promotion that affects these components is still in progress or was cancelled before completion. ")
	body.WriteString("This means that your automated promotion MR will **include these changes** in addition to your changes!\n\n")
	body.WriteString("2. Someone made a change directly to one of the directories representing promotion targets. ")
	body.WriteString("These changes will be **overridden** by the automated promotion MRs unless changes are made to their respective branches.\n\n")
	body.WriteString("## Diffs\n\n")

	for title, diffOutput := range diffOutputMap {
		body.WriteString(title + "\n\n")
		body.WriteString("<details><summary>Diff (Click to expand)</summary>\n\n")
		body.WriteString(diffOutput)
		body.WriteString("\n</details>\n\n")
	}

	return body.String()
}

package githubapi

import (
	"context"
	"testing"

	"github.com/go-test/deep"
	"github.com/google/go-github/v62/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	log "github.com/sirupsen/logrus"
)

func TestGenerateFlatMapfromFileTree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	filesSHAs := make(map[string]string)

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposContentsByOwnerByRepoByPath,
			[]github.RepositoryContent{
				{
					Type: github.String("file"),
					Path: github.String("some/path/file1"),
					SHA:  github.String("fffff1"),
				},
				{
					Type: github.String("file"),
					Path: github.String("some/path/file2"),
					SHA:  github.String("fffff2"),
				},
				{
					Type: github.String("dir"),
					Path: github.String("some/path/dir1"),
					SHA:  github.String("fffff3"),
				},
			},
			[]github.RepositoryContent{
				{
					Type: github.String("file"),
					Path: github.String("some/path/dir1/file4"),
					SHA:  github.String("fffff4"),
				},
				{
					Type: github.String("dir"),
					Path: github.String("some/path/dir1/nested_dir1/"),
					SHA:  github.String("fffff3"),
				},
			},
			[]github.RepositoryContent{
				{
					Type: github.String("file"),
					Path: github.String("some/path/dir1/nested_dir1/file5"),
					SHA:  github.String("fffff5"),
				},
			},
		),
	)
	ghClientPair := GhClientPair{v3Client: github.NewClient(mockedHTTPClient)}

	ghPrClientDetails := GhPrClientDetails{
		Ctx:          ctx,
		GhClientPair: &ghClientPair,
		Owner:        "AnOwner",
		Repo:         "Arepo",
		PrNumber:     120,
		Ref:          "Abranch",
		PrLogger: log.WithFields(log.Fields{
			"repo":     "AnOwner/Arepo",
			"prNumber": 120,
		}),
	}
	expectedFilesSHAs := map[string]string{
		"file1":                  "fffff1",
		"file2":                  "fffff2",
		"dir1/file4":             "fffff4",
		"dir1/nested_dir1/file5": "fffff5",
	}

	defaultBranch := "main"
	targetPath := "some/path"
	generateFlatMapfromFileTree(&ghPrClientDetails, &targetPath, &targetPath, &defaultBranch, filesSHAs)
	if diff := deep.Equal(expectedFilesSHAs, filesSHAs); diff != nil {
		for _, l := range diff {
			t.Error(l)
		}
	}
}

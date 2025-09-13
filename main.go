package main

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v74/github"
)

func Last24HourChangesOnBranch(rs github.RepositoriesService, branch string, ctx context.Context, owner string, repo string) {

}

func main() {
	client := github.NewClient(nil).WithAuthToken("")
	rs := client.Repositories
	ctx := context.Background()
	user := "fijizxli"
	opt := &github.RepositoryListByUserOptions{Type: "owner", Sort: "updated", Direction: "desc"}
	RepoService := github.RepositoriesService(*rs)

	//GET REPOS
	repos, _, err := RepoService.ListByUser(ctx, user, opt)
	if err != nil {
		fmt.Println(err)
		return
	}

	//LIST REPOS
	for _, repo := range repos {
		fmt.Printf("Repo: %s, ID: %d\n", repo.GetName(), repo.GetID())
	}

	testrepo := repos[0]
	fmt.Printf("testrepo: %s", testrepo.GetName())

	//GET BRANCHES
	var branches []*github.Branch
	branchOpts := &github.BranchListOptions{
		ListOptions: github.ListOptions{PerPage: 2},
	}
	branchOpts.Page = 1
	for {
		b, resp, err := rs.ListBranches(ctx, user, testrepo.GetName(), branchOpts)
		for i := range b {
			fmt.Printf("Branch: %s\n", b[i].GetName())
		}
		if err != nil {
			fmt.Errorf("fetch branches: %w", err)
		}
		branches = append(branches, b...)
		if resp.NextPage == 0 {
			break
		}
		branchOpts.Page = resp.NextPage
	}

	testbranch := branches[0]
	fmt.Printf("testbranch: %s", testbranch.GetName())

	//GET COMMITS
	commitPerPageListOptions := &github.ListOptions{PerPage: 10}
	commitPerPageListOptions.Page = 1
	for {
		since := time.Now().Add(-80 * time.Hour)
		commitOpt := &github.CommitsListOptions{Author: user, ListOptions: *commitPerPageListOptions, SHA: testbranch.GetName(), Since: since}
		commits, resp, err := client.Repositories.ListCommits(ctx, user, testrepo.GetName(), commitOpt)
		fmt.Println("Commits:", len(commits))
		fmt.Println("resp:", resp.Status)

		if len(commits) == 0 {
			fmt.Println("No commits in the last 24 hours")
			break
		}

		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println(commits)
		for _, commit := range commits {
			fmt.Printf("Commit: %s, Author: %s, Date: %s\n", commit.GetSHA(), commit.GetAuthor().GetLogin(), commit.GetCommit().GetAuthor().GetDate())
			fmt.Printf("Message: %s\n", commit.GetCommit().GetMessage())
			fmt.Printf("Files changed: %d\n", commit.GetStats().GetTotal())
			fmt.Println("File names:")
			for _, file := range commit.Files {
				fmt.Printf("- %s\n", file.GetFilename())
			}
			fmt.Printf("Changes: %d additions, %d deletions\n", commit.GetStats().GetAdditions(), commit.GetStats().GetDeletions())
		}
		if resp.NextPage == 0 {
			break
		}
		commitPerPageListOptions.Page = resp.NextPage
	}
}

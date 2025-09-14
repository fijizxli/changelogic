package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/go-github/v74/github"
)

func GetReposByUser(rs github.RepositoriesService, ctx context.Context, user string) []*github.Repository {
	opt := &github.RepositoryListByUserOptions{Type: "owner", Sort: "updated", Direction: "desc"}
	log.Println("Fetching repos for user:", user)
	repos, _, err := rs.ListByUser(ctx, user, opt)
	log.Println("Number of repos found:", len(repos))
	if err != nil {
		log.Fatalln(err)
		return nil
	}
	if len(repos) == 0 {
		log.Println("No repos found for user:", user)
		return nil
	}
	return repos
}

func GetBranchesOnRepo(rs github.RepositoriesService, ctx context.Context, owner string, repo string) []*github.Branch {
	var branches []*github.Branch
	branchOpts := &github.BranchListOptions{
		ListOptions: github.ListOptions{PerPage: 3},
	}
	branchOpts.Page = 1
	for {
		log.Println("Fetching branches for repo:", repo, "page:", branchOpts.Page)
		b, resp, err := rs.ListBranches(ctx, owner, repo, branchOpts)
		if err != nil {
			log.Fatalln("fetch branches: %w", err)
		}
		branches = append(branches, b...)
		if resp.NextPage == 0 {
			break
		}
		branchOpts.Page = resp.NextPage
	}
	log.Println("Number of branches found:", len(branches))
	return branches
}

func GetCommitsOnBranch(rs github.RepositoriesService, branch string, ctx context.Context, owner string, repo string, since time.Time) []*github.RepositoryCommit {
	commitPerPageListOptions := &github.ListOptions{PerPage: 10}
	commitPerPageListOptions.Page = 1
	allCommits := []*github.RepositoryCommit{}
	for {
		commitOpt := &github.CommitsListOptions{Author: owner, ListOptions: *commitPerPageListOptions, SHA: branch, Since: since}
		log.Println("Fetching commits for branch:", branch, "page:", commitPerPageListOptions.Page)
		commits, resp, err := rs.ListCommits(ctx, owner, repo, commitOpt)
		allCommits = append(allCommits, commits...)
		log.Printf("Fetched %d commits, response: %s\n", len(commits), resp.Status)

		if len(commits) == 0 {
			log.Println("No commits in the last 24 hours")
			return nil
		}
		if err != nil {
			log.Fatalln(err)
			return nil
		}

		if resp.NextPage == 0 {
			break
		}

		commitPerPageListOptions.Page = resp.NextPage
	}
	log.Println("Number of commits found:", len(allCommits))
	return allCommits
}

func GetCommitDetails(rs github.RepositoriesService, ctx context.Context, owner string, repo string, sha string, commits []*github.RepositoryCommit) []*github.RepositoryCommit {
	allCommits := []*github.RepositoryCommit{}
	for _, commit := range commits {
		log.Printf("Commit: %s, Author: %s, Date: %s\n", commit.GetSHA(), commit.GetAuthor().GetLogin(), commit.GetCommit().GetAuthor().GetDate())
		log.Printf("Message: %s\n", commit.GetCommit().GetMessage())
		commitDetails, _, err := rs.GetCommit(ctx, owner, repo, *commit.SHA, nil)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		for _, file := range commitDetails.Files {
			log.Printf("Files changed: %d\n", commitDetails.GetStats().GetTotal())
			log.Printf("- %s\n", file.GetFilename())
			log.Printf("  Additions: %d, Deletions: %d, Changes: %d\n", file.GetAdditions(), file.GetDeletions(), file.GetChanges())
			log.Println("Filename:", file.GetFilename())
			log.Println("diff:")
			log.Println(file.GetPatch())
		}
		allCommits = append(allCommits, commitDetails)
	}
	return allCommits
}

func main() {
	client := github.NewClient(nil).WithAuthToken("")
	rs := client.Repositories
	ctx := context.Background()
	user := "fijizxli"
	RepoService := github.RepositoriesService(*rs)
	testrepo := "changelogic"
	testbranch := "main"

	/* using static testrepo and testbranch for easier testing and to reduce API calls
	repos := GetReposByUser(RepoService, ctx, user)
	testrepo := repos[0].GetName()
	branches := GetBranchesOnRepo(RepoService, ctx, user, testrepo)
	testbranch := branches[0].GetName()
	*/
	since := time.Now().Add(-24 * time.Hour)
	commits := GetCommitsOnBranch(RepoService, testbranch, ctx, user, testrepo, since)
	log.Println("Commits in the last 24 hours:", len(commits))

	GetCommitDetails(RepoService, ctx, user, testrepo, testbranch, commits)

	//GET COMMITS
	/*
		commitPerPageListOptions := &github.ListOptions{PerPage: 10}
		commitPerPageListOptions.Page = 1
		for {
			since := time.Now().Add(-24 * time.Hour)
			commitOpt := &github.CommitsListOptions{Author: user, ListOptions: *commitPerPageListOptions, SHA: testbranch, Since: since}
			commits, resp, err := client.Repositories.ListCommits(ctx, user, testrepo, commitOpt)
			//fmt.Println("Commits:", len(commits))
			//fmt.Println("resp:", resp.Status)

			if len(commits) == 0 {
				fmt.Println("No commits in the last 24 hours")
				break
			}

			if err != nil {
				fmt.Println(err)
				return
			}

			for _, commit := range commits {
				fmt.Printf("Commit: %s, Author: %s, Date: %s\n", commit.GetSHA(), commit.GetAuthor().GetLogin(), commit.GetCommit().GetAuthor().GetDate())
				fmt.Printf("Message: %s\n", commit.GetCommit().GetMessage())
				commitDetails, _, err := rs.GetCommit(ctx, user, testrepo, *commit.SHA, nil)
				if err != nil {
					fmt.Println(err)
					return
				}
				for _, file := range commitDetails.Files {
					fmt.Printf("Files changed: %d\n", commitDetails.GetStats().GetTotal())
					fmt.Printf("- %s\n", file.GetFilename())
					fmt.Printf("  Additions: %d, Deletions: %d, Changes: %d\n", file.GetAdditions(), file.GetDeletions(), file.GetChanges())
					fmt.Println("Filename:", file.GetFilename())
					fmt.Println("diff:")
					fmt.Println(file.GetPatch())
				}
			}
			if resp.NextPage == 0 {
				break
			}
			commitPerPageListOptions.Page = resp.NextPage
		}
	*/
}

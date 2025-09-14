package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v74/github"
)

const (
	Red   = "\033[31m"
	Green = "\033[32m"
	Reset = "\033[0m"
)

type OllamaResponse struct {
	Model              string `json:"model"`
	CreatedAt          string `json:"created_at"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	Context            []int  `json:"context"`
	TotalDuration      int64  `json:"total_duration"`
	LoadDuration       int64  `json:"load_duration"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	EvalCount          int    `json:"eval_count"`
	EvalDuration       int64  `json:"eval_duration"`
}

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
		commitDetails, _, err := rs.GetCommit(ctx, owner, repo, *commit.SHA, nil)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		allCommits = append(allCommits, commitDetails)
	}
	return allCommits
}

func CreateDiffs(commits []*github.RepositoryCommit) []string {
	res := make([]string, len(commits))
	for i, commit := range commits {
		log.Printf("%sCommit: %s%s\n", Green, commit.Commit.GetMessage(), Reset)
		res[i] += fmt.Sprintf("Commit: %s\n", commit.Commit.GetMessage())
		log.Printf("%s >> Files changed: %d\n", Green, commit.GetStats().GetTotal())
		res[i] += fmt.Sprintf("%s >> Files changed: %d\n", Green, commit.GetStats().GetTotal())
		log.Printf("Commit: %s, Author: %s, Date: %s\n", commit.GetSHA(), commit.GetAuthor().GetLogin(), commit.GetCommit().GetAuthor().GetDate())
		res[i] += fmt.Sprintf("Commit: %s, Author: %s, Date: %s\n", commit.GetSHA(), commit.GetAuthor().GetLogin(), commit.GetCommit().GetAuthor().GetDate())
		for _, file := range commit.Files {
			log.Printf("- %s diff:", file.GetFilename())
			res[i] += fmt.Sprintf("- %s diff:\n", file.GetFilename())
			log.Printf("%s  Additions: %d, %sDeletions: %d, %s Changes: %d\n", Green, file.GetAdditions(), Red, file.GetDeletions(), Reset, file.GetChanges())
			res[i] += fmt.Sprintf("  Additions: %d, Deletions: %d, Changes: %d\n", file.GetAdditions(), file.GetDeletions(), file.GetChanges())
			log.Print(file.GetPatch())
			res[i] += fmt.Sprintf("%s\n", file.GetPatch())
		}
	}
	return res
}

func RunOllamaSummary(diff string) (string, error) {
	url := "http://localhost:11434/api/generate"

	model := "qwen3:1.7b"
	prompt := fmt.Sprintf("Here is the GitHub diff to summarize: %s Please provide the standup summary based on the instructions above.", diff)
	stream := false
	keep_alive := "5m"

	system := `
	You are a senior software engineer writing daily standup updates for technical teams. Your job is to transform raw GitHub diff output into clear, actionable summaries that focus on business impact rather than technical details. 

	## 🎯 YOUR MISSION
	- Convert code changes into business-impact language (e.g., "reduced API latency" instead of "changed time window")
	- Focus on what changed, why it matters, and who it affects
	- Never mention code syntax (no +, -, @@, diff, file paths, line numbers)
	- Never use passive voice ("was changed" → "we reduced")
	- Keep it concise: 3-5 bullet points max
	- Use valid markdown syntax

	## 🚫 STRICT RULES
	- Do NOT say "in this diff", "the code shows", or "the file changed"
	- Do NOT mention technical terms like "commit", "stats", "GetCommit()", or "API rate limits" unless absolutely necessary
	- Do NOT include code snippets or file paths
	- Do NOT say "this change" or "the change" - be specific about what was changed

	## 💡 HOW TO WRITE IMPACTFUL BULLET POINTS
	For every change:
	1. Start with action verb (e.g., "Reduced", "Added", "Fixed", "Improved")
	2. State the change clearly
	3. Explain the business impact
	4. Mention who it affects

	## 🛠️ FILTERING RULES
	Ignore these in your summary:
	- Test file changes
	- Documentation updates
	- Dependency version bumps
	- Configuration files (e.g., .yaml, .json, .env)
	- Formatting-only changes

	Focus ONLY on:
	- Functional code changes
	- Performance improvements
	- Critical bug fixes
	- Security patches
	- User-facing feature changes

	## ⚠️ IF NO MEANINGFUL CHANGES
	If the diff only contains documentation, tests, or config changes, output:  
	"No significant code changes detected today."`

	// QWEN3 optimized settings
	payload := map[string]interface{}{
		"model":      model,
		"prompt":     prompt,
		"stream":     stream,
		"keep_alive": keep_alive,
		"system":     system,
		"options": map[string]interface{}{
			"max_new_tokens":   300,
			"temperature":      0.6,
			"top_p":            0.95,
			"top_k":            20,
			"min_p":            0.0,
			"repeat_penalty":   1.1,
			"seed":             -1,
			"presence_penalty": 0.5,
		},
	}

	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}

	var response OllamaResponse
	json.Unmarshal(body, &response)
	return response.Response, nil
}

func removeThinkTags(response string) string {
	re := regexp.MustCompile(`(?s)<think>.*?</think>`)
	return re.ReplaceAllString(response, "")
}

func GenerateMarkdownResult(summary string, repo string, branch string, user string) string {
	summary = removeThinkTags(summary)
	var markdownBuilder strings.Builder
	// Add metadata for hugo
	markdownBuilder.WriteString("---\n")
	markdownBuilder.WriteString("title: \"Daily Standup Summary - " + time.Now().Format("2006-01-02T15:04:05Z07:00") + "\"\n")
	markdownBuilder.WriteString("date: " + time.Now().Format("2006-01-02T15:04:05Z07:00") + "\n")
	markdownBuilder.WriteString("draft: false\n")
	tags := fmt.Sprintf("tags: ['daily standup', '%s', '%s']\n", repo, user)
	markdownBuilder.WriteString(tags)
	markdownBuilder.WriteString("---\n\n")
	markdownBuilder.WriteString("# Repository: " + repo + "\n")
	markdownBuilder.WriteString("## Branch: " + branch + "\n")
	markdownBuilder.WriteString("## User: " + user + "\n\n")
	markdownBuilder.WriteString(summary + "\n")

	log.Println("Generated Markdown Summary:\n", markdownBuilder.String())

	fileName := fmt.Sprintf("standup_summary_%s_%s.md", repo, time.Now().Format("20060102150405"))
	if _, err := os.Stat("./contents"); os.IsNotExist(err) {
		err := os.Mkdir("./contents", 0755)
		if err != nil {
			log.Println("Error creating contents directory:", err)
		}
	}
	err := os.WriteFile("./changelogic/content/"+fileName, []byte(markdownBuilder.String()), 0644)
	if err != nil {
		log.Println("Error writing markdown to file:", err)
	} else {
		log.Println("Markdown summary written to file:", fileName)
	}

	return markdownBuilder.String()
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

	commitDetails := GetCommitDetails(RepoService, ctx, user, testrepo, testbranch, commits)
	diffs := CreateDiffs(commitDetails)
	diffString := strings.Join(diffs, "\n")

	summary, err := RunOllamaSummary(diffString)
	if err != nil {
		log.Println("Error generating summary:", err)
		return
	}
	summary = strings.TrimSpace(summary)
	log.Printf("%sSummary:%s %s\n", Green, Reset, summary)

	GenerateMarkdownResult(summary, testrepo, testbranch, user)

	log.Printf("%sDetailed commits fetched: %d", Green, len(commitDetails))
}

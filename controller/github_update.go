package controller

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const (
	gitFetchCacheTTL = 30 * time.Second
	gitCommandTimeout = 30 * time.Second
)

var (
	gitFetchMu       sync.Mutex
	gitFetchCacheAt  time.Time
	gitFetchCacheOut *gitRepoStatus
)

type gitCommitInfo struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

type gitRepoStatus struct {
	IsRepo        bool            `json:"is_repo"`
	Branch        string          `json:"branch"`
	Remote        string          `json:"remote"`
	RemoteURL     string          `json:"remote_url"`
	LocalHead     string          `json:"local_head"`
	RemoteHead    string          `json:"remote_head"`
	CommitsBehind int             `json:"commits_behind"`
	CommitsAhead  int             `json:"commits_ahead"`
	Dirty         bool            `json:"dirty"`
	LatestLocal   *gitCommitInfo  `json:"latest_local,omitempty"`
	LatestRemote  *gitCommitInfo  `json:"latest_remote,omitempty"`
	Incoming      []gitCommitInfo `json:"incoming,omitempty"`
	AppVersion    string          `json:"app_version"`
	FetchedAt     int64           `json:"fetched_at"`
}

func resolveGitWorkDir() (string, error) {
	if env := strings.TrimSpace(os.Getenv("GIT_WORK_DIR")); env != "" {
		if info, err := os.Stat(filepath.Join(env, ".git")); err == nil && info.IsDir() {
			return env, nil
		}
		return "", fmt.Errorf("GIT_WORK_DIR=%s does not contain a .git directory", env)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(filepath.Join(wd, ".git")); err == nil && info.IsDir() {
		return wd, nil
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
			return dir, nil
		}
	}
	return "", errors.New("current working directory is not a git repository")
}

func runGit(dir string, args ...string) (string, error) {
	ctxDone := make(chan struct{})
	defer close(ctxDone)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out strings.Builder
	var errOut strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(errOut.String())
			if msg == "" {
				msg = err.Error()
			}
			return out.String(), errors.New(msg)
		}
		return out.String(), nil
	case <-time.After(gitCommandTimeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", errors.New("git command timed out")
	}
}

func detectRemote(dir string) (string, string, error) {
	remotes, err := runGit(dir, "remote")
	if err != nil {
		return "", "", err
	}
	list := strings.Fields(remotes)
	if len(list) == 0 {
		return "", "", errors.New("no git remote configured")
	}
	pick := list[0]
	for _, name := range list {
		if name == "origin" {
			pick = name
			break
		}
	}
	if pick == list[0] {
		for _, name := range list {
			if strings.Contains(strings.ToLower(name), "new-api") || strings.EqualFold(name, "upstream") {
				pick = name
				break
			}
		}
	}
	urlOut, _ := runGit(dir, "remote", "get-url", pick)
	return pick, sanitizeRemoteURL(strings.TrimSpace(urlOut)), nil
}

func sanitizeRemoteURL(url string) string {
	if url == "" {
		return url
	}
	lower := strings.ToLower(url)
	prefixIdx := -1
	if i := strings.Index(lower, "://"); i >= 0 {
		prefixIdx = i + 3
	} else if strings.HasPrefix(lower, "ssh://") {
		prefixIdx = 6
	}
	if prefixIdx < 0 || prefixIdx >= len(url) {
		return url
	}
	rest := url[prefixIdx:]
	atIdx := strings.Index(rest, "@")
	if atIdx < 0 {
		return url
	}
	slashIdx := strings.Index(rest, "/")
	if slashIdx >= 0 && atIdx > slashIdx {
		return url
	}
	// credentials present before first path segment — redact them
	return url[:prefixIdx] + "***@" + rest[atIdx+1:]
}

func parseLogLine(line string) *gitCommitInfo {
	parts := strings.SplitN(line, "\x1f", 4)
	if len(parts) < 4 {
		return nil
	}
	return &gitCommitInfo{
		SHA:     strings.TrimSpace(parts[0]),
		Author:  strings.TrimSpace(parts[1]),
		Date:    strings.TrimSpace(parts[2]),
		Message: strings.TrimSpace(parts[3]),
	}
}

func collectCommits(dir string, rev string, limit int) ([]gitCommitInfo, error) {
	format := "%H\x1f%an\x1f%aI\x1f%s"
	args := []string{"log", "--pretty=format:" + format, "-n", strconv.Itoa(limit)}
	if rev != "" {
		args = append(args, rev)
	}
	out, err := runGit(dir, args...)
	if err != nil {
		return nil, err
	}
	commits := make([]gitCommitInfo, 0, limit)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		if info := parseLogLine(line); info != nil {
			commits = append(commits, *info)
		}
	}
	return commits, nil
}

func buildRepoStatus(dir string, forceFetch bool) (*gitRepoStatus, error) {
	gitFetchMu.Lock()
	defer gitFetchMu.Unlock()
	if !forceFetch && gitFetchCacheOut != nil && time.Since(gitFetchCacheAt) < gitFetchCacheTTL {
		copied := *gitFetchCacheOut
		return &copied, nil
	}

	branchOut, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, err
	}
	branch := strings.TrimSpace(branchOut)
	remoteName, remoteURL, err := detectRemote(dir)
	if err != nil {
		return nil, err
	}

	if _, err := runGit(dir, "fetch", "--quiet", "--prune", remoteName); err != nil {
		return nil, fmt.Errorf("git fetch failed: %w", err)
	}

	localHead, err := runGit(dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	localHead = strings.TrimSpace(localHead)

	remoteRef := remoteName + "/" + branch
	remoteHeadBytes, err := runGit(dir, "rev-parse", remoteRef)
	if err != nil {
		return nil, fmt.Errorf("remote ref %s not found: %w", remoteRef, err)
	}
	remoteHead := strings.TrimSpace(remoteHeadBytes)

	countOut, err := runGit(dir, "rev-list", "--left-right", "--count", "HEAD..."+remoteRef)
	if err != nil {
		return nil, err
	}
	var ahead, behind int
	if parts := strings.Fields(countOut); len(parts) == 2 {
		ahead, _ = strconv.Atoi(parts[0])
		behind, _ = strconv.Atoi(parts[1])
	}

	statusOut, _ := runGit(dir, "status", "--porcelain")
	dirty := strings.TrimSpace(statusOut) != ""

	pending, _ := collectCommits(dir, "HEAD.."+remoteRef, 20)
	latestLocal, _ := collectCommits(dir, "HEAD", 1)
	latestRemote, _ := collectCommits(dir, remoteRef, 1)

	status := &gitRepoStatus{
		IsRepo:        true,
		Branch:        branch,
		Remote:        remoteName,
		RemoteURL:     remoteURL,
		LocalHead:     localHead,
		RemoteHead:    remoteHead,
		CommitsAhead:  ahead,
		CommitsBehind: behind,
		Dirty:         dirty,
		AppVersion:    common.Version,
		FetchedAt:     time.Now().Unix(),
	}
	if len(latestLocal) > 0 {
		info := latestLocal[0]
		status.LatestLocal = &info
	}
	if len(latestRemote) > 0 {
		info := latestRemote[0]
		status.LatestRemote = &info
	}
	status.Incoming = pending

	gitFetchCacheAt = time.Now()
	gitFetchCacheOut = status
	copied := *status
	return &copied, nil
}

func CheckUpdateStatus(c *gin.Context) {
	dir, err := resolveGitWorkDir()
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
			"data":    gin.H{"is_repo": false},
		})
		return
	}
	if _, err := exec.LookPath("git"); err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": "git executable not found on server; install git to enable update checks",
			"data":    gin.H{"is_repo": false},
		})
		return
	}
	force := c.Query("force") == "1"
	status, err := buildRepoStatus(dir, force)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
			"data":    gin.H{"is_repo": true},
		})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": status})
}

func ApplyUpdate(c *gin.Context) {
	dir, err := resolveGitWorkDir()
	if err != nil {
		c.JSON(200, gin.H{"success": false, "message": err.Error()})
		return
	}
	if _, err := exec.LookPath("git"); err != nil {
		c.JSON(200, gin.H{"success": false, "message": "git executable not found on server"})
		return
	}

	statusOut, _ := runGit(dir, "status", "--porcelain")
	if strings.TrimSpace(statusOut) != "" {
		c.JSON(200, gin.H{
			"success": false,
			"message": "工作区存在未提交的改动，为避免覆盖已本地修改，已终止更新。请先处理 git status 中的内容后再试。",
		})
		return
	}

	branchOut, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		c.JSON(200, gin.H{"success": false, "message": err.Error()})
		return
	}
	branch := strings.TrimSpace(branchOut)
	remoteName, _, err := detectRemote(dir)
	if err != nil {
		c.JSON(200, gin.H{"success": false, "message": err.Error()})
		return
	}

	pullOut, err := runGit(dir, "pull", "--ff-only", remoteName, branch)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": "git pull 失败: " + err.Error(),
			"data":    gin.H{"output": pullOut},
		})
		return
	}

	gitFetchMu.Lock()
	gitFetchCacheAt = time.Time{}
	gitFetchCacheOut = nil
	gitFetchMu.Unlock()

	newHead, _ := runGit(dir, "rev-parse", "HEAD")
	c.JSON(200, gin.H{
		"success": true,
		"message": "代码已更新，需重新构建前端与后端并重启进程后生效。",
		"data": gin.H{
			"output":     strings.TrimSpace(pullOut),
			"new_head":   strings.TrimSpace(newHead),
			"branch":     branch,
			"remote":     remoteName,
			"restart_required": true,
		},
	})
}

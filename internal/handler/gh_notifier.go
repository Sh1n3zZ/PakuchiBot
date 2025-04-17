package handler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"PakuchiBot/internal/bot"

	"github.com/google/go-github/v45/github"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

type GithubNotifier struct {
	client       *github.Client
	bot          *zero.Ctx
	notifyConfig GithubNotifyConfig
	lastCheck    map[string]time.Time
}

type RepoConfig struct {
	Owner       string   `mapstructure:"owner"`
	Name        string   `mapstructure:"name"`
	MonitorType []string `mapstructure:"monitor_type"` // commit, release, issue, pr
}

type NotifyTarget struct {
	Type  string   `mapstructure:"type"` // group, private
	ID    int64    `mapstructure:"id"`
	Repos []string `mapstructure:"repos"` // format: "owner/name", empty means all repositories
}

type GithubNotifyConfig struct {
	Enabled       bool           `mapstructure:"enabled"`
	Interval      int            `mapstructure:"interval"` // check interval (minutes)
	Token         string         `mapstructure:"token"`    // GitHub API Token
	Repositories  []RepoConfig   `mapstructure:"repositories"`
	NotifyTargets []NotifyTarget `mapstructure:"notify_targets"`
}

func RegisterGitHubHandler() {
	if !bot.Config.GitHub.Enabled {
		return
	}

	githubNotifyConfig := GithubNotifyConfig{
		Enabled:       bot.Config.GitHub.Enabled,
		Interval:      bot.Config.GitHub.Interval,
		Token:         bot.Config.GitHub.Token,
		Repositories:  make([]RepoConfig, 0),
		NotifyTargets: make([]NotifyTarget, 0),
	}

	for _, repo := range bot.Config.GitHub.Repositories {
		githubNotifyConfig.Repositories = append(githubNotifyConfig.Repositories, RepoConfig{
			Owner:       repo.Owner,
			Name:        repo.Name,
			MonitorType: repo.MonitorType,
		})
	}

	for _, target := range bot.Config.GitHub.NotifyTargets {
		notifyTarget := NotifyTarget{
			Type:  target.Type,
			ID:    target.ID,
			Repos: make([]string, 0),
		}
		githubNotifyConfig.NotifyTargets = append(githubNotifyConfig.NotifyTargets, notifyTarget)
	}

	githubHandler := NewGithubNotifier(zero.GetBot(bot.Config.Bot.SelfID), githubNotifyConfig)
	githubHandler.Register()
	log.Printf("GitHub notifier registered with %d repositories and %d notify targets",
		len(githubNotifyConfig.Repositories), len(githubNotifyConfig.NotifyTargets))
}

func NewGithubNotifier(bot *zero.Ctx, config GithubNotifyConfig) *GithubNotifier {
	var client *github.Client
	if config.Token != "" {
		ts := github.BasicAuthTransport{
			Username: config.Token,
			Password: "x-oauth-basic",
		}
		client = github.NewClient(ts.Client())
	} else {
		client = github.NewClient(nil)
	}

	return &GithubNotifier{
		client:       client,
		bot:          bot,
		notifyConfig: config,
		lastCheck:    make(map[string]time.Time),
	}
}

func (g *GithubNotifier) Register() {
	zero.OnCommand("github").Handle(func(ctx *zero.Ctx) {
		args := ctx.State["args"].(string)
		argParts := strings.Fields(args)

		if len(argParts) == 0 {
			ctx.Send("请指定操作，例如 /github status")
			return
		}

		switch argParts[0] {
		case "status":
			g.handleStatusCommand(ctx)
		case "list":
			g.handleListCommand(ctx)
		case "subscribe":
			g.handleSubscribeCommand(ctx, argParts[1:])
		case "unsubscribe":
			g.handleUnsubscribeCommand(ctx, argParts[1:])
		default:
			ctx.Send("未知操作，支持的操作：status, list, subscribe, unsubscribe")
		}
	})

	if g.notifyConfig.Enabled && len(g.notifyConfig.Repositories) > 0 {
		go g.startNotifierLoop()
	}
}

func (g *GithubNotifier) handleStatusCommand(ctx *zero.Ctx) {
	if !g.notifyConfig.Enabled {
		ctx.Send("GitHub 通知功能未启用")
		return
	}

	status := fmt.Sprintf("GitHub 通知功能已启用\n监控仓库数：%d\n通知目标数：%d\n检查间隔：%d分钟",
		len(g.notifyConfig.Repositories),
		len(g.notifyConfig.NotifyTargets),
		g.notifyConfig.Interval)
	ctx.Send(status)
}

func (g *GithubNotifier) handleListCommand(ctx *zero.Ctx) {
	if !g.notifyConfig.Enabled || len(g.notifyConfig.Repositories) == 0 {
		ctx.Send("暂无监控的GitHub仓库")
		return
	}

	repoList := "监控的GitHub仓库列表：\n"
	for i, repo := range g.notifyConfig.Repositories {
		repoList += fmt.Sprintf("%d. %s/%s [%s]\n",
			i+1,
			repo.Owner,
			repo.Name,
			strings.Join(repo.MonitorType, ", "))
	}
	ctx.Send(repoList)
}

func (g *GithubNotifier) handleSubscribeCommand(ctx *zero.Ctx, args []string) {
	if !g.notifyConfig.Enabled {
		ctx.Send("GitHub 通知功能未启用")
		return
	}

	if len(args) < 1 {
		ctx.Send("请指定要订阅的仓库，例如: /github subscribe owner/repo")
		return
	}

	repoPath := args[0]
	parts := strings.Split(repoPath, "/")
	if len(parts) != 2 {
		ctx.Send("仓库格式错误，应为 owner/repo")
		return
	}

	found := false
	for _, repo := range g.notifyConfig.Repositories {
		if repo.Owner == parts[0] && repo.Name == parts[1] {
			found = true
			break
		}
	}

	if !found {
		ctx.Send(fmt.Sprintf("仓库 %s 不在监控列表中", repoPath))
		return
	}

	var targetType string
	var targetID int64

	if ctx.Event.GroupID != 0 {
		targetType = "group"
		targetID = ctx.Event.GroupID
	} else {
		targetType = "private"
		targetID = ctx.Event.UserID
	}

	var targetIndex = -1
	for i, target := range g.notifyConfig.NotifyTargets {
		if target.Type == targetType && target.ID == targetID {
			targetIndex = i
			break
		}
	}

	if targetIndex >= 0 {
		for _, repo := range g.notifyConfig.NotifyTargets[targetIndex].Repos {
			if repo == repoPath {
				ctx.Send(fmt.Sprintf("已经订阅了仓库 %s", repoPath))
				return
			}
		}

		g.notifyConfig.NotifyTargets[targetIndex].Repos = append(
			g.notifyConfig.NotifyTargets[targetIndex].Repos,
			repoPath,
		)
	} else {
		g.notifyConfig.NotifyTargets = append(g.notifyConfig.NotifyTargets, NotifyTarget{
			Type:  targetType,
			ID:    targetID,
			Repos: []string{repoPath},
		})
	}

	ctx.Send(fmt.Sprintf("成功订阅仓库 %s", repoPath))
}

func (g *GithubNotifier) handleUnsubscribeCommand(ctx *zero.Ctx, args []string) {
	if !g.notifyConfig.Enabled {
		ctx.Send("GitHub 通知功能未启用")
		return
	}

	if len(args) < 1 {
		ctx.Send("请指定要取消订阅的仓库，例如: /github unsubscribe owner/repo")
		return
	}

	repoPath := args[0]

	var targetType string
	var targetID int64

	if ctx.Event.GroupID != 0 {
		targetType = "group"
		targetID = ctx.Event.GroupID
	} else {
		targetType = "private"
		targetID = ctx.Event.UserID
	}

	var targetIndex = -1
	for i, target := range g.notifyConfig.NotifyTargets {
		if target.Type == targetType && target.ID == targetID {
			targetIndex = i
			break
		}
	}

	if targetIndex < 0 {
		ctx.Send("未订阅任何仓库")
		return
	}

	var repoIndex = -1
	for i, repo := range g.notifyConfig.NotifyTargets[targetIndex].Repos {
		if repo == repoPath {
			repoIndex = i
			break
		}
	}

	if repoIndex < 0 {
		ctx.Send(fmt.Sprintf("未订阅仓库 %s", repoPath))
		return
	}

	repos := g.notifyConfig.NotifyTargets[targetIndex].Repos
	g.notifyConfig.NotifyTargets[targetIndex].Repos = append(
		repos[:repoIndex],
		repos[repoIndex+1:]...,
	)

	if len(g.notifyConfig.NotifyTargets[targetIndex].Repos) == 0 {
		g.notifyConfig.NotifyTargets = append(
			g.notifyConfig.NotifyTargets[:targetIndex],
			g.notifyConfig.NotifyTargets[targetIndex+1:]...,
		)
	}

	ctx.Send(fmt.Sprintf("成功取消订阅仓库 %s", repoPath))
}

func (g *GithubNotifier) startNotifierLoop() {
	interval := time.Duration(g.notifyConfig.Interval) * time.Minute
	if interval < time.Minute {
		interval = 5 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	g.checkAllRepositories()

	for range ticker.C {
		g.checkAllRepositories()
	}
}

func (g *GithubNotifier) checkAllRepositories() {
	for _, repo := range g.notifyConfig.Repositories {
		repoKey := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
		lastCheckTime, exists := g.lastCheck[repoKey]
		if !exists {
			g.lastCheck[repoKey] = time.Now()
			continue
		}

		for _, monitorType := range repo.MonitorType {
			switch strings.ToLower(monitorType) {
			case "commit":
				g.checkCommits(repo.Owner, repo.Name, lastCheckTime)
			case "release":
				g.checkReleases(repo.Owner, repo.Name, lastCheckTime)
			case "issue":
				g.checkIssues(repo.Owner, repo.Name, lastCheckTime)
			case "pr":
				g.checkPullRequests(repo.Owner, repo.Name, lastCheckTime)
			}
		}

		g.lastCheck[repoKey] = time.Now()
	}
}

func (g *GithubNotifier) checkCommits(owner, repo string, since time.Time) {
	ctx := context.Background()
	opts := &github.CommitsListOptions{
		Since: since,
		ListOptions: github.ListOptions{
			PerPage: 10,
		},
	}

	commits, _, err := g.client.Repositories.ListCommits(ctx, owner, repo, opts)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"owner": owner,
			"repo":  repo,
			"error": err,
		}).Error("failed to check commits")
		return
	}

	if len(commits) == 0 {
		return
	}

	for i := len(commits) - 1; i >= 0; i-- {
		commit := commits[i]
		if commit.Commit == nil || commit.Commit.Message == nil {
			continue
		}

		commitTime := commit.Commit.Author.GetDate()
		if commitTime.Before(since) || commitTime.Equal(since) {
			continue
		}

		msg := fmt.Sprintf("🔄 GitHub 提交更新\n仓库：%s/%s\n作者：%s\n提交时间：%s\n\n%s\n\n详情：%s",
			owner,
			repo,
			commit.Commit.Author.GetName(),
			commitTime.Format("2006-01-02 15:04:05"),
			*commit.Commit.Message,
			commit.GetHTMLURL())

		repoPath := fmt.Sprintf("%s/%s", owner, repo)
		g.sendToTargets(msg, repoPath)
	}
}

func (g *GithubNotifier) checkReleases(owner, repo string, since time.Time) {
	ctx := context.Background()
	opts := &github.ListOptions{
		PerPage: 5,
	}

	releases, _, err := g.client.Repositories.ListReleases(ctx, owner, repo, opts)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"owner": owner,
			"repo":  repo,
			"error": err,
		}).Error("failed to check releases")
		return
	}

	for _, release := range releases {
		releaseTime := release.GetCreatedAt().Time
		if releaseTime.Before(since) || releaseTime.Equal(since) {
			continue
		}

		msg := fmt.Sprintf("🚀 GitHub 新版本发布\n仓库：%s/%s\n版本：%s\n发布时间：%s\n\n%s\n\n详情：%s",
			owner,
			repo,
			release.GetName(),
			releaseTime.Format("2006-01-02 15:04:05"),
			release.GetBody(),
			release.GetHTMLURL())

		repoPath := fmt.Sprintf("%s/%s", owner, repo)
		g.sendToTargets(msg, repoPath)
	}
}

func (g *GithubNotifier) checkIssues(owner, repo string, since time.Time) {
	ctx := context.Background()
	opts := &github.IssueListByRepoOptions{
		Since: since,
		State: "all",
		ListOptions: github.ListOptions{
			PerPage: 5,
		},
	}

	issues, _, err := g.client.Issues.ListByRepo(ctx, owner, repo, opts)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"owner": owner,
			"repo":  repo,
			"error": err,
		}).Error("failed to check issues")
		return
	}

	for _, issue := range issues {
		if issue.IsPullRequest() {
			continue
		}

		issueTime := issue.GetCreatedAt()
		updateTime := issue.GetUpdatedAt()

		if (issueTime.Before(since) || issueTime.Equal(since)) &&
			(updateTime.Before(since) || updateTime.Equal(since)) {
			continue
		}

		action := "更新"
		if issueTime.After(since) {
			action = "创建"
		}

		msg := fmt.Sprintf("📝 GitHub Issue %s\n仓库：%s/%s\n标题：%s\n状态：%s\n创建者：%s\n%s时间：%s\n\n详情：%s",
			action,
			owner,
			repo,
			issue.GetTitle(),
			issue.GetState(),
			issue.GetUser().GetLogin(),
			action,
			issueTime.Format("2006-01-02 15:04:05"),
			issue.GetHTMLURL())

		repoPath := fmt.Sprintf("%s/%s", owner, repo)
		g.sendToTargets(msg, repoPath)
	}
}

func (g *GithubNotifier) checkPullRequests(owner, repo string, since time.Time) {
	ctx := context.Background()
	opts := &github.PullRequestListOptions{
		State: "all",
		ListOptions: github.ListOptions{
			PerPage: 5,
		},
	}

	prs, _, err := g.client.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"owner": owner,
			"repo":  repo,
			"error": err,
		}).Error("failed to check pull requests")
		return
	}

	for _, pr := range prs {
		prTime := pr.GetCreatedAt()
		updateTime := pr.GetUpdatedAt()

		if (prTime.Before(since) || prTime.Equal(since)) &&
			(updateTime.Before(since) || updateTime.Equal(since)) {
			continue
		}

		action := "更新"
		if prTime.After(since) {
			action = "创建"
		}

		msg := fmt.Sprintf("🔀 GitHub Pull Request %s\n仓库：%s/%s\n标题：%s\n状态：%s\n创建者：%s\n%s时间：%s\n\n详情：%s",
			action,
			owner,
			repo,
			pr.GetTitle(),
			pr.GetState(),
			pr.GetUser().GetLogin(),
			action,
			prTime.Format("2006-01-02 15:04:05"),
			pr.GetHTMLURL())

		repoPath := fmt.Sprintf("%s/%s", owner, repo)
		g.sendToTargets(msg, repoPath)
	}
}

func (g *GithubNotifier) sendToTargets(messageContent string, repoPath string) {
	for _, target := range g.notifyConfig.NotifyTargets {
		shouldSend := len(target.Repos) == 0

		if !shouldSend {
			for _, subscribedRepo := range target.Repos {
				if subscribedRepo == repoPath {
					shouldSend = true
					break
				}
			}
		}

		if !shouldSend {
			continue
		}

		switch strings.ToLower(target.Type) {
		case "group":
			g.bot.SendGroupMessage(target.ID, message.Text(messageContent))
		case "private":
			g.bot.SendPrivateMessage(target.ID, message.Text(messageContent))
		default:
			logrus.WithFields(logrus.Fields{
				"target_type": target.Type,
				"target_id":   target.ID,
			}).Warn("unknown notify target type")
		}
	}
}

// Reserved methods for backward compatibility
func (g *GithubNotifier) sendToAllTargets(messageContent string) {
	for _, target := range g.notifyConfig.NotifyTargets {
		switch strings.ToLower(target.Type) {
		case "group":
			g.bot.SendGroupMessage(target.ID, message.Text(messageContent))
		case "private":
			g.bot.SendPrivateMessage(target.ID, message.Text(messageContent))
		default:
			logrus.WithFields(logrus.Fields{
				"target_type": target.Type,
				"target_id":   target.ID,
			}).Warn("unknown notify target type")
		}
	}
}

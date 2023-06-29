package main

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/andygrunwald/go-jira"
	"github.com/charmbracelet/log"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

func main() {
	log.SetPrefix("Jira 🍪 ")
	log.SetTimeFormat(time.Kitchen)
	log.Helper()
	// 初始化 config
	if err := GetConfig(); err != nil {
		log.Fatal(err)
	}
	token := viper.GetString("jira_token")
	jiraURL := viper.GetString("jira_host")
	jiraClient := MustGetJiraClient(token, jiraURL)
	// 初始化 cli
	app := &cli.App{
		Name:      "Jira Resolver",
		Usage:     "帮助你应对 kevin 的每日 Jira Ticket Resolve 任务",
		ArgsUsage: "Kevin 的告警转义换行符后输入",
		Action: func(cCtx *cli.Context) error {
			content := cCtx.Args().Get(0)
			var wg errgroup.Group
			log.Info("开始获取 issues")
			lo.ForEach(
				lo.Filter(strings.Split(content, "\\n"),
					func(item string, index int) bool {
						return strings.HasPrefix(item, jiraURL)
					},
				), func(item string, _ int) {
					wg.Go(func() error {
						handler := IssueHandler{
							JiraClient: jiraClient,
							Ctx:        cCtx.Context,
							Link:       item,
						}
						issue, err := handler.GetIssue()
						if err != nil {
							return err
						}
						err = handler.Resolve(issue)
						if err != nil {
							return err
						}
						return nil
					})
				})
			if err := wg.Wait(); err != nil {
				return err
			}
			log.Info("issues 处理完成")
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Error(err)
	}
}

const defaultConfig = `
jira_host = "" # 例如 https://jira.shanbay.com
jira_token = "" # 例如 1234567890
`

func GetConfig() error {
	viper.SetConfigName(".jira_master")
	viper.AddConfigPath("$HOME")
	cfgPath, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	viper.AddConfigPath(cfgPath)
	if err = viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Error("未找到配置文件，将创建默认配置文件")
			f, err := os.Create(cfgPath + "/.jira_master.toml")
			if err != nil {
				return err
			}
			_, err = f.WriteString(defaultConfig)
			log.Info("默认配置文件创建成功")
			log.Info("请修改配置文件后重新运行, 配置位于 " + cfgPath + "/.jira_master.toml")
			if err != nil {
				return err
			}
			os.Exit(0)
		}
	}
	return nil
}

func MustGetJiraClient(token, url string) *jira.Client {
	tp := jira.BearerAuthTransport{
		Token: token,
	}
	jiraClient, err := jira.NewClient(tp.Client(), url)
	if err != nil {
		log.Fatal(err)
	}
	return jiraClient
}

func handlerIssueIDByLink(link string) string {
	arr := strings.Split(link, "/")
	return arr[len(arr)-1]
}

type IssueHandler struct {
	JiraClient *jira.Client
	Ctx        context.Context
	Link       string
}

func (h *IssueHandler) GetIssue() (*jira.Issue, error) {
	issueID := handlerIssueIDByLink(h.Link)
	issue, _, err := h.JiraClient.Issue.Get(issueID, nil)
	if err != nil {
		return nil, err
	}
	log.Info("Issue", issue.Key, issue.Fields.Summary)
	return issue, nil
}

func (h *IssueHandler) Resolve(issue *jira.Issue) error {
	ts, _, err := h.JiraClient.Issue.GetTransitionsWithContext(h.Ctx, issue.ID)
	if err != nil {
		return err
	}
	for _, t := range ts {
		if t.Name == "Resolve" {
			_, err = h.JiraClient.Issue.DoTransition(issue.ID, t.ID)
			if err != nil {
				return err
			}
			log.Info("解决", issue.Key, issue.Fields.Summary)
		}
	}
	return nil
}

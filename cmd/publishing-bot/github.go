/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

func githubClient(token string) *github.Client {
	// create github client
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

func ReportOnIssue(e error, logs, token, org, repo string, issue int) error {
	ctx := context.Background()
	client := githubClient(token)

	// filter out token, if it happens to be in the log (it shouldn't!)
	logs = strings.Replace(logs, token, "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX", -1)

	// who am I?
	myself, resp, err := client.Users.Get(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to get own user: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get own user: HTTP code %d", resp.StatusCode)
	}

	// create new newComment
	body := fmt.Sprintf("The last publishing run failed: %v\n\n```%s```\n", e, tail(logs, 65000))
	newComment, resp, err := client.Issues.CreateComment(ctx, org, repo, issue, &github.IssueComment{
		Body: &body,
	})
	if err != nil {
		return fmt.Errorf("failed to comment on issue #%d: %v", issue, err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("failed to comment on issue #%d: HTTP code %d", issue, resp.StatusCode)
	}

	// re-open issue in case it was closed
	_, resp, err = client.Issues.Edit(ctx, org, repo, issue, &github.IssueRequest{
		State: github.String("open"),
	})
	if err != nil {
		return fmt.Errorf("failed to re-open issue #%d: %v", issue, err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("failed to re-open issue #%d: HTTP code %d", issue, resp.StatusCode)
	}

	// delete all other comments from this user
	comments, resp, err := client.Issues.ListComments(ctx, org, repo, issue, &github.IssueListCommentsOptions{})
	if err != nil {
		return fmt.Errorf("failed to get github comments of issue #%d: %v", issue, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get github comments of issue #%d: HTTP code %d", issue, resp.StatusCode)
	}
	for _, c := range comments {
		if *c.User.ID == *myself.ID && *c.ID != *newComment.ID {
			resp, err = client.Issues.DeleteComment(ctx, org, repo, *c.ID)
		}
	}

	return nil
}

func CloseIssue(token, org, repo string, issue int) error {
	ctx := context.Background()
	client := githubClient(token)

	_, resp, err := client.Issues.Edit(ctx, org, repo, issue, &github.IssueRequest{
		State: github.String("closed"),
	})
	if err != nil {
		return fmt.Errorf("failed to close issue #%d: %v", issue, err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("failed to close issue #%d: HTTP code %d", issue, resp.StatusCode)
	}

	return nil
}

// tail returns lines with a maximum of the given bytes in length. Only complete lines are
// returned, with the exception if the is only one line, then "..." are put in front.
// maxBytes must be at least 3.
func tail(msg string, maxBytes int) string {
	msg = strings.Trim(msg, "\n")
	if len(msg) <= maxBytes {
		return msg
	}
	lines := strings.Split(msg, "\n")
	if len(lines) == 1 {
		if len(lines[0]) <= maxBytes {
			return lines[0]
		}
		return "..." + lines[0][max(0, len(lines[0])-maxBytes+3):]
	}

	prefix := "..."
	ret := []string{}
	n := len(prefix)
	for i := len(lines) - 1; i >= 0; i-- {
		if n+len("\n")+len(lines[i]) > maxBytes {
			if len(ret) == 0 {
				return "..." + lines[0][max(0, len(lines[0])-maxBytes+3):]
			}
			break
		}
		ret = append(ret, lines[i])
		n += len(lines[i]) + len("\n")
	}

	return strings.Join(reverse(append(ret, prefix)), "\n")
}

func reverse(s []string) []string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

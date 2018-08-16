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
	body := transfromLogToGithubFormat(logs, 50, fmt.Sprintf("/reopen\n\nThe last publishing run failed: %v", e))

	newComment, resp, err := client.Issues.CreateComment(ctx, org, repo, issue, &github.IssueComment{
		Body: &body,
	})
	if err != nil {
		return fmt.Errorf("failed to comment on issue #%d: %v", issue, err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("failed to comment on issue #%d: HTTP code %d", issue, resp.StatusCode)
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
			if err != nil {
				return fmt.Errorf("failed to delete github comment %d of issue #%d: %v", *c.ID, issue, err)
			}
			if resp.StatusCode >= 300 {
				return fmt.Errorf("failed to delete github comment %d of issue #%d: HTTP code %d", *c.ID, issue, resp.StatusCode)
			}
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

func transfromLogToGithubFormat(original string, maxLines int, headings ...string) string {
	logCount := 0
	transformed := NewLogBuilderWithMaxBytes(65000, original).
		AddHeading(headings...).
		AddHeading("```").
		Trim("\n").
		Split("\n").
		Reverse().
		Filter(func(line string) bool {
			if logCount < maxLines {
				if strings.HasPrefix(line, "+") {
					logCount++
				}
				return true
			}
			return false
		}).
		Reverse().
		Join("\n").
		AddTailing("```").
		Log()
	return transformed
}

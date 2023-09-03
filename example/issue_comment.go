// Copyright 2018 Palantir Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-github/v53/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
)

type PRCommentHandler struct {
	githubapp.ClientCreator

	preamble string
}

func (h *PRCommentHandler) Handles() []string {
	return []string{"issue_comment"}
}

func (h *PRCommentHandler) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	var event github.IssueCommentEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse issue comment event payload")
	}

	// Uncomment this if you want to listen only to PR issues
	// if !event.GetIssue().IsPullRequest() {
	// 	zerolog.Ctx(ctx).Debug().Msg("Issue comment event is not for a pull request")
	// 	return nil
	// }

	repo := event.GetRepo()
	prNum := event.GetIssue().GetNumber()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	ctx, logger := githubapp.PreparePRContext(ctx, installationID, repo, event.GetIssue().GetNumber())

	logger.Debug().Msgf("Event action is %s", event.GetAction())
	if event.GetAction() != "created" {
		return nil
	}

	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	repoOwner := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	author := event.GetComment().GetUser().GetLogin()
	body := event.GetComment().GetBody()

	if strings.HasSuffix(author, "[bot]") {
		logger.Debug().Msg("Issue comment was created by a bot")
		return nil
	}

	// Find command beginning with a slash and followed by a word, may contain dashes (-) and multiple words
	// Only matches the first slash command
	pattern := regexp.MustCompile(`^\/\w+(?:-\w+)*`)
	slash_command := "None"
	if match := pattern.FindString(body); match != "" {
		slash_command = match
		fmt.Println("Slash command found:", match)
	}

	logger.Debug().Msgf("Echoing comment on %s/%s#%d by %s", repoOwner, repoName, prNum, author)
	msg := fmt.Sprintf("%s\n%s said\n```\n%s\n```\nFound the slash command: `%s`\n", h.preamble, author, body, slash_command)

	// Answer with an issue comment
	prComment := github.IssueComment{
		Body: &msg,
	}

	if _, _, err := client.Issues.CreateComment(ctx, repoOwner, repoName, prNum, &prComment); err != nil {
		logger.Error().Err(err).Msg("Failed to comment on pull request")
	}

	if slash_command == "/create-branch" {
		// Get current reference of main branch
		// TODO: Get the default branch using the API

		// Get the reference to the latest commit of the main branch
		// currentReference := "f704106dedc914b1eb0c3ee1a5e4a7f8003e1d97"
		// currentURL := "https://api.github.com/repos/cloud-architecting/github-app-test/git/commits/f704106dedc914b1eb0c3ee1a5e4a7f8003e1d97"
		currentRef, _, err := client.Git.GetRef(ctx, repoOwner, repoName, "heads/main")
		if err != nil {
			logger.Error().Err(err).Msg("Failed to get current reference")
			return nil
		}
		logMsg := fmt.Sprintf("Current ref is: %s", currentRef)
		logger.Debug().Msg(logMsg)

		// Create new branch
		gitObjType := "commit"
		newGitObj := github.GitObject{
			Type: &gitObjType,
			SHA:  currentRef.Object.SHA,
		}

		newRefName := "refs/heads/myNewBranch"
		dummyURL := ""
		dummyNodeID := ""
		newRef := github.Reference{
			Ref:    &newRefName,
			URL:    &dummyURL,
			Object: &newGitObj,
			NodeID: &dummyNodeID,
		}

		newBranchRef, _, err := client.Git.CreateRef(ctx, repoOwner, repoName, &newRef)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to create new branch")
			return nil
		}
		logMsg = fmt.Sprintf("New branch ref is: %s", newBranchRef)
		logger.Debug().Msg(logMsg)
	}

	if slash_command == "/testpr" {
		title := "First PR"
		head := "pr-branch"
		base := "main"
		prBody := "This is a PR created by a bot"
		newPRContent := github.NewPullRequest{
			Title: &title,
			Head:  &head,
			Base:  &base,
			Body:  &prBody,
		}

		newPR, _, err := client.PullRequests.Create(ctx, repoOwner, repoName, &newPRContent)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to create pull request")
		}
		logMsg := fmt.Sprintf("Created pull request with ID %v and title %s\n", *newPR.Number, *newPR.Title)
		logger.Debug().Msg(logMsg)
	}

	return nil
}

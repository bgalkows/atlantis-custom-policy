// Copyright 2017 HootSuite Media Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// Modified hereafter by contributors to runatlantis/atlantis.

package webhooks

import (
	"fmt"
	"regexp"

	"errors"

	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/logging"
)

const SlackKind = "slack"
const HttpKind = "http"
const ApplyEvent = "apply"

//go:generate pegomock generate --package mocks -o mocks/mock_sender.go Sender

// Sender sends webhooks.
type Sender interface {
	// Send sends the webhook (if the implementation thinks it should).
	Send(log logging.SimpleLogging, applyResult ApplyResult) error
}

// ApplyResult is the result of a terraform apply.
type ApplyResult struct {
	Workspace   string
	Repo        models.Repo
	Pull        models.PullRequest
	User        models.User
	Success     bool
	Directory   string
	ProjectName string
}

// MultiWebhookSender sends multiple webhooks for each one it's configured for.
type MultiWebhookSender struct {
	Webhooks []Sender
}

type Config struct {
	Event          string
	WorkspaceRegex string
	BranchRegex    string
	Kind           string
	Channel        string
	URL            string
}

type Clients struct {
	Slack SlackClient
	Http  *HttpClient
}

func NewMultiWebhookSender(configs []Config, clients Clients) (*MultiWebhookSender, error) {
	var webhooks []Sender
	for _, c := range configs {
		wr, err := regexp.Compile(c.WorkspaceRegex)
		if err != nil {
			return nil, err
		}
		br, err := regexp.Compile(c.BranchRegex)
		if err != nil {
			return nil, err
		}
		if c.Kind == "" || c.Event == "" {
			return nil, errors.New("must specify \"kind\" and \"event\" keys for webhooks")
		}
		if c.Event != ApplyEvent {
			return nil, fmt.Errorf("\"event: %s\" not supported. Only \"event: %s\" is supported right now", c.Event, ApplyEvent)
		}
		switch c.Kind {
		case SlackKind:
			if !clients.Slack.TokenIsSet() {
				return nil, errors.New("must specify top-level \"slack-token\" if using a webhook of \"kind: slack\"")
			}
			if c.Channel == "" {
				return nil, errors.New("must specify \"channel\" if using a webhook of \"kind: slack\"")
			}
			slack, err := NewSlack(wr, br, c.Channel, clients.Slack)
			if err != nil {
				return nil, err
			}
			webhooks = append(webhooks, slack)
		case HttpKind:
			if c.URL == "" {
				return nil, errors.New("must specify \"url\" if using a webhook of \"kind: http\"")
			}
			httpWebhook := &HttpWebhook{
				Client:         clients.Http,
				WorkspaceRegex: wr,
				BranchRegex:    br,
				URL:            c.URL,
			}
			webhooks = append(webhooks, httpWebhook)
		default:
			return nil, fmt.Errorf("\"kind: %s\" not supported. Only \"kind: %s\" and \"kind: %s\" are supported right now", c.Kind, SlackKind, HttpKind)
		}
	}

	return &MultiWebhookSender{
		Webhooks: webhooks,
	}, nil
}

// Send sends the webhook using its Webhooks.
func (w *MultiWebhookSender) Send(log logging.SimpleLogging, result ApplyResult) error {
	for _, w := range w.Webhooks {
		if err := w.Send(log, result); err != nil {
			log.Warn("error sending webhook: %s", err)
		}
	}
	return nil
}

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
	"z/hooks"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yansal/sql/build"
)

func main() {
	log.SetFlags(0)
	if err := main1(); err != nil {
		log.Fatal(err)
	}
}

func main1() error {
	var (
		c = client{
			token:      os.Getenv("TOKEN"),
			httpclient: &http.Client{Transport: hooks.Wrap(http.DefaultTransport)},
		}
		ctx      = context.Background()
		username = os.Getenv("USER")
		cursor   string
	)
	if len(os.Args) >= 2 {
		username = os.Args[1]
	}

	db, err := sql.Open("sqlite3", "db.sqlite")
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	for {
		resource, err := c.listUserComments(ctx, username, cursor)
		if err != nil {
			return err
		}
		for i := range resource.Data.User.IssueComments.Nodes {
			if err := insertComment(tx, username, &resource.Data.User.IssueComments.Nodes[i]); err != nil {
				return err
			}
		}
		if !resource.Data.User.IssueComments.PageInfo.HasNextPage {
			break
		}
		cursor = resource.Data.User.IssueComments.PageInfo.EndCursor
	}

	return tx.Commit()
}

type client struct {
	token      string
	httpclient *http.Client
}

func (c *client) listUserComments(ctx context.Context, username string, cursor string) (*listUserCommentsResource, error) {
	method := http.MethodPost
	const url = `https://api.github.com/graphql`
	body := struct {
		OperationName string                 `json:"operationName,omitempty"`
		Query         string                 `json:"query"`
		Variables     map[string]interface{} `json:"variables,omitempty"`
	}{
		OperationName: "listUserComments",
		Query:         listUserCommentsQuery,
		Variables:     map[string]interface{}{"user": username},
	}
	if cursor != "" {
		body.Variables["cursor"] = cursor
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("expected %d, got %d (body %q)", http.StatusOK, resp.StatusCode, b)
	}
	dec := json.NewDecoder(resp.Body)
	dec.DisallowUnknownFields()
	var dest listUserCommentsResource
	if err := dec.Decode(&dest); err != nil {
		return nil, err
	}
	return &dest, nil
}

type listUserCommentsResource struct {
	Data struct {
		User struct {
			IssueComments struct {
				PageInfo *struct {
					EndCursor   string `json:"endCursor"`
					HasNextPage bool   `json:"hasNextPage"`
				} `json:"pageInfo"`
				Nodes []userComment `json:"nodes"`
			} `json:"issueComments"`
		} `json:"user"`
	} `json:"data"`
}

type userComment struct {
	URL            string    `json:"url"`
	CreatedAt      time.Time `json:"createdAt"`
	Body           string    `json:"body"`
	ReactionGroups []struct {
		Content string `json:"content"`
		Users   struct {
			TotalCount int `json:"totalCount"`
		} `json:"users"`
	} `json:"reactionGroups"`
}

func (c *userComment) reactionCount(content string) int {
	for i := range c.ReactionGroups {
		if c.ReactionGroups[i].Content == content {
			return c.ReactionGroups[i].Users.TotalCount
		}
	}
	panic(fmt.Sprintf("unknown content %s", content))
}

const listUserCommentsQuery = `query listUserComments($user: String!, $cursor: String) {
	user(login: $user) {
	  issueComments(first: 100, after: $cursor) {
		pageInfo {
			endCursor
			hasNextPage
		}
		nodes {
		  url
		  createdAt
		  body
		  reactionGroups {
			content
			users {
			  totalCount
			}
		  }
		}
	  }
	}
  }`

func insertComment(tx *sql.Tx, username string, comment *userComment) error {
	query, args := build.InsertInto("comments",
		"user",
		"body",
		"created_at",
		"url",
		"count_thumbs_up",
		"count_thumbs_down",
		"count_laugh",
		"count_hooray",
		"count_confused",
		"count_heart",
		"count_rocket",
		"count_eyes",
	).Values(
		build.Bind(username),
		build.Bind(comment.Body),
		build.Bind(comment.CreatedAt),
		build.Bind(comment.URL),
		build.Bind(comment.reactionCount("THUMBS_UP")),
		build.Bind(comment.reactionCount("THUMBS_DOWN")),
		build.Bind(comment.reactionCount("LAUGH")),
		build.Bind(comment.reactionCount("HOORAY")),
		build.Bind(comment.reactionCount("CONFUSED")),
		build.Bind(comment.reactionCount("HEART")),
		build.Bind(comment.reactionCount("ROCKET")),
		build.Bind(comment.reactionCount("EYES")),
	).Build()
	if _, err := tx.Exec(query, args...); err != nil {
		return err
	}
	return nil
}

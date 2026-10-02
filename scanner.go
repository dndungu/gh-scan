package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/go-github/v62/github"
)

type Scanner struct {
	client *github.Client
	token  string
}

func NewScanner(client *github.Client, token string) *Scanner {
	return &Scanner{client: client, token: token}
}

// DiscoverOrgs lists all organizations the authenticated user belongs to.
func (s *Scanner) DiscoverOrgs(ctx context.Context) ([]string, error) {
	opts := &github.ListOptions{PerPage: 100}
	var orgs []string
	for {
		page, resp, err := s.client.Organizations.List(ctx, "", opts)
		if err != nil {
			return nil, fmt.Errorf("listing orgs: %w", err)
		}
		for _, o := range page {
			if o.Login != nil {
				orgs = append(orgs, *o.Login)
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return orgs, nil
}

// listAllRepos fetches all repos for a given owner (org or user).
func (s *Scanner) listAllRepos(ctx context.Context, owner string, isUser bool) ([]*github.Repository, error) {
	var all []*github.Repository
	page := 1
	for {
		var repos []*github.Repository
		var resp *github.Response
		var err error

		if isUser {
			repos, resp, err = s.client.Repositories.List(ctx, owner, &github.RepositoryListOptions{
				ListOptions: github.ListOptions{PerPage: 100, Page: page},
			})
		} else {
			repos, resp, err = s.client.Repositories.ListByOrg(ctx, owner, &github.RepositoryListByOrgOptions{
				ListOptions: github.ListOptions{PerPage: 100, Page: page},
			})
		}
		if err != nil {
			return nil, err
		}
		all = append(all, repos...)
		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}
	return all, nil
}

// FetchReadme downloads README.md (any README* variant) rendered as raw text.
func (s *Scanner) FetchReadme(ctx context.Context, owner, repo string) (string, error) {
	rm, _, err := s.client.Repositories.GetReadme(ctx, owner, repo, nil)
	if err != nil {
		return "", err
	}
	content, err := rm.GetContent()
	if err != nil {
		return "", err
	}
	return content, nil
}

// RepoDoc is one repo's collected data.
type RepoDoc struct {
	Owner       string
	Name        string
	Description string
	URL         string
	Readme      string
	Err         string
}

// Collect scans all orgs + personal account concurrently.
func (s *Scanner) Collect(ctx context.Context, orgFilter string, includePersonal bool) ([]RepoDoc, error) {
	orgs := []string{}
	if orgFilter != "" {
		orgs = strings.Split(orgFilter, ",")
		for i := range orgs {
			orgs[i] = strings.TrimSpace(orgs[i])
		}
	} else {
		discovered, err := s.DiscoverOrgs(ctx)
		if err != nil {
			return nil, err
		}
		orgs = discovered
	}

	type task struct {
		owner  string
		isUser bool
	}
	var tasks []task
	for _, o := range orgs {
		tasks = append(tasks, task{o, false})
	}
	if includePersonal {
		me, _, err := s.client.Users.Get(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("getting authenticated user: %w", err)
		}
		tasks = append(tasks, task{me.GetLogin(), true})
	}

	var mu sync.Mutex
	var docs []RepoDoc
	sem := make(chan struct{}, 8) // concurrency limit
	var wg sync.WaitGroup

	for _, t := range tasks {
		repos, err := s.listAllRepos(ctx, t.owner, t.isUser)
		if err != nil {
			return nil, fmt.Errorf("listing repos for %s: %w", t.owner, err)
		}
		for _, r := range repos {
			// Keep the inventory focused on repositories created under this
			// account or organization; exclude forks regardless of source owner.
			if r.GetFork() {
				continue
			}
			wg.Add(1)
			go func(owner, name string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				doc := RepoDoc{
					Owner:       owner,
					Name:        name,
					Description: r.GetDescription(),
					URL:         r.GetHTMLURL(),
				}
				readme, err := s.FetchReadme(ctx, owner, name)
				if err != nil {
					doc.Err = "no README or fetch failed"
				} else {
					doc.Readme = readme
				}
				mu.Lock()
				docs = append(docs, doc)
				mu.Unlock()
			}(t.owner, r.GetName())
		}
	}
	wg.Wait()

	// Sort by owner, then name
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Owner != docs[j].Owner {
			return docs[i].Owner < docs[j].Owner
		}
		return docs[i].Name < docs[j].Name
	})
	return docs, nil
}

// RenderMarkdown produces the final single-file markdown.
func RenderMarkdown(docs []RepoDoc) string {
	var b strings.Builder
	b.WriteString("# GitHub Repository Inventory\n\n")
	b.WriteString(fmt.Sprintf("_%d repositories total_\n\n---\n\n", len(docs)))

	currentOwner := ""
	for _, d := range docs {
		if d.Owner != currentOwner {
			currentOwner = d.Owner
			fmt.Fprintf(&b, "## Organization / Account: `%s`\n\n", currentOwner)
		}
		fmt.Fprintf(&b, "### 📦 %s\n\n", d.Name)
		if d.URL != "" {
			fmt.Fprintf(&b, "- **URL:** <%s>\n", d.URL)
		}
		if d.Description != "" {
			fmt.Fprintf(&b, "- **Description:** %s\n", d.Description)
		}
		b.WriteString("\n---\n\n")
	}
	return b.String()
}

// WriteReadmes saves fetched READMEs under outputs/owner/repo/README.md.
func WriteReadmes(docs []RepoDoc, root string) error {
	for _, d := range docs {
		if d.Err != "" {
			continue
		}
		if !safePathComponent(d.Owner) || !safePathComponent(d.Name) {
			return fmt.Errorf("invalid repository path %q/%q", d.Owner, d.Name)
		}
		path := filepath.Join(root, d.Owner, d.Name, "README.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s/%s README: %w", d.Owner, d.Name, err)
		}
		if err := os.WriteFile(path, []byte(d.Readme), 0o644); err != nil {
			return fmt.Errorf("writing %s/%s README: %w", d.Owner, d.Name, err)
		}
	}
	return nil
}

func safePathComponent(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\`)
}

func RunCLI(ctx context.Context, s *Scanner, outPath, orgFilter string, includePersonal bool) error {
	docs, err := s.Collect(ctx, orgFilter, includePersonal)
	if err != nil {
		return err
	}
	if err := WriteReadmes(docs, "outputs"); err != nil {
		return err
	}
	return os.WriteFile(outPath, []byte(RenderMarkdown(docs)), 0o644)
}

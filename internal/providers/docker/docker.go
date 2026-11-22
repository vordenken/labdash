// Package docker is a minimal Docker Engine API client over a unix socket
// built on the standard library only.
package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// Container is the subset of container state the dashboard uses.
type Container struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	State   string            `json:"state"`  // running, exited, paused, restarting, created, dead
	Status  string            `json:"status"` // human readable, e.g. "Up 3 hours (healthy)"
	Health  string            `json:"health"` // healthy, unhealthy, starting or ""
	Project string            `json:"project"`
	Sablier bool              `json:"sablier"`
	Created int64             `json:"created"`
	Labels  map[string]string `json:"-"`
}

// Snapshot is an immutable view of all containers keyed by name.
type Snapshot struct {
	Taken      time.Time             `json:"taken"`
	Containers map[string]*Container `json:"containers"`
	Err        string                `json:"error,omitempty"`
}

// Client talks to the Docker Engine API.
type Client struct {
	http *http.Client
	base string
}

func NewClient(socketPath string) *Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
		MaxIdleConns:    4,
		IdleConnTimeout: 90 * time.Second,
	}
	return &Client{http: &http.Client{Transport: tr}, base: "http://docker"}
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values) (*http.Response, error) {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Message == "" {
			e.Message = strings.TrimSpace(string(body))
		}
		return nil, &APIError{Status: resp.StatusCode, Message: e.Message}
	}
	return resp, nil
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("docker: %d %s", e.Status, e.Message) }

type apiContainer struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Created int64             `json:"Created"`
	Labels  map[string]string `json:"Labels"`
}

// List returns all containers (including stopped ones).
func (c *Client) List(ctx context.Context) ([]*Container, error) {
	q := url.Values{"all": {"1"}}
	resp, err := c.do(ctx, http.MethodGet, "/containers/json", q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var raw []apiContainer
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]*Container, 0, len(raw))
	for _, r := range raw {
		name := ""
		if len(r.Names) > 0 {
			name = strings.TrimPrefix(r.Names[0], "/")
		}
		out = append(out, &Container{
			ID:      r.ID,
			Name:    name,
			Image:   r.Image,
			State:   r.State,
			Status:  r.Status,
			Health:  parseHealth(r.Status),
			Project: r.Labels["com.docker.compose.project"],
			Sablier: r.Labels["sablier.enable"] == "true",
			Created: r.Created,
			Labels:  r.Labels,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func parseHealth(status string) string {
	switch {
	case strings.Contains(status, "(healthy)"):
		return "healthy"
	case strings.Contains(status, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(status, "(health: starting)"):
		return "starting"
	}
	return ""
}

// Action performs start, stop or restart on a container.
func (c *Client) Action(ctx context.Context, id, action string) error {
	switch action {
	case "start", "stop", "restart":
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
	if !validID(id) {
		return errors.New("invalid container id")
	}
	q := url.Values{}
	if action != "start" {
		q.Set("t", "10")
	}
	resp, err := c.do(ctx, http.MethodPost, "/containers/"+id+"/"+action, q)
	if err != nil {
		var ae *APIError
		// 304: already in the requested state.
		if errors.As(err, &ae) && ae.Status == http.StatusNotModified {
			return nil
		}
		return err
	}
	resp.Body.Close()
	return nil
}

func validID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}

// Events streams container lifecycle events and calls fn for each one
// until ctx is cancelled or the connection drops.
func (c *Client) Events(ctx context.Context, fn func(action, name string)) error {
	filters, _ := json.Marshal(map[string][]string{"type": {"container"}})
	q := url.Values{"filters": {string(filters)}}
	resp, err := c.do(ctx, http.MethodGet, "/events", q)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(bufio.NewReader(resp.Body))
	for {
		var ev struct {
			Action string `json:"Action"`
			Actor  struct {
				Attributes map[string]string `json:"Attributes"`
			} `json:"Actor"`
		}
		if err := dec.Decode(&ev); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		fn(ev.Action, ev.Actor.Attributes["name"])
	}
}

// Watcher keeps an up-to-date Snapshot by polling and listening to events.
type Watcher struct {
	client   *Client
	interval time.Duration
	log      *slog.Logger

	mu   sync.RWMutex
	snap Snapshot
	subs map[chan Snapshot]struct{}
	kick chan struct{}
}

func NewWatcher(client *Client, interval time.Duration, log *slog.Logger) *Watcher {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &Watcher{
		client:   client,
		interval: interval,
		log:      log,
		snap:     Snapshot{Containers: map[string]*Container{}},
		subs:     map[chan Snapshot]struct{}{},
		kick:     make(chan struct{}, 1),
	}
}

func (w *Watcher) Snapshot() Snapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.snap
}

// Subscribe returns a channel receiving every new snapshot. Slow receivers
// drop intermediate snapshots rather than blocking the watcher.
func (w *Watcher) Subscribe() (<-chan Snapshot, func()) {
	ch := make(chan Snapshot, 1)
	w.mu.Lock()
	w.subs[ch] = struct{}{}
	w.mu.Unlock()
	return ch, func() {
		w.mu.Lock()
		delete(w.subs, ch)
		w.mu.Unlock()
	}
}

// Refresh requests an immediate poll.
func (w *Watcher) Refresh() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// Run blocks until ctx is done.
func (w *Watcher) Run(ctx context.Context) {
	go w.eventLoop(ctx)
	w.poll(ctx)
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.poll(ctx)
		case <-w.kick:
			w.poll(ctx)
		}
	}
}

func (w *Watcher) eventLoop(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := w.client.Events(ctx, func(action, name string) {
			switch action {
			case "start", "die", "stop", "kill", "pause", "unpause", "destroy", "create", "rename", "health_status: healthy", "health_status: unhealthy":
				w.Refresh()
			default:
				if strings.HasPrefix(action, "health_status") {
					w.Refresh()
				}
			}
		})
		if ctx.Err() != nil {
			return
		}
		w.log.Warn("docker event stream ended, reconnecting", "err", err, "backoff", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (w *Watcher) poll(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	list, err := w.client.List(cctx)
	snap := Snapshot{Taken: time.Now(), Containers: map[string]*Container{}}
	if err != nil {
		snap.Err = err.Error()
		w.log.Warn("docker list failed", "err", err)
		// Keep the last known containers so the UI degrades gracefully.
		snap.Containers = w.Snapshot().Containers
	} else {
		for _, c := range list {
			snap.Containers[c.Name] = c
		}
	}
	w.mu.Lock()
	w.snap = snap
	subs := make([]chan Snapshot, 0, len(w.subs))
	for ch := range w.subs {
		subs = append(subs, ch)
	}
	w.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- snap:
		default:
			// Drop stale snapshot and replace with the newest.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- snap:
			default:
			}
		}
	}
}

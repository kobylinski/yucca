package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Types mirroring the daemon API responses.
// Defined here to avoid importing daemon package (which embeds UI assets).

type ProjectInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type CredentialSource struct {
	Type     string `json:"type"`
	FilePath string `json:"file_path,omitempty"`
	FileKey  string `json:"file_key,omitempty"`
}

type CredentialMeta struct {
	Alias       string           `json:"alias"`
	Policy      string           `json:"policy"`
	Source      CredentialSource `json:"source"`
	Context     string           `json:"context,omitempty"`
	ValueLength int              `json:"value_length"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type SecretRequest struct {
	ID          string `json:"id"`
	Alias       string `json:"alias"`
	Reason      string `json:"reason"`
	ProjectPath string `json:"project_path"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) Health() error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/health")
	if err != nil {
		return fmt.Errorf("daemon not reachable: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) FetchProjects() ([]ProjectInfo, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/projects")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var projects []ProjectInfo
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (c *Client) FetchCredentials(slug string) (map[string]CredentialMeta, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/projects/" + slug + "/credentials")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var creds map[string]CredentialMeta
	if err := json.NewDecoder(resp.Body).Decode(&creds); err != nil {
		return nil, err
	}
	return creds, nil
}

func (c *Client) FetchPendingRequests() ([]SecretRequest, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/requests")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var reqs []SecretRequest
	if err := json.NewDecoder(resp.Body).Decode(&reqs); err != nil {
		return nil, err
	}
	return reqs, nil
}

func (c *Client) ApproveRequest(id, value, policy string) error {
	body, _ := json.Marshal(map[string]string{"value": value, "policy": policy})
	resp, err := c.httpClient.Post(c.baseURL+"/api/requests/"+id+"/approve", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) DenyRequest(id string) error {
	resp, err := c.httpClient.Post(c.baseURL+"/api/requests/"+id+"/deny", "application/json", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) UpdateCredential(slug, alias string, value, policy string) error {
	payload := map[string]string{}
	if value != "" {
		payload["value"] = value
	}
	if policy != "" {
		payload["policy"] = policy
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("PUT", c.baseURL+"/api/projects/"+slug+"/credentials/"+alias, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) DeleteCredential(slug, alias string) error {
	req, err := http.NewRequest("DELETE", c.baseURL+"/api/projects/"+slug+"/credentials/"+alias, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) RevealCredential(slug, alias string) (string, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/projects/" + slug + "/credentials/" + alias + "/reveal")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result["value"], nil
}

func (c *Client) CreateCredential(slug, alias, value, policy string) error {
	body, _ := json.Marshal(map[string]string{
		"alias":  alias,
		"value":  value,
		"policy": policy,
	})
	resp, err := c.httpClient.Post(c.baseURL+"/api/projects/"+slug+"/credentials", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errResp map[string]string
		if json.NewDecoder(resp.Body).Decode(&errResp) == nil {
			if msg, ok := errResp["error"]; ok {
				return fmt.Errorf("%s", msg)
			}
		}
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) SetCredentialContext(slug, alias, context string) error {
	body, _ := json.Marshal(map[string]string{"context": context})
	req, err := http.NewRequest("PUT", c.baseURL+"/api/projects/"+slug+"/credentials/"+alias+"/context", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

type ActiveSession struct {
	ProjectSlug string    `json:"project_slug"`
	ProjectPath string    `json:"project_path"`
	ProjectName string    `json:"project_name"`
	LastSeen    time.Time `json:"last_seen"`
}

func (c *Client) FetchSessions() ([]ActiveSession, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/sessions")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var sessions []ActiveSession
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (c *Client) CopyCredential(toSlug, fromSlug, alias string) error {
	body, _ := json.Marshal(map[string]string{
		"from_project_slug": fromSlug,
		"alias":             alias,
	})
	resp, err := c.httpClient.Post(c.baseURL+"/api/projects/"+toSlug+"/credentials/copy", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

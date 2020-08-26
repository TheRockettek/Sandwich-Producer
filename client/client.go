package client

import (
	"errors"
	"io"
	"net/http"
	"sync"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// Client represents the REST client
type Client struct {
	Token string

	HTTP    *http.Client
	Buckets *sync.Map

	// We will manually add the API version
	APIVersion string

	// Used to safely create URLs and is filled if empty
	URLHost   string
	URLScheme string
	UserAgent string
}

// NewClient makes a new client
func NewClient(token string) *Client {
	return &Client{
		Token:      token,
		HTTP:       http.DefaultClient,
		APIVersion: "6",
		URLHost:    "discord.com",
		URLScheme:  "https",
	}
}

// FetchJSON attempts to convert the response into a JSON structure
func (c *Client) FetchJSON(method string, url string, body io.Reader, structure interface{}) (err error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return
	}

	res, err := c.HandleRequest(req)
	defer res.Body.Close()
	if err != nil {
		return
	}

	err = json.NewDecoder(res.Body).Decode(structure)
	if err != nil {
		return err
	}

	return
}

// HandleRequest makes a request to the Discord API
// TODO: Buckets and handle ratelimiting
func (c *Client) HandleRequest(req *http.Request) (res *http.Response, err error) {
	req.URL.Path = "/api/v" + c.APIVersion + req.URL.Path

	// Fill out Host and Scheme if it is empty
	if req.URL.Host == "" {
		req.URL.Host = c.URLHost
	}
	if req.URL.Scheme == "" {
		req.URL.Scheme = c.URLScheme
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bot "+c.Token)
	}

	res, err = c.HTTP.Do(req)
	if err != nil {
		return
	}

	if res.StatusCode == http.StatusUnauthorized {
		err = errors.New("Invalid token passed")
		return
	}

	return
}

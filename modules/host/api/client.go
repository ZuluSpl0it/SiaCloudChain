package api

// Code generated by api2. DO NOT EDIT.

import (
	"context"
	"net/url"

	"github.com/starius/api2"
)

type Client struct {
	api2client *api2.Client
}

var _ HandlerHTTPapi = (*Client)(nil)

func NewClient(baseURL string, opts ...api2.Option) (*Client, error) {
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, err
	}
	routes := GetRoutes(nil)
	api2client := api2.NewClient(routes, baseURL, opts...)
	return &Client{
		api2client: api2client,
	}, nil
}

func (c *Client) DownloadWithToken(ctx context.Context, req *DownloadWithTokenRequest) (res *DownloadWithTokenResponse, err error) {
	res = &DownloadWithTokenResponse{}
	err = c.api2client.Call(ctx, res, req)
	if err != nil {
		return nil, err
	}
	return
}

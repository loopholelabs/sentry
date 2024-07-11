// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/loopholelabs/logging"

	"github.com/loopholelabs/sentry/pkg/rpc"
)

type DialFunc func() (io.ReadWriteCloser, error)

const (
	maxBackoff = time.Second
	minBackoff = time.Millisecond * 5
)

var (
	OptionsErr = errors.New("invalid options")
)

type Client struct {
	rpc    *rpc.Client
	ctx    context.Context
	cancel context.CancelFunc
	dial   DialFunc
	logger logging.Logger
	wg     sync.WaitGroup
}

func New(options *Options) (*Client, error) {
	if !validOptions(options) {
		return nil, OptionsErr
	}
	c := &Client{
		dial:   options.Dial,
		logger: options.Logger,
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.rpc = rpc.NewClient(options.Handle, options.Logger)
	c.wg.Add(1)
	go c.loop()
	return c, nil
}

func (c *Client) Close() error {
	c.cancel()
	c.wg.Wait()
	return nil
}

func (c *Client) connect() io.ReadWriteCloser {
	var err error
	var backoff time.Duration
	var conn io.ReadWriteCloser
	for {
		select {
		case <-c.ctx.Done():
			return nil
		default:
		}
		conn, err = c.dial()
		if err == nil {
			return conn
		}
		select {
		case <-c.ctx.Done():
			return nil
		default:
		}
		c.logger.Errorf("[Client] unable to create connection: %v\n", err)
		if backoff == 0 {
			backoff = minBackoff
		} else if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
		c.logger.Infof("[Client] retrying in %s\n", backoff)
		time.Sleep(backoff)
	}
}

func (c *Client) loop() {
	for {
		select {
		case <-c.ctx.Done():
			goto OUT
		default:
		}
		c.logger.Info("[Client] creating connection")
		conn := c.connect()
		if conn == nil {
			goto OUT
		}
		c.logger.Info("[Client] connection created")
		c.rpc.HandleConnection(conn)
	}
OUT:
	c.wg.Done()
}

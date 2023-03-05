package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type Collection struct {
	// http Client (in case any transport optimization)
	cc *http.Client

	// any error raised in progress
	me sync.Mutex
	er error

	// response
	resp ResponseData
	// url list to request
	urls []string

	// progress is on, will be closed on error
	rn chan struct{}
	// active queury buffer
	qw chan struct{}

	// once stop function to close rn chan
	stop func()
}

func NewCollection(data []string, qln int) *Collection {
	if qln <= 0 {
		qln = 4
	}

	ds := &Collection{
		cc: http.DefaultClient,

		resp: make(ResponseData, len(data)),
		urls: data,

		rn: make(chan struct{}),
		qw: make(chan struct{}, qln),
	}

	var once sync.Once
	ds.stop = func() {
		once.Do(func() {
			close(ds.rn)
		})
	}

	return ds
}

func (d *Collection) do(pctx context.Context, idx int) {
	req, err := http.NewRequest("GET", d.urls[idx], nil)
	if err != nil {
		err = fmt.Errorf("%s: %w", err.Error(), ErrBadURL)
		d.stopWithError(err)
		return
	}

	dn := make(chan error, 1)

	ctx, cancel := context.WithTimeout(pctx, 1*time.Second)
	defer cancel()

	req = req.WithContext(ctx)

	go func() {
		dn <- func(r *http.Response, err error) error {
			if err != nil {
				return err
			}
			defer r.Body.Close()

			if r.StatusCode >= 400 {
				err = fmt.Errorf("url item: %s: remote server code = %d", d.urls[idx], r.StatusCode)
				return err
			}

			var body []byte
			body, err = io.ReadAll(r.Body)
			if err != nil {
				return err
			}

			d.resp[idx] = URLResult{
				URL:    d.urls[idx],
				Result: string(body),
			}

			return nil
		}(d.cc.Do(req))
	}()

	select {
	case <-d.rn:
		return

	case <-ctx.Done():
		<-dn
		switch err = ctx.Err(); {
		case errors.Is(err, context.Canceled):
			// request was canceled by client
			err = nil
		case errors.Is(err, context.DeadlineExceeded):
			err = fmt.Errorf("url item = %s: %w", d.urls[idx], err)
		}

	case err = <-dn:
		if err != nil {
			err = fmt.Errorf("url item: %s: %w", err.Error(), ErrUprocessableEntity)
		}
	}

	if err != nil {
		d.stopWithError(err)
	}
}

func (d *Collection) stopWithError(err error) {
	d.me.Lock()
	if d.er == nil {
		d.er = err
	}
	d.me.Unlock()

	d.stop()
}

func (d *Collection) Walk(pctx context.Context) (err error) {
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

	wg := sync.WaitGroup{}

	var i int

walk:
	for off := len(d.urls); i < off; i++ {
		select {
		case <-d.rn:
			cancel()
			break walk

		case d.qw <- struct{}{}:
			wg.Add(1)

			go func(idx int) {
				defer func() {
					<-d.qw
					wg.Done()
				}()

				d.do(ctx, idx)
			}(i)
		}
	}

	wg.Wait()

	return nil
}

// ResponseData represents server response for the requested URLs list
type ResponseData []URLResult

// URLResult represetns response item in the ResponseData
type URLResult struct {
	URL    string `json:"url"`
	Result string `json:"result"`
}

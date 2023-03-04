package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

var (
	ErrBadURL             = errors.New("bad url")
	ErrJSON               = errors.New("json error")
	ErrUprocessableEntity = errors.New("unprocessable entity")
)

func main() {

}

type Disclosure struct {
	// http Client (in case any transport optimization)
	cc *http.Client

	// any error raised in progress
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

func NewDisclosure(data []string, qln int) *Disclosure {
	if qln <= 0 {
		qln = 4
	}

	ds := &Disclosure{
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

func (d *Disclosure) do(pctx context.Context, idx int) {
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
				err = fmt.Errorf("remote server code = %d", r.StatusCode)
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
		err = ctx.Err()

	case err = <-dn:
		if err != nil {
			err = fmt.Errorf("url item: %s: %w", err.Error(), ErrUprocessableEntity)
		}
	}

	if err != nil {
		d.stopWithError(err)
	}
}

func (d *Disclosure) stopWithError(err error) {
	if d.er == nil {
		d.er = err
	}
	d.stop()
}

func (d *Disclosure) Walk(pctx context.Context) (err error) {
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

func handler(w http.ResponseWriter, r *http.Request) {
	var data []string
	if err := readJSON(r.Body, &data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(data) > 20 {
		http.Error(w, "", http.StatusRequestEntityTooLarge)
		return
	}

	ds := NewDisclosure(data, 4)
	ds.Walk(r.Context())

	if ds.er != nil {
		st := http.StatusInternalServerError

		if errors.Is(ds.er, ErrBadURL) {
			st = http.StatusBadRequest
		}

		if errors.Is(ds.er, ErrUprocessableEntity) {
			st = 422
		}

		if errors.Is(ds.er, context.DeadlineExceeded) {
			st = http.StatusRequestTimeout
		}

		http.Error(w, ds.er.Error(), st)
		return
	}

	responseJSON(w, ds.resp, http.StatusOK)
}

func readJSON(r io.ReadCloser, v interface{}) error {
	defer r.Close()

	jsonDecoder := json.NewDecoder(r)
	if err := jsonDecoder.Decode(&v); err != nil {
		// uncomment to validate Content-Type
		//if t := r.Header.Get("Content-Type"); t != "" && !strings.Contains(t, "application/json") {
		//	err = fmt.Errorf("received content-type: %w", err)
		//}
		return fmt.Errorf("%s: %w", err.Error(), ErrJSON)
	}

	return nil
}

// responseJSON writes encode object to the http ResponseWriter with
// given status http code
func responseJSON(w http.ResponseWriter, v interface{}, code int) error {
	if code <= 0 {
		code = http.StatusOK
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)

	encoder := json.NewEncoder(w)
	return encoder.Encode(v)
}

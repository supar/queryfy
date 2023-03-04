package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHeavyTimeoutDisclosure_do(t *testing.T) {
	var cnt int32
	// heavy handler
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc, _ := strconv.Atoi(r.URL.Query().Get("t"))

		select {
		case <-time.After(time.Duration(sc) * time.Millisecond):
		case <-r.Context().Done():
			return
		}

		// intime response
		atomic.AddInt32(&cnt, 1)

		fmt.Fprintln(w, `fake data response`)
	}))
	defer ts.Close()

	data := make([]string, 9)
	for i, off := 0, len(data); i < off; i++ {
		if i == 5 {
			data[i] = ts.URL + "/?t=" + strconv.Itoa(300*i)
		} else {
			data[i] = ts.URL + "/?t=" + strconv.Itoa(900)
		}
	}

	ctx := context.Background()
	ds := NewDisclosure(data, 4)
	ds.Walk(ctx)

	if int(cnt) == len(data) {
		t.Error("all queries done against heavy timeout")
	}

	if ds.er == nil || ds.er != context.DeadlineExceeded {
		t.Errorf("do(), expected error = %v, but got = %v", context.DeadlineExceeded, ds.er)
	}
}

func Test_handler(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ecode, _ := strconv.Atoi(r.URL.Query().Get("ecode"))
		switch ecode {
		case 404:
			w.WriteHeader(http.StatusNotFound)
			return
		}

		fmt.Fprintln(w, `fake data response`)
	}))
	defer ts.Close()

	tests := []struct {
		name    string
		body    string
		expCode int
		expBody string
	}{
		{
			name:    "over 20 items",
			body:    `["/1","/2","/3","/4","/5","/6","/7","/8","/9","/10","/11","/12","/13","/14","/15","/16","/17","/18","/19","/20","/21"]`,
			expCode: http.StatusRequestEntityTooLarge,
		},
		{
			name:    "bad url",
			body:    `["/1"]`,
			expCode: 422,
		},
		{
			name:    "bad url",
			body:    `["/1"`,
			expCode: 400,
		},
		{
			name:    "response ok",
			body:    `["` + ts.URL + `/1"]`,
			expCode: http.StatusOK,
		},
		{
			name:    "remote response not 200",
			body:    `["` + ts.URL + `/1","` + ts.URL + `/?ecode=404"]`,
			expCode: 422,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			req, _ := http.NewRequest(
				"POST", "/",
				strings.NewReader(tt.body))
			req.Header.Add("Content-Type", "application/json")

			handler(w, req)

			if w.Code != tt.expCode {
				t.Errorf("handler(), expected http code = %d, but got = %d", tt.expCode, w.Code)
				t.Error(w.Body.String())
			}

			if tt.expCode == http.StatusOK && len(tt.expBody) > 0 {
				assert.JSONEq(t, tt.expBody, w.Body.String())
			}
		})
	}
}

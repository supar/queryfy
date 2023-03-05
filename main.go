package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

var (
	ErrBadURL             = errors.New("bad url")
	ErrJSON               = errors.New("json error")
	ErrUprocessableEntity = errors.New("unprocessable entity")
)

var (
	addrStr  = flag.String("a", ":8080", "address:port to listen")
	limitStr = flag.Uint("l", 100, "limit incoming requests")
)

func main() {
	flag.Parse()

	lmr := newLimiter(uint8(*limitStr))

	srv := &http.Server{
		Addr:    *addrStr,
		Handler: lmr.middleware(http.HandlerFunc(handler)),
	}

	wait := newShutdownSignal(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Shutdown: %v", err)
		}
	})

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Printf("Start: %v", err)
	}

	<-wait
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

	ds := NewCollection(data, 4)
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

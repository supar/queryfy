package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

func Test_limiter_middleware(t *testing.T) {
	var wa, wb sync.WaitGroup

	lm := 4
	stop := make(chan struct{})

	lmr := newLimiter(uint8(lm))
	hnd := lmr.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// routine blocks limiter
		wb.Done()
		<-stop
		//}

		w.WriteHeader(http.StatusOK)
	}))

	// send wait requests
	for i := 0; i < lm; i++ {
		// these are blocked requests
		wa.Add(1)
		wb.Add(1)

		go func(idx int) {
			// decrease  active
			defer wa.Done()

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/?q="+strconv.Itoa(idx), nil)

			hnd.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected code = %d, but got code = %d", http.StatusOK, w.Code)
			}
		}(i)
	}

	// wait untill all routines block limiter
	wb.Wait()

	// test expectation
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/?q=", nil)
	hnd.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected code = %d, but got code = %d", http.StatusServiceUnavailable, w.Code)
	}

	// unfreezes routines
	close(stop)
	wa.Wait()

	if lmr.cnt != 0 {
		t.Errorf("limiter.cnt value must be decreased to zero, but got %d", lmr.cnt)
	}
}

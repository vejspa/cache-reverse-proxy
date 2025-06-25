package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

type cacheData struct {
	header http.Header
	body   []byte
	age    time.Time
	status int
}
type cache struct {
	mu   sync.RWMutex
	data map[string]cacheData
	ttl  time.Duration
}

func newCache(ttl time.Duration) *cache {
	return &cache{
		data: make(map[string]cacheData),
		ttl:  ttl,
	}
}

func newReverseProxy() *httputil.ReverseProxy {
	target, err := url.Parse("https://dummyjson.com")

	if err != nil {
		log.Fatal("could not parse server url")
	}

	// TODO remove x-forward-for
	d := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host

		ip, _, err := net.SplitHostPort(req.RemoteAddr)
		if err == nil {
			req.Header.Add("X-Forwarded-For", ip)
		}
		log.Println(ip)
	}

	return &httputil.ReverseProxy{
		FlushInterval: 10 * time.Millisecond,
		Director:      d,
	}
}

func main() {
	if err := run(); err != nil {
		log.Panic("unexpected error during runtime", err)
	}
}

func run() error {
	rp := newReverseProxy()
	c := newCache(3 * time.Hour)

	rp.ModifyResponse = func(res *http.Response) error {
		if res.Request.Method != http.MethodGet {
			return nil
		}

		key := res.Request.RequestURI

		c.mu.RLock()
		_, ok := c.data[key]
		c.mu.RUnlock()

		if !ok {
			b, err := io.ReadAll(res.Body)

			if err != nil {
				return err
			}

			err = res.Body.Close()

			if err != nil {
				return err
			}

			res.Body = io.NopCloser(bytes.NewReader(b))

			c.data[key] = cacheData{
				header: res.Header.Clone(),
				body:   b,
				age:    time.Now(),
				status: res.StatusCode,
			}

			res.Header.Add("X-Cache", "MISS")
			res.Header.Add("X-RP", fmt.Sprintf("%p", rp))

			return nil
		}

		return nil
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			c.mu.RLock()
			d, ok := c.data[r.RequestURI]
			c.mu.RUnlock()

			if ok {
				for k, vv := range d.header {
					for _, v := range vv {
						w.Header().Add(k, v)
					}
				}

				w.Header().Set("X-Cache", "HIT")
				w.Header().Set("X-RPHandleFunc", fmt.Sprintf("%p", rp))

				w.WriteHeader(d.status)

				_, err := w.Write(d.body)

				if err != nil {
					log.Println("can't write to body")

					return
				}

				return
			}
		}

		rp.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("Reverse-proxy listening on %s", srv.Addr)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}

	return nil
}

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/usnistgov/ndn-dpdk/ndn"
	"github.com/usnistgov/ndn-dpdk/ndn/an"
	"github.com/usnistgov/ndn-dpdk/ndn/segmented"
	"go.uber.org/zap"
)

func handleV2(w http.ResponseWriter, req *http.Request) {
	req.URL.Scheme = upstreamScheme
	req.URL.Host = upstreamHost

	if name := parseBlobRequest(req.URL); name != nil && req.Method == http.MethodGet {
		proxyToNDN(w, req, name)
	} else {
		proxyToHTTP(w, req, 0)
	}
}

func makeLogEntry(req *http.Request) *zap.Logger {
	return logger.With(zap.String("method", req.Method), zap.String("uri", req.URL.Path))
}

func parseBlobRequest(url *url.URL) ndn.Name {
	comps := strings.Split(url.Path, "/")
	if len(comps) != 5 || comps[3] != "blobs" || !strings.HasPrefix(comps[4], "sha256:") {
		return nil
	}

	blobDigest, e := hex.DecodeString(comps[4][7:])
	if e != nil || len(blobDigest) != sha256.Size {
		return nil
	}

	name := append(ndn.Name{}, upstreamPrefix...)
	name = append(name,
		ndn.MakeNameComponent(an.TtGenericNameComponent, []byte(comps[2])),
		ndn.MakeNameComponent(an.TtGenericNameComponent, blobDigest),
	)
	return name
}

var ndnThrottle sync.Mutex

func proxyToNDN(w http.ResponseWriter, req *http.Request, name ndn.Name) {
	logEntry := makeLogEntry(req).With(zap.Stringer("name", name))
	logEntry.Debug("fetch queued")
	t0 := time.Now()
	ndnThrottle.Lock()
	logEntry.Debug("fetch start", zap.Duration("waited", time.Since(t0)))

	timeout, cancel := context.WithTimeout(req.Context(), ndnFetchTimeout)
	defer cancel()

	fetcher := segmented.Fetch(name, ndnFetchOptions)
	t0 = time.Now()
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		historyCounts := make([]int, 3)
		for i := range historyCounts {
			historyCounts[i] = -1
		}

		for t1 := range ticker.C {
			cnt, total := fetcher.Count(), fetcher.EstimatedTotal()
			logEntry.Debug("fetch progress",
				zap.Duration("elapsed", t1.Sub(t0)),
				zap.Int("count", cnt),
				zap.Int("estimated-total", total),
				zap.Float64("ratio", float64(cnt)/float64(total)),
			)

			if cnt == historyCounts[0] {
				logEntry.Warn("fetch no progress, aborting")
				cancel()
			}
			copy(historyCounts, historyCounts[1:])
			historyCounts[len(historyCounts)-1] = cnt
		}
	}()

	chunks := make(chan []byte)
	fetchErr := make(chan error)
	sendSize := 0
	sendDone := make(chan struct{})
	go func() { fetchErr <- fetcher.Chunks(timeout, chunks) }()
	go func() {
		defer close(sendDone)
		for chunk := range chunks {
			if sendSize == 0 {
				w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
				w.Header().Set("Content-Type", "application/octet-stream")
			}
			sendSize += len(chunk)
			w.Write(chunk)
		}
	}()
	e, _ := <-fetchErr, <-sendDone
	ticker.Stop()
	ndnThrottle.Unlock()

	logEntry = logEntry.With(zap.Duration("duration", time.Since(t0)), zap.Int("size", sendSize), zap.Error(e))

	if e == nil {
		logEntry.Debug("fetch success")
		return
	}

	if sendSize == 0 {
		logEntry.Warn("fetch error, redirecting")
		redirectToHTTP(w, req)
	} else {
		logEntry.Warn("fetch error, proxying")
		proxyToHTTP(w, req, sendSize)
	}
}

func redirectToHTTP(w http.ResponseWriter, req *http.Request) {
	logEntry := makeLogEntry(req)
	logEntry.Debug("redirect", zap.Stringer("dest", req.URL))
	http.Redirect(w, req, req.URL.String(), http.StatusTemporaryRedirect)
}

func proxyToHTTP(w http.ResponseWriter, req *http.Request, rangeStart int) {
	logEntry := makeLogEntry(req).With(zap.Stringer("upstream", req.URL))
	if rangeStart > 0 {
		logEntry = logEntry.With(zap.Int("range-start", rangeStart))
	}

	uReq, e := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), req.Body)
	if e != nil {
		logEntry.Warn("proxy error", zap.Error(e))
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	uReq.Header = req.Header.Clone()
	if rangeStart > 0 {
		uReq.Header.Set("Range", "bytes="+strconv.Itoa(rangeStart)+"-")
	}

	resp, e := http.DefaultClient.Do(uReq)
	if e != nil {
		logEntry.Warn("proxy error", zap.Error(e))
		if rangeStart == 0 {
			w.WriteHeader(http.StatusBadGateway)
		}
		return
	}

	if rangeStart == 0 {
		for header, headerSlice := range resp.Header {
			for _, headerValue := range headerSlice {
				w.Header().Add(header, headerValue)
			}
		}
		w.WriteHeader(resp.StatusCode)
	}

	logEntry.Debug("proxied",
		zap.Int("status", resp.StatusCode),
		zap.String("length", resp.Header.Get("Content-Length")),
	)
	io.Copy(w, resp.Body)
}
